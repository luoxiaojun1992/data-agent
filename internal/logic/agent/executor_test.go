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
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	domaintaskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
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
	exec     *AgentExecutor
	registry *adkruntime.Registry
	rt       *adkruntime.Runtime
	runs     *domaintaskmocks.TaskRunService
	notif    *notificationmocks.NotificationService
	patches  *gomonkey.Patches
	adkSess  adksession.Service
	// completed controls the default GetRun mock. When false (default),
	// GetRun returns a non-cancelled running run — exercising the
	// "no save_task_result" retry/failure path. When flipped to true via
	// setRunCompleted, subsequent GetRun calls return a completed run
	// (the LLM-driven tool is simulated to have fired).
	completed atomic.Bool
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
	runs := domaintaskmocks.NewTaskRunService(t)
	notif := notificationmocks.NewNotificationService(t)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	exec := NewAgentExecutor(registry, adkSess, runs, notif, cbReg)

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
	// GetRun is consulted several times during Execute: wasRunCancelled() and
	// the post-run "did save_task_result get called?" check. Default both
	// return a non-cancelled run that has NOT been completed (so the retry
	// path is exercised). Tests that want success use setRunCompleted to flip
	// the result for subsequent GetRun calls.
	te := &testExecutor{exec: exec, registry: registry, rt: rt, runs: runs, notif: notif, patches: patches, adkSess: adkSess}
	runs.On("GetRun", mock.Anything).Return(func(id string) *domaintask.TaskRun {
		if te.completed.Load() {
			return &domaintask.TaskRun{ID: "run_1", Status: domaintask.StatusCompleted}
		}
		return &domaintask.TaskRun{ID: "run_1", Status: domaintask.StatusRunning}
	}, nil).Maybe()
	// Empty-session tests trigger a session-ID write-back.
	runs.On("UpdateRunSessionID", mock.Anything, mock.Anything).Return(nil).Maybe()
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

// setRunCompleted flips the default GetRun mock to return a completed run for
// all subsequent calls, simulating the LLM having called save_task_result.
func (te *testExecutor) setRunCompleted() {
	te.completed.Store(true)
}

// patchADKCreateError makes the ADK session Create fail.
func (te *testExecutor) patchADKCreateError(err error) {
	te.patches.ApplyMethodReturn(te.adkSess, "Create", (*adksession.CreateResponse)(nil), err)
}

func sampleRun() *domaintask.TaskRun {
	return &domaintask.TaskRun{
		ID:        "run_1",
		TaskID:    "task_1",
		UserID:    "u1",
		Type:      "agent_exec",
		ModelID:   "m1",
		SessionID: "sess_1",
		Status:    domaintask.StatusQueued,
		Params:    map[string]interface{}{"message": "分析营收"},
	}
}

// ── Execute: success ──

// In the save_task_result flow, success means: the LLM called the
// save_task_result tool during its turn. The fakeLLM doesn't call tools, so
// we simulate that by mocking GetRun to return Status=completed after the
// run. The executor reads the run to confirm the result was persisted and
// notifies the user without calling UpdateRunResult itself.
func TestExecute_Success(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "营收增长了 12%"})
	run := sampleRun()
	te.setRunCompleted() // simulate save_task_result having been called

	te.runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), run)
	require.NoError(t, err)

	te.runs.AssertCalled(t, "UpdateRunStatus", "run_1", domaintask.StatusRunning)
	te.notif.AssertCalled(t, "Send", mock.Anything, mock.Anything, "task", []string{"u1"})

	// In the save_task_result flow, the executor does NOT call
	// UpdateRunResult / UpdateRunError(completed) itself — the LLM-driven tool
	// does. Asserting their absence prevents regressions where the executor
	// re-introduces a parallel completion path.
	te.runs.AssertNotCalled(t, "UpdateRunResult", mock.Anything, mock.Anything)
	te.runs.AssertNotCalled(t, "UpdateRunError", mock.Anything, mock.Anything)
}

// ── Execute: runtime error → failure path ──

