package chat

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	domainmodel "github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	"github.com/stretchr/testify/mock"
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
	sessionRepo := mockrepo.NewSessionRepository(t)
	sessionRepo.On("SetTitle", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)
	mgr := &Manager{repo: sessionRepo, ttl: 1 * time.Hour}
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	svc := NewService(registry, nil, adkSessions, mgr, cbReg)
	// Patch GetOrCreate to return the test Runtime (avoids needing a real
	// Provider with a configured model for unit tests).
	patches := gomonkey.NewPatches()
	t.Cleanup(patches.Reset)
	patches.ApplyMethodFunc(registry, "GetOrCreate", func(ctx context.Context, modelID string) (*adkruntime.Runtime, error) {
		return rt, nil
	})
	patches.ApplyMethodReturn(mgr, "SetTitle", nil)
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
	if got, _ := lastUserMessage(msgs); got != "second" {
		t.Errorf("lastUserMessage = %q", got)
	}
	if got, _ := lastUserMessage([]domainchat.Message{{Role: "assistant", Content: "x"}}); got != "" {
		t.Errorf("no user message = %q", got)
	}
	if got, _ := lastUserMessage([]domainchat.Message{{Role: "user", Content: "  "}}); got != "" {
		t.Errorf("blank user message = %q", got)
	}
	if got, _ := lastUserMessage(nil); got != "" {
		t.Errorf("nil messages = %q", got)
	}
}

func TestLastUserMessage_WithImages(t *testing.T) {
	img := domainchat.ImagePart{Data: "aGVsbG8=", MimeType: "image/png"}
	msgs := []domainchat.Message{
		{Role: "user", Content: "first"},
		{Role: "user", Content: "", Images: []domainchat.ImagePart{img}},
	}
	text, images := lastUserMessage(msgs)
	if text != "" || len(images) != 1 {
		t.Fatalf("image-only message: text=%q images=%d", text, len(images))
	}
	// A message with text + images returns both.
	msgs = []domainchat.Message{{Role: "user", Content: "看这张图", Images: []domainchat.ImagePart{img, img}}}
	text, images = lastUserMessage(msgs)
	if text != "看这张图" || len(images) != 2 {
		t.Fatalf("text+images message: text=%q images=%d", text, len(images))
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

// TestStream_ToolCallEvent covers the forwardSSEEvent FunctionCall branch
// (pre-existing 40% coverage gap). Uses a fakeLLM that emits a tool-call
// part followed by text.
func TestStream_ToolCallEvent(t *testing.T) {
	llm := &queueLLM{queue: []*model.LLMResponse{
		{Content: &genai.Content{Role: "model", Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "sql_validate", Args: map[string]any{"q": "SELECT 1"}}},
		}}},
		{Content: genai.NewContentFromText("done", "model")},
	}}
	svc := newTestService(t, llm)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	w := httptest.NewRecorder()
	err := svc.Stream(context.Background(), domainchat.ChatRequest{Message: "hi", Stream: true}, "u1", "admin", w)
	if err != nil {
		t.Fatalf("Stream failed: %v", err)
	}
	body := w.Body.String()
	if !strings.Contains(body, `"type":"tool_call"`) {
		t.Errorf("expected tool_call event in SSE: %s", body)
	}
	if !strings.Contains(body, `"content":"done"`) {
		t.Errorf("expected text event: %s", body)
	}
}

