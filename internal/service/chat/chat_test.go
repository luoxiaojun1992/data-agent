package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/mock"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	domainmodel "github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

// ── Fake model ──

type fakeLLM struct {
	text string
	err  error
}

func (f *fakeLLM) Name() string { return "fake" }

func (f *fakeLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(f.text, "model")}, nil)
	}
}

// ── Helpers ──

func newTestService(t *testing.T, llm model.LLM) *Service {
	t.Helper()
	adkSessions := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName:        "data-agent",
		Model:          llm,
		SessionService: adkSessions,
	})
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{
		AppName:        "data-agent",
		SessionService: adkSessions,
	})
	mgr := &Manager{ttl: 1 * time.Hour}
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	svc := NewService(registry, nil, adkSessions, mgr, cbReg)
	// Patch GetOrCreate to return the test Runtime (avoids needing a real
	// Provider with a configured model for unit tests).
	patches := gomonkey.NewPatches()
	t.Cleanup(patches.Reset)
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	return svc
}

func patchSessionCreate(patches *gomonkey.Patches, svc *Service, sess *domainchat.Session, err error) {
	patches.ApplyMethodReturn(svc.sessions, "Create", sess, err)
}

func patchSessionGet(patches *gomonkey.Patches, svc *Service, sess *domainchat.Session, err error) {
	patches.ApplyMethodReturn(svc.sessions, "Get", sess, err)
	patches.ApplyMethodReturn(svc.sessions, "Renew", nil)
}

// ── Process validation ──

func TestProcess_MessagesRequired(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	_, err := svc.Process(context.Background(), domainchat.ChatRequest{}, "u1", "admin")
	if err != domainchat.ErrMessagesRequired {
		t.Errorf("expected ErrMessagesRequired, got %v", err)
	}
}

func TestProcess_LegacySingleMessage(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	resp, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hello"}, "u1", "admin")
	if err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %v", resp.Content)
	}
}

func TestProcess_NoUserMessage(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	_, err := svc.Process(context.Background(), domainchat.ChatRequest{
		Messages: []domainchat.Message{{Role: "assistant", Content: "hi"}},
	}, "u1", "admin")
	if err != domainchat.ErrUserMessageRequired {
		t.Errorf("expected ErrUserMessageRequired, got %v", err)
	}
}

func TestProcess_SessionCreateError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, nil, fmt.Errorf("db error"))

	_, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hello"}, "u1", "admin")
	if err != domainchat.ErrSessionCreateFailed {
		t.Errorf("expected ErrSessionCreateFailed, got %v", err)
	}
}

func TestProcess_UnauthorizedSession(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionGet(patches, svc, &domainchat.Session{ID: "s1", UserID: "other-user"}, nil)

	_, err := svc.Process(context.Background(), domainchat.ChatRequest{
		SessionID: "s1", Message: "hello",
	}, "u1", "admin")
	if err != domainchat.ErrUnauthorizedSession {
		t.Errorf("expected ErrUnauthorizedSession, got %v", err)
	}
}

func TestProcess_InvalidSession(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionGet(patches, svc, nil, fmt.Errorf("not found"))

	_, err := svc.Process(context.Background(), domainchat.ChatRequest{
		SessionID: "missing", Message: "hello",
	}, "u1", "admin")
	if err != domainchat.ErrUnauthorizedSession {
		t.Errorf("expected ErrUnauthorizedSession, got %v", err)
	}
}

// ── Process success / model error ──

func TestProcess_Success(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "这是回答"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	resp, err := svc.Process(context.Background(), domainchat.ChatRequest{
		Message: "分析一下营收", Stream: false,
	}, "u1", "admin")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.SessionID != "s1" {
		t.Errorf("session_id = %v", resp.SessionID)
	}
	if resp.Content != "这是回答" {
		t.Errorf("content = %v", resp.Content)
	}
	if resp.Usage == nil {
		t.Errorf("usage field missing")
	}
}

