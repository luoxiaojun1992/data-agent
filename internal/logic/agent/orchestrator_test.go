package agent

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	domainchatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	domaintaskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func newTestOrchestrator(t *testing.T) (*Orchestrator, *domainchatmocks.SessionService, *domaintaskmocks.TaskService) {
	t.Helper()
	sessions := domainchatmocks.NewSessionService(t)
	tasks := domaintaskmocks.NewTaskService(t)
	return NewOrchestrator(sessions, tasks, nil), sessions, tasks
}

func TestCreateAgentTask_Success(t *testing.T) {
	orch, sessions, tasks := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent", mock.Anything).Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	tk := &domaintask.Task{ID: "task_1", CreatedAt: time.Now()}
	run := &domaintask.TaskRun{SessionID: "s1", Status: domaintask.StatusPending}
	tasks.On("CreateTask", "u1", "agent", []string{"stats_engine"}, mock.Anything, "", "", "", (*time.Time)(nil)).Return(tk, run, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{
		Title:      "t",
		SkillChain: []string{"stats_engine"},
		Params:     map[string]interface{}{"k": 1},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.TaskID != "task_1" || resp.SessionID != "s1" || resp.Status != "pending" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.Note != "" {
		t.Errorf("success path should not carry a note: %q", resp.Note)
	}
}

func TestCreateAgentTask_NoTaskService(t *testing.T) {
	sessions := domainchatmocks.NewSessionService(t)
	orch := NewOrchestrator(sessions, nil, nil) // no task service
	sessions.On("Create", "u1", "agent", mock.Anything).Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if resp.TaskID != "task_memory_fallback" {
		t.Errorf("fallback task id = %v", resp.TaskID)
	}
	if resp.Note == "" {
		t.Errorf("fallback should carry a note")
	}
	if resp.Status != "queued" {
		t.Errorf("fallback status = %v", resp.Status)
	}
}

func TestCreateAgentTask_SessionError(t *testing.T) {
	orch, sessions, _ := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent", mock.Anything).Return((*domainchat.Session)(nil), errSessionDown)

	_, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err == nil {
		t.Error("expected session error")
	}
}

func TestCreateAgentTask_TaskError(t *testing.T) {
	orch, sessions, tasks := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent", mock.Anything).Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	tasks.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return((*domaintask.Task)(nil), (*domaintask.TaskRun)(nil), errQueueDown)

	_, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err == nil {
		t.Error("expected task creation error")
	}
}

func TestCreateAgentTask_NilSkillChainNormalized(t *testing.T) {
	orch, sessions, tasks := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent", mock.Anything).Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	tk := &domaintask.Task{ID: "task_2", CreatedAt: time.Now()}
	run := &domaintask.TaskRun{SessionID: "s1", Status: domaintask.StatusQueued}
	// Nil skill chain should be normalized to empty slice, not nil.
	tasks.On("CreateTask", "u1", "agent", []string{}, mock.Anything, "", "", "", (*time.Time)(nil)).Return(tk, run, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.TaskID != "task_2" {
		t.Errorf("task id = %v", resp.TaskID)
	}
}

// sentinel errors for test clarity.
var (
	errSessionDown = errString("session store unavailable")
	errQueueDown   = errString("queue unavailable")
)

type errString string

func (e errString) Error() string { return string(e) }

// ── enrichTaskParams / lastUserMessageText / hasUserMessageKey (SPEC-063) ──

func TestEnrichTaskParams_InjectsTitleAndMessage(t *testing.T) {
	req := CreateAgentTaskRequest{
		Title:    "营收分析",
		Messages: []domainchat.Message{{Role: "user", Content: "分析上季度营收"}},
	}
	params := enrichTaskParams(req)
	assert.Equal(t, "营收分析", params["title"])
	assert.Equal(t, "分析上季度营收", params["message"])
}

func TestEnrichTaskParams_CallerKeysTakePrecedence(t *testing.T) {
	// Caller-provided query/message must not be overwritten.
	req := CreateAgentTaskRequest{
		Title:    "t",
		Messages: []domainchat.Message{{Role: "user", Content: "from-messages"}},
		Params:   map[string]interface{}{"query": "from-caller"},
	}
	params := enrichTaskParams(req)
	assert.Equal(t, "from-caller", params["query"])
	// message not injected because a message key (query) already exists.
	_, hasMsg := params["message"]
	assert.False(t, hasMsg)
}

func TestEnrichTaskParams_DoesNotOverwriteExistingMessage(t *testing.T) {
	req := CreateAgentTaskRequest{
		Messages: []domainchat.Message{{Role: "user", Content: "ignored"}},
		Params:   map[string]interface{}{"message": "kept"},
	}
	params := enrichTaskParams(req)
	assert.Equal(t, "kept", params["message"])
}

func TestEnrichTaskParams_NoMessages(t *testing.T) {
	req := CreateAgentTaskRequest{Title: "t", Params: map[string]interface{}{"k": 1}}
	params := enrichTaskParams(req)
	assert.Equal(t, "t", params["title"])
	assert.Equal(t, 1, params["k"])
	_, hasMsg := params["message"]
	assert.False(t, hasMsg, "no message injected when there are no user messages")
}

func TestEnrichTaskParams_NilParams(t *testing.T) {
	req := CreateAgentTaskRequest{Title: "t", Messages: []domainchat.Message{{Role: "user", Content: "hi"}}}
	params := enrichTaskParams(req)
	assert.Equal(t, "t", params["title"])
	assert.Equal(t, "hi", params["message"])
}

func TestEnrichTaskParams_InjectsImages(t *testing.T) {
	img := domainchat.ImagePart{Data: "aGVsbG8=", MimeType: "image/png"}
	req := CreateAgentTaskRequest{
		Title:  "看图分析",
		Images: []domainchat.ImagePart{img, img},
	}
	params := enrichTaskParams(req)
	encoded, ok := params["images"].(string)
	assert.True(t, ok, "images should be injected as a JSON string")
	decoded, err := domainchat.DecodeImages(encoded)
	assert.NoError(t, err)
	assert.Len(t, decoded, 2)
	assert.Equal(t, "image/png", decoded[0].MimeType)
}

func TestEnrichTaskParams_DoesNotOverwriteExistingImages(t *testing.T) {
	req := CreateAgentTaskRequest{
		Title:  "t",
		Images: []domainchat.ImagePart{{Data: "aGVsbG8=", MimeType: "image/png"}},
		Params: map[string]interface{}{"images": "already-set"},
	}
	params := enrichTaskParams(req)
	assert.Equal(t, "already-set", params["images"])
}

func TestLastUserMessageText(t *testing.T) {
	assert.Equal(t, "second", lastUserMessageText([]domainchat.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: "second"},
	}))
	assert.Equal(t, "", lastUserMessageText([]domainchat.Message{{Role: "assistant", Content: "x"}}))
	assert.Equal(t, "", lastUserMessageText([]domainchat.Message{{Role: "user", Content: "  "}}))
	assert.Equal(t, "", lastUserMessageText(nil))
}

func TestHasUserMessageKey(t *testing.T) {
	assert.True(t, hasUserMessageKey(map[string]interface{}{"query": "q"}))
	assert.True(t, hasUserMessageKey(map[string]interface{}{"description": "d"}))
	assert.False(t, hasUserMessageKey(map[string]interface{}{"title": "t"}))
	assert.False(t, hasUserMessageKey(map[string]interface{}{}))
	assert.False(t, hasUserMessageKey(nil))
}

// TestCreateAgentTask_ParamsEnrichedWithMessage verifies the task is created
// with Params carrying the injected user message (SPEC-063 deriveUserMessage).
func TestCreateAgentTask_ParamsEnrichedWithMessage(t *testing.T) {
	orch, sessions, tasks := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent", mock.Anything).Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	tk := &domaintask.Task{ID: "task_1", CreatedAt: time.Now()}
	run := &domaintask.TaskRun{SessionID: "s1", Status: domaintask.StatusQueued}
	// Capture the params passed to CreateTask.
	var capturedParams map[string]interface{}
	tasks.On("CreateTask", "u1", "agent", []string{}, mock.MatchedBy(func(p map[string]interface{}) bool {
		capturedParams = p
		return p["message"] == "分析营收" && p["title"] == "Q3分析"
	}), "", "", "", (*time.Time)(nil)).Return(tk, run, nil)

	_, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{
		Title:    "Q3分析",
		Messages: []domainchat.Message{{Role: "user", Content: "分析营收"}},
	})
	require.NoError(t, err)
	assert.Equal(t, "分析营收", capturedParams["message"])
	assert.Equal(t, "Q3分析", capturedParams["title"])
}