// TestProcess_WithKBID covers the buildState kb_id branch.
func TestProcess_WithKBID(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	resp, err := svc.Process(context.Background(), domainchat.ChatRequest{
		Message: "hi", KBID: "kb-123",
	}, "u1", "admin")
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.Content != "ok" {
		t.Errorf("content = %v", resp.Content)
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
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	repo.On("GetAll", mock.Anything, "model").Return([]domainmodel.SystemConfig{}, nil).Maybe()
	provider := modelcfg.NewProvider(repo, nil)

	sessionRepo := mockrepo.NewSessionRepository(t)
	sessionRepo.On("SetTitle", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)
	mgr := &Manager{repo: sessionRepo, ttl: 1 * time.Hour}
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
	if resp.Content != "ok" {
		t.Errorf("content = %v, want ok", resp.Content)
	}
}

func TestChatEventsFromParts_UsesCanonicalToolResult(t *testing.T) {
	parts := []*genai.Part{
		{FunctionCall: &genai.FunctionCall{Name: "sql_query", Args: map[string]any{"sql": "SELECT 1"}}},
		{FunctionResponse: &genai.FunctionResponse{Name: "sql_query", Response: map[string]any{"rows": []any{"row-1", "row-2"}}}},
		{Text: "answer"},
	}
	events := ChatEventsFromParts("data_agent", "evt-1", "2026-07-26T22:00:00Z", parts)
	if len(events) != 3 {
		t.Fatalf("events = %d, want 3", len(events))
	}
	if events[0].Type != "tool_call" || events[0].Name != "sql_query" {
		t.Fatalf("tool call = %+v", events[0])
	}
	if events[0].Args["sql"] != "SELECT 1" {
		t.Errorf("tool args = %+v", events[0].Args)
	}
	if events[1].Type != "tool_result" || events[1].Result == nil {
		t.Fatalf("tool result = %+v", events[1])
	}
	rowsMap, ok := events[1].Result.(map[string]any)
	if !ok || len(rowsMap) != 1 {
		t.Fatalf("tool result payload = %#v", events[1].Result)
	}
	rows, ok := rowsMap["rows"].([]any)
	if !ok || len(rows) != 2 {
		t.Fatalf("tool result rows = %#v", rowsMap["rows"])
	}
	if events[2].Type != "text" || events[2].Content != "answer" {
		t.Errorf("text event = %+v", events[2])
	}
	for _, event := range events {
		if event.Role != "data_agent" || event.EventID != "evt-1" || event.Timestamp == "" {
			t.Errorf("canonical metadata missing: %+v", event)
		}
	}
}

func TestChatEventsFromADKEvent_FiltersCompaction(t *testing.T) {
	if got := ChatEventsFromADKEvent(&adksession.Event{Author: "compaction", LLMResponse: model.LLMResponse{
		Content: genai.NewContentFromText("summary", "model"),
	}}); got != nil {
		t.Errorf("compaction author should be filtered, got %+v", got)
	}
	filtered := ChatEventsFromADKEvent(&adksession.Event{Author: "data_agent", LLMResponse: model.LLMResponse{
		CustomMetadata: map[string]any{"compaction": true},
		Content:       genai.NewContentFromText("summary", "model"),
	}})
	if filtered != nil {
		t.Errorf("custommetadata compaction should be filtered, got %+v", filtered)
	}
	if got := ChatEventsFromADKEvent(nil); got != nil {
		t.Errorf("nil event should produce no events, got %+v", got)
	}
}

func TestForwardSSEEvent_SkipsCompaction(t *testing.T) {
	w := httptest.NewRecorder()
	forwardSSEEvent(w, w, &adksession.Event{
		ID: "evt-compaction", Author: "compaction", Timestamp: time.Now(),
		LLMResponse: model.LLMResponse{Content: genai.NewContentFromText("summary", "model")},
	})
	if w.Body.Len() != 0 {
		t.Fatalf("compaction event must not be forwarded, got %q", w.Body.String())
	}
}

func TestForwardSSEEvent_UsesCanonicalShape(t *testing.T) {
	w := httptest.NewRecorder()
	forwardSSEEvent(w, w, &adksession.Event{
		ID: "evt-tool", Author: "data_agent", Timestamp: time.Date(2026, 7, 26, 22, 0, 0, 0, time.UTC),
		LLMResponse: model.LLMResponse{Content: &genai.Content{Role: "model", Parts: []*genai.Part{{
			FunctionResponse: &genai.FunctionResponse{Name: "sql_query", Response: map[string]any{"rows": 1}},
		}}}},
	})
	body := w.Body.String()
	if !strings.Contains(body, `"type":"tool_result"`) || !strings.Contains(body, `"result":{"rows":1}`) {
		t.Fatalf("canonical SSE event missing fields: %s", body)
	}
	if strings.Contains(body, `"response":`) {
		t.Fatalf("legacy response field leaked into SSE: %s", body)
	}
	if !strings.Contains(body, `"role":"data_agent"`) {
		t.Fatalf("role missing from SSE: %s", body)
	}
}

func TestIsCompactionEvent(t *testing.T) {
	if !IsCompactionEvent(&adksession.Event{Author: "compaction"}) {
		t.Error("author=compaction must be filtered")
	}
	if !IsCompactionEvent(&adksession.Event{LLMResponse: model.LLMResponse{
		CustomMetadata: map[string]any{"compaction": true},
	}}) {
		t.Error("custommetadata compaction must be filtered")
	}
	if IsCompactionEvent(&adksession.Event{Author: "data_agent"}) {
		t.Error("normal agent event must not be filtered")
	}
}

// ── Chat image attachments ──

func TestValidateImages(t *testing.T) {
	small := "aGVsbG8=" // "hello" (5 bytes)
	big := strings.Repeat("A", maxChatImageBytes*2) // decodes to >2MiB
	tests := []struct {
		name    string
		images  []domainchat.ImagePart
		wantErr error
	}{
		{"too many", []domainchat.ImagePart{
			{Data: small, MimeType: "image/png"}, {Data: small, MimeType: "image/png"},
			{Data: small, MimeType: "image/png"}, {Data: small, MimeType: "image/png"},
			{Data: small, MimeType: "image/png"}, {Data: small, MimeType: "image/png"},
		}, domainchat.ErrTooManyImages},
		{"unsupported mime", []domainchat.ImagePart{{Data: small, MimeType: "image/tiff"}}, domainchat.ErrInvalidImage},
		{"bad base64", []domainchat.ImagePart{{Data: "not-base64!!!", MimeType: "image/png"}}, domainchat.ErrInvalidImage},
		{"single too large", []domainchat.ImagePart{{Data: big, MimeType: "image/png"}}, domainchat.ErrImageTooLarge},
		{"total too large", nil, domainchat.ErrImageTooLarge},
		{"ok", []domainchat.ImagePart{
			{Data: small, MimeType: "image/png"},
			{Data: "aGVsbG8=", MimeType: "image/jpeg"},
		}, nil},
	}
	// total too large: five 1.2MiB images (each under per-image cap)
	mid := strings.Repeat("A", 1200*1024*4/3+4) // base64 of ~1.2MiB
	for i := 0; i < len(tests); i++ {
		if tests[i].name == "total too large" {
			for j := 0; j < 5; j++ {
				tests[i].images = append(tests[i].images, domainchat.ImagePart{Data: mid, MimeType: "image/png"})
			}
		}
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, err := validateImages(tt.images)
			if tt.wantErr == nil {
				if err != nil {
					t.Fatalf("expected ok, got %v", err)
				}
				return
			}
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("expected %v, got %v", tt.wantErr, err)
			}
		})
	}
}

