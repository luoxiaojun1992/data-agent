// Package agent contains the orchestration logic shared by the chat and agent
// flows. The AgentExecutor (this file) implements worker.TaskExecutor so the
// worker pool can delegate async/scheduled task execution here (SPEC-063).
//
// It reuses the real-time Runtime.RunAndCollect execution path — async tasks
// execute with identical semantics to a real-time chat turn — and owns all DB
// write-back (status/result/error) and user notification, fixing the three
// pool.go defects: no-op stub, in-memory task rebuild (no DB load), and no
// result/error write-back or notification.
package agent

import (
	"context"
	"fmt"
	"log"
	"strings"

	"google.golang.org/adk/session"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/service/notification"
)

// AgentExecutor implements the worker.TaskExecutor contract by reusing the
// real-time agent execution path (Runtime.RunAndCollect) for async/scheduled
// task runs. It maps RFC §16 processRun:
//
//  1. mark the run running
//  2. create the ADK session with identity state injected (same as
//     chat.Service.prepareRun)
//  3. resolve the per-model Runtime via the Registry (SPEC-062, by
//     run.ModelID)
//  4. run the agent turn (Runtime.RunAndCollect) under a circuit breaker
//  5. on success: persist the result + mark completed + notify the user
//     on failure: persist the error + mark failed + notify the user
//
// The executor does NOT import worker; it satisfies worker.TaskExecutor
// structurally (duck typing). The compile-time assertion lives in wire.go
// where both packages are in scope.
type AgentExecutor struct {
	registry    *adkruntime.Registry                        // SPEC-062: per-model Runtime resolution
	adkSessions  session.Service                             // create ADK session + inject identity state
	runs        domaintask.TaskRunService                    // run status/result/error write-back
	notif       notification.NotificationService             // completion/failure notification
	cbReg       *security.CircuitBreakerRegistry             // protects Runtime.Run from cascading failures
}

// NewAgentExecutor wires the executor with its dependencies. All are required
// in production; tests inject mocks / in-memory implementations.
func NewAgentExecutor(
	registry *adkruntime.Registry,
	adkSessions session.Service,
	runs domaintask.TaskRunService,
	notif notification.NotificationService,
	cbReg *security.CircuitBreakerRegistry,
) *AgentExecutor {
	return &AgentExecutor{
		registry:   registry,
		adkSessions: adkSessions,
		runs:       runs,
		notif:      notif,
		cbReg:      cbReg,
	}
}

// Execute runs a single async/scheduled agent run to completion (or failure).
// Returns the execution error so the worker pool can apply its retry/DLQ
// policy; the executor has already persisted the failure status + error by then.
//
// Task-run flow (see RFC §16 / spec task-result-retry):
//  1. Mark running, create ADK session with run's task_id and run_id in state.
//  2. First LLM run (no retry hint). If the model called save_task_result,
//     UpdateRunResult already set status=completed — the executor just
//     notifies the user and returns.
//  3. If the LLM finished without saving, send a follow-up prompt on the
//     SAME session that asks the model to call save_task_result with its
//     final answer. This preserves the conversation context the model
//     needs to produce a coherent result.
//  4. After the retry, if save_task_result was called → completed. If still
//     missing OR the retry itself errored, mark the run failed with the
//     cause (content of the LLM's last message, or the underlying error).
//  5. Cancellation is respected: a run cancelled mid-execution keeps its
//     cancelled status (no overwrite with completed/failed).
func (e *AgentExecutor) Execute(ctx context.Context, run *domaintask.TaskRun) error {
	// 0. Respect cancellation.
	if run.Status == domaintask.StatusCancelled {
		return nil
	}

	// 1. Mark running.
	_ = e.runs.UpdateRunStatus(run.ID, domaintask.StatusRunning)

	// 2. Create ADK session with identity + task_id + run_id injected.
	//    Session is owned by run.UserID (the task creator), so it appears
	//    in their chat session list and respects their permissions.
	state := buildRunState(run)
	resp, cerr := e.adkSessions.Create(ctx, &session.CreateRequest{
		AppName:   e.registry.AppName(),
		UserID:    run.UserID,
		SessionID: run.SessionID,
		State:     state,
	})
	if cerr != nil {
		err := fmt.Errorf("adk session init: %w", cerr)
		e.failRun(run, err)
		return err
	}
	runSessionID := run.SessionID
	if resp != nil && resp.Session.ID() != "" {
		runSessionID = resp.Session.ID()
	}
	state["session_id"] = runSessionID

	// Write the session_id back to the TaskRun so the run → session link
	// is visible in the API and UI.
	if runSessionID != run.SessionID {
		run.SessionID = runSessionID
		e.writeSessionID(run.ID, runSessionID)
	}

	// 3. Resolve the task-mode Runtime.
	rt, rErr := e.registry.GetOrCreateWithInstruction(ctx, run.ModelID, adkruntime.TaskInstructionSuffix)
	if rErr != nil {
		err := fmt.Errorf("resolve runtime: %w", rErr)
		e.failRun(run, err)
		return err
	}

	// 4. First LLM run.
	message := deriveUserMessageFromParams(run.Params)
	runCfg := adkruntime.RunConfig{StateDelta: state}
	var firstContent string
	firstErr := e.runProtected(ctx, rt, run, runSessionID, message, runCfg, &firstContent)

	// 5. Respect cancellation.
	if e.wasRunCancelled(run.ID) {
		return firstErr
	}

	// 6. Was save_task_result called during the first run?
	latest, lErr := e.runs.GetRun(run.ID)
	if lErr == nil && latest != nil && latest.Status == domaintask.StatusCompleted {
		e.notifyRun(run, "任务完成", fmt.Sprintf("任务 %q 已完成", run.TaskID))
		return nil
	}

	// 7. Retry with hint.
	if firstErr != nil {
		e.failRun(run, fmt.Errorf("llm runtime error (no save_task_result): %w", firstErr))
		return firstErr
	}

	retryPrompt := "Your previous turn finished without calling the save_task_result function tool. " +
		"The system has NO saved result for this task and will mark it FAILED if you do not call the tool. " +
		"Call save_task_result NOW with the final answer (or a summary) as the `content` argument. " +
		"Do NOT just write a text response — you must invoke the tool."

	var retryContent string
	retryErr := e.runProtected(ctx, rt, run, runSessionID, retryPrompt, runCfg, &retryContent)
	if e.wasRunCancelled(run.ID) {
		return retryErr
	}
	if retryErr == nil {
		latest, lErr = e.runs.GetRun(run.ID)
		if lErr == nil && latest != nil && latest.Status == domaintask.StatusCompleted {
			e.notifyRun(run, "任务完成", fmt.Sprintf("任务 %q 已完成", run.TaskID))
			return nil
		}
	}
	failReason := "save_task_result was not called after the LLM turn (one retry attempted)"
	if retryErr != nil {
		failReason = "save_task_result was not called; retry turn also failed: " + retryErr.Error()
	} else if strings.TrimSpace(retryContent) != "" {
		failReason = "save_task_result was not called; last LLM response: " + truncateForError(retryContent)
	}
	failErr := fmt.Errorf("%s", failReason)
	e.failRun(run, failErr)
	return failErr
}

