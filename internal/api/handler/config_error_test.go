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
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// TestConfigHandler_Put_ServiceError verifies Put returns 500 when the config
// service Upsert call fails.
func TestConfigHandler_Put_ServiceError(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("Upsert", mock.Anything, "models", "k", "v").Return(errStr("db down"))
	h := NewConfigHandler(cfgSvc, nil)
	c, w := newCfgGin("PUT", "/sysconfig/models", `{"key":"k","value":"v"}`)
	c.Params = gin.Params{{Key: "namespace", Value: "models"}}
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

// TestRegisterSysConfigRoutes verifies that RegisterSysConfigRoutes wires the
// sysconfig GET/PUT and change-password endpoints. This exercises the
// previously uncovered RegisterSysConfigRoutes function.
func TestRegisterSysConfigRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("List", mock.Anything, "models", 1, 20).Return([]model.SystemConfig{{Key: "k", Value: "v"}}, int64(1), nil)
	cfgSvc.On("Upsert", mock.Anything, "models", "k", "v").Return(nil)
	h := NewConfigHandler(cfgSvc, nil)
	admin := r.Group("/api/v1/admin")
	RegisterSysConfigRoutes(admin, h, nil)

	// GET /api/v1/admin/sysconfig/models → 200
	req := httptest.NewRequest("GET", "/api/v1/admin/sysconfig/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Get expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "configs") {
		t.Errorf("expected configs field in body, got %s", w.Body.String())
	}

	// PUT /api/v1/admin/sysconfig/models → 200
	req = httptest.NewRequest("PUT", "/api/v1/admin/sysconfig/models", strings.NewReader(`{"key":"k","value":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Put expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "已保存") {
		t.Errorf("expected saved message in body, got %s", w.Body.String())
	}
}
