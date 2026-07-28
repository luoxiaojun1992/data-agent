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
// tasks. It maps RFC §16 processTask:
//
//  1. mark the task running
//  2. create the ADK session with identity state injected (same as
//     chat.Service.prepareRun)
//  3. resolve the per-model Runtime via the Registry (SPEC-062, by
//     task.ModelID)
//  4. run the agent turn (Runtime.RunAndCollect) under a circuit breaker
//  5. on success: persist the result + mark completed + notify the user
//     on failure: persist the error + mark failed + notify the user
//
// The executor does NOT import worker; it satisfies worker.TaskExecutor
// structurally (duck typing). The compile-time assertion lives in wire.go
// where both packages are in scope.
type AgentExecutor struct {
	registry    *adkruntime.Registry         // SPEC-062: per-model Runtime resolution
	adkSessions  session.Service              // create ADK session + inject identity state
	tasks       domaintask.TaskService        // load/status/result/error write-back
	notif       notification.NotificationService // completion/failure notification
	cbReg       *security.CircuitBreakerRegistry // protects Runtime.Run from cascading failures
}

// NewAgentExecutor wires the executor with its dependencies. All are required
// in production; tests inject mocks / in-memory implementations.
func NewAgentExecutor(
	registry *adkruntime.Registry,
	adkSessions session.Service,
	tasks domaintask.TaskService,
	notif notification.NotificationService,
	cbReg *security.CircuitBreakerRegistry,
) *AgentExecutor {
	return &AgentExecutor{
		registry:   registry,
		adkSessions: adkSessions,
		tasks:      tasks,
		notif:      notif,
		cbReg:      cbReg,
	}
}

// Execute runs a single async/scheduled agent task to completion (or failure).
// Returns the execution error so the worker pool can apply its retry/DLQ
// policy; the executor has already persisted the failure status + error by then.
//
// Task-result flow (see RFC §16 / spec task-result-retry):
//  1. Mark running, create ADK session with task_id in state.
//  2. First LLM run (no retry hint). If the model called save_task_result,
//     UpdateTaskResult already set status=completed — the executor just
//     notifies the user and returns.
//  3. If the LLM finished without saving, send a follow-up prompt on the
//     SAME session that asks the model to call save_task_result with its
//     final answer. This preserves the conversation context the model
//     needs to produce a coherent result.
//  4. After the retry, if save_task_result was called → completed. If still
//     missing OR the retry itself errored, mark the task failed with the
//     cause (content of the LLM's last message, or the underlying error).
//  5. Cancellation is respected: a task cancelled mid-run keeps its
//     cancelled status (no overwrite with completed/failed).
func (e *AgentExecutor) Execute(ctx context.Context, t *domaintask.Task) error {
	// 0. Respect cancellation: a task cancelled before the worker picked it up
	//    (e.g. user cancelled a still-queued task) is skipped entirely.
	if t.Status == domaintask.StatusCancelled {
		return nil
	}

	// 1. Mark running.
	_ = e.tasks.UpdateStatus(t.ID, domaintask.StatusRunning)

	// 2. Create ADK session with identity + task_id injected into state.
	//    Create is idempotent (upsert), so re-runs on the same session are safe.
	//    When t.SessionID is empty, the ADK service auto-generates a session
	//    ID — we capture it from the response and use it for the run.
	state := buildExecutorState(t)
	resp, cerr := e.adkSessions.Create(ctx, &session.CreateRequest{
		AppName:   e.registry.AppName(),
		UserID:    t.UserID,
		SessionID: t.SessionID,
		State:     state,
	})
	if cerr != nil {
		err := fmt.Errorf("adk session init: %w", cerr)
		e.failTask(t, err)
		return err
	}
	runSessionID := t.SessionID
	if resp != nil && resp.Session.ID() != "" {
		runSessionID = resp.Session.ID()
	}
	state["session_id"] = runSessionID

	// 3. Resolve the task-mode Runtime. Empty ModelID falls back to default.
	//    The task instruction suffix forces the LLM to call save_task_result
	//    before considering its turn done.
	rt, rErr := e.registry.GetOrCreateWithInstruction(ctx, t.ModelID, adkruntime.TaskInstructionSuffix)
	if rErr != nil {
		err := fmt.Errorf("resolve runtime: %w", rErr)
		e.failTask(t, err)
		return err
	}

	// 4. First LLM run.
	message := deriveUserMessage(t)
	runCfg := adkruntime.RunConfig{StateDelta: state}
	var firstContent string
	firstErr := e.runProtected(ctx, rt, t, runSessionID, message, runCfg, &firstContent)

	// 5. Respect cancellation (user might have cancelled during the run).
	if e.wasCancelled(t.ID) {
		return firstErr
	}

	// 6. Was save_task_result called during the first run? Check by re-reading
	//    the task from DB — UpdateTaskResult sets status=completed atomically.
	latest, lErr := e.tasks.GetTask(t.ID)
	if lErr == nil && latest != nil && latest.Status == domaintask.StatusCompleted {
		e.notify(t, "任务完成", fmt.Sprintf("任务 %q 已完成", t.ID))
		return nil
	}

	// 7. The LLM didn't call save_task_result (or errored before getting to it).
	//    If the first run produced an error, surface it; otherwise retry with
	//    the same session and a reminder prompt so the model can produce the
	//    final result while keeping its prior context.
	if firstErr != nil {
		e.failTask(t, fmt.Errorf("llm runtime error (no save_task_result): %w", firstErr))
		return firstErr
	}

	retryPrompt := "Your previous turn finished without calling save_task_result. " +
		"Please call the save_task_result tool NOW with the final answer (or a summary) as the content argument. " +
		"Without that call the task will be marked failed."

	var retryContent string
	retryErr := e.runProtected(ctx, rt, t, runSessionID, retryPrompt, runCfg, &retryContent)
	if e.wasCancelled(t.ID) {
		return retryErr
	}
	if retryErr == nil {
		// Re-check whether the retry managed to call save_task_result.
		latest, lErr = e.tasks.GetTask(t.ID)
		if lErr == nil && latest != nil && latest.Status == domaintask.StatusCompleted {
			e.notify(t, "任务完成", fmt.Sprintf("任务 %q 已完成", t.ID))
			return nil
		}
	}
	// Both attempts failed to call save_task_result — record the failure.
	failReason := "save_task_result was not called after the LLM turn (one retry attempted)"
	if retryErr != nil {
		failReason = "save_task_result was not called; retry turn also failed: " + retryErr.Error()
	} else if strings.TrimSpace(retryContent) != "" {
		failReason = "save_task_result was not called; last LLM response: " + truncateForError(retryContent)
	}
	failErr := fmt.Errorf("%s", failReason)
	e.failTask(t, failErr)
	return failErr
}

