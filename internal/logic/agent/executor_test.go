package agent

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	domaintaskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	notificationmocks "github.com/luoxiaojun1992/data-agent/internal/service/notification/mocks"
)

// ── Fake LLM ──

// fakeLLM yields a single final response with the configured text (or an
// error). It implements model.LLM the same way chat_test.go's fakeLLM does,
// so the Runtime built from it behaves identically to a real chat turn.
type fakeLLM struct {
	text string
	err  error
}

func (f *fakeLLM) Name() string { return "fake" }

func (f *fakeLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(f.text, "model")}, nil)
	}
}

// ── Test harness ──

// testExecutor bundles an executor with its mocks so each test can configure
// expectations and assert on the recorded calls. The Registry is patched via
// gomonkey to return a real Runtime (built with a fakeLLM), mirroring
// chat_test.go's approach so async execution uses the same code path.
type testExecutor struct {
	exec       *AgentExecutor
	registry   *adkruntime.Registry
	rt         *adkruntime.Runtime
	tasks      *domaintaskmocks.TaskService
	notif      *notificationmocks.NotificationService
	patches    *gomonkey.Patches
	adkSess    adksession.Service
	// completed controls the default GetTask mock. When false (default),
	// GetTask returns a non-cancelled running task — exercising the
	// "no save_task_result" retry/failure path. When flipped to true via
	// setTaskCompleted, subsequent GetTask calls return a completed task
	// (the LLM-driven tool is simulated to have fired).
	completed   atomic.Bool
}

func newTestExecutor(t *testing.T, llm model.LLM) *testExecutor {
	t.Helper()
	adkSess := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName:        "data-agent",
		Model:          llm,
		SessionService: adkSess,
	})
	require.NoError(t, err)
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{
		AppName:        "data-agent",
		SessionService: adkSess,
	})
	tasks := domaintaskmocks.NewTaskService(t)
	notif := notificationmocks.NewNotificationService(t)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	exec := NewAgentExecutor(registry, adkSess, tasks, notif, cbReg)

	patches := gomonkey.NewPatches()
	t.Cleanup(patches.Reset)
	// Patch GetOrCreate / GetOrCreateWithInstruction to return the test Runtime
	// (avoids needing a real Provider/model config for unit tests) — same
	// pattern as chat_test.go.
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodFunc(registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	// GetTask is consulted twice during Execute: wasCancelled() and the
	// post-run "did save_task_result get called?" check. Default both
	// return a non-cancelled task that has NOT been completed (so the
	// retry path is exercised). Tests that want success use
	// setTaskCompleted to flip the result for subsequent GetTask calls.
	te := &testExecutor{exec: exec, registry: registry, rt: rt, tasks: tasks, notif: notif, patches: patches, adkSess: adkSess}
	tasks.On("GetTask", mock.Anything).Return(func(id string) *domaintask.Task {
		if te.completed.Load() {
			return &domaintask.Task{ID: "task_1", Status: domaintask.StatusCompleted}
		}
		return &domaintask.Task{ID: "task_1", Status: domaintask.StatusRunning}
	}, nil).Maybe()
	return te
}

// patchGetOrCreateError makes Registry.GetOrCreateWithInstruction return an
// error (runtime resolution failure). Useful for the resolve-runtime failure
// path.
func (te *testExecutor) patchGetOrCreateError(err error) {
	te.patches.ApplyMethodFunc(te.registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return nil, err
	})
}

// setTaskCompleted flips the default GetTask mock to return a completed task
// for all subsequent calls, simulating the LLM having called save_task_result.
func (te *testExecutor) setTaskCompleted() {
	te.completed.Store(true)
}

// patchADKCreateError makes the ADK session Create fail.
func (te *testExecutor) patchADKCreateError(err error) {
	te.patches.ApplyMethodReturn(te.adkSess, "Create", (*adksession.CreateResponse)(nil), err)
}