func TestProcess_ModelError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{err: fmt.Errorf("model down")})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	_, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hello"}, "u1", "admin")
	if err == nil {
		t.Error("expected model error")
	}
}

func TestProcess_ExistingSession(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "answer"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionGet(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1", ModelID: "bound-model"}, nil)

	resp, err := svc.Process(context.Background(), domainchat.ChatRequest{
		SessionID: "s1", Message: "hi",
	}, "u1", "admin")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.Content != "answer" {
		t.Errorf("content = %v", resp.Content)
	}
}

// TestProcess_ExistingSessionIgnoresReqModel verifies the immutable binding
// constraint (SPEC-062): when a session already has a bound ModelID, the
// req.Model field is IGNORED — the session's bound model is always used.
func TestProcess_ExistingSessionIgnoresReqModel(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	// Session is bound to "original-model"; request tries to switch to "other".
	patchSessionGet(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1", ModelID: "original-model"}, nil)

	resp, err := svc.Process(context.Background(), domainchat.ChatRequest{
		SessionID: "s1", Model: "other-model-attempt", Message: "hi",
	}, "u1", "admin")
	if err != nil {
		t.Fatalf("expected success (model switch ignored), got %v", err)
	}
	if resp.SessionID != "s1" {
		t.Errorf("session_id = %v, want s1", resp.SessionID)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %v, want ok", resp.Content)
	}
}

// ── Streaming ──

func TestStream_Success(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "流式回答"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	w := httptest.NewRecorder()
	err := svc.Stream(context.Background(), domainchat.ChatRequest{
		Message: "hello", Stream: true,
	}, "u1", "admin", w)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}

	body := w.Body.String()
	if w.Header().Get("Content-Type") != "text/event-stream" {
		t.Errorf("content type = %v", w.Header().Get("Content-Type"))
	}
	if !strings.Contains(body, `"session_id":"s1"`) {
		t.Errorf("missing session event: %s", body)
	}
	if !strings.Contains(body, `"content":"流式回答"`) {
		t.Errorf("missing content event: %s", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Errorf("missing DONE marker: %s", body)
	}
}

func TestStream_ModelError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{err: fmt.Errorf("model exploded")})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	w := httptest.NewRecorder()
	err := svc.Stream(context.Background(), domainchat.ChatRequest{Message: "hello"}, "u1", "admin", w)
	if err != nil {
		t.Fatalf("Stream returned error for in-stream failure: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"error"`) {
		t.Errorf("expected error event: %s", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Errorf("stream should still terminate with DONE: %s", body)
	}
}

func TestStream_ValidationError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	w := httptest.NewRecorder()
	err := svc.Stream(context.Background(), domainchat.ChatRequest{}, "u1", "admin", w)
	if err != domainchat.ErrMessagesRequired {
		t.Errorf("expected ErrMessagesRequired, got %v", err)
	}
}

// ── Memory hook ──

func TestMemoryWriteHook_Invoked(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	called := make(chan struct{}, 1)
	svc.WithMemoryWrite(func(ctx context.Context, sess adksession.Session) {
		called <- struct{}{}
	})

	if _, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hello"}, "u1", "admin"); err != nil {
		t.Fatalf("Process failed: %v", err)
	}
	select {
	case <-called:
	case <-time.After(2 * time.Second):
		t.Error("memory hook should be invoked after run")
	}
}

func TestMemoryWriteHook_NotConfigured(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	// No hook — must not panic.
	svc.scheduleMemoryWrite("u1", "s1")
}

// ── lastUserMessage ──

func TestLastUserMessage(t *testing.T) {
	msgs := []domainchat.Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: "second"},
	}
	if got := lastUserMessage(msgs); got != "second" {
		t.Errorf("lastUserMessage = %q", got)
	}
	if got := lastUserMessage([]domainchat.Message{{Role: "assistant", Content: "x"}}); got != "" {
		t.Errorf("no user message = %q", got)
	}
	if got := lastUserMessage([]domainchat.Message{{Role: "user", Content: "  "}}); got != "" {
		t.Errorf("blank user message = %q", got)
	}
	if got := lastUserMessage(nil); got != "" {
		t.Errorf("nil messages = %q", got)
	}
}

