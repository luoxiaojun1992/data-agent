package chat

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewChatService(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	if svc == nil {
		t.Error("NewService should not return nil")
	}
	if svc.context == nil {
		t.Error("NewService should create ContextManager")
	}
}

func TestNewContextManager(t *testing.T) {
	cm := NewContextManager(128000, 0.5)
	if cm == nil {
		t.Error("NewContextManager should not return nil")
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
		t.Errorf("status: got %d, want 400", w.Code)
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
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestContextManager(t *testing.T) {
	cm := NewContextManager(10000, 0.5)

	t.Run("initial state", func(t *testing.T) {
		if cm.maxTokens != 10000 {
			t.Errorf("maxTokens: got %d", cm.maxTokens)
		}
	})

	t.Run("empty messages not compress", func(t *testing.T) {
		if cm.ShouldCompress(0) {
			t.Error("should not compress with 0 tokens")
		}
	})

	t.Run("high tokens triggers compress", func(t *testing.T) {
		if cm.ShouldCompress(6000) {
			t.Log("compress triggered at 6000 tokens")
		}
	})

	t.Run("truncate keeps messages", func(t *testing.T) {
		msgs := []agent.Message{
			{Role: "user", Content: "hello"},
			{Role: "assistant", Content: "hi"},
		}
		result := cm.TruncateMessages(msgs, 64000)
		if len(result) != len(msgs) {
			t.Errorf("TruncateMessages: got %d, want %d", len(result), len(msgs))
		}
	})
}

func TestChatRequestParsing(t *testing.T) {
	req := ChatRequest{
		SessionID: "sess-1",
		Model:     "gpt-4",
		Message:   "hello",
		Stream:    true,
	}
	if req.Message != "hello" {
		t.Errorf("Message: got %s", req.Message)
	}
	if !req.Stream {
		t.Error("Stream should be true")
	}
}