func sampleTask() *domaintask.Task {
	return &domaintask.Task{
		ID:        "task_1",
		SessionID: "sess_1",
		UserID:    "u1",
		Type:      "agent",
		ModelID:   "m1",
		Status:    domaintask.StatusQueued,
		Params:    map[string]interface{}{"message": "分析营收"},
		MaxRetries: 3,
	}
}

// ── Execute: success ──

// In the new save_task_result flow, success means: the LLM called the
// save_task_result tool during its turn. The fakeLLM doesn't call tools, so
// we simulate that by mocking GetTask to return Status=completed after the
// run. The executor reads the task to confirm the result was persisted and
// notifies the user without calling completeTask itself.
func TestExecute_Success(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "营收增长了 12%"})
	tk := sampleTask()
	te.setTaskCompleted() // simulate save_task_result having been called

	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), tk)
	require.NoError(t, err)

	te.tasks.AssertCalled(t, "UpdateStatus", "task_1", domaintask.StatusRunning)
	te.notif.AssertCalled(t, "Send", mock.Anything, mock.Anything, "task", []string{"u1"})

	// In the save_task_result flow, the executor does NOT call
	// UpdateTaskResult / UpdateStatus(completed) itself — the LLM-driven tool
	// does. Asserting their absence prevents regressions where the executor
	// re-introduces a parallel completion path.
	te.tasks.AssertNotCalled(t, "UpdateTaskResult", mock.Anything, mock.Anything)
	te.tasks.AssertNotCalled(t, "UpdateError", mock.Anything, mock.Anything)
}

// ── Execute: runtime error → failure path ──

func TestExecute_RuntimeError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{err: fmt.Errorf("model timeout")})
	tk := sampleTask()

	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	te.tasks.On("UpdateError", "task_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), tk)
	require.Error(t, err)

	te.tasks.AssertCalled(t, "UpdateStatus", "task_1", domaintask.StatusRunning)
	te.tasks.AssertCalled(t, "UpdateError", "task_1", mock.Anything)
	te.tasks.AssertNotCalled(t, "UpdateTaskResult", mock.Anything, mock.Anything)
	te.notif.AssertCalled(t, "Send", "任务失败", mock.Anything, "task", []string{"u1"})

	// Verify the error message is persisted.
	for _, c := range te.tasks.Calls {
		if c.Method == "UpdateError" {
			assert.Contains(t, c.Arguments.String(1), "model timeout")
		}
	}
}

// ── Execute: ADK session create failure → failTask ──

func TestExecute_ADKSessionCreateError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	te.patchADKCreateError(fmt.Errorf("mongo down"))
	tk := sampleTask()

	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	te.tasks.On("UpdateError", "task_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), tk)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adk session init")

	te.tasks.AssertCalled(t, "UpdateError", "task_1", mock.Anything)
	te.tasks.AssertNotCalled(t, "UpdateTaskResult", mock.Anything, mock.Anything)
}

// ── Execute: runtime resolve failure → failTask ──

func TestExecute_RuntimeResolveError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	te.patchGetOrCreateError(fmt.Errorf("model not found"))
	tk := sampleTask()

	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	te.tasks.On("UpdateError", "task_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), tk)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve runtime")

	te.tasks.AssertCalled(t, "UpdateError", "task_1", mock.Anything)
}

// ── Execute: system-owned task skips notification ──

func TestExecute_SystemUserSkipsNotification(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "scheduled result"})
	tk := sampleTask()
	tk.UserID = "system" // scheduled task
	te.setTaskCompleted()

	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)

	err := te.exec.Execute(context.Background(), tk)
	require.NoError(t, err)

	// System user must not receive a notification.
	te.notif.AssertNotCalled(t, "Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ── Execute: nil notifier does not panic (defensive) ──

func TestExecute_NilNotifier(t *testing.T) {
	adkSess := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName: "data-agent", Model: &fakeLLM{text: "ok"}, SessionService: adkSess,
	})
	require.NoError(t, err)
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{AppName: "data-agent", SessionService: adkSess})
	tasks := domaintaskmocks.NewTaskService(t)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	exec := NewAgentExecutor(registry, adkSess, tasks, nil, cbReg) // nil notifier

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodFunc(registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return rt, nil
	})

	tasks.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	tasks.On("UpdateError", mock.Anything, mock.Anything).Return(nil)
	// wasCancelled re-check (returns a non-cancelled task).
	tasks.On("GetTask", mock.Anything).Return(&domaintask.Task{Status: domaintask.StatusRunning}, nil)

	require.NotPanics(t, func() {
		_ = exec.Execute(context.Background(), sampleTask())
	})
}