// runProtected invokes Runtime.RunAndCollect inside the "agent" circuit
// breaker, storing the final text in *content. When no breaker registry is
// wired (defensive nil), the call runs unprotected. sessionID is the resolved
// ADK session ID (may differ from t.SessionID when the latter was empty and the
// ADK service auto-generated one).
func (e *AgentExecutor) runProtected(ctx context.Context, rt *adkruntime.Runtime, t *domaintask.Task, sessionID, message string, runCfg adkruntime.RunConfig, content *string) error {
	run := func() error {
		text, err := rt.RunAndCollect(ctx, t.UserID, sessionID, message, runCfg)
		*content = text
		return err
	}
	if e.cbReg == nil {
		return run()
	}
	return e.cbReg.GetOrCreate("agent").Call(run)
}

// failTask persists the failure error (UpdateError sets error + status=failed
// atomically) and notifies the user.
func (e *AgentExecutor) failTask(t *domaintask.Task, err error) {
	_ = e.tasks.UpdateError(t.ID, err.Error())
	e.notify(t, "任务失败", fmt.Sprintf("任务 %q 失败: %v", t.ID, err))
}

// notify sends a notification to the task owner. System-owned tasks
// (UserID == "system", e.g. scheduled tasks) skip notification to avoid
// spamming a non-human recipient.
func (e *AgentExecutor) notify(t *domaintask.Task, title, body string) {
	if e.notif == nil || t.UserID == "" || t.UserID == "system" {
		return
	}
	if _, err := e.notif.Send(title, body, "task", []string{t.UserID}); err != nil {
		log.Printf("[executor] notification send failed for user %s (task %s): %v", t.UserID, t.ID, err)
	}
}

// wasCancelled re-loads the task and reports whether it was cancelled. Used
// after RunAndCollect to avoid overwriting a cancellation that arrived during
// execution (e.g. a user cancelling a long-running task).
func (e *AgentExecutor) wasCancelled(id string) bool {
	latest, err := e.tasks.GetTask(id)
	if err != nil || latest == nil {
		return false
	}
	return latest.Status == domaintask.StatusCancelled
}

// buildExecutorState constructs the ADK session state map with identity
// injection, mirroring chat.Service.buildState. task_id is included so tools
// can correlate the run with its originating task.
func buildExecutorState(t *domaintask.Task) map[string]any {
	state := map[string]any{
		"user_id":    t.UserID,
		"session_id": t.SessionID,
		"task_id":    t.ID,
	}
	if kbID, ok := t.Params["kb_id"].(string); ok && kbID != "" {
		state["kb_id"] = kbID
	}
	return state
}

// deriveUserMessage extracts the user message from Task.Params by convention.
// Priority: query > message > prompt > description > title. Whitespace-only
// values are skipped. Returns "" when none are present.
func deriveUserMessage(t *domaintask.Task) string {
	for _, key := range []string{"query", "message", "prompt", "description"} {
		if v, ok := t.Params[key].(string); ok && strings.TrimSpace(v) != "" {
			return v
		}
	}
	if v, ok := t.Params["title"].(string); ok && strings.TrimSpace(v) != "" {
		return v
	}
	return ""
}

// truncateForError clamps text used in failure error messages so the DB
// error field doesn't blow up on long LLM outputs.
func truncateForError(s string) string {
	const max = 500
	if len(s) <= max {
		return s
	}
	return s[:max] + "..."
}
