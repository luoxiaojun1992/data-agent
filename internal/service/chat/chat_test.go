package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/mock"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"github.com/gin-gonic/gin"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
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
	mgr := &Manager{ttl: 1 * time.Hour}
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	return NewService(rt, adkSessions, mgr, cbReg)
}

func newGinContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func patchSessionCreate(patches *gomonkey.Patches, svc *Service, sess *Session, err error) {
	patches.ApplyMethodReturn(svc.sessions, "Create", sess, err)
}

func patchSessionGet(patches *gomonkey.Patches, svc *Service, sess *Session, err error) {
	patches.ApplyMethodReturn(svc.sessions, "Get", sess, err)
	patches.ApplyMethodReturn(svc.sessions, "Renew", nil)
}

// ── HandleChat validation ──

func TestHandleChat_InvalidJSON(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	c, w := newGinContext("POST", "/chat", "not-json")
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChat_EmptyMessages(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	c, w := newGinContext("POST", "/chat", `{"messages": []}`)
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChat_NoUserMessage(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/chat", `{"messages": [{"role":"assistant","content":"hi"}]}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400 for missing user message, got %d", w.Code)
	}
}

func TestHandleChat_SessionCreateError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, nil, fmt.Errorf("db error"))

	c, w := newGinContext("POST", "/chat", `{"message": "hello"}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleChat_UnauthorizedSession(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionGet(patches, svc, &Session{ID: "s1", UserID: "other-user"}, nil)

	c, w := newGinContext("POST", "/chat", `{"session_id": "s1", "message": "hello"}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleChat_InvalidSession(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionGet(patches, svc, nil, fmt.Errorf("not found"))

	c, w := newGinContext("POST", "/chat", `{"session_id": "missing", "message": "hello"}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

// ── Non-stream chat ──

func TestHandleChat_NonStream_Success(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "这是回答"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/chat", `{"message": "分析一下营收", "stream": false}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("parse response: %v", err)
	}
	if resp["session_id"] != "s1" {
		t.Errorf("session_id = %v", resp["session_id"])
	}
	if resp["content"] != "这是回答" {
		t.Errorf("content = %v", resp["content"])
	}
	if _, ok := resp["usage"]; !ok {
		t.Errorf("usage field missing")
	}
}

func TestHandleChat_NonStream_ModelError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{err: fmt.Errorf("model down")})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/chat", `{"message": "hello", "stream": false}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleChat_ExistingSession(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "answer"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionGet(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/chat", `{"session_id": "s1", "message": "hi"}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ── Streaming chat ──

func TestHandleChat_Stream_Success(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "流式回答"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/chat", `{"message": "hello", "stream": true}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)

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

func TestHandleChat_Stream_ModelError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{err: fmt.Errorf("model exploded")})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/chat", `{"message": "hello", "stream": true}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)

	body := w.Body.String()
	if !strings.Contains(body, `"error"`) {
		t.Errorf("expected error event: %s", body)
	}
	if !strings.HasSuffix(strings.TrimSpace(body), "data: [DONE]") {
		t.Errorf("stream should still terminate with DONE: %s", body)
	}
}

// ── Memory hook ──

func TestMemoryWriteHook_Invoked(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	called := make(chan struct{}, 1)
	svc.WithMemoryWrite(func(ctx context.Context, sess adksession.Session) {
		called <- struct{}{}
	})

	c, w := newGinContext("POST", "/chat", `{"message": "hello"}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)
	if w.Code != http.StatusOK {
		t.Fatalf("chat failed: %d", w.Code)
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
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	// No hook — must not panic.
	svc.scheduleMemoryWrite("u1", "s1")
}

// ── lastUserMessage ──

func TestLastUserMessage(t *testing.T) {
	msgs := []Message{
		{Role: "user", Content: "first"},
		{Role: "assistant", Content: "reply"},
		{Role: "user", Content: "second"},
	}
	if got := lastUserMessage(msgs); got != "second" {
		t.Errorf("lastUserMessage = %q", got)
	}
	if got := lastUserMessage([]Message{{Role: "assistant", Content: "x"}}); got != "" {
		t.Errorf("no user message = %q", got)
	}
	if got := lastUserMessage([]Message{{Role: "user", Content: "  "}}); got != "" {
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

	s, err := m.Create("user1", "chat")
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	if s.UserID != "user1" || s.Type != "chat" || s.Status != "active" {
		t.Errorf("unexpected session: %+v", s)
	}

	t.Run("db error", func(t *testing.T) {
		m2, repo2 := newTestManager(t)
		repo2.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))
		if _, err := m2.Create("user1", "chat"); err == nil {
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

	// Not found.
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

func TestHandleChat_ADKSessionCreateError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "ok"})
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)
	patches.ApplyMethodReturn(svc.adkSessions, "Create", (*adksession.CreateResponse)(nil), fmt.Errorf("mongo down"))

	c, w := newGinContext("POST", "/chat", `{"message": "hello"}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRunAndCollect_SkipsNonFinalEvents(t *testing.T) {
	// LLM first returns a function call (non-final event), then final text.
	// Without the tool registered, ADK surfaces an error function response,
	// but the loop still completes with the final answer.
	llm := &queueLLM{queue: []*model.LLMResponse{
		{Content: &genai.Content{Role: "model", Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "unknown_tool", Args: map[string]any{}}},
		}}},
		{Content: genai.NewContentFromText("最终答案", "model")},
	}}
	svc := newTestService(t, llm)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/chat", `{"message": "hi"}`)
	c.Set("user_id", "u1")
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["content"] != "最终答案" {
		t.Errorf("content = %v", resp["content"])
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

	// Get fails → hook must NOT fire; just verify no panic and goroutine exits.
	select {
	case <-done:
		t.Error("hook should not fire when session load fails")
	case <-time.After(500 * time.Millisecond):
	}
}