// runProtected invokes Runtime.RunAndCollect inside the "agent" circuit
// breaker, storing the final text in *content. When no breaker registry is
// wired (defensive nil), the call runs unprotected.
func (e *AgentExecutor) runProtected(ctx context.Context, rt *adkruntime.Runtime, run *domaintask.TaskRun, sessionID, message string, runCfg adkruntime.RunConfig, content *string) error {
	runFn := func() error {
		text, err := rt.RunAndCollect(ctx, run.UserID, sessionID, message, runCfg)
		*content = text
		return err
	}
	if e.cbReg == nil {
		return runFn()
	}
	return e.cbReg.GetOrCreate("agent").Call(runFn)
}

// failRun persists the failure error and notifies the user.
func (e *AgentExecutor) failRun(run *domaintask.TaskRun, err error) {
	_ = e.runs.UpdateRunError(run.ID, err.Error())
	e.notifyRun(run, "任务失败", fmt.Sprintf("任务 %q 失败: %v", run.TaskID, err))
}

// notifyRun sends a notification to the run owner.
func (e *AgentExecutor) notifyRun(run *domaintask.TaskRun, title, body string) {
	if e.notif == nil || run.UserID == "" || run.UserID == "system" {
		return
	}
	if _, err := e.notif.Send(title, body, "task", []string{run.UserID}); err != nil {
		log.Printf("[executor] notification send failed for user %s (run %s): %v", run.UserID, run.ID, err)
	}
}

// writeSessionID persists the ADK-generated session ID back to the TaskRun
// so the run → session → chat history link is visible.
func (e *AgentExecutor) writeSessionID(runID, sessionID string) {
	_ = e.runs.UpdateRunSessionID(runID, sessionID)
}

// wasRunCancelled re-loads the run and reports whether it was cancelled.
func (e *AgentExecutor) wasRunCancelled(id string) bool {
	latest, err := e.runs.GetRun(id)
	if err != nil || latest == nil {
		return false
	}
	return latest.Status == domaintask.StatusCancelled
}

// buildRunState constructs the ADK session state map with identity injection.
func buildRunState(run *domaintask.TaskRun) map[string]any {
	state := map[string]any{
		"user_id":    run.UserID,
		"session_id": run.SessionID,
		"task_id":    run.TaskID,
		"run_id":     run.ID,
	}
	if kbID, ok := run.Params["kb_id"].(string); ok && kbID != "" {
		state["kb_id"] = kbID
	}
	return state
}

// deriveUserMessageFromParams extracts the user message from Params.
func deriveUserMessageFromParams(params map[string]interface{}) string {
	for _, key := range []string{"query", "message", "prompt", "description"} {
		if v, ok := params[key].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	if v, ok := params["title"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return ""
}

// truncateForError clamps text used in failure error messages.
func truncateForError(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