// TestCreateAgentTask_WithProvider exercises the resolveModel path with a
// real Provider (default model resolution). Covers the provider non-nil
// branch of resolveModel (SPEC-062 §5.5.1).
func TestCreateAgentTask_WithProvider(t *testing.T) {
	sessions := domainchatmocks.NewSessionService(t)
	tasks := domaintaskmocks.NewTaskService(t)
	repo := mockrepo.NewSysConfigRepository(t)
	raw, _ := json.Marshal([]modelcfg.ModelEntry{
		{ID: "def-model", Name: "Default", Type: modelcfg.ModelTypeLLM, IsDefault: true},
	})
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: string(raw)}, nil)
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	provider := modelcfg.NewProvider(repo, nil)
	orch := NewOrchestrator(sessions, tasks, provider)

	sessions.On("Create", "u1", "agent", "def-model").Return(&domainchat.Session{ID: "s1", UserID: "u1", ModelID: "def-model"}, nil)
	tk := &domaintask.Task{ID: "task_1", CreatedAt: time.Now()}
	run := &domaintask.TaskRun{SessionID: "s1", Status: domaintask.StatusQueued}
	tasks.On("CreateTask", "u1", "agent", []string{}, mock.Anything, "def-model", "", "", (*time.Time)(nil)).Return(tk, run, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.TaskID != "task_1" {
		t.Errorf("task id = %v", resp.TaskID)
	}
}

// TestResolveModel_WithExplicitModel verifies that an explicit req.Model is
// used as-is (no provider default resolution needed).
func TestResolveModel_WithExplicitModel(t *testing.T) {
	sessions := domainchatmocks.NewSessionService(t)
	tasks := domaintaskmocks.NewTaskService(t)
	orch := NewOrchestrator(sessions, tasks, nil) // nil provider — explicit model should still work

	sessions.On("Create", "u1", "agent", "explicit-model").Return(&domainchat.Session{ID: "s1", UserID: "u1", ModelID: "explicit-model"}, nil)
	tasks.On("CreateTask", "u1", "agent", []string{}, mock.Anything, "explicit-model", "", "", (*time.Time)(nil)).Return(
		&domaintask.Task{ID: "t1"}, &domaintask.TaskRun{SessionID: "s1", Status: domaintask.StatusQueued}, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{
		Title: "t", Model: "explicit-model",
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.TaskID != "t1" {
		t.Errorf("task id = %v", resp.TaskID)
	}
}