// ── Execute: empty SessionID uses the ADK-generated session ID ──

// TestExecute_EmptySessionID exercises the POST /tasks path where a task has no
// session binding (SessionID=""). The executor creates an ADK session (which
// auto-generates an ID) and uses that ID for Run, so execution succeeds.
func TestExecute_EmptySessionID(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "async result"})
	tk := sampleTask()
	tk.SessionID = "" // task created via POST /tasks without a session binding
	te.setTaskCompleted()

	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), tk)
	require.NoError(t, err, "empty SessionID should not fail — executor creates an ADK session")
}

// ── Execute: nil circuit breaker runs unprotected (defensive) ──

func TestExecute_NilCircuitBreaker(t *testing.T) {
	adkSess := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName: "data-agent", Model: &fakeLLM{text: "ok"}, SessionService: adkSess,
	})
	require.NoError(t, err)
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{AppName: "data-agent", SessionService: adkSess})
	tasks := domaintaskmocks.NewTaskService(t)
	notif := notificationmocks.NewNotificationService(t)
	exec := NewAgentExecutor(registry, adkSess, tasks, notif, nil) // nil cbReg

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodFunc(registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return rt, nil
	})

	tasks.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	tasks.On("UpdateError", mock.Anything, mock.Anything).Return(nil)
	tasks.On("GetTask", mock.Anything).Return(&domaintask.Task{Status: domaintask.StatusRunning}, nil)
	notif.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	// fakeLLM doesn't call save_task_result — the executor retries and
	// eventually fails the task. With nil circuit breaker the run is
	// unprotected (defensive code path). Expect no panic, run completes.
	err = exec.Execute(context.Background(), sampleTask())
	require.Error(t, err)
}

// ── Execute: cancellation handling (SPEC-063) ──

// TestExecute_SkipsAlreadyCancelledTask verifies a task cancelled before the
// worker picked it up is skipped entirely (no running status, no execution).
func TestExecute_SkipsAlreadyCancelledTask(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	tk := sampleTask()
	tk.Status = domaintask.StatusCancelled // cancelled before pickup

	err := te.exec.Execute(context.Background(), tk)
	require.NoError(t, err)

	// Must NOT transition to running or call the model.
	te.tasks.AssertNotCalled(t, "UpdateStatus", "task_1", domaintask.StatusRunning)
	te.tasks.AssertNotCalled(t, "UpdateTaskResult", mock.Anything, mock.Anything)
	te.notif.AssertNotCalled(t, "Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestExecute_CancelledDuringExecution verifies that a task cancelled while
// RunAndCollect is in flight keeps its cancelled status — the executor must NOT
// overwrite it with completed/failed. Built without newTestExecutor so the
// wasCancelled GetTask can return a cancelled task (newTestExecutor's .Maybe()
// would otherwise shadow it).
func TestExecute_CancelledDuringExecution(t *testing.T) {
	adkSess := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName: "data-agent", Model: &fakeLLM{text: "result"}, SessionService: adkSess,
	})
	require.NoError(t, err)
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{AppName: "data-agent", SessionService: adkSess})
	tasks := domaintaskmocks.NewTaskService(t)
	notif := notificationmocks.NewNotificationService(t)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	exec := NewAgentExecutor(registry, adkSess, tasks, notif, cbReg)

	patches := gomonkey.NewPatches()
	t.Cleanup(patches.Reset)
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodFunc(registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return rt, nil
	})

	tk := sampleTask()
	tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	// wasCancelled re-check returns a CANCELLED task → executor must skip
	// save_task_result logic and not overwrite cancelled with completed/failed.
	tasks.On("GetTask", "task_1").Return(&domaintask.Task{ID: "task_1", Status: domaintask.StatusCancelled}, nil)

	err = exec.Execute(context.Background(), tk)
	require.NoError(t, err)

	tasks.AssertNotCalled(t, "UpdateTaskResult", mock.Anything, mock.Anything)
	tasks.AssertNotCalled(t, "UpdateStatus", "task_1", domaintask.StatusCompleted)
	notif.AssertNotCalled(t, "Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ── deriveUserMessage: key priority (L1) ──

func TestDeriveUserMessage(t *testing.T) {
	cases := []struct {
		name   string
		params map[string]interface{}
		want   string
	}{
		{"query wins", map[string]interface{}{"query": "q", "message": "m", "title": "t"}, "q"},
		{"message next", map[string]interface{}{"message": "m", "prompt": "p", "title": "t"}, "m"},
		{"prompt next", map[string]interface{}{"prompt": "p", "description": "d", "title": "t"}, "p"},
		{"description next", map[string]interface{}{"description": "d", "title": "t"}, "d"},
		{"title fallback", map[string]interface{}{"title": "t"}, "t"},
		{"empty params", map[string]interface{}{}, ""},
		{"nil params", nil, ""},
		{"blank value skipped", map[string]interface{}{"query": "  ", "message": "real"}, "real"},
		{"non-string ignored", map[string]interface{}{"query": 123, "title": "t"}, "t"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			tk := &domaintask.Task{Params: tc.params}
			assert.Equal(t, tc.want, deriveUserMessage(tk))
		})
	}
}

