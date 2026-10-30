package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
)

// ── List pagination clamps ──

func TestSessionHandler_List_ClampPage(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	// page=0, page_size=200 → page=1, pageSize=15
	mgr.On("ListByUserPaged", "u1", "", 1, 15).Return([]*domainchat.Session{{ID: "s1"}}, int64(1), nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions?page=0&page_size=200")
	c.Set("user_id", "u1")
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSessionHandler_List_Error(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("ListByUserPaged", "u1", "", 1, 15).Return(([]*domainchat.Session)(nil), int64(0), errStr("db down"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions")
	c.Set("user_id", "u1")
	h.List(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── TokenUsage stat-store error ──

func TestSessionHandler_TokenUsage_StoreError(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	h := NewSessionHandler(mgr)
	h.SetSessionStatStore(&fakeSessionStatStore{err: errStr("counter down")})

	c, w := newSessionGin("GET", "/sessions/s1/token-usage")
	c.Set("user_id", "u1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.TokenUsage(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── Delete permanent / busy ──

func TestSessionHandler_Delete_Permanent_Error(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	mgr.On("HardDelete", "s1").Return(errStr("hard delete failed"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("DELETE", "/sessions/s1?permanent=true")
	c.Set("user_id", "u1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Delete(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestSessionHandler_Delete_Busy(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	mgr.On("Delete", "s1").Return(domainchat.ErrSessionBusy)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("DELETE", "/sessions/s1")
	c.Set("user_id", "u1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Delete(c)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

// ── ClearHistory error ──

func TestSessionHandler_ClearHistory_Error(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "s1").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	mgr.On("ClearHistory", "s1").Return(errStr("clear failed"))
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("DELETE", "/sessions/s1/history")
	c.Set("user_id", "u1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.ClearHistory(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── verifyOwnership nil-session defensive branch ──

func TestSessionHandler_Get_NilSession(t *testing.T) {
	mgr := chatmocks.NewSessionService(t)
	mgr.On("Get", "s1").Return((*domainchat.Session)(nil), nil)
	h := NewSessionHandler(mgr)
	c, w := newSessionGin("GET", "/sessions/s1")
	c.Set("user_id", "u1")
	c.Params = gin.Params{{Key: "id", Value: "s1"}}
	h.Get(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── denyAdminManagingAdmin authorization guard ──

func TestDenyAdminManagingAdmin_Aborts(t *testing.T) {
	c, w := newSessionGin("PUT", "/users/u1")
	c.Set("role", "admin")
	denyAdminManagingAdmin(c, model.RoleAdmin)
	if !c.IsAborted() {
		t.Fatal("expected aborted for admin managing admin")
	}
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestDenyAdminManagingAdmin_AllowsUserTarget(t *testing.T) {
	c, _ := newSessionGin("PUT", "/users/u1")
	c.Set("role", "admin")
	denyAdminManagingAdmin(c, model.RoleUser)
	if c.IsAborted() {
		t.Error("admin managing a regular user should not be aborted")
	}
}

func TestDenyAdminManagingAdmin_AllowsNonAdmin(t *testing.T) {
	c, _ := newSessionGin("PUT", "/users/u1")
	// no role set (or system_admin) → guard is a no-op
	denyAdminManagingAdmin(c, model.RoleSystemAdmin)
	if c.IsAborted() {
		t.Error("non-admin caller should not be aborted")
	}
}