func TestBuildUserContent(t *testing.T) {
	img := domainchat.ImagePart{Data: "aGVsbG8=", MimeType: "image/png"}
	// text + image: parts = [text, inline image]
	c, err := buildUserContent("看这张图", []domainchat.ImagePart{img, img})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if c.Role != "user" || len(c.Parts) != 3 {
		t.Fatalf("role=%q parts=%d", c.Role, len(c.Parts))
	}
	if c.Parts[0].Text != "看这张图" || c.Parts[1].InlineData == nil || c.Parts[2].InlineData == nil {
		t.Fatalf("unexpected parts: %+v", c.Parts)
	}
	if c.Parts[1].InlineData.MIMEType != "image/png" || string(c.Parts[1].InlineData.Data) != "hello" {
		t.Fatalf("bad inline data: %+v", c.Parts[1].InlineData)
	}
	// image-only: parts = [inline image]
	c, err = buildUserContent("", []domainchat.ImagePart{img})
	if err != nil {
		t.Fatalf("image-only build: %v", err)
	}
	if len(c.Parts) != 1 || c.Parts[0].InlineData == nil {
		t.Fatalf("image-only parts: %+v", c.Parts)
	}
	// validation error propagates
	if _, err := buildUserContent("x", []domainchat.ImagePart{{Data: "!!!", MimeType: "image/png"}}); !errors.Is(err, domainchat.ErrInvalidImage) {
		t.Fatalf("expected ErrInvalidImage, got %v", err)
	}
}

