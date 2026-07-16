package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewChatService(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	if svc == nil {
		t.Error("NewService should not return nil")
	}
}

func TestNewContextManager(t *testing.T) {
	cm := NewContextManager(128000, 0.5)
	if cm == nil {
		t.Error("NewContextManager should not return nil")
	}
}

func TestContextManager_ShouldCompress(t *testing.T) {
	cm := NewContextManager(10000, 0.5)
	if cm.ShouldCompress(0) {
		t.Error("0 tokens should not compress")
	}
}

func TestContextManager_TruncateMessages(t *testing.T) {
	cm := NewContextManager(100000, 0.5)
	msgs := []agent.Message{
		{Role: "user", Content: "hello"},
		{Role: "assistant", Content: "hi there"},
	}
	result := cm.TruncateMessages(msgs, 64000)
	if len(result) != 2 {
		t.Errorf("got %d messages, want 2", len(result))
	}
}

func TestHandleChat_InvalidBody(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/chat", strings.NewReader(`{bad`))
	c.Request.Header.Set("Content-Type", "application/json")
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d", w.Code)
	}
}

func TestHandleChat_MissingMessages(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/chat", strings.NewReader(`{}`))
	c.Request.Header.Set("Content-Type", "application/json")
	svc.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d", w.Code)
	}
}

func TestChatRequest(t *testing.T) {
	req := ChatRequest{SessionID: "sess-1", Model: "gpt-4", Message: "hi", Stream: true}
	if req.Model != "gpt-4" {
		t.Errorf("Model: got %s", req.Model)
	}
	if !req.Stream {
		t.Error("Stream should be true")
	}
}
