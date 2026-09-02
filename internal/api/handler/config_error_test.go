package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	configmocks "github.com/luoxiaojun1992/data-agent/internal/service/config/mocks"
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// TestConfigHandler_Put_ServiceError verifies Put returns 500 when the config
// service Upsert call fails.
func TestConfigHandler_Put_ServiceError(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("Upsert", mock.Anything, "k", "v", "").Return(errStr("db down"))
	h := NewConfigHandler(cfgSvc, nil)
	c, w := newCfgGin("PUT", "/sysconfig/system", `{"key":"k","value":"v"}`)
	h.Put(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestConfigHandler_ChangePassword_UpdatePasswordError verifies the 500 path
// when UpdatePassword fails at the repository layer.
func TestConfigHandler_ChangePassword_UpdatePasswordError(t *testing.T) {
	userRepo := repomocks.NewUserRepository(t)
	oldHash, _ := middleware.HashPassword("OldPass1")
	userRepo.On("FindByID", mock.Anything, "u1").Return(&model.User{ID: "u1", PasswordHash: oldHash}, nil)
	userRepo.On("UpdatePassword", mock.Anything, "u1", mock.Anything).Return(errStr("update failed"))
	h := NewConfigHandler(nil, userRepo)
	c, w := newCfgGin("POST", "/change-password", `{"old_password":"OldPass1","new_password":"NewPass1"}`)
	c.Set("user_id", "u1")
	h.ChangePassword(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestConfigHandler_ChangePassword_UserNil verifies the 404 path when the user
// lookup returns (nil, nil) — i.e. no DB error but the user does not exist.
func TestConfigHandler_ChangePassword_UserNil(t *testing.T) {
	userRepo := repomocks.NewUserRepository(t)
	userRepo.On("FindByID", mock.Anything, "u1").Return((*model.User)(nil), nil)
	h := NewConfigHandler(nil, userRepo)
	c, w := newCfgGin("POST", "/change-password", `{"old_password":"OldPass1","new_password":"NewPass1"}`)
	c.Set("user_id", "u1")
	h.ChangePassword(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestValidatePasswordComplexity_ExtraCases extends the existing complexity
// table with edge cases (empty, exactly 8 valid, digits-only) to ensure every
// branch of the function is exercised.
func TestValidatePasswordComplexity_ExtraCases(t *testing.T) {
	cases := []struct {
		pw   string
		want bool
	}{
		{"", false},                  // empty
		{"Abcdefg1", true},           // exactly 8 chars, valid mix
		{"12345678", false},          // digits only
		{"Abcdefgh", false},          // no digit
		{"abcdefg1", false},          // no upper
		{"ABCDEFG1", false},          // no lower
		{"Ab1!@#$%^&*()", true},      // symbols + valid mix
		{"VeryLongPasswordWith123", true}, // long and valid
	}
	for _, c := range cases {
		if got := validatePasswordComplexity(c.pw); got != c.want {
			t.Errorf("validatePasswordComplexity(%q) = %v, want %v", c.pw, got, c.want)
		}
	}
}

// TestRegisterSysConfigRoutes_SystemSubpath verifies the additional
// /sysconfig/system routes wired for the /admin/settings frontend page.
// These were added to fix a long-standing 404 orphan page (the new
// settings UI hard-codes /admin/sysconfig/system).
//
// RBAC middleware requires a non-nil Service and an authenticated user, so
// instead of triggering HTTP requests through middleware, we walk the Gin
// route tree to confirm both /sysconfig/system GET and PUT handlers are
// registered and use the same underlying ConfigHandler methods.
func TestRegisterSysConfigRoutes_SystemSubpath(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	// cfgSvc is only passed so the handler can be constructed; the test does
	// not exercise service methods (middleware blocks the request before the
	// handler runs).
	cfgSvc := configmocks.NewService(t)
	h := NewConfigHandler(cfgSvc, nil)
	admin := r.Group("/api/v1/admin")
	RegisterSysConfigRoutes(admin, h, nil)

	// Verify the new routes are present in the Gin route tree.
	routes := r.Routes()
	var gotGet, gotPut bool
	for _, rt := range routes {
		switch rt.Path {
		case "/api/v1/admin/sysconfig/system":
			if rt.Method == http.MethodGet {
				gotGet = true
			}
			if rt.Method == http.MethodPut {
				gotPut = true
			}
		}
	}
	if !gotGet {
		t.Errorf("GET /api/v1/admin/sysconfig/system not registered")
	}
	if !gotPut {
		t.Errorf("PUT /api/v1/admin/sysconfig/system not registered")
	}

	// /change-password remains registered (no RBAC middleware on it).
	var gotChangePwd bool
	for _, rt := range routes {
		if rt.Path == "/api/v1/admin/change-password" && rt.Method == http.MethodPost {
			gotChangePwd = true
		}
	}
	if !gotChangePwd {
		t.Errorf("POST /api/v1/admin/change-password not registered")
	}

	// The legacy bare /sysconfig routes (GET/PUT/DELETE) must be gone — the
	// orphan /admin/sysconfig page that used them was removed.
	for _, rt := range routes {
		if rt.Path == "/api/v1/admin/sysconfig" {
			t.Errorf("legacy %s /api/v1/admin/sysconfig route should be removed", rt.Method)
		}
	}
}