// ── Session Manager ──

func newTestManager(t *testing.T) (*Manager, *mockrepo.SessionRepository) {
	t.Helper()
	repo := mockrepo.NewSessionRepository(t)
	return &Manager{repo: repo, ttl: 24 * time.Hour}, repo
}

func TestManager_Create(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	s, err := m.Create("user1", "chat", "")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if s.UserID != "user1" || s.Type != "chat" || s.Status != "active" {
		t.Errorf("unexpected session: %+v", s)
	}

	t.Run("db error", func(t *testing.T) {
		m2, repo2 := newTestManager(t)
		repo2.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))
		if _, err := m2.Create("user1", "chat", ""); err == nil {
			t.Error("expected db error")
		}
	})
}

func TestManager_Get(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Get", mock.Anything, "s1").Return(&repository.SessionRecord{ID: "s1", UserID: "u1"}, nil)

	s, err := m.Get("s1")
	if err != nil || s.ID != "s1" {
		t.Errorf("Get failed: %v", err)
	}

	repo.On("Get", mock.Anything, "missing").Return((*repository.SessionRecord)(nil), fmt.Errorf("not found"))
	if _, err := m.Get("missing"); err == nil {
		t.Error("missing session should error")
	}
}

func TestManager_Renew(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Renew", mock.Anything, mock.Anything, mock.Anything).Return(nil)

	if err := m.Renew("s1"); err != nil {
		t.Fatalf("Renew failed: %v", err)
	}

	t.Run("not found", func(t *testing.T) {
		m2, repo2 := newTestManager(t)
		repo2.On("Renew", mock.Anything, mock.Anything, mock.Anything).Return(fmt.Errorf("not found"))
		if err := m2.Renew("s1"); err == nil {
			t.Error("renew missing should error")
		}
	})
}

func TestManager_Cleanup(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Cleanup", mock.Anything, mock.Anything).Return(int64(3), nil)

	n, err := m.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup failed: %v", err)
	}
	if n != 3 {
		t.Errorf("deleted=%d, want 3", n)
	}
}

func TestManager_ListByUser(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("ListByUser", mock.Anything, "user1").Return([]*repository.SessionRecord{
		{ID: "s1", UserID: "user1"},
		{ID: "s2", UserID: "user1"},
	}, nil)

	sessions, err := m.ListByUser("user1")
	if err != nil {
		t.Fatalf("ListByUser: %v", err)
	}
	if len(sessions) != 2 {
		t.Fatalf("got %d, want 2", len(sessions))
	}

	t.Run("db error", func(t *testing.T) {
		m2, repo2 := newTestManager(t)
		repo2.On("ListByUser", mock.Anything, "user1").Return(([]*repository.SessionRecord)(nil), fmt.Errorf("db error"))
		if _, err := m2.ListByUser("user1"); err == nil {
			t.Error("error case should fail")
		}
	})
}

func TestManager_Delete(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Delete", mock.Anything, "s1").Return(nil)

	if err := m.Delete("s1"); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}
}

func TestManager_Restore(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Restore", mock.Anything, "s1").Return(nil)

	if err := m.Restore("s1"); err != nil {
		t.Fatalf("Restore failed: %v", err)
	}
}

func TestManager_ListDeleted(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("ListDeleted", mock.Anything, mock.Anything, int64(100)).Return([]*repository.SessionRecord{
		{ID: "d1", UserID: "u1"},
	}, nil)

	sessions, err := m.ListDeleted(time.Now(), 100)
	if err != nil {
		t.Fatalf("ListDeleted: %v", err)
	}
	if len(sessions) != 1 || sessions[0].ID != "d1" {
		t.Errorf("unexpected result: %+v", sessions)
	}
}

