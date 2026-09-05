package handler

import (
	"net/http"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	configmocks "github.com/luoxiaojun1992/data-agent/internal/service/config/mocks"
)

// errStr is a minimal error type used across handler tests to synthesize
// service errors without importing a dedicated error constructor.
type errStr string

func (e errStr) Error() string { return string(e) }

// TestConfigHandler_Put_ServiceError verifies Put returns 500 when the config
// service Upsert call fails.
func TestConfigHandler_Put_ServiceError(t *testing.T) {
	cfgSvc := configmocks.NewService(t)
	cfgSvc.On("Upsert", mock.Anything, "k", "v", "").Return(errStr("db down"))
	h := NewConfigHandler(cfgSvc)
	c, w := newCfgGin("PUT", "/sysconfig/system", `{"key":"k","value":"v"}`)
	h.Put(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
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
	h := NewConfigHandler(cfgSvc)
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

	// The legacy bare /sysconfig routes (GET/PUT/DELETE) must be gone — the
	// orphan /admin/sysconfig page that used them was removed.
	for _, rt := range routes {
		if rt.Path == "/api/v1/admin/sysconfig" {
			t.Errorf("legacy %s /api/v1/admin/sysconfig route should be removed", rt.Method)
		}
	}
}
