package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	rolemocks "github.com/luoxiaojun1992/data-agent/internal/service/role/mocks"
)

// TestRoleHandler_Create_ServiceError verifies Create returns 500 when the
// underlying service fails.
func TestRoleHandler_Create_ServiceError(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("Create", mock.Anything, "editor", "编辑", []string{"read"}).
		Return((*model.Role)(nil), errStr("db down"))
	h := NewRoleHandler(svc)
	c, w := newRoleGin("POST", "/roles", `{"name":"editor","display_name":"编辑","permissions":["read"]}`)
	h.Create(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestRoleHandler_Update_InvalidBody verifies Update returns 400 for
// unparseable JSON.
func TestRoleHandler_Update_InvalidBody(t *testing.T) {
	svc := rolemocks.NewService(t)
	h := NewRoleHandler(svc)
	c, w := newRoleGin("PUT", "/roles/r1", "not-json")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.Update(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestRoleHandler_Update_ServiceError verifies Update returns 500 when the
// underlying service fails.
func TestRoleHandler_Update_ServiceError(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("Update", mock.Anything, "r1", []string{"read"}).Return(errStr("db down"))
	h := NewRoleHandler(svc)
	c, w := newRoleGin("PUT", "/roles/r1", `{"permissions":["read"]}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.Update(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestRoleHandler_Delete_ServiceError verifies Delete returns 500 when the
// underlying service fails.
func TestRoleHandler_Delete_ServiceError(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("Delete", mock.Anything, "r1").Return(errStr("db down"))
	h := NewRoleHandler(svc)
	c, w := newRoleGin("DELETE", "/roles/r1", "")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.Delete(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestRegisterRoleRoutes verifies that RegisterRoleRoutes wires the role
// CRUD routes and permissions endpoint on the given router group. This
// exercises the previously uncovered RegisterRoleRoutes function.
func TestRegisterRoleRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := rolemocks.NewService(t)
	svc.On("List", mock.Anything).Return([]model.Role{{ID: "r1", Name: "admin"}}, nil)
	svc.On("ListPermissions").Return([]model.PermissionInfo{{Name: "user_manage"}})
	h := NewRoleHandler(svc)
	api := r.Group("/api/v1")
	RegisterRoleRoutes(api, h)

	// GET /api/v1/roles → 200 (List)
	req := httptest.NewRequest("GET", "/api/v1/roles", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("List expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "admin") {
		t.Errorf("expected role in body, got %s", w.Body.String())
	}

	// GET /api/v1/permissions → 200 (ListPermissions)
	req = httptest.NewRequest("GET", "/api/v1/permissions", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ListPermissions expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "user_manage") {
		t.Errorf("expected permission in body, got %s", w.Body.String())
	}
}