func TestExecute_RuntimeError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{err: fmt.Errorf("model timeout")})
	run := sampleRun()

	te.runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)
	te.runs.On("UpdateRunError", "run_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), run)
	require.Error(t, err)

	te.runs.AssertCalled(t, "UpdateRunStatus", "run_1", domaintask.StatusRunning)
	te.runs.AssertCalled(t, "UpdateRunError", "run_1", mock.Anything)
	te.runs.AssertNotCalled(t, "UpdateRunResult", mock.Anything, mock.Anything)
	te.notif.AssertCalled(t, "Send", "任务失败", mock.Anything, "task", []string{"u1"})

	// Verify the error message is persisted.
	for _, c := range te.runs.Calls {
		if c.Method == "UpdateRunError" {
			assert.Contains(t, c.Arguments.String(1), "model timeout")
		}
	}
}

// ── Execute: ADK session create failure → failRun ──

func TestExecute_ADKSessionCreateError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	te.patchADKCreateError(fmt.Errorf("mongo down"))
	run := sampleRun()

	te.runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)
	te.runs.On("UpdateRunError", "run_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "adk session init")

	te.runs.AssertCalled(t, "UpdateRunError", "run_1", mock.Anything)
	te.runs.AssertNotCalled(t, "UpdateRunResult", mock.Anything, mock.Anything)
}

// ── Execute: runtime resolve failure → failRun ──

func TestExecute_RuntimeResolveError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	te.patchGetOrCreateError(fmt.Errorf("model not found"))
	run := sampleRun()

	te.runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)
	te.runs.On("UpdateRunError", "run_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve runtime")

	te.runs.AssertCalled(t, "UpdateRunError", "run_1", mock.Anything)
}

// ── Execute: system-owned run skips notification ──

func TestExecute_SystemUserSkipsNotification(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "scheduled result"})
	run := sampleRun()
	run.UserID = "system" // scheduled task
	te.setRunCompleted()

	te.runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)

	err := te.exec.Execute(context.Background(), run)
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
	runs := domaintaskmocks.NewTaskRunService(t)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	exec := NewAgentExecutor(registry, adkSess, runs, nil, cbReg) // nil notifier

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodFunc(registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return rt, nil
	})

	runs.On("UpdateRunStatus", mock.Anything, mock.Anything).Return(nil)
	runs.On("UpdateRunError", mock.Anything, mock.Anything).Return(nil)
	// wasRunCancelled / save_task_result re-checks (returns a non-cancelled,
	// non-completed run).
	runs.On("GetRun", mock.Anything).Return(&domaintask.TaskRun{Status: domaintask.StatusRunning}, nil)

	require.NotPanics(t, func() {
		_ = exec.Execute(context.Background(), sampleRun())
	})
}

// ── Execute: empty SessionID uses the ADK-generated session ID ──

// TestExecute_EmptySessionID exercises the POST /tasks path where a run has no
// session binding (SessionID=""). The executor creates an ADK session (which
// auto-generates an ID) and uses that ID for Run, so execution succeeds.
func TestExecute_EmptySessionID(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "async result"})
	run := sampleRun()
	run.SessionID = "" // task created via POST /tasks without a session binding
	te.setRunCompleted()

	te.runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), run)
	require.NoError(t, err, "empty SessionID should not fail — executor creates an ADK session")

	// The ADK-generated session ID must be written back to the run.
	te.runs.AssertCalled(t, "UpdateRunSessionID", "run_1", mock.Anything)
}

// ── Execute: nil circuit breaker runs unprotected (defensive) ──

func TestExecute_NilCircuitBreaker(t *testing.T) {
	adkSess := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName: "data-agent", Model: &fakeLLM{text: "ok"}, SessionService: adkSess,
	})
	require.NoError(t, err)
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{AppName: "data-agent", SessionService: adkSess})
	runs := domaintaskmocks.NewTaskRunService(t)
	notif := notificationmocks.NewNotificationService(t)
	exec := NewAgentExecutor(registry, adkSess, runs, notif, nil) // nil cbReg

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodFunc(registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return rt, nil
	})

	runs.On("UpdateRunStatus", mock.Anything, mock.Anything).Return(nil)
	runs.On("UpdateRunError", mock.Anything, mock.Anything).Return(nil)
	runs.On("GetRun", mock.Anything).Return(&domaintask.TaskRun{Status: domaintask.StatusRunning}, nil)
	notif.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, nil)

	// fakeLLM doesn't call save_task_result — the executor retries and
	// eventually fails the run. With nil circuit breaker the run is
	// unprotected (defensive code path). Expect no panic, run completes.
	err = exec.Execute(context.Background(), sampleRun())
	require.Error(t, err)
}