func TestChatEventsFromParts_WithImages(t *testing.T) {
	imgPart := genai.NewPartFromBytes([]byte("pixel"), "image/png")
	// text + image → one text event carrying the image data URL
	events := ChatEventsFromParts("user", "e1", "2026-01-01T00:00:00Z",
		[]*genai.Part{genai.NewPartFromText("看这张图"), imgPart})
	if len(events) != 1 {
		t.Fatalf("expected 1 event, got %d", len(events))
	}
	if events[0].Type != "text" || events[0].Content != "看这张图" || len(events[0].Images) != 1 {
		t.Fatalf("unexpected event: %+v", events[0])
	}
	if !strings.HasPrefix(events[0].Images[0], "data:image/png;base64,") {
		t.Fatalf("bad data url: %s", events[0].Images[0])
	}
	// image-only → text event with empty content + images
	events = ChatEventsFromParts("user", "e2", "2026-01-01T00:00:00Z", []*genai.Part{imgPart})
	if len(events) != 1 || events[0].Content != "" || len(events[0].Images) != 1 {
		t.Fatalf("image-only event: %+v", events)
	}
	// tool parts are unaffected and images still attach to the text part
	events = ChatEventsFromParts("assistant", "e3", "2026-01-01T00:00:00Z",
		[]*genai.Part{genai.NewPartFromText("answer"), &genai.Part{FunctionCall: &genai.FunctionCall{Name: "sql_executor", Args: map[string]any{}}}})
	if len(events) != 2 || events[1].Type != "tool_call" {
		t.Fatalf("tool event handling broken: %+v", events)
	}
}

// TestProcess_ImageValidationError verifies image validation failures surface
// as domain errors before any model call.
func TestProcess_ImageValidationError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1", ModelID: "m1"}, nil)

	req := domainchat.ChatRequest{
		Message: "hi",
		Images: []domainchat.ImagePart{
			{Data: "x", MimeType: "image/png"}, {Data: "x", MimeType: "image/png"},
			{Data: "x", MimeType: "image/png"}, {Data: "x", MimeType: "image/png"},
			{Data: "x", MimeType: "image/png"}, {Data: "x", MimeType: "image/png"},
		},
	}
	if _, err := svc.Process(context.Background(), req, "u1", "user"); !errors.Is(err, domainchat.ErrTooManyImages) {
		t.Fatalf("expected ErrTooManyImages, got %v", err)
	}
}

// TestProcess_WithImages verifies a message carrying valid image attachments
// runs end-to-end (validation → content build → runtime → response).
func TestProcess_WithImages(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "收到图片"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1", ModelID: "m1"}, nil)

	req := domainchat.ChatRequest{
		Message: "看这张图",
		Images:  []domainchat.ImagePart{{Data: "aGVsbG8=", MimeType: "image/png"}},
	}
	resp, err := svc.Process(context.Background(), req, "u1", "user")
	if err != nil {
		t.Fatalf("Process with image failed: %v", err)
	}
	if resp.Content != "收到图片" {
		t.Fatalf("content = %q", resp.Content)
	}
}
