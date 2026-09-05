package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/service/humanchannel"
)

func newHumanChannelHandler(t *testing.T) (*HumanChannelHandler, *humanchannel.Hub, *chatmocks.SessionService) {
	hub := humanchannel.NewHub()
	mgr := chatmocks.NewSessionService(t)
	return NewHumanChannelHandler(hub, mgr), hub, mgr
}

// replyGin builds a gin context for the reply endpoint with a JSON body and
// path params set.
func replyGin(path string, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodPost, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Params = gin.Params{
		{Key: "session_id", Value: "s1"},
		{Key: "request_id", Value: "req_1"},
	}
	return c, w
}

func TestHumanChannelReplyOK(t *testing.T) {
	h, hub, mgr := newHumanChannelHandler(t)
	subID, _ := hub.Subscribe("s1")
	requestID, _, err := hub.Request("s1", humanchannel.Event{Type: humanchannel.TypeConfirm, Hint: "x"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer hub.Unsubscribe("s1", subID)

	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	c, w := replyGin("/api/v1/chat/s1/human-channel/"+requestID+"/reply", `{"confirmed":true}`)
	c.Params = gin.Params{{Key: "session_id", Value: "s1"}, {Key: "request_id", Value: requestID}}
	c.Set("user_id", "u1")
	c.Set("role", "user")

	h.HandleReply(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHumanChannelReplySessionNotFound(t *testing.T) {
	h, _, mgr := newHumanChannelHandler(t)
	mgr.On("Get", "s1").Return((*domainchat.Session)(nil), errStr("not found"))
	c, w := replyGin("/api/v1/chat/s1/human-channel/req_1/reply", `{"confirmed":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.HandleReply(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHumanChannelReplyOwnershipDenied(t *testing.T) {
	h, _, mgr := newHumanChannelHandler(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "other"}, nil)
	c, w := replyGin("/api/v1/chat/s1/human-channel/req_1/reply", `{"confirmed":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.HandleReply(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHumanChannelReplySystemAdminExempt(t *testing.T) {
	h, hub, mgr := newHumanChannelHandler(t)
	subID, _ := hub.Subscribe("s1")
	requestID, _, err := hub.Request("s1", humanchannel.Event{Type: humanchannel.TypeConfirm, Hint: "x"})
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer hub.Unsubscribe("s1", subID)

	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "other"}, nil)
	c, w := replyGin("/api/v1/chat/s1/human-channel/"+requestID+"/reply", `{"confirmed":true}`)
	c.Params = gin.Params{{Key: "session_id", Value: "s1"}, {Key: "request_id", Value: requestID}}
	c.Set("user_id", "admin-1")
	c.Set("role", "system_admin")
	h.HandleReply(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 for system_admin, got %d: %s", w.Code, w.Body.String())
	}
}

func TestHumanChannelReplyUnknownRequest(t *testing.T) {
	h, hub, mgr := newHumanChannelHandler(t)
	subID, _ := hub.Subscribe("s1")
	defer hub.Unsubscribe("s1", subID)

	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	c, w := replyGin("/api/v1/chat/s1/human-channel/req_999/reply", `{"confirmed":true}`)
	c.Params = gin.Params{{Key: "session_id", Value: "s1"}, {Key: "request_id", Value: "req_999"}}
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.HandleReply(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for unknown request, got %d", w.Code)
	}
}

func TestHumanChannelReplyNoChannel(t *testing.T) {
	h, _, mgr := newHumanChannelHandler(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	c, w := replyGin("/api/v1/chat/s1/human-channel/req_1/reply", `{"confirmed":true}`)
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.HandleReply(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing channel, got %d", w.Code)
	}
}

func TestHumanChannelReplyInvalidBody(t *testing.T) {
	h, hub, mgr := newHumanChannelHandler(t)
	subID, _ := hub.Subscribe("s1")
	defer hub.Unsubscribe("s1", subID)

	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	c, w := replyGin("/api/v1/chat/s1/human-channel/req_1/reply", `not-json`)
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.HandleReply(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

func TestHumanChannelSSEOwnershipDenied(t *testing.T) {
	h, _, mgr := newHumanChannelHandler(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "other"}, nil)
	c, w := newSessionGin("GET", "/api/v1/chat/s1/human-channel")
	c.Params = gin.Params{{Key: "session_id", Value: "s1"}}
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.HandleChannel(c)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d", w.Code)
	}
}

func TestHumanChannelSSENotFound(t *testing.T) {
	h, _, mgr := newHumanChannelHandler(t)
	mgr.On("Get", "s1").Return((*domainchat.Session)(nil), errStr("not found"))
	c, w := newSessionGin("GET", "/api/v1/chat/s1/human-channel")
	c.Params = gin.Params{{Key: "session_id", Value: "s1"}}
	c.Set("user_id", "u1")
	c.Set("role", "user")
	h.HandleChannel(c)
	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestHumanChannelSSEStreamsEvent(t *testing.T) {
	h, hub, mgr := newHumanChannelHandler(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	ctx, cancel := context.WithCancel(context.Background())
	c.Request = httptest.NewRequest("GET", "/api/v1/chat/s1/human-channel", nil).WithContext(ctx)
	c.Params = gin.Params{{Key: "session_id", Value: "s1"}}
	c.Set("user_id", "u1")
	c.Set("role", "user")

	done := make(chan struct{})
	go func() {
		h.HandleChannel(c)
		close(done)
	}()

	// Wait for the subscription to register, then push an event through the hub.
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, _, err := hub.Request("s1", humanchannel.Event{Type: humanchannel.TypeConfirm, Hint: "delete?"}); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for channel subscription")
		}
		time.Sleep(5 * time.Millisecond)
	}

	// Give the SSE handler a moment to forward the event.
	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	ct := w.Header().Get("Content-Type")
	if !strings.Contains(ct, "text/event-stream") {
		t.Fatalf("expected text/event-stream, got %q", ct)
	}
	body := w.Body.String()
	if !strings.Contains(body, "confirm") {
		t.Fatalf("expected confirm event in SSE body, got %q", body)
	}
	var decoded map[string]any
	line := ""
	for _, l := range strings.Split(body, "\n") {
		if strings.HasPrefix(l, "data: ") {
			line = strings.TrimPrefix(l, "data: ")
			break
		}
	}
	if line == "" {
		t.Fatalf("no data line in SSE body: %q", body)
	}
	if err := json.Unmarshal([]byte(line), &decoded); err != nil {
		t.Fatalf("decode event: %v", err)
	}
	if decoded["type"] != "confirm" {
		t.Fatalf("expected confirm, got %v", decoded["type"])
	}
	if decoded["hint"] != "delete?" {
		t.Fatalf("expected hint, got %v", decoded["hint"])
	}
}