// ── buildExecutorState: identity injection (L1) ──

func TestBuildExecutorState(t *testing.T) {
	t.Run("basic identity", func(t *testing.T) {
		tk := &domaintask.Task{ID: "t1", UserID: "u1", SessionID: "s1"}
		st := buildExecutorState(tk)
		assert.Equal(t, "u1", st["user_id"])
		assert.Equal(t, "s1", st["session_id"])
		assert.Equal(t, "t1", st["task_id"])
		_, ok := st["kb_id"]
		assert.False(t, ok)
	})
	t.Run("with kb_id", func(t *testing.T) {
		tk := &domaintask.Task{ID: "t1", UserID: "u1", SessionID: "s1", Params: map[string]interface{}{"kb_id": "kb-9"}}
		st := buildExecutorState(tk)
		assert.Equal(t, "kb-9", st["kb_id"])
	})
	t.Run("blank kb_id omitted", func(t *testing.T) {
		tk := &domaintask.Task{ID: "t1", UserID: "u1", SessionID: "s1", Params: map[string]interface{}{"kb_id": ""}}
		st := buildExecutorState(tk)
		_, ok := st["kb_id"]
		assert.False(t, ok)
	})
}

// ── Execute: notification send failure is logged, not fatal ──

func TestExecute_NotificationSendError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	tk := sampleTask()
	te.setTaskCompleted() // simulate save_task_result fired

	te.tasks.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	// notif.Send fails — the executor must log and continue (not panic).
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).
		Return(nil, fmt.Errorf("notif service down"))

	require.NotPanics(t, func() {
		err := te.exec.Execute(context.Background(), tk)
		require.NoError(t, err, "notification failure must not fail the task")
	})
	te.notif.AssertNumberOfCalls(t, "Send", 1)
}

// TestFailTask_PersistsErrorAndNotifies verifies the failure path
// (UpdateError + notify). completeTask was removed when the executor was
// refactored to read save_task_result from the task DB row rather than
// driving completion itself.
func TestFailTask_PersistsErrorAndNotifies(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	tk := sampleTask()
	te.tasks.On("UpdateError", "task_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", mock.Anything).Return(nil, nil)

	te.exec.failTask(tk, fmt.Errorf("boom"))
	te.tasks.AssertCalled(t, "UpdateError", "task_1", "boom")
	te.notif.AssertNumberOfCalls(t, "Send", 1)
}

// ensure strings import is used (assert message helper).
var _ = strings.Contains