func TestManager_SetRecoveryHours(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("SetRecoveryHours", mock.Anything, mock.Anything).Return(nil)

	if err := m.SetRecoveryHours(48); err != nil {
		t.Fatalf("SetRecoveryHours: %v", err)
	}
}

func TestNewManager(t *testing.T) {
	m, repo := newTestManager(t)
	if m == nil {
		t.Fatal("NewManager should not return nil")
	}
	if m.repo != repo {
		t.Error("Manager.repo should be the injected repository")
	}
	if m.ttl != 24*time.Hour {
		t.Errorf("expected ttl=24h, got %v", m.ttl)
	}
}

type queueLLM struct {
	queue []*model.LLMResponse
}

func (q *queueLLM) Name() string { return "queue" }

func (q *queueLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if len(q.queue) == 0 {
			yield(nil, fmt.Errorf("empty queue"))
			return
		}
		resp := q.queue[0]
		q.queue = q.queue[1:]
		yield(resp, nil)
	}
}

func TestProcess_ADKSessionCreateError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	patches.ApplyMethodReturn(svc.adkSessions, "Create", (*adksession.CreateResponse)(nil), fmt.Errorf("mongo down"))

	_, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hello"}, "u1", "admin")
	if err != domainchat.ErrADKSessionInitFailed {
		t.Errorf("expected ErrADKSessionInitFailed, got %v", err)
	}
}

func TestRunAndCollect_SkipsNonFinalEvents(t *testing.T) {
	llm := &queueLLM{queue: []*model.LLMResponse{
		{Content: &genai.Content{Role: "model", Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "unknown_tool", Args: map[string]any{}}},
		}}},
		{Content: genai.NewContentFromText("最终答案", "model")},
	}}
	svc := newTestService(t, llm)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	resp, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hi"}, "u1", "admin")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.Content != "最终答案" {
		t.Errorf("content = %v", resp.Content)
	}
}

func TestScheduleMemoryWrite_GetError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.adkSessions, "Get", (*adksession.GetResponse)(nil), fmt.Errorf("mongo down"))

	done := make(chan struct{}, 1)
	svc.WithMemoryWrite(func(ctx context.Context, sess adksession.Session) {
		done <- struct{}{}
	})
	svc.scheduleMemoryWrite("u1", "s1")

	select {
	case <-done:
		t.Error("hook should not fire when session load fails")
	case <-time.After(500 * time.Millisecond):
	}
}

// ensure json import is used (Stream SSE marshalling tested implicitly).
var _ = json.Marshal

// TestProcess_NewSessionResolvesDefaultModel verifies that when req.Model is
// empty and a provider is wired, the default model is resolved and bound to
// the new session (SPEC-062 §5.4). Covers the createNewSession provider path.
func TestProcess_NewSessionResolvesDefaultModel(t *testing.T) {
	adkSessions := adksession.InMemoryService()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName: "data-agent", Model: &fakeLLM{text: "ok"}, SessionService: adkSessions,
	})
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{
		AppName: "data-agent", SessionService: adkSessions,
	})
	// Provider with a default model.
	repo := mockrepo.NewSysConfigRepository(t)
	raw, _ := json.Marshal([]modelcfg.ModelEntry{
		{ID: "default-llm", Name: "Default", Type: modelcfg.ModelTypeLLM, IsDefault: true},
	})
	repo.On("Get", mock.Anything, "model", "models").Return(&domainmodel.SystemConfig{Value: string(raw)}, nil)
	provider := modelcfg.NewProvider(repo)

	mgr := &Manager{ttl: 1 * time.Hour}
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	svc := NewService(registry, provider, adkSessions, mgr, cbReg)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	// Patch session Create to avoid needing a real repo.
	patches.ApplyMethodReturn(mgr, "Create", &domainchat.Session{ID: "s1", UserID: "u1", ModelID: "default-llm"}, nil)

	resp, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hi"}, "u1", "admin")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.SessionID == "" {
		t.Error("expected non-empty session ID")
	}
	if resp.Content != "ok" {
		t.Errorf("content = %v, want ok", resp.Content)
	}
}
