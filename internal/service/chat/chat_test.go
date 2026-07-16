package chat

import (
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
	var largeMessages []agent.Message
	for i := 0; i < 20; i++ {
		largeMessages = append(largeMessages, agent.Message{
			Role:    "user",
			Content: strings.Repeat("a", 10000), // ~2500 tokens each
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