// ── Execute: cancellation handling (SPEC-063) ──

// TestExecute_SkipsAlreadyCancelledRun verifies a run cancelled before the
// worker picked it up is skipped entirely (no running status, no execution).
func TestExecute_SkipsAlreadyCancelledRun(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	run := sampleRun()
	run.Status = domaintask.StatusCancelled // cancelled before pickup

	err := te.exec.Execute(context.Background(), run)
	require.NoError(t, err)

	// Must NOT transition to running or call the model.
	te.runs.AssertNotCalled(t, "UpdateRunStatus", "run_1", domaintask.StatusRunning)
	te.runs.AssertNotCalled(t, "UpdateRunResult", mock.Anything, mock.Anything)
	te.notif.AssertNotCalled(t, "Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// TestExecute_CancelledDuringExecution verifies that a run cancelled while
// RunAndCollectContent is in flight keeps its cancelled status — the executor
// must NOT overwrite it with completed/failed. Built without newTestExecutor
// so the wasRunCancelled GetRun can return a cancelled run
// (newTestExecutor's .Maybe() would otherwise shadow it).
func TestExecute_CancelledDuringExecution(t *testing.T) {
	adkSess := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName: "data-agent", Model: &fakeLLM{text: "result"}, SessionService: adkSess,
	})
	require.NoError(t, err)
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{AppName: "data-agent", SessionService: adkSess})
	runs := domaintaskmocks.NewTaskRunService(t)
	notif := notificationmocks.NewNotificationService(t)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	exec := NewAgentExecutor(registry, adkSess, runs, notif, cbReg)

	patches := gomonkey.NewPatches()
	t.Cleanup(patches.Reset)
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodFunc(registry, "GetOrCreateWithInstruction", func(ctx context.Context, modelID string, suffix string) (*adkruntime.Runtime, error) {
		return rt, nil
	})

	run := sampleRun()
	runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)
	// wasRunCancelled re-check returns a CANCELLED run → executor must skip
	// save_task_result logic and not overwrite cancelled with completed/failed.
	runs.On("GetRun", "run_1").Return(&domaintask.TaskRun{ID: "run_1", Status: domaintask.StatusCancelled}, nil)

	err = exec.Execute(context.Background(), run)
	require.NoError(t, err)

	runs.AssertNotCalled(t, "UpdateRunResult", mock.Anything, mock.Anything)
	runs.AssertNotCalled(t, "UpdateRunStatus", "run_1", domaintask.StatusCompleted)
	notif.AssertNotCalled(t, "Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ── deriveUserMessageFromParams: key priority (L1) ──

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
			text, images := deriveUserMessageFromParams(tc.params)
			assert.Equal(t, tc.want, text)
			assert.Empty(t, images)
		})
	}
}

// ── deriveUserMessageFromParams: image recovery (task images) ──

func TestDeriveUserMessageFromParams_RecoversImages(t *testing.T) {
	img := domainchat.ImagePart{Data: "aGVsbG8=", MimeType: "image/png"}
	encoded, err := domainchat.EncodeImages([]domainchat.ImagePart{img})
	require.NoError(t, err)

	text, images := deriveUserMessageFromParams(map[string]interface{}{
		"message": "看这张图",
		"images":  encoded,
	})
	assert.Equal(t, "看这张图", text)
	require.Len(t, images, 1)
	assert.Equal(t, img.Data, images[0].Data)
	assert.Equal(t, img.MimeType, images[0].MimeType)
}

func TestDeriveUserMessageFromParams_MalformedImagesIgnored(t *testing.T) {
	text, images := deriveUserMessageFromParams(map[string]interface{}{
		"message": "hi",
		"images":  "not-json{{{",
	})
	assert.Equal(t, "hi", text)
	assert.Empty(t, images, "malformed images JSON must be ignored, not fatal")

	// Non-string images values are ignored too.
	_, images = deriveUserMessageFromParams(map[string]interface{}{"images": 42})
	assert.Empty(t, images)
}

// ── buildTaskContent: multimodal content assembly ──

