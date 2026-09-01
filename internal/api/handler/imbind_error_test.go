package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
)

// stubRBACAllow patches (*rbacsvc.Service).HasPermission to always grant,
// so route-level permission checks pass in wiring tests.
func stubRBACAllow(t *testing.T) *rbacsvc.Service {
	t.Helper()
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "HasPermission",
		func(_ context.Context, _ string, _ string) (bool, error) { return true, nil })
	t.Cleanup(patches.Reset)
	return svc
}

// TestRegisterIMBindRoutes_NilSvc verifies that RegisterIMBindRoutes wires
// GET/PUT /im/bind and that the nil-service 503 path is reachable through the
// router. This exercises the previously-uncovered RegisterIMBindRoutes
// function and confirms the route dispatches to the handler.
func TestRegisterIMBindRoutes_NilSvc(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewIMBindHandler(nil)
	rg := r.Group("/api/im/bind")
	rg.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	RegisterIMBindRoutes(rg, h, stubRBACAllow(t))

	// GET with nil svc → 503
	req := httptest.NewRequest("GET", "/api/im/bind", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("GET expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "数据库不可用") {
		t.Errorf("expected 503 body to mention db unavailable, got %s", w.Body.String())
	}

	// PUT with nil svc → 503
	req = httptest.NewRequest("PUT", "/api/im/bind", strings.NewReader(`{"k":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("PUT expected 503, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "数据库不可用") {
		t.Errorf("expected 503 body to mention db unavailable, got %s", w.Body.String())
	}
}

// TestRegisterIMBindRoutes_WithService verifies that the registered routes
// dispatch to a real BindService-backed handler, exercising the success path
// through the router (rather than calling the handler methods directly).
func TestRegisterIMBindRoutes_WithService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	repo := mockrepo.NewIMBindRepository(t)
	repo.On("Get", mock.Anything, "u1").Return(map[string]interface{}{"open_id": "ou_123"}, nil)
	repo.On("Upsert", mock.Anything, "u1", mock.Anything).Return(nil)
	svc := im.NewBindService(repo)
	h := NewIMBindHandler(svc)
	rg := r.Group("/api/im/bind")
	rg.Use(func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() })
	RegisterIMBindRoutes(rg, h, stubRBACAllow(t))

	// GET with real svc → 200
	req := httptest.NewRequest("GET", "/api/im/bind", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("GET expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ou_123") {
		t.Errorf("expected open_id in body, got %s", w.Body.String())
	}

	// PUT with real svc → 200
	req = httptest.NewRequest("PUT", "/api/im/bind", strings.NewReader(`{"open_id":"ou_new"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("PUT expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "ok") {
		t.Errorf("expected status ok in body, got %s", w.Body.String())
	}
}
