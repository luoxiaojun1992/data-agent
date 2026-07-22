package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	usermocks "github.com/luoxiaojun1992/data-agent/internal/service/user/mocks"
)

func newUserGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestUserHandler_List(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("List", mock.Anything, "", int64(0), int64(20), "created_at", "desc").
		Return([]model.User{{ID: "u1"}}, int64(1), nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("GET", "/users", "")
	h.List(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserHandler_Get(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Username: "alice"}, nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("GET", "/users/u1", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserHandler_Get_NotFound(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "missing").Return((*model.User)(nil), nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("GET", "/users/missing", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.Get(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestUserHandler_Create(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Create", mock.Anything, "bob", "secret", "admin").
		Return(&model.User{ID: "u1", Username: "bob", Role: model.RoleAdmin}, nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("POST", "/users", `{"username":"bob","password":"secret","role":"admin"}`)
	h.Create(c)
	if w.Code != http.StatusCreated {
		t.Errorf("expected 201, got %d", w.Code)
	}
}

func TestUserHandler_Create_Duplicate(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Create", mock.Anything, "bob", "secret", "admin").
		Return((*model.User)(nil), errUserDup)
	h := NewUserHandler(svc)
	c, w := newUserGin("POST", "/users", `{"username":"bob","password":"secret","role":"admin"}`)
	h.Create(c)
	if w.Code != http.StatusConflict {
		t.Errorf("expected 409, got %d", w.Code)
	}
}

func TestUserHandler_Create_InvalidBody(t *testing.T) {
	svc := usermocks.NewService(t)
	h := NewUserHandler(svc)
	c, w := newUserGin("POST", "/users", "not-json")
	h.Create(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUserHandler_UpdateRole(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleUser}, nil)
	svc.On("UpdateRole", mock.Anything, "u1", model.RoleAdmin).Return(nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("PUT", "/users/u1", `{"role":"admin"}`)
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.UpdateRole(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserHandler_UpdateRole_SystemAdminForbidden(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleSystemAdmin}, nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("PUT", "/users/u1", `{"role":"admin"}`)
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.UpdateRole(c)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestUserHandler_UpdateRole_InvalidRole(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleUser}, nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("PUT", "/users/u1", `{"role":"bogus"}`)
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.UpdateRole(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestUserHandler_ToggleStatus(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleUser}, nil)
	svc.On("ToggleStatus", mock.Anything, "u1").Return(nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("PATCH", "/users/u1/status", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.ToggleStatus(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserHandler_ToggleStatus_SystemAdminForbidden(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleSystemAdmin}, nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("PATCH", "/users/u1/status", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.ToggleStatus(c)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestUserHandler_Delete(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleUser}, nil)
	svc.On("Delete", mock.Anything, "u1").Return(nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("DELETE", "/users/u1", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.Delete(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestUserHandler_Delete_SystemAdminForbidden(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleSystemAdmin}, nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("DELETE", "/users/u1", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.Delete(c)
	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// errUserDup wraps the duplicate error so errors.Is matches.
type userDupErr struct{}

func (userDupErr) Error() string { return "用户名已存在" }
func (userDupErr) Is(target error) bool {
	// Match usersvc.ErrDuplicate by message for the test stub.
	return target != nil && target.Error() == "用户名已存在"
}

var errUserDup = userDupErr{}
