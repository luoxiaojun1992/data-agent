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
func (e *AgentExecutor) Execute(ctx context.Context, t *domaintask.Task) error {
	// 1. Mark running (RFC §16 step 2).
	_ = e.tasks.UpdateStatus(t.ID, domaintask.StatusRunning)

	// 2. Create ADK session with identity injected into state (mirrors
	//    chat.Service.prepareRun). Create is idempotent (upsert), so re-runs
	//    on the same session are safe. When t.SessionID is empty (tasks created
	//    via POST /tasks without a session binding), the ADK service
	//    auto-generates a session ID — we capture it from the response and use
	//    it for the run so Run can find the session it just created.
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
	// Keep the state's session_id consistent with the actual ADK session.
	state["session_id"] = runSessionID

	// 3. Resolve the per-model Runtime (SPEC-062). Empty ModelID falls back
	//    to the default model inside the Registry.
	rt, rErr := e.registry.GetOrCreate(ctx, t.ModelID)
	if rErr != nil {
		err := fmt.Errorf("resolve runtime: %w", rErr)
		e.failTask(t, err)
		return err
	}

	// 4. Derive the user message from Task.Params and run the agent turn under
	//    a circuit breaker (same breaker key as chat so a model outage trips
	//    both paths consistently).
	message := deriveUserMessage(t)
	runCfg := adkruntime.RunConfig{StateDelta: state}

	var content string
	execErr := e.runProtected(ctx, rt, t, runSessionID, message, runCfg, &content)

	// 5. Write back result/error + notify.
	if execErr != nil {
		e.failTask(t, execErr)
		return execErr
	}
	e.completeTask(t, content)
	return nil
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

// completeTask persists the success result and marks the task completed, then
// notifies the user. UpdateTaskResult already sets status=completed atomically
// at the repository level; the explicit UpdateStatus(completed) makes the state
// transition visible at the service layer (and verifiable in tests).
func (e *AgentExecutor) completeTask(t *domaintask.Task, content string) {
	result := map[string]interface{}{"content": content, "status": "success"}
	_ = e.tasks.UpdateTaskResult(t.ID, result)
	_ = e.tasks.UpdateStatus(t.ID, domaintask.StatusCompleted)
	e.notify(t, "任务完成", fmt.Sprintf("任务 %q 已完成", t.ID))
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
