package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
)

// TestChatHandler_HandleChat_Process_ErrSessionCreateFailed verifies that a
// Process error of ErrSessionCreateFailed maps to HTTP 500.
func TestChatHandler_HandleChat_Process_ErrSessionCreateFailed(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return((*domainchat.ChatResponse)(nil), domainchat.ErrSessionCreateFailed)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"message":"hi"}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), domainchat.ErrSessionCreateFailed.Error()) {
		t.Errorf("expected error body to contain %q, got %s",
			domainchat.ErrSessionCreateFailed.Error(), w.Body.String())
	}
}

// TestChatHandler_HandleChat_Process_ErrADKSessionInitFailed verifies that a
// Process error of ErrADKSessionInitFailed maps to HTTP 500.
func TestChatHandler_HandleChat_Process_ErrADKSessionInitFailed(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return((*domainchat.ChatResponse)(nil), domainchat.ErrADKSessionInitFailed)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"message":"hi"}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), domainchat.ErrADKSessionInitFailed.Error()) {
		t.Errorf("expected error body to contain %q, got %s",
			domainchat.ErrADKSessionInitFailed.Error(), w.Body.String())
	}
}

// TestChatHandler_HandleChat_Process_ErrMessagesRequired verifies that a
// Process error of ErrMessagesRequired maps to HTTP 400.
func TestChatHandler_HandleChat_Process_ErrMessagesRequired(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return((*domainchat.ChatResponse)(nil), domainchat.ErrMessagesRequired)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"message":"hi"}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), domainchat.ErrMessagesRequired.Error()) {
		t.Errorf("expected error body to contain %q, got %s",
			domainchat.ErrMessagesRequired.Error(), w.Body.String())
	}
}

// TestChatHandler_HandleChat_Process_ErrUserMessageRequired verifies that a
// Process error of ErrUserMessageRequired maps to HTTP 400.
func TestChatHandler_HandleChat_Process_ErrUserMessageRequired(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return((*domainchat.ChatResponse)(nil), domainchat.ErrUserMessageRequired)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"message":"hi"}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), domainchat.ErrUserMessageRequired.Error()) {
		t.Errorf("expected error body to contain %q, got %s",
			domainchat.ErrUserMessageRequired.Error(), w.Body.String())
	}
}

// TestChatHandler_HandleChat_Stream_ErrSessionCreateFailed verifies that a
// Stream error of ErrSessionCreateFailed (returned before any SSE bytes are
// flushed) maps to HTTP 500.
func TestChatHandler_HandleChat_Stream_ErrSessionCreateFailed(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Stream", mock.Anything, mock.Anything, "u1", "admin", mock.Anything).
		Return(domainchat.ErrSessionCreateFailed)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"stream":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), domainchat.ErrSessionCreateFailed.Error()) {
		t.Errorf("expected error body to contain %q, got %s",
			domainchat.ErrSessionCreateFailed.Error(), w.Body.String())
	}
}

// TestChatHandler_HandleChat_Stream_ErrADKSessionInitFailed verifies that a
// Stream error of ErrADKSessionInitFailed maps to HTTP 500.
func TestChatHandler_HandleChat_Stream_ErrADKSessionInitFailed(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Stream", mock.Anything, mock.Anything, "u1", "admin", mock.Anything).
		Return(domainchat.ErrADKSessionInitFailed)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"stream":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), domainchat.ErrADKSessionInitFailed.Error()) {
		t.Errorf("expected error body to contain %q, got %s",
			domainchat.ErrADKSessionInitFailed.Error(), w.Body.String())
	}
}

// TestChatHandler_HandleChat_Stream_ErrUserMessageRequired verifies that a
// Stream error of ErrUserMessageRequired maps to HTTP 400.
func TestChatHandler_HandleChat_Stream_ErrUserMessageRequired(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Stream", mock.Anything, mock.Anything, "u1", "admin", mock.Anything).
		Return(domainchat.ErrUserMessageRequired)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"stream":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), domainchat.ErrUserMessageRequired.Error()) {
		t.Errorf("expected error body to contain %q, got %s",
			domainchat.ErrUserMessageRequired.Error(), w.Body.String())
	}
}

// TestRegisterChatRoutes verifies that RegisterChatRoutes wires POST /chat on
// the given router group. This exercises the previously uncovered
// RegisterChatRoutes function.
func TestRegisterChatRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return(&domainchat.ChatResponse{SessionID: "s1", Content: "hi", Usage: map[string]int{}}, nil)
	h := NewChatHandler(svc)
	api := r.Group("/api/v1/chat")
	api.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Set("role", "admin"); c.Next() })
	RegisterChatRoutes(api, h)

	req := httptest.NewRequest("POST", "/api/v1/chat", strings.NewReader(`{"message":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "s1") {
		t.Errorf("expected session id in body, got %s", w.Body.String())
	}
}
