package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"go.mongodb.org/mongo-driver/mongo"
)

// ── Helpers ──

func newTestService() *Service {
	engine := &agent.Engine{}
	sessions := &Manager{ttl: 1 * time.Hour}
	auditor := security.NewAuditor(nil)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	return NewService(engine, sessions, auditor, cbReg)
}

func newGinContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// ── HandleChat Tests ──

func TestHandleChat_InvalidJSON(t *testing.T) {
	svc := newTestService()
	c, w := newGinContext("POST", "/chat", "not-json")
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChat_EmptyMessages(t *testing.T) {
	svc := newTestService()
	body := `{"messages": []}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleChat_LegacySingleMessage(t *testing.T) {
	svc := newTestService()

	// Mock session Create
	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_legacy",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	// Mock engine.Run
	patches.ApplyMethodReturn(svc.engine, "Run", &agent.ChatResponse{
		Content: "Hello!",
		Usage:   agent.Usage{TotalTokens: 10},
	}, nil)

	body := `{"message": "Hello"}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleChat_NonStream_Success(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_new",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	patches.ApplyMethodReturn(svc.engine, "Run", &agent.ChatResponse{
		Content: "response content",
		Usage:   agent.Usage{PromptTokens: 5, CompletionTokens: 3, TotalTokens: 8},
	}, nil)

	body := `{"messages": [{"role": "user", "content": "hi"}]}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal response: %v", err)
	}
	if resp["content"] != "response content" {
		t.Errorf("content: got %v", resp["content"])
	}
	if resp["session_id"] != "sess_new" {
		t.Errorf("session_id: got %v", resp["session_id"])
	}
}

func TestHandleChat_NonStream_EngineError(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_err",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	patches.ApplyMethodReturn(svc.engine, "Run", (*agent.ChatResponse)(nil), fmt.Errorf("engine failure"))

	body := `{"messages": [{"role": "user", "content": "hi"}]}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestHandleChat_SessionCreateError(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", (*Session)(nil), fmt.Errorf("db error"))
	defer patches.Reset()

	body := `{"messages": [{"role": "user", "content": "hi"}]}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestHandleChat_ExistingSession_Success(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Get", &Session{
		ID:     "sess_existing",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.sessions, "Renew", nil)
	patches.ApplyMethodReturn(svc.engine, "Run", &agent.ChatResponse{
		Content: "existing session response",
		Usage:   agent.Usage{TotalTokens: 5},
	}, nil)

	body := `{"session_id": "sess_existing", "messages": [{"role": "user", "content": "continue"}]}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["content"] != "existing session response" {
		t.Errorf("content: got %v", resp["content"])
	}
}

func TestHandleChat_ExistingSession_Unauthorized(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Get", (*Session)(nil), fmt.Errorf("not found"))
	defer patches.Reset()

	body := `{"session_id": "sess_bad", "messages": [{"role": "user", "content": "test"}]}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleChat_ExistingSession_WrongUser(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Get", &Session{
		ID:     "sess_other",
		UserID: "other-user",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	body := `{"session_id": "sess_other", "messages": [{"role": "user", "content": "test"}]}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestHandleChat_Stream_NoFlusher(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_stream",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	patches.ApplyMethodReturn(svc.engine, "RunStream", nil)

	body := `{"messages": [{"role": "user", "content": "stream"}], "stream": true}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	// RunStream returns nil, so SSE streaming completes without error
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ===== handleStream with no flusher (non-flushing ResponseWriter) =====

func TestHandleChat_Stream_NoFlusherWriter(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_stream_noflush",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	body := `{"messages": [{"role": "user", "content": "stream test"}], "stream": true}`
	c, w := newGinContext("POST", "/chat", body)

	// Mock c.Writer to not implement http.Flusher by setting it to nil interface
	// Then mock c.Header and c.JSON to avoid nil pointer dereference
	type noFlushResponseWriter struct{ httptest.ResponseRecorder }

	// Use gomonkey to replace c.Header (called before flusher check)
	patches.ApplyMethodFunc(c, "Header", func(key, value string) {
		// no-op; avoid nil Writer header access
	})

	patches.ApplyMethodFunc(c, "JSON", func(code int, obj interface{}) {
		w.WriteHeader(code)
	})

	// c.Writer = nil (nil gin.ResponseWriter) → type assertion to http.Flusher fails
	c.Writer = nil

	svc.HandleChat(c)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHandleChat_ContextCompression(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_compress",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.engine, "Run", &agent.ChatResponse{
		Content: "compressed response",
		Usage:   agent.Usage{TotalTokens: 100},
	}, nil)

	// Messages with large content to trigger compression (>64000 tokens threshold)
	// Each message: 10000 ASCII chars → (10000+3)/4 = 2500 tokens per message
	// 30 messages * 2500 = 75000 tokens > 64000 threshold
	var largeMessages []agent.Message
	for i := 0; i < 30; i++ {
		largeMessages = append(largeMessages, agent.Message{
			Role:    "user",
			Content: strings.Repeat("a", 10000),
		})
	}
	reqBody, _ := json.Marshal(map[string]interface{}{
		"messages": largeMessages,
	})

	c, w := newGinContext("POST", "/chat", string(reqBody))
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ── ContextManager Tests ──

func TestNewContextManager(t *testing.T) {
	cm := NewContextManager(100000, 0.8)
	if cm.maxTokens != 100000 {
		t.Errorf("maxTokens: got %d", cm.maxTokens)
	}
	if cm.threshold != 0.8 {
		t.Errorf("threshold: got %f", cm.threshold)
	}
}

func TestShouldCompress_UnderThreshold(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	if cm.ShouldCompress(40000) {
		t.Error("should not compress when under 50% of 100K")
	}
}

func TestShouldCompress_OverThreshold(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	if !cm.ShouldCompress(60000) {
		t.Error("should compress when over 50% of 100K")
	}
}

func TestShouldCompress_AtThreshold(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	if cm.ShouldCompress(50000) {
		t.Error("should not compress at exactly 50% (uses > not >=)")
	}
}

func TestShouldCompress_ZeroMaxTokens(t *testing.T) {
	cm := NewContextManager(0, 0.5)
	if cm.ShouldCompress(1000000) {
		t.Error("should not compress when maxTokens is 0")
	}
}

func TestShouldCompress_NegativeMaxTokens(t *testing.T) {
	cm := NewContextManager(-1, 0.5)
	if cm.ShouldCompress(100) {
		t.Error("should not compress when maxTokens is negative")
	}
}

func TestShouldCompress_ZeroThreshold(t *testing.T) {
	cm := NewContextManager(1000, 0)
	if !cm.ShouldCompress(1) {
		t.Error("should compress any tokens when threshold is 0")
	}
}

func TestTruncateMessages_Empty(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	result := cm.TruncateMessages([]agent.Message{}, 1000)
	if len(result) != 0 {
		t.Errorf("expected empty, got %d messages", len(result))
	}
}

func TestTruncateMessages_ZeroMaxTokens(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "user", Content: "hello"},
	}
	result := cm.TruncateMessages(msgs, 0)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

func TestTruncateMessages_KeepsSystemMessage(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "system", Content: "You are helpful"},
		{Role: "user", Content: "hello"},
	}
	// Set maxTokens very low so only system fits
	result := cm.TruncateMessages(msgs, 2)
	if len(result) < 1 {
		t.Fatal("expected at least system message")
	}
	if result[0].Role != "system" {
		t.Errorf("first message should be system, got %s", result[0].Role)
	}
}

func TestTruncateMessages_KeepsLastNMessages(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "msg1"},
		{Role: "assistant", Content: "reply1"},
		{Role: "user", Content: "msg2"},
		{Role: "assistant", Content: "reply2"},
	}
	// Enough tokens to keep all
	result := cm.TruncateMessages(msgs, 100)
	if len(result) != 5 {
		t.Errorf("expected 5 messages, got %d", len(result))
	}
}

func TestTruncateMessages_NoSystemMessage(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
	}
	// Very low token limit
	result := cm.TruncateMessages(msgs, 1)
	if len(result) == 0 {
		t.Error("expected at least one message")
	}
	// Should keep most recent message(s) that fit
	lastMsg := result[len(result)-1]
	if lastMsg.Content != "c" {
		t.Errorf("expected last message to be 'c', got %q", lastMsg.Content)
	}
}

func TestTruncateMessages_TokenLimit(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "user", Content: strings.Repeat("x", 100)},
		{Role: "assistant", Content: strings.Repeat("y", 100)},
	}
	// Each message: ~25 tokens (100 chars / 4). Max 26 should allow only one.
	result := cm.TruncateMessages(msgs, 26)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
}

// ── Session Tests ──

func TestSession_Struct(t *testing.T) {
	now := time.Now()
	recovery := now.Add(24 * time.Hour)
	s := Session{
		ID:            "sess_test123",
		UserID:        "user_abc",
		Type:          "chat",
		Status:        "active",
		CreatedAt:     now,
		UpdatedAt:     now,
		ExpiresAt:     now.Add(1 * time.Hour),
		DeletedAt:     nil,
		RecoveryUntil: &recovery,
	}

	if s.ID != "sess_test123" {
		t.Errorf("ID: got %s", s.ID)
	}
	if s.UserID != "user_abc" {
		t.Errorf("UserID: got %s", s.UserID)
	}
	if s.Type != "chat" {
		t.Errorf("Type: got %s", s.Type)
	}
	if s.Status != "active" {
		t.Errorf("Status: got %s", s.Status)
	}
	if s.DeletedAt != nil {
		t.Error("DeletedAt should be nil")
	}
	if s.RecoveryUntil == nil {
		t.Error("RecoveryUntil should not be nil")
	}
}

func TestSession_ZeroValue(t *testing.T) {
	var s Session
	if s.ID != "" {
		t.Error("zero ID should be empty")
	}
	if s.Status != "" {
		t.Error("zero Status should be empty")
	}
}

// ── CompressSummary Tests ──

func TestCompressSummary_Empty(t *testing.T) {
	result := CompressSummary([]agent.Message{})
	if !strings.Contains(result, "Previous conversation summary") {
		t.Errorf("unexpected summary: %s", result)
	}
}

func TestCompressSummary_WithMessages(t *testing.T) {
	msgs := []agent.Message{
		{Role: "user", Content: "Hello, how are you?"},
		{Role: "assistant", Content: "I'm doing great!"},
	}
	result := CompressSummary(msgs)
	if !strings.Contains(result, "user: Hello") {
		t.Errorf("should contain user message: %s", result)
	}
	if !strings.Contains(result, "assistant: I'm doing great!") {
		t.Errorf("should contain assistant message: %s", result)
	}
}

func TestCompressSummary_SkipsSystem(t *testing.T) {
	msgs := []agent.Message{
		{Role: "system", Content: "You are helpful."},
		{Role: "user", Content: "Hi"},
	}
	result := CompressSummary(msgs)
	if strings.Contains(result, "system:") {
		t.Error("should not include system message")
	}
}

func TestCompressSummary_TruncatesLongContent(t *testing.T) {
	longContent := strings.Repeat("x", 500)
	msgs := []agent.Message{
		{Role: "user", Content: longContent},
	}
	result := CompressSummary(msgs)
	if strings.Contains(result, longContent) {
		t.Error("long content should be truncated")
	}
	if !strings.Contains(result, "...") {
		t.Error("truncated content should end with '...'")
	}
}

// ── EstimateTokens Tests ──

func TestEstimateTokens_Empty(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty string should return 0")
	}
}

func TestEstimateTokens_ASCII(t *testing.T) {
	tokens := EstimateTokens("hello world") // 11 chars -> ~3 tokens
	if tokens <= 0 {
		t.Error("should return positive token count")
	}
}

func TestEstimateTokens_CJK(t *testing.T) {
	tokens := EstimateTokens("你好世界") // 4 CJK chars -> ~3 tokens
	if tokens <= 0 {
		t.Error("should return positive token count for CJK")
	}
}

// ── NewService Tests ──

func TestNewService_DefaultValues(t *testing.T) {
	engine := &agent.Engine{}
	sessions := &Manager{ttl: 1 * time.Hour}
	auditor := security.NewAuditor(nil)
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())

	svc := NewService(engine, sessions, auditor, cbReg)
	if svc.engine != engine {
		t.Error("engine not set")
	}
	if svc.sessions != sessions {
		t.Error("sessions not set")
	}
	if svc.auditor != auditor {
		t.Error("auditor not set")
	}
	if svc.cbReg != cbReg {
		t.Error("cbReg not set")
	}
	if svc.context == nil {
		t.Error("context manager should be initialized")
	}
	if svc.context.maxTokens != 128000 {
		t.Errorf("default maxTokens: got %d", svc.context.maxTokens)
	}
	if svc.context.threshold != 0.5 {
		t.Errorf("default threshold: got %f", svc.context.threshold)
	}
}

// ── ChatRequest Tests ──

func TestChatRequest_Struct(t *testing.T) {
	req := ChatRequest{
		SessionID: "sess_1",
		Model:     "gpt-4",
		Messages:  []agent.Message{{Role: "user", Content: "test"}},
		Message:   "legacy",
		Stream:    true,
	}

	if req.SessionID != "sess_1" {
		t.Errorf("SessionID: got %s", req.SessionID)
	}
	if req.Model != "gpt-4" {
		t.Errorf("Model: got %s", req.Model)
	}
	if len(req.Messages) != 1 {
		t.Errorf("Messages length: got %d", len(req.Messages))
	}
	if req.Message != "legacy" {
		t.Errorf("Message: got %s", req.Message)
	}
	if !req.Stream {
		t.Error("Stream should be true")
	}
}

// ===== HandleChat Stream with Engine Error =====

func TestHandleChat_Stream_EngineError(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_stream_err",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	patches.ApplyMethodReturn(svc.engine, "RunStream", fmt.Errorf("stream engine failure"))

	body := `{"messages": [{"role": "user", "content": "stream error test"}], "stream": true}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	// Stream mode sets headers and returns 200 even on error (SSE protocol)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	// Response should contain [DONE] marker even on error
	if !strings.Contains(w.Body.String(), "[DONE]") {
		t.Error("stream response should contain [DONE] marker")
	}
	// Should also contain the error message in SSE format
	if !strings.Contains(w.Body.String(), "stream engine failure") {
		t.Error("stream response should contain error message")
	}
}

// ===== HandleChat Circuit Breaker Open =====

func TestHandleChat_CircuitBreakerOpen(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_cb",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	// Create a circuit breaker that is already open
	// We mock GetOrCreate to return a pre-opened circuit breaker
	cb := security.NewCircuitBreaker(security.CircuitBreakerConfig{
		MaxFailures: 1,
		CooldownSec: 3600, // Long cooldown ensures it stays open
		TimeoutSec:  10,
	})
	// Force it open by triggering failures
	for i := 0; i < 2; i++ {
		_ = cb.Call(func() error { return fmt.Errorf("fail") })
	}

	patches.ApplyMethodReturn(svc.cbReg, "GetOrCreate", cb)

	body := `{"messages": [{"role": "user", "content": "trigger cb"}]}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

// ===== HandleChat with Existing Session and User =====

func TestHandleChat_ExistingSession_WithUserID(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Get", &Session{
		ID:     "sess_user1",
		UserID: "user-1",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.sessions, "Renew", nil)
	patches.ApplyMethodReturn(svc.engine, "Run", &agent.ChatResponse{
		Content: "hello user-1",
		Usage:   agent.Usage{TotalTokens: 5},
	}, nil)

	body := `{"session_id": "sess_user1", "messages": [{"role": "user", "content": "hi"}]}`
	c, w := newGinContext("POST", "/chat", body)
	c.Set("user_id", "user-1")

	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["content"] != "hello user-1" {
		t.Errorf("content: got %v", resp["content"])
	}
}

func TestHandleChat_NoSessionID_NoUserID_CreatesSession(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_auto",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	patches.ApplyMethodReturn(svc.engine, "Run", &agent.ChatResponse{
		Content: "auto session response",
		Usage:   agent.Usage{TotalTokens: 10},
	}, nil)

	body := `{"messages": [{"role": "user", "content": "hi"}]}`
	c, w := newGinContext("POST", "/chat", body)
	// No user_id set

	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

// ===== Session Struct Edge Cases =====

func TestSession_ExpiresAt(t *testing.T) {
	now := time.Now()
	future := now.Add(2 * time.Hour)
	past := now.Add(-1 * time.Hour)

	t.Run("future expiry", func(t *testing.T) {
		s := Session{
			ID:        "sess_future",
			ExpiresAt: future,
		}
		if !s.ExpiresAt.After(now) {
			t.Error("ExpiresAt should be in the future")
		}
	})

	t.Run("past expiry", func(t *testing.T) {
		s := Session{
			ID:        "sess_past",
			ExpiresAt: past,
		}
		if !s.ExpiresAt.Before(now) {
			t.Error("ExpiresAt should be in the past")
		}
	})

	t.Run("zero value", func(t *testing.T) {
		s := Session{}
		if !s.ExpiresAt.IsZero() {
			t.Error("zero ExpiresAt should be zero time")
		}
	})
}

func TestSession_DeletedAt(t *testing.T) {
	now := time.Now()

	t.Run("not deleted", func(t *testing.T) {
		s := Session{ID: "sess_active"}
		if s.DeletedAt != nil {
			t.Error("DeletedAt should be nil for non-deleted session")
		}
	})

	t.Run("deleted", func(t *testing.T) {
		s := Session{
			ID:        "sess_deleted",
			DeletedAt: &now,
			Status:    "deleted",
		}
		if s.DeletedAt == nil {
			t.Error("DeletedAt should not be nil for deleted session")
		}
		if !s.DeletedAt.Equal(now) {
			t.Error("DeletedAt should match the set time")
		}
	})
}

func TestSession_RecoveryUntil(t *testing.T) {
	now := time.Now()
	recovery := now.Add(24 * time.Hour)

	t.Run("with recovery window", func(t *testing.T) {
		s := Session{
			ID:            "sess_recoverable",
			DeletedAt:     &now,
			RecoveryUntil: &recovery,
		}
		if s.RecoveryUntil == nil {
			t.Error("RecoveryUntil should not be nil")
		}
		if !s.RecoveryUntil.After(now) {
			t.Error("RecoveryUntil should be after now")
		}
	})

	t.Run("nil recovery", func(t *testing.T) {
		s := Session{ID: "sess_norecovery"}
		if s.RecoveryUntil != nil {
			t.Error("RecoveryUntil should be nil for non-deleted session")
		}
	})
}

func TestSession_AllFields(t *testing.T) {
	now := time.Now()
	expiry := now.Add(1 * time.Hour)
	deletedAt := now.Add(-30 * time.Minute)
	recovery := deletedAt.Add(24 * time.Hour)

	s := Session{
		ID:            "sess_full",
		UserID:        "user_full",
		Type:          "agent",
		Status:        "deleted",
		CreatedAt:     now.Add(-2 * time.Hour),
		UpdatedAt:     deletedAt,
		ExpiresAt:     expiry,
		DeletedAt:     &deletedAt,
		RecoveryUntil: &recovery,
	}

	if s.ID != "sess_full" {
		t.Errorf("ID: got %s", s.ID)
	}
	if s.UserID != "user_full" {
		t.Errorf("UserID: got %s", s.UserID)
	}
	if s.Type != "agent" {
		t.Errorf("Type: got %s", s.Type)
	}
	if s.Status != "deleted" {
		t.Errorf("Status: got %s", s.Status)
	}
	if !s.CreatedAt.Before(s.UpdatedAt) {
		t.Error("CreatedAt should be before UpdatedAt")
	}
	if !s.DeletedAt.Equal(deletedAt) {
		t.Errorf("DeletedAt mismatch")
	}
	if s.RecoveryUntil == nil || !s.RecoveryUntil.Equal(recovery) {
		t.Error("RecoveryUntil mismatch")
	}
}

func TestSession_AgentType(t *testing.T) {
	s := Session{
		ID:   "sess_agent",
		Type: "agent",
	}
	if s.Type != "agent" {
		t.Errorf("Session type should be 'agent', got %q", s.Type)
	}
}

// ===== ContextManager Enhanced Tests =====

func TestShouldCompress_ThresholdZero(t *testing.T) {
	// threshold=0 means always compress (any tokens > 0)
	cm := NewContextManager(1000, 0)
	if !cm.ShouldCompress(1) {
		t.Error("threshold=0 should compress when currentTokens=1 (1 > 0)")
	}
}

func TestShouldCompress_ThresholdOne(t *testing.T) {
	// threshold=1.0 means compress only when exceeding 100%
	cm := NewContextManager(1000, 1.0)
	if cm.ShouldCompress(1000) {
		t.Error("threshold=1.0 should not compress at exactly 100% (> not >=)")
	}
	if !cm.ShouldCompress(1001) {
		t.Error("threshold=1.0 should compress when exceeding 100%")
	}
}

func TestTruncateMessages_MultipleSystemMessages(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "system", Content: "sys1"},
		{Role: "system", Content: "sys2"}, // ignored
		{Role: "user", Content: "hello"},
	}
	result := cm.TruncateMessages(msgs, 100)
	if len(result) < 2 {
		t.Fatal("expected at least system + user messages")
	}
	if result[0].Role != "system" {
		t.Errorf("first message should be system, got %s", result[0].Role)
	}
	if result[0].Content != "sys1" {
		t.Errorf("should keep first system message, got %q", result[0].Content)
	}
}

func TestTruncateMessages_OnlySystem(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "system", Content: "system prompt"},
	}
	result := cm.TruncateMessages(msgs, 100)
	if len(result) != 1 {
		t.Errorf("expected 1 message, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("expected system role, got %s", result[0].Role)
	}
}

func TestTruncateMessages_NegativeMaxTokens(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "user", Content: "hello"},
	}
	result := cm.TruncateMessages(msgs, -1)
	if len(result) != 1 {
		t.Errorf("negative maxTokens should return all messages, got %d", len(result))
	}
}

// ===== EstimateTokens Enhanced Tests =====

func TestEstimateTokens_LongString(t *testing.T) {
	long := strings.Repeat("hello world ", 100) // ~1200 chars
	tokens := EstimateTokens(long)
	if tokens <= 0 {
		t.Error("long string should return positive token count")
	}
}

func TestEstimateTokens_MixedCJKAndASCII(t *testing.T) {
	tokens := EstimateTokens("Hello世界") // mixed
	if tokens <= 0 {
		t.Error("mixed content should return positive token count")
	}
}

// ===== CompressSummary Enhanced Tests =====

func TestCompressSummary_LongAndShortMessages(t *testing.T) {
	longContent := strings.Repeat("A", 500)
	msgs := []agent.Message{
		{Role: "user", Content: longContent},
		{Role: "assistant", Content: "OK"},
	}
	result := CompressSummary(msgs)
	if !strings.Contains(result, "A") {
		t.Error("should contain truncated long message")
	}
	if !strings.Contains(result, "OK") {
		t.Error("should contain short message in full")
	}
}

func TestCompressSummary_OnlySystem(t *testing.T) {
	msgs := []agent.Message{
		{Role: "system", Content: "System prompt"},
	}
	result := CompressSummary(msgs)
	// System messages are skipped
	if !strings.Contains(result, "Previous conversation summary") {
		t.Error("should contain summary header")
	}
	if strings.Contains(result, "System prompt") {
		t.Error("should not include system message content")
	}
}

// ===== ChatRequest Zero Value =====

func TestChatRequest_ZeroValue(t *testing.T) {
	var req ChatRequest
	if req.SessionID != "" {
		t.Error("zero SessionID should be empty")
	}
	if req.Model != "" {
		t.Error("zero Model should be empty")
	}
	if req.Message != "" {
		t.Error("zero Message should be empty")
	}
	if req.Stream {
		t.Error("zero Stream should be false")
	}
}

// ===== ContextManager Enhanced Tests =====

func TestTruncateMessages_CJKContent(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	// CJK characters count as ~2 chars per token
	msgs := []agent.Message{
		{Role: "system", Content: "你是一个助手"},
		{Role: "user", Content: "你好，世界！"},
		{Role: "assistant", Content: "你好！有什么可以帮助你的吗？"},
	}
	result := cm.TruncateMessages(msgs, 100)
	if len(result) < 2 {
		t.Errorf("expected at least 2 messages, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("expected system message first, got %s", result[0].Role)
	}
}

func TestTruncateMessages_SingleCharPerMessage(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "user", Content: "a"},
		{Role: "assistant", Content: "b"},
		{Role: "user", Content: "c"},
		{Role: "assistant", Content: "d"},
		{Role: "user", Content: "e"},
	}
	// Very restrictive token limit — only last few chars fit
	result := cm.TruncateMessages(msgs, 3)
	if len(result) > 0 && result[len(result)-1].Content != "e" {
		t.Error("last message should be 'e'")
	}
}

func TestTruncateMessages_AllTruncated(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "system", Content: "A very long system prompt that takes many tokens"},
		{Role: "user", Content: "hello"},
	}
	// Very small limit — system doesn't fit, returns empty
	result := cm.TruncateMessages(msgs, 1)
	// System message alone takes more than 1 token
	if len(result) > 1 {
		t.Errorf("expected at most 1 message, got %d", len(result))
	}
}

func TestTruncateMessages_PreserveSystemIfFits(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "system", Content: "sys"},
		{Role: "user", Content: "a"},
	}
	// Generous limit — both should fit
	result := cm.TruncateMessages(msgs, 100)
	if len(result) != 2 {
		t.Errorf("expected 2 messages, got %d", len(result))
	}
	if result[0].Role != "system" {
		t.Errorf("expected system first, got %s", result[0].Role)
	}
}

func TestTruncateMessages_AllSameLength(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "user", Content: "aaaa"},     // ~1 token
		{Role: "assistant", Content: "bbbb"}, // ~1 token
		{Role: "user", Content: "cccc"},     // ~1 token
	}
	// Limit to 2 tokens — only last 2 should fit
	result := cm.TruncateMessages(msgs, 2)
	if len(result) > 3 {
		t.Errorf("expected at most 3 messages, got %d", len(result))
	}
}

// ===== CompressSummary Exact Boundary =====

func TestCompressSummary_Exactly200Chars(t *testing.T) {
	exact200 := strings.Repeat("x", 200)
	msgs := []agent.Message{
		{Role: "user", Content: exact200},
	}
	result := CompressSummary(msgs)
	// Content is exactly 200, so should NOT be truncated (only >200)
	if !strings.Contains(result, exact200) {
		t.Error("200-char content should not be truncated")
	}
}

func TestCompressSummary_201Chars(t *testing.T) {
	content201 := strings.Repeat("x", 201)
	msgs := []agent.Message{
		{Role: "user", Content: content201},
	}
	result := CompressSummary(msgs)
	// Content is >200, so should be truncated to 200 chars + "..."
	if !strings.Contains(result, "...") {
		t.Error("should contain ... for truncated content")
	}
	if strings.Contains(result, content201) {
		t.Error("full 201-char content should not appear")
	}
}

func TestCompressSummary_MultipleRoles(t *testing.T) {
	msgs := []agent.Message{
		{Role: "system", Content: "System prompt"},
		{Role: "user", Content: "Question?"},
		{Role: "assistant", Content: "Answer!"},
		{Role: "tool", Content: "Tool result"},
	}
	result := CompressSummary(msgs)
	if strings.Contains(result, "system:") {
		t.Error("should not include system message")
	}
	if !strings.Contains(result, "user:") {
		t.Error("should include user message")
	}
	if !strings.Contains(result, "assistant:") {
		t.Error("should include assistant message")
	}
	if !strings.Contains(result, "tool:") {
		t.Error("should include tool message")
	}
}

// ===== CompressSummary with System-Only =====

func TestCompressSummary_OnlySystemMessage(t *testing.T) {
	msgs := []agent.Message{
		{Role: "system", Content: "You are a helpful assistant."},
	}
	result := CompressSummary(msgs)
	if !strings.Contains(result, "Previous conversation summary") {
		t.Error("should contain summary header")
	}
	// Should not contain any role content since system is skipped
	if strings.Contains(result, "You are") {
		t.Error("should not contain system message content")
	}
}

// ===== EstimateTokens Enhanced =====

func TestEstimateTokens_Japense(t *testing.T) {
	tokens := EstimateTokens("こんにちは世界")
	if tokens <= 0 {
		t.Error("should return positive token count for Japanese")
	}
}

func TestEstimateTokens_Korean(t *testing.T) {
	tokens := EstimateTokens("안녕하세요 세계")
	if tokens <= 0 {
		t.Error("should return positive token count for Korean")
	}
}

func TestEstimateTokens_Symbols(t *testing.T) {
	tokens := EstimateTokens("!@#$%^&*()")
	if tokens <= 0 {
		t.Error("should return positive token count for symbols")
	}
}

func TestEstimateTokens_Newlines(t *testing.T) {
	tokens := EstimateTokens("line1\nline2\nline3\n")
	if tokens <= 0 {
		t.Error("should return positive token count for newline-separated text")
	}
}

// ===== Session Enhanced Tests =====

func TestSession_ChatType(t *testing.T) {
	s := Session{
		ID:   "sess_chat",
		Type: "chat",
	}
	if s.Type != "chat" {
		t.Errorf("Session type should be 'chat', got %q", s.Type)
	}
}

func TestSession_StatusVariants(t *testing.T) {
	statuses := []string{"active", "expired", "deleted"}
	for _, status := range statuses {
		t.Run(status, func(t *testing.T) {
			s := Session{ID: "sess_test", Status: status}
			if s.Status != status {
				t.Errorf("expected status %q, got %q", status, s.Status)
			}
		})
	}
}

func TestSession_CreatedAtBeforeUpdatedAt(t *testing.T) {
	now := time.Now()
	past := now.Add(-1 * time.Hour)
	s := Session{
		ID:        "sess_time",
		CreatedAt: past,
		UpdatedAt: now,
	}
	if !s.CreatedAt.Before(s.UpdatedAt) {
		t.Error("CreatedAt should be before UpdatedAt")
	}
}

func TestSession_ZeroExpiresAt(t *testing.T) {
	var s Session
	if !s.ExpiresAt.IsZero() {
		t.Error("zero ExpiresAt should be zero time")
	}
}

func TestSession_NilDeletedAt(t *testing.T) {
	s := Session{ID: "sess_active", Status: "active"}
	if s.DeletedAt != nil {
		t.Error("active session should have nil DeletedAt")
	}
}

func TestSession_NilRecoveryUntil(t *testing.T) {
	s := Session{ID: "sess_active", Status: "active"}
	if s.RecoveryUntil != nil {
		t.Error("active session should have nil RecoveryUntil")
	}
}

// ===== ChatRequest with Model Field =====

func TestChatRequest_WithModel(t *testing.T) {
	req := ChatRequest{
		Model:    "gpt-4-turbo",
		Messages: []agent.Message{{Role: "user", Content: "test"}},
	}
	if req.Model != "gpt-4-turbo" {
		t.Errorf("Model: got %s", req.Model)
	}
}

func TestChatRequest_NonStreamDefault(t *testing.T) {
	req := ChatRequest{
		Messages: []agent.Message{{Role: "user", Content: "test"}},
	}
	if req.Stream {
		t.Error("default Stream should be false")
	}
}

// ===== HandleChat with Empty Body =====

func TestHandleChat_EmptyBody(t *testing.T) {
	svc := newTestService()
	c, w := newGinContext("POST", "/chat", "")
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ===== ContextManager Additional Thresholds =====

func TestShouldCompress_LargeThreshold(t *testing.T) {
	cm := NewContextManager(1000, 0.99)
	if cm.ShouldCompress(990) {
		t.Error("should not compress at 99% threshold with 990/1000 tokens")
	}
	if !cm.ShouldCompress(991) {
		t.Error("should compress at 99% threshold with 991/1000 tokens")
	}
}

func TestShouldCompress_TinyMaxTokens(t *testing.T) {
	cm := NewContextManager(10, 0.5)
	if cm.ShouldCompress(5) {
		t.Error("should not compress at exactly 50% of 10")
	}
	if !cm.ShouldCompress(6) {
		t.Error("should compress at >50% of 10")
	}
}

// ===== ContextManager MaxTokens Boundary =====

func TestTruncateMessages_ExactTokenLimit(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	// Single-char messages: EstimateTokens("a") = 1/4 + 1 = 1 token
	// EstimateTokens("b") = 1/4 + 1 = 1 token
	// Total = 2 tokens, limit = 2 → both fit
	msgs := []agent.Message{
		{Role: "user", Content: "a"},      // ~1 token
		{Role: "assistant", Content: "b"}, // ~1 token
	}
	result := cm.TruncateMessages(msgs, 2)
	if len(result) != 2 {
		t.Errorf("expected 2 messages with exact fit, got %d", len(result))
	}
}

// ===== EstimateTokens CJK Range Boundaries =====

func TestEstimateTokens_CJKBoundary(t *testing.T) {
	// Character at the CJK boundary (U+4E00 is start of CJK)
	tokens := EstimateTokens("\u4E00\u4E01\u9FFF\uA000")
	if tokens <= 0 {
		t.Error("should return positive token count for boundary CJK chars")
	}
}

// ===== ChatRequest SessionID =====

func TestChatRequest_WithSessionID(t *testing.T) {
	req := ChatRequest{
		SessionID: "sess-123",
		Messages:  []agent.Message{{Role: "user", Content: "hi"}},
	}
	if req.SessionID != "sess-123" {
		t.Errorf("SessionID: got %s", req.SessionID)
	}
}

// ===== handleStream with RunStream callback invocation =====

func TestHandleChat_Stream_WithCallbackChunks(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_stream_cb",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	// Use ApplyMethodFunc so RunStream actually invokes the callback
	patches.ApplyMethodFunc(svc.engine, "RunStream", func(ctx context.Context, req agent.ChatRequest, callback func(string) error) error {
		_ = callback("Hello")
		_ = callback(" world")
		_ = callback("!")
		return nil
	})

	body := `{"messages": [{"role": "user", "content": "say hello"}], "stream": true}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "Hello") {
		t.Error("should contain Hello chunk")
	}
	if !strings.Contains(bodyStr, "world") {
		t.Error("should contain world chunk")
	}
	if !strings.Contains(bodyStr, "!") {
		t.Error("should contain ! chunk")
	}
	if !strings.Contains(bodyStr, "[DONE]") {
		t.Error("should contain [DONE] marker")
	}
}

func TestHandleChat_Stream_ExistingSession(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(svc.sessions, "Get", &Session{
		ID:     "sess_existing_stream",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	patches.ApplyMethodReturn(svc.sessions, "Renew", nil)
	patches.ApplyMethodFunc(svc.engine, "RunStream", func(ctx context.Context, req agent.ChatRequest, callback func(string) error) error {
		return nil
	})
	defer patches.Reset()

	body := `{"session_id": "sess_existing_stream", "messages": [{"role": "user", "content": "continue"}], "stream": true}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "[DONE]") {
		t.Error("should contain [DONE] marker")
	}
}

// ===== handleStream RunStream error with callback =====

func TestHandleChat_Stream_RunStreamErrorWithCallback(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_stream_err2",
		UserID: "",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	patches.ApplyMethodFunc(svc.engine, "RunStream", func(ctx context.Context, req agent.ChatRequest, callback func(string) error) error {
		_ = callback("partial_chunk")
		return fmt.Errorf("mid-stream failure")
	})

	body := `{"messages": [{"role": "user", "content": "test"}], "stream": true}`
	c, w := newGinContext("POST", "/chat", body)
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	bodyStr := w.Body.String()
	if !strings.Contains(bodyStr, "partial_chunk") {
		t.Error("should contain partial_chunk before error")
	}
	if !strings.Contains(bodyStr, "mid-stream failure") {
		t.Error("should contain error message in SSE format")
	}
	if !strings.Contains(bodyStr, "[DONE]") {
		t.Error("should contain [DONE] marker")
	}
}

// ===== HandleChat Stream with user_id set =====

func TestHandleChat_Stream_WithUserID(t *testing.T) {
	svc := newTestService()

	patches := gomonkey.ApplyMethodReturn(svc.sessions, "Create", &Session{
		ID:     "sess_stream_user",
		UserID: "user-1",
		Type:   "chat",
		Status: "active",
	}, nil)
	defer patches.Reset()

	patches.ApplyMethodFunc(svc.engine, "RunStream", func(ctx context.Context, req agent.ChatRequest, callback func(string) error) error {
		return nil
	})

	body := `{"messages": [{"role": "user", "content": "stream"}], "stream": true}`
	c, w := newGinContext("POST", "/chat", body)
	c.Set("user_id", "user-1")
	svc.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "[DONE]") {
		t.Error("should contain [DONE] marker")
	}
}

// ===== Session Manager tests (mocking mongo.Collection) =====

func TestNewManager(t *testing.T) {
	db := &mongo.Database{}
	coll := &mongo.Collection{}

	patches := gomonkey.ApplyMethodReturn(db, "Collection", coll)
	defer patches.Reset()

	mgr := NewManager(db, time.Hour)
	if mgr == nil {
		t.Error("NewManager should not return nil")
	}
	if mgr.ttl != time.Hour {
		t.Errorf("ttl: got %v, want 1h", mgr.ttl)
	}
	// coll should be set via Collection call
	if mgr.coll != coll {
		t.Error("coll should be set to the mocked collection")
	}
}

func TestManager_Create_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", &mongo.InsertOneResult{}, nil)
	defer patches.Reset()

	sess, err := mgr.Create("user-1", "chat")
	if err != nil {
		t.Fatalf("Create should not error: %v", err)
	}
	if sess.UserID != "user-1" {
		t.Errorf("UserID: got %q, want %q", sess.UserID, "user-1")
	}
	if sess.Type != "chat" {
		t.Errorf("Type: got %q, want %q", sess.Type, "chat")
	}
	if sess.Status != "active" {
		t.Errorf("Status: got %q, want active", sess.Status)
	}
	if sess.ID == "" {
		t.Error("Session ID should not be empty")
	}
	if !strings.HasPrefix(sess.ID, "sess_") {
		t.Errorf("Session ID should start with sess_: %s", sess.ID)
	}
}

func TestManager_Create_Error(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "InsertOne", (*mongo.InsertOneResult)(nil), fmt.Errorf("mongo insert failed"))
	defer patches.Reset()

	_, err := mgr.Create("user-1", "chat")
	if err == nil {
		t.Error("Create should return error on mongo failure")
	}
	if !strings.Contains(err.Error(), "create session") {
		t.Errorf("error should mention 'create session': %v", err)
	}
}

func TestManager_Get_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	singleResult := &mongo.SingleResult{}
	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(coll, "FindOne", singleResult)
	patches.ApplyMethodFunc(singleResult, "Decode", func(v interface{}) error {
		s := v.(*Session)
		s.ID = "sess-1"
		s.UserID = "user-1"
		s.Type = "chat"
		s.Status = "active"
		s.ExpiresAt = time.Now().Add(24 * time.Hour)
		return nil
	})
	defer patches.Reset()

	sess, err := mgr.Get("sess-1")
	if err != nil {
		t.Fatalf("Get should not error: %v", err)
	}
	if sess.ID != "sess-1" {
		t.Errorf("ID: got %q", sess.ID)
	}
	if sess.UserID != "user-1" {
		t.Errorf("UserID: got %q", sess.UserID)
	}
}

func TestManager_Get_NotFound(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	singleResult := &mongo.SingleResult{}
	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(coll, "FindOne", singleResult)
	patches.ApplyMethodReturn(singleResult, "Decode", fmt.Errorf("mongo: no documents in result"))
	defer patches.Reset()

	_, err := mgr.Get("nonexistent")
	if err == nil {
		t.Error("Get should error for nonexistent session")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestManager_Get_Expired(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	singleResult := &mongo.SingleResult{}
	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(coll, "FindOne", singleResult)
	patches.ApplyMethodFunc(singleResult, "Decode", func(v interface{}) error {
		s := v.(*Session)
		s.ID = "sess-expired"
		s.UserID = "user-1"
		s.ExpiresAt = time.Now().Add(-1 * time.Hour) // expired
		return nil
	})
	defer patches.Reset()

	_, err := mgr.Get("sess-expired")
	if err == nil {
		t.Error("Get should error for expired session")
	}
	if !strings.Contains(err.Error(), "expired") {
		t.Errorf("error should mention 'expired': %v", err)
	}
}

func TestManager_Renew_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)
	defer patches.Reset()

	err := mgr.Renew("sess-1")
	if err != nil {
		t.Fatalf("Renew should not error: %v", err)
	}
}

func TestManager_Renew_Error(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", (*mongo.UpdateResult)(nil), fmt.Errorf("mongo update failed"))
	defer patches.Reset()

	err := mgr.Renew("sess-1")
	if err == nil {
		t.Error("Renew should error on mongo failure")
	}
}

func TestManager_Renew_NotFound(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 0}, nil)
	defer patches.Reset()

	err := mgr.Renew("nonexistent")
	if err == nil {
		t.Error("Renew should error when MatchedCount is 0")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestManager_Cleanup(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "DeleteMany", &mongo.DeleteResult{}, nil)
	defer patches.Reset()

	// Cleanup doesn't return errors, just verify it doesn't panic
	mgr.Cleanup(context.Background())
}

func TestManager_ListByUser_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	cursor := &mongo.Cursor{}
	nextCalls := 0

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(coll, "Find", cursor, nil)
	patches.ApplyMethodFunc(cursor, "Next", func(ctx context.Context) bool {
		nextCalls++
		return nextCalls <= 2
	})
	patches.ApplyMethodFunc(cursor, "Decode", func(v interface{}) error {
		s := v.(*Session)
		s.ID = fmt.Sprintf("sess-%d", nextCalls)
		s.UserID = "user-1"
		s.Type = "chat"
		s.Status = "active"
		s.ExpiresAt = time.Now().Add(time.Hour)
		return nil
	})
	patches.ApplyMethodReturn(cursor, "Close", nil)
	defer patches.Reset()

	sessions := mgr.ListByUser("user-1")
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions, got %d", len(sessions))
	}
}

func TestManager_ListByUser_FindError(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "Find", (*mongo.Cursor)(nil), fmt.Errorf("mongo find failed"))
	defer patches.Reset()

	sessions := mgr.ListByUser("user-1")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions on error, got %d", len(sessions))
	}
}

func TestManager_ListByUser_WithExpired(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	cursor := &mongo.Cursor{}
	nextCalls := 0

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(coll, "Find", cursor, nil)
	patches.ApplyMethodFunc(cursor, "Next", func(ctx context.Context) bool {
		nextCalls++
		return nextCalls <= 2
	})
	patches.ApplyMethodFunc(cursor, "Decode", func(v interface{}) error {
		s := v.(*Session)
		if nextCalls == 1 {
			// Expired session
			s.ID = "sess-expired"
			s.UserID = "user-1"
			s.ExpiresAt = time.Now().Add(-1 * time.Hour)
		} else {
			// Active session
			s.ID = "sess-active"
			s.UserID = "user-1"
			s.ExpiresAt = time.Now().Add(time.Hour)
		}
		return nil
	})
	patches.ApplyMethodReturn(cursor, "Close", nil)
	defer patches.Reset()

	sessions := mgr.ListByUser("user-1")
	if len(sessions) != 2 {
		t.Errorf("expected 2 sessions (both returned even if expired), got %d", len(sessions))
	}
	// Both should appear; the expired one gets Status set to "expired" but is still returned
	if sessions[0].Status != "expired" {
		t.Errorf("expired session should have status 'expired', got %q", sessions[0].Status)
	}
}

func TestManager_Delete_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)
	defer patches.Reset()

	err := mgr.Delete("sess-1")
	if err != nil {
		t.Fatalf("Delete should not error: %v", err)
	}
}

func TestManager_Delete_Error(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", (*mongo.UpdateResult)(nil), fmt.Errorf("mongo update failed"))
	defer patches.Reset()

	err := mgr.Delete("sess-1")
	if err == nil {
		t.Error("Delete should error on mongo failure")
	}
}

func TestManager_Delete_NotFound(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 0}, nil)
	defer patches.Reset()

	err := mgr.Delete("nonexistent")
	if err == nil {
		t.Error("Delete should error when MatchedCount is 0")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestManager_Restore_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 1}, nil)
	defer patches.Reset()

	err := mgr.Restore("sess-1")
	if err != nil {
		t.Fatalf("Restore should not error: %v", err)
	}
}

func TestManager_Restore_Error(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", (*mongo.UpdateResult)(nil), fmt.Errorf("mongo update failed"))
	defer patches.Reset()

	err := mgr.Restore("sess-1")
	if err == nil {
		t.Error("Restore should error on mongo failure")
	}
}

func TestManager_Restore_NotFound(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateOne", &mongo.UpdateResult{MatchedCount: 0}, nil)
	defer patches.Reset()

	err := mgr.Restore("nonexistent")
	if err == nil {
		t.Error("Restore should error when MatchedCount is 0")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error should mention 'not found': %v", err)
	}
}

func TestManager_ListDeleted_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	cursor := &mongo.Cursor{}
	nextCalls := 0

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(coll, "Find", cursor, nil)
	patches.ApplyMethodFunc(cursor, "Next", func(ctx context.Context) bool {
		nextCalls++
		return nextCalls <= 1
	})
	patches.ApplyMethodFunc(cursor, "Decode", func(v interface{}) error {
		s := v.(*Session)
		s.ID = "sess-deleted"
		s.UserID = "user-1"
		s.Status = "deleted"
		return nil
	})
	patches.ApplyMethodReturn(cursor, "Close", nil)
	defer patches.Reset()

	sessions := mgr.ListDeleted("user-1")
	if len(sessions) != 1 {
		t.Errorf("expected 1 session, got %d", len(sessions))
	}
	if sessions[0].Status != "deleted" {
		t.Errorf("status: got %q", sessions[0].Status)
	}
}

func TestManager_ListDeleted_FindError(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "Find", (*mongo.Cursor)(nil), fmt.Errorf("mongo find failed"))
	defer patches.Reset()

	sessions := mgr.ListDeleted("user-1")
	if len(sessions) != 0 {
		t.Errorf("expected 0 sessions on error, got %d", len(sessions))
	}
}

func TestManager_SetRecoveryHours_Success(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateMany", &mongo.UpdateResult{}, nil)
	defer patches.Reset()

	err := mgr.SetRecoveryHours(48)
	if err != nil {
		t.Fatalf("SetRecoveryHours should not error: %v", err)
	}
}

func TestManager_SetRecoveryHours_Invalid(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	t.Run("zero", func(t *testing.T) {
		err := mgr.SetRecoveryHours(0)
		if err == nil {
			t.Error("SetRecoveryHours(0) should return error")
		}
	})

	t.Run("negative", func(t *testing.T) {
		err := mgr.SetRecoveryHours(-1)
		if err == nil {
			t.Error("SetRecoveryHours(-1) should return error")
		}
	})

	t.Run("too large", func(t *testing.T) {
		err := mgr.SetRecoveryHours(200)
		if err == nil {
			t.Error("SetRecoveryHours(200) should return error")
		}
	})
}

func TestManager_SetRecoveryHours_UpdateError(t *testing.T) {
	coll := &mongo.Collection{}
	mgr := &Manager{coll: coll, ttl: time.Hour}

	patches := gomonkey.ApplyMethodReturn(coll, "UpdateMany", (*mongo.UpdateResult)(nil), fmt.Errorf("mongo update failed"))
	defer patches.Reset()

	err := mgr.SetRecoveryHours(24)
	if err == nil {
		t.Error("SetRecoveryHours should error on mongo failure")
	}
}
