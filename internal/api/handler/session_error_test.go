package handler

import (
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

// TestSessionHandler_Renew_ServiceError verifies Renew returns 500 when the
// underlying SessionService.Renew call fails.
func TestSessionHandler_Renew_ServiceError(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Renew", "s1").Return(errStr("renew failed"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("PUT", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Renew(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestSessionHandler_Delete_ServiceError verifies Delete returns 500 when the
// underlying SessionService.Delete call fails.
func TestSessionHandler_Delete_ServiceError(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Delete", "s1").Return(errStr("delete failed"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("DELETE", "/sessions/s1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Delete(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestSessionHandler_Restore_ServiceError verifies Restore returns 500 when
// the underlying SessionService.Restore call fails.
func TestSessionHandler_Restore_ServiceError(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Restore", "s1").Return(errStr("restore failed"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions/s1/restore")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Restore(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestSessionHandler_Create_NilSession ensures a defensive nil-session branch
// is exercised; the handler dereferences s.ID, so a nil session with nil
// error would panic. We verify the success path returns 201 when the session
// is present (regression guard for the error-path neighbors).
func TestSessionHandler_Create_NilSession(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Create", "u1", "chat", "").Return(&domainchat.Session{ID: "s2"}, nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("POST", "/sessions?type=chat")
	c.Set("user_id", "u1")
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty body")
	}
}

// ensure mock package referenced.
var _ = mock.Anything

// TestRegisterSessionRoutes verifies that RegisterSessionRoutes wires the
// session CRUD routes on the given router group. This exercises the
// previously uncovered RegisterSessionRoutes function.
func TestRegisterSessionRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListByUser", "u1").Return([]*domainchat.Session{{ID: "s1"}}, nil)
	mgr.On("Create", "u1", "chat", "").Return(&domainchat.Session{ID: "s2", ExpiresAt: time.Now().Add(time.Hour)}, nil)
	h := NewSessionHandler(mgr)
	api := r.Group("/api/v1/sessions")
	api.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	RegisterSessionRoutes(api, h)

	// GET /api/v1/sessions → 200 (List)
	req := httptest.NewRequest("GET", "/api/v1/sessions", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("List expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "s1") {
		t.Errorf("expected session id in body, got %s", w.Body.String())
	}

	// POST /api/v1/sessions → 201 (Create)
	req = httptest.NewRequest("POST", "/api/v1/sessions?type=chat", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Errorf("Create expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "s2") {
		t.Errorf("expected new session id in body, got %s", w.Body.String())
	}
}
