package agent

import (
	"context"
	"fmt"
	"iter"
	"strings"
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
	exec      *AgentExecutor
	registry  *adkruntime.Registry
	rt        *adkruntime.Runtime
	tasks     *domaintaskmocks.TaskService
	notif     *notificationmocks.NotificationService
	patches   *gomonkey.Patches
	adkSess   adksession.Service
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
	// Patch GetOrCreate to return the test Runtime (avoids needing a real
	// Provider/model config for unit tests) — same pattern as chat_test.go.
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})

	return &testExecutor{exec: exec, registry: registry, rt: rt, tasks: tasks, notif: notif, patches: patches, adkSess: adkSess}
}

// patchGetOrCreateError makes Registry.GetOrCreate return an error (runtime
// resolution failure). Useful for the resolve-runtime failure path.
func (te *testExecutor) patchGetOrCreateError(err error) {
	te.patches.ApplyMethodFunc(te.registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return nil, err
	})
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

func TestExecute_Success(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "营收增长了 12%"})
	tk := sampleTask()

	// The executor calls: UpdateStatus(running) → (run) → UpdateTaskResult →
	// UpdateStatus(completed) → notif.Send. TaskService methods are best-effort
	// (errors ignored) so returning nil is fine; we assert they were CALLED.
	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	te.tasks.On("UpdateTaskResult", "task_1", mock.Anything).Return(nil)
	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusCompleted).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), tk)
	require.NoError(t, err)

	te.tasks.AssertCalled(t, "UpdateStatus", "task_1", domaintask.StatusRunning)
	te.tasks.AssertCalled(t, "UpdateTaskResult", "task_1", mock.Anything)
	te.tasks.AssertCalled(t, "UpdateStatus", "task_1", domaintask.StatusCompleted)
	te.notif.AssertCalled(t, "Send", mock.Anything, mock.Anything, "task", []string{"u1"})

	// §9.5: UpdateStatus(running) must precede UpdateStatus(completed).
	runningIdx, completedIdx := -1, -1
	for i, c := range te.tasks.Calls {
		if c.Method == "UpdateStatus" && len(c.Arguments) > 1 && c.Arguments[1] == domaintask.StatusRunning {
			runningIdx = i
		}
		if c.Method == "UpdateStatus" && len(c.Arguments) > 1 && c.Arguments[1] == domaintask.StatusCompleted {
			completedIdx = i
		}
	}
	assert.GreaterOrEqual(t, runningIdx, 0, "UpdateStatus(running) should be called")
	assert.GreaterOrEqual(t, completedIdx, 0, "UpdateStatus(completed) should be called")
	assert.Less(t, runningIdx, completedIdx, "UpdateStatus(running) must precede UpdateStatus(completed)")

	// Verify the result carries the LLM output.
	for _, c := range te.tasks.Calls {
		if c.Method == "UpdateTaskResult" {
			res := c.Arguments.Get(1).(map[string]interface{})
			assert.Equal(t, "营收增长了 12%", res["content"])
			assert.Equal(t, "success", res["status"])
		}
	}
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

	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusRunning).Return(nil)
	te.tasks.On("UpdateTaskResult", "task_1", mock.Anything).Return(nil)
	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusCompleted).Return(nil)

	err := te.exec.Execute(context.Background(), tk)
	require.NoError(t, err)

	// System user must not receive a notification.
	te.notif.AssertNotCalled(t, "Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
	te.tasks.AssertCalled(t, "UpdateStatus", "task_1", domaintask.StatusCompleted)
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

	tasks.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	tasks.On("UpdateTaskResult", mock.Anything, mock.Anything).Return(nil)

	require.NotPanics(t, func() {
		_ = exec.Execute(context.Background(), sampleTask())
	})
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

	tasks.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	tasks.On("UpdateTaskResult", mock.Anything, mock.Anything).Return(nil)
	notif.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	err = exec.Execute(context.Background(), sampleTask())
	require.NoError(t, err)
	tasks.AssertCalled(t, "UpdateStatus", "task_1", domaintask.StatusCompleted)
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

	te.tasks.On("UpdateStatus", mock.Anything, mock.Anything).Return(nil)
	te.tasks.On("UpdateTaskResult", mock.Anything, mock.Anything).Return(nil)
	// notif.Send fails — the executor must log and continue (not panic).
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).
		Return(nil, fmt.Errorf("notif service down"))

	require.NotPanics(t, func() {
		err := te.exec.Execute(context.Background(), tk)
		require.NoError(t, err, "notification failure must not fail the task")
	})
	te.notif.AssertNumberOfCalls(t, "Send", 1)
}

func TestCompleteTask_PersistsResultAndNotifies(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	tk := sampleTask()
	te.tasks.On("UpdateTaskResult", "task_1", mock.Anything).Return(nil)
	te.tasks.On("UpdateStatus", "task_1", domaintask.StatusCompleted).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", mock.Anything).Return(nil, nil)

	te.exec.completeTask(tk, "final answer")
	te.tasks.AssertCalled(t, "UpdateTaskResult", "task_1", mock.Anything)
	te.tasks.AssertCalled(t, "UpdateStatus", "task_1", domaintask.StatusCompleted)
	te.notif.AssertNumberOfCalls(t, "Send", 1)
}

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
