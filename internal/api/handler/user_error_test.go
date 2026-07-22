package handler

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	usersvc "github.com/luoxiaojun1992/data-agent/internal/service/user"
	usermocks "github.com/luoxiaojun1992/data-agent/internal/service/user/mocks"
)

// svcErr is a sentinel error type used to drive non-duplicate service error
// branches in the user handler. It is distinct from usersvc.ErrDuplicate so
// the Create handler falls through to the 500 path.
type svcErr struct{ msg string }

func (e svcErr) Error() string { return e.msg }
func (e svcErr) Is(target error) bool {
	// Only match usersvc.ErrDuplicate when explicitly asked; never match here.
	return false
}

var errUserService = svcErr{"user service unavailable"}

// TestUserHandler_List_ServiceError verifies the List endpoint returns 500
// when the underlying service fails.
func TestUserHandler_List_ServiceError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("List", mock.Anything, "", int64(0), int64(20), "created_at", "desc").
		Return(([]model.User)(nil), int64(0), errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("GET", "/users", "")
	h.List(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_Get_ServiceError verifies the Get endpoint returns 500
// when the service returns an error (distinct from a nil user, nil error).
func TestUserHandler_Get_ServiceError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return((*model.User)(nil), errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("GET", "/users/u1", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.Get(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_Create_GenericError verifies the Create endpoint returns 500
// when the service returns a non-duplicate error.
func TestUserHandler_Create_GenericError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Create", mock.Anything, "bob", "secret", "admin").
		Return((*model.User)(nil), errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("POST", "/users", `{"username":"bob","password":"secret","role":"admin"}`)
	h.Create(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if errors.Is(errUserService, usersvc.ErrDuplicate) {
		t.Error("errUserService should not be ErrDuplicate")
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_UpdateRole_GetError verifies UpdateRole returns 500 when the
// initial svc.Get call fails.
func TestUserHandler_UpdateRole_GetError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return((*model.User)(nil), errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("PUT", "/users/u1", `{"role":"admin"}`)
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.UpdateRole(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_UpdateRole_InvalidBody verifies UpdateRole returns 400 when
// the request body cannot be parsed as JSON.
func TestUserHandler_UpdateRole_InvalidBody(t *testing.T) {
	svc := usermocks.NewService(t)
	h := NewUserHandler(svc)
	c, w := newUserGin("PUT", "/users/u1", "not-json")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.UpdateRole(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_UpdateRole_NotFound verifies UpdateRole returns 404 when the
// user does not exist.
func TestUserHandler_UpdateRole_NotFound(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "missing").Return((*model.User)(nil), nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("PUT", "/users/missing", `{"role":"admin"}`)
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.UpdateRole(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_UpdateRole_UpdateError verifies UpdateRole returns 500 when
// the underlying UpdateRole service call fails.
func TestUserHandler_UpdateRole_UpdateError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleUser}, nil)
	svc.On("UpdateRole", mock.Anything, "u1", model.RoleAdmin).Return(errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("PUT", "/users/u1", `{"role":"admin"}`)
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.UpdateRole(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_ToggleStatus_GetError verifies ToggleStatus returns 500 when
// svc.Get fails.
func TestUserHandler_ToggleStatus_GetError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return((*model.User)(nil), errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("PATCH", "/users/u1/status", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.ToggleStatus(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_ToggleStatus_NotFound verifies ToggleStatus returns 404 when
// the user does not exist.
func TestUserHandler_ToggleStatus_NotFound(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "missing").Return((*model.User)(nil), nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("PATCH", "/users/missing/status", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.ToggleStatus(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_ToggleStatus_ToggleError verifies ToggleStatus returns 404
// when the underlying ToggleStatus service call fails (handler maps service
// errors to 404 per the source contract).
func TestUserHandler_ToggleStatus_ToggleError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleUser}, nil)
	svc.On("ToggleStatus", mock.Anything, "u1").Return(errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("PATCH", "/users/u1/status", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.ToggleStatus(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_Delete_GetError verifies Delete returns 500 when svc.Get
// fails.
func TestUserHandler_Delete_GetError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return((*model.User)(nil), errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("DELETE", "/users/u1", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.Delete(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_Delete_NotFound verifies Delete returns 404 when the user
// does not exist.
func TestUserHandler_Delete_NotFound(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "missing").Return((*model.User)(nil), nil)
	h := NewUserHandler(svc)
	c, w := newUserGin("DELETE", "/users/missing", "")
	c.Params = gin.Params{{Key: "id", Value: "missing"}}
	h.Delete(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestUserHandler_Delete_DeleteError verifies Delete returns 500 when the
// underlying Delete service call fails.
func TestUserHandler_Delete_DeleteError(t *testing.T) {
	svc := usermocks.NewService(t)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Role: model.RoleUser}, nil)
	svc.On("Delete", mock.Anything, "u1").Return(errUserService)
	h := NewUserHandler(svc)
	c, w := newUserGin("DELETE", "/users/u1", "")
	c.Params = gin.Params{{Key: "id", Value: "u1"}}
	h.Delete(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestRegisterUserRoutes verifies that RegisterUserRoutes wires the user
// CRUD routes on the given router group. This exercises the previously
// uncovered RegisterUserRoutes function.
func TestRegisterUserRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := usermocks.NewService(t)
	svc.On("List", mock.Anything, "", int64(0), int64(20), "created_at", "desc").
		Return([]model.User{{ID: "u1"}}, int64(1), nil)
	svc.On("Get", mock.Anything, "u1").Return(&model.User{ID: "u1", Username: "alice"}, nil)
	h := NewUserHandler(svc)
	api := r.Group("/api/v1")
	RegisterUserRoutes(api, h)

	// GET /api/v1/users → 200 (List)
	req := httptest.NewRequest("GET", "/api/v1/users", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("List expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "u1") {
		t.Errorf("expected user in body, got %s", w.Body.String())
	}

	// GET /api/v1/users/u1 → 200 (Get)
	req = httptest.NewRequest("GET", "/api/v1/users/u1", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Get expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "alice") {
		t.Errorf("expected username in body, got %s", w.Body.String())
	}
}
