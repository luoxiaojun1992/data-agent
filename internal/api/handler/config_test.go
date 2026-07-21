package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	configmocks "github.com/luoxiaojun1992/data-agent/internal/service/config/mocks"
	rolemocks "github.com/luoxiaojun1992/data-agent/internal/service/role/mocks"
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func newCfgGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestConfigHandler_Get(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("GetAll", mock.Anything, "models").Return([]model.SystemConfig{{Key: "k"}}, nil)
	h := NewConfigHandler(cfgSvc, nil, nil)
	c, w := newCfgGin("GET", "/sysconfig/models", "")
	c.Params = gin.Params{{Key: "namespace", Value: "models"}}
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestConfigHandler_Get_Error(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("GetAll", mock.Anything, "models").Return(([]model.SystemConfig)(nil), errStr("db"))
	h := NewConfigHandler(cfgSvc, nil, nil)
	c, w := newCfgGin("GET", "/sysconfig/models", "")
	c.Params = gin.Params{{Key: "namespace", Value: "models"}}
	h.Get(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestConfigHandler_Put(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("Upsert", mock.Anything, "models", "k", "v").Return(nil)
	h := NewConfigHandler(cfgSvc, nil, nil)
	c, w := newCfgGin("PUT", "/sysconfig/models", `{"key":"k","value":"v"}`)
	c.Params = gin.Params{{Key: "namespace", Value: "models"}}
	h.Put(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestConfigHandler_Put_InvalidBody(t *testing.T) {
	h := NewConfigHandler(nil, nil, nil)
	c, w := newCfgGin("PUT", "/sysconfig/models", "not-json")
	c.Params = gin.Params{{Key: "namespace", Value: "models"}}
	h.Put(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestValidatePasswordComplexity(t *testing.T) {
	cases := []struct{ pw string; want bool }{
		{"Abc12345", true},
		{"short", false},
		{"alllowercase123", false},
		{"ALLUPPER123", false},
		{"NoDigitsHere", false},
		{"Ab1", false}, // too short
	}
	for _, c := range cases {
		if got := validatePasswordComplexity(c.pw); got != c.want {
			t.Errorf("validatePasswordComplexity(%q) = %v, want %v", c.pw, got, c.want)
		}
	}
}

func TestConfigHandler_ChangePassword_Success(t *testing.T) {
	userRepo := repomocks.NewUserRepository(t)
	oldHash, _ := middleware.HashPassword("OldPass1")
	userRepo.On("FindByID", mock.Anything, "u1").Return(&model.User{ID: "u1", PasswordHash: oldHash}, nil)
	userRepo.On("UpdatePassword", mock.Anything, "u1", mock.Anything).Return(nil)
	h := NewConfigHandler(nil, nil, userRepo)
	c, w := newCfgGin("POST", "/change-password", `{"old_password":"OldPass1","new_password":"NewPass1"}`)
	c.Set("user_id", "u1")
	h.ChangePassword(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestConfigHandler_ChangePassword_WeakPassword(t *testing.T) {
	h := NewConfigHandler(nil, nil, repomocks.NewUserRepository(t))
	c, w := newCfgGin("POST", "/change-password", `{"old_password":"old","new_password":"weak"}`)
	c.Set("user_id", "u1")
	h.ChangePassword(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestConfigHandler_ChangePassword_InvalidBody(t *testing.T) {
	h := NewConfigHandler(nil, nil, repomocks.NewUserRepository(t))
	c, w := newCfgGin("POST", "/change-password", "not-json")
	c.Set("user_id", "u1")
	h.ChangePassword(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestConfigHandler_ChangePassword_UserNotFound(t *testing.T) {
	userRepo := repomocks.NewUserRepository(t)
	userRepo.On("FindByID", mock.Anything, "u1").Return((*model.User)(nil), errStr("not found"))
	h := NewConfigHandler(nil, nil, userRepo)
	c, w := newCfgGin("POST", "/change-password", `{"old_password":"OldPass1","new_password":"NewPass1"}`)
	c.Set("user_id", "u1")
	h.ChangePassword(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestConfigHandler_ChangePassword_WrongOldPassword(t *testing.T) {
	userRepo := repomocks.NewUserRepository(t)
	oldHash, _ := middleware.HashPassword("CorrectOld1")
	userRepo.On("FindByID", mock.Anything, "u1").Return(&model.User{ID: "u1", PasswordHash: oldHash}, nil)
	h := NewConfigHandler(nil, nil, userRepo)
	c, w := newCfgGin("POST", "/change-password", `{"old_password":"WrongOld1","new_password":"NewPass1"}`)
	c.Set("user_id", "u1")
	h.ChangePassword(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestConfigHandler_ListRoles(t *testing.T) {
	roleSvc := rolemocks.NewService(t)
	roleSvc.On("List", mock.Anything).Return([]model.Role{{ID: "r1"}}, nil)
	h := NewConfigHandler(nil, roleSvc, nil)
	c, w := newCfgGin("GET", "/roles", "")
	h.ListRoles(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestConfigHandler_CreateRole(t *testing.T) {
	roleSvc := rolemocks.NewService(t)
	roleSvc.On("Create", mock.Anything, "editor", "编辑", []string{"read"}).Return(&model.Role{ID: "r1"}, nil)
	h := NewConfigHandler(nil, roleSvc, nil)
	c, w := newCfgGin("POST", "/roles", `{"name":"editor","display_name":"编辑","permissions":["read"]}`)
	h.CreateRole(c)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestConfigHandler_UpdateRole(t *testing.T) {
	roleSvc := rolemocks.NewService(t)
	roleSvc.On("Update", mock.Anything, "r1", []string{"read"}).Return(nil)
	h := NewConfigHandler(nil, roleSvc, nil)
	c, w := newCfgGin("PUT", "/roles/r1", `{"permissions":["read"]}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.UpdateRole(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestConfigHandler_DeleteRole(t *testing.T) {
	roleSvc := rolemocks.NewService(t)
	roleSvc.On("Delete", mock.Anything, "r1").Return(nil)
	h := NewConfigHandler(nil, roleSvc, nil)
	c, w := newCfgGin("DELETE", "/roles/r1", "")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.DeleteRole(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}
