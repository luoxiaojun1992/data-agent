package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	rolemocks "github.com/luoxiaojun1992/data-agent/internal/service/role/mocks"
)

func newRoleGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestRoleHandler_List(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("List", mock.Anything).Return([]model.Role{{ID: "r1", Name: "admin"}}, nil)
	h := NewRoleHandler(svc)
	c, w := newRoleGin("GET", "/roles", "")
	h.List(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["total"] != float64(1) {
		t.Errorf("total = %v", resp["total"])
	}
}

func TestRoleHandler_List_Error(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("List", mock.Anything).Return(([]model.Role)(nil), errStr("db down"))
	h := NewRoleHandler(svc)
	c, w := newRoleGin("GET", "/roles", "")
	h.List(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRoleHandler_ListPermissions(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("ListPermissions").Return([]model.PermissionInfo{{Name: "user_manage", Description: "用户管理"}})
	h := NewRoleHandler(svc)
	c, w := newRoleGin("GET", "/permissions", "")
	h.ListPermissions(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRoleHandler_Create(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("Create", mock.Anything, "editor", "编辑", []string{"read"}).
		Return(&model.Role{ID: "r2", Name: "editor", DisplayName: "编辑", Permissions: []string{"read"}, Type: "custom"}, nil)
	h := NewRoleHandler(svc)
	c, w := newRoleGin("POST", "/roles", `{"name":"editor","display_name":"编辑","permissions":["read"]}`)
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["id"] != "r2" {
		t.Errorf("id = %v", resp["id"])
	}
}

func TestRoleHandler_Create_InvalidBody(t *testing.T) {
	svc := rolemocks.NewService(t)
	h := NewRoleHandler(svc)
	c, w := newRoleGin("POST", "/roles", "not-json")
	h.Create(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRoleHandler_Update(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("Update", mock.Anything, "r1", []string{"read"}).Return(nil)
	h := NewRoleHandler(svc)
	c, w := newRoleGin("PUT", "/roles/r1", `{"permissions":["read"]}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.Update(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestRoleHandler_Delete(t *testing.T) {
	svc := rolemocks.NewService(t)
	svc.On("Delete", mock.Anything, "r1").Return(nil)
	h := NewRoleHandler(svc)
	c, w := newRoleGin("DELETE", "/roles/r1", "")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.Delete(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestNewRoleHandler(t *testing.T) {
	h := NewRoleHandler(nil)
	if h == nil {
		t.Error("handler should not be nil")
	}
}

var _ = context.Background
