package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
)

func newChatGinContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestChatHandler_HandleChat_NonStream(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return(&domainchat.ChatResponse{SessionID: "s1", Content: "hi", Usage: map[string]int{}}, nil)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"message":"hello"}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp domainchat.ChatResponse
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.SessionID != "s1" || resp.Content != "hi" {
		t.Errorf("unexpected response: %+v", resp)
	}
}

func TestChatHandler_HandleChat_InvalidBody(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	h := NewChatHandler(svc)
	c, w := newChatGinContext("POST", "/chat", "not-json")
	h.HandleChat(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatHandler_HandleChat_UnauthorizedSession(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return((*domainchat.ChatResponse)(nil), domainchat.ErrUnauthorizedSession)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"session_id":"s1","message":"hi"}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("expected 401, got %d", w.Code)
	}
}

func TestChatHandler_HandleChat_ModelError(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Process", mock.Anything, mock.Anything, "u1", "admin").
		Return((*domainchat.ChatResponse)(nil), errors.New("model down"))
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"message":"hi"}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestChatHandler_HandleChat_Stream(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Stream", mock.Anything, mock.Anything, "u1", "admin", mock.Anything).
		Run(func(args mock.Arguments) {
			w := args.Get(4).(http.ResponseWriter)
			_, _ = w.Write([]byte("data: {\"type\":\"text\",\"content\":\"hi\"}\n\ndata: [DONE]\n\n"))
		}).
		Return(nil)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"message":"hello","stream":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	body := w.Body.String()
	if !strings.Contains(body, "data: [DONE]") {
		t.Errorf("missing DONE marker: %s", body)
	}
}

func TestChatHandler_HandleChat_StreamValidationError(t *testing.T) {
	svc := chatmocks.NewChatService(t)
	svc.On("Stream", mock.Anything, mock.Anything, "u1", "admin", mock.Anything).
		Return(domainchat.ErrMessagesRequired)
	h := NewChatHandler(svc)

	c, w := newChatGinContext("POST", "/chat", `{"stream":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "admin")
	h.HandleChat(c)
	// Validation error before SSE starts → should map to 400.
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestChatErrorStatus(t *testing.T) {
	cases := []struct {
		err  error
		want int
	}{
		{domainchat.ErrMessagesRequired, http.StatusBadRequest},
		{domainchat.ErrUserMessageRequired, http.StatusBadRequest},
		{domainchat.ErrUnauthorizedSession, http.StatusUnauthorized},
		{domainchat.ErrSessionCreateFailed, http.StatusInternalServerError},
		{domainchat.ErrADKSessionInitFailed, http.StatusInternalServerError},
		{errors.New("unknown"), http.StatusServiceUnavailable},
	}
	for _, c := range cases {
		if got := chatErrorStatus(c.err); got != c.want {
			t.Errorf("chatErrorStatus(%v) = %d, want %d", c.err, got, c.want)
		}
	}
}

// ensure context import is used.
var _ = context.Background