func TestBuildTaskContent(t *testing.T) {
	img := domainchat.ImagePart{Data: "aGVsbG8=", MimeType: "image/png"}

	// text + image → [text, inline image]
	c, err := buildTaskContent("看这张图", []domainchat.ImagePart{img, img})
	require.NoError(t, err)
	assert.Equal(t, "user", c.Role)
	require.Len(t, c.Parts, 3)
	assert.Equal(t, "看这张图", c.Parts[0].Text)
	require.NotNil(t, c.Parts[1].InlineData)
	assert.Equal(t, "image/png", c.Parts[1].InlineData.MIMEType)
	assert.Equal(t, []byte("hello"), c.Parts[1].InlineData.Data)

	// image-only → [inline image]
	c, err = buildTaskContent("", []domainchat.ImagePart{img})
	require.NoError(t, err)
	require.Len(t, c.Parts, 1)
	assert.NotNil(t, c.Parts[0].InlineData)

	// validation errors propagate
	_, err = buildTaskContent("x", []domainchat.ImagePart{{Data: "!!!", MimeType: "image/png"}})
	assert.Error(t, err)
}

// ── Execute: invalid image params fail the run before any LLM call ──

func TestExecute_InvalidImageParamsFailsRun(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "should not run"})
	run := sampleRun()
	// Valid JSON envelope but the payload fails base64/mime validation.
	run.Params["images"] = `[{"data":"!!!not-base64","mime_type":"image/png"}]`

	te.runs.On("UpdateRunStatus", "run_1", domaintask.StatusRunning).Return(nil)
	te.runs.On("UpdateRunError", "run_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).Return(nil, nil)

	err := te.exec.Execute(context.Background(), run)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid task image attachments")

	te.runs.AssertCalled(t, "UpdateRunError", "run_1", mock.Anything)
	te.runs.AssertNotCalled(t, "UpdateRunResult", mock.Anything, mock.Anything)
}

// ── buildRunState: identity injection (L1) ──

func TestBuildRunState(t *testing.T) {
	t.Run("basic identity", func(t *testing.T) {
		run := &domaintask.TaskRun{ID: "r1", TaskID: "t1", UserID: "u1", SessionID: "s1"}
		st := buildRunState(run)
		assert.Equal(t, "u1", st["user_id"])
		assert.Equal(t, "s1", st["session_id"])
		assert.Equal(t, "t1", st["task_id"])
		assert.Equal(t, "r1", st["run_id"])
		_, ok := st["kb_id"]
		assert.False(t, ok)
	})
	t.Run("with kb_id", func(t *testing.T) {
		run := &domaintask.TaskRun{ID: "r1", TaskID: "t1", UserID: "u1", SessionID: "s1", Params: map[string]interface{}{"kb_id": "kb-9"}}
		st := buildRunState(run)
		assert.Equal(t, "kb-9", st["kb_id"])
	})
	t.Run("blank kb_id omitted", func(t *testing.T) {
		run := &domaintask.TaskRun{ID: "r1", TaskID: "t1", UserID: "u1", SessionID: "s1", Params: map[string]interface{}{"kb_id": ""}}
		st := buildRunState(run)
		_, ok := st["kb_id"]
		assert.False(t, ok)
	})
}

// ── Execute: notification send failure is logged, not fatal ──

func TestExecute_NotificationSendError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	run := sampleRun()
	te.setRunCompleted() // simulate save_task_result fired

	te.runs.On("UpdateRunStatus", mock.Anything, mock.Anything).Return(nil)
	// notif.Send fails — the executor must log and continue (not panic).
	te.notif.On("Send", mock.Anything, mock.Anything, "task", []string{"u1"}).
		Return(nil, fmt.Errorf("notif service down"))

	require.NotPanics(t, func() {
		err := te.exec.Execute(context.Background(), run)
		require.NoError(t, err, "notification failure must not fail the task")
	})
	te.notif.AssertNumberOfCalls(t, "Send", 1)
}

// TestFailRun_PersistsErrorAndNotifies verifies the failure path
// (UpdateRunError + notify).
func TestFailRun_PersistsErrorAndNotifies(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "ok"})
	run := sampleRun()
	te.runs.On("UpdateRunError", "run_1", mock.Anything).Return(nil)
	te.notif.On("Send", mock.Anything, mock.Anything, "task", mock.Anything).Return(nil, nil)

	te.exec.failRun(run, fmt.Errorf("boom"))
	te.runs.AssertCalled(t, "UpdateRunError", "run_1", "boom")
	te.notif.AssertNumberOfCalls(t, "Send", 1)
}

// ensure strings import is used (assert message helper).
var _ = strings.Contains
