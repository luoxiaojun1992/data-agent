package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
)

func newSessionGin(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(""))
	return c, w
}

func TestSessionHandler_List(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListByUser", "u1").Return([]*domainchat.Session{{ID: "s1"}}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions")
	c.Set("user_id", "u1")
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	sessions, _ := resp["sessions"].([]any)
	if len(sessions) != 1 {
		t.Errorf("sessions = %v", sessions)
	}
}

func TestSessionHandler_Create(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Create", "u1", "chat").Return(&domainchat.Session{ID: "s2", ExpiresAt: time.Now().Add(time.Hour)}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions?type=chat")
	c.Set("user_id", "u1")
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["session_id"] != "s2" {
		t.Errorf("session_id = %v", resp["session_id"])
	}
}

func TestSessionHandler_Create_Error(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Create", "u1", "chat").Return((*domainchat.Session)(nil), errStr("db"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions")
	c.Set("user_id", "u1")
	h.Create(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSessionHandler_Get(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1"}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_Get_NotFound(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "missing").Return((*domainchat.Session)(nil), errStr("not found"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/missing")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.Get(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestSessionHandler_Renew(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Renew", "s1").Return(nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("PUT", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Renew(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_Delete(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Delete", "s1").Return(nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("DELETE", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Delete(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_Restore(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Restore", "s1").Return(nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions/s1/restore")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Restore(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSessionHandler_ListDeleted(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListDeleted", mock.Anything, mock.Anything).Return([]*domainchat.Session{
		{ID: "d1", UserID: "u1"},
		{ID: "d2", UserID: "other"},
	}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/deleted")
	c.Set("user_id", "u1")
	h.ListDeleted(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	sessions, _ := resp["sessions"].([]any)
	if len(sessions) != 1 {
		t.Errorf("expected 1 user session, got %d", len(sessions))
	}
}

func TestSessionHandler_ListDeleted_Error(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListDeleted", mock.Anything, mock.Anything).Return(([]*domainchat.Session)(nil), errStr("db"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/deleted")
	c.Set("user_id", "u1")
	h.ListDeleted(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestNewSessionHandler(t *testing.T) {
	h := NewSessionHandler(nil)
	if h == nil {
		t.Error("handler should not be nil")
	}
}
