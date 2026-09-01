package middleware

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// stubHasPermission patches (*rbacsvc.Service).HasPermission to return the
// given result for any input. The patch binds to a fresh instance, so any
// *rbacsvc.Service passed to RequirePermission will hit the stub.
func stubHasPermission(t *testing.T, has bool, err error) {
	t.Helper()
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "HasPermission",
		func(_ context.Context, _ string, _ string) (bool, error) {
			return has, err
		})
	t.Cleanup(patches.Reset)
}

// runRequire executes a request through a test router whose target route is
// guarded by RequirePermission(svc, perm), with the given context values set.
func runRequire(t *testing.T, svc *rbacsvc.Service, perm string, sets map[string]any) *httptest.ResponseRecorder {
	t.Helper()
	gin.SetMode(gin.TestMode)
	r := gin.New()
	r.Use(func(c *gin.Context) {
		for k, v := range sets {
			c.Set(k, v)
		}
	})
	r.GET("/test", RequirePermission(svc, perm), func(c *gin.Context) {
		c.Status(http.StatusOK)
	})
	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	r.ServeHTTP(w, req)
	return w
}

func TestRequirePermission_RBACManage_SystemAdmin(t *testing.T) {
	w := runRequire(t, nil, "rbac:manage", map[string]any{"role": "system_admin"})
	if w.Code != http.StatusOK {
		t.Errorf("system_admin should pass rbac:manage, got %d", w.Code)
	}
}

func TestRequirePermission_RBACManage_AdminRejected(t *testing.T) {
	w := runRequire(t, nil, "rbac:manage", map[string]any{"role": "admin"})
	if w.Code != http.StatusForbidden {
		t.Errorf("admin should be rejected for rbac:manage, got %d", w.Code)
	}
}

func TestRequirePermission_RBACManage_UserRejected(t *testing.T) {
	w := runRequire(t, nil, "rbac:manage", map[string]any{"role": "user"})
	if w.Code != http.StatusForbidden {
		t.Errorf("user should be rejected for rbac:manage, got %d", w.Code)
	}
}

func TestRequirePermission_RBACManage_NonStringRole(t *testing.T) {
	w := runRequire(t, nil, "rbac:manage", map[string]any{"role": 123})
	if w.Code != http.StatusForbidden {
		t.Errorf("non-string role should be rejected for rbac:manage, got %d", w.Code)
	}
}

func TestRequirePermission_MissingUserID(t *testing.T) {
	stubHasPermission(t, true, nil)
	w := runRequire(t, &rbacsvc.Service{}, "some:perm", map[string]any{"role": "admin"})
	if w.Code != http.StatusForbidden {
		t.Errorf("missing user_id should be 403, got %d", w.Code)
	}
}

func TestRequirePermission_NilService(t *testing.T) {
	w := runRequire(t, nil, "some:perm", map[string]any{"user_id": "u1", "role": "admin"})
	if w.Code != http.StatusInternalServerError {
		t.Errorf("nil service should fail closed with 500, got %d", w.Code)
	}
}

func TestRequirePermission_HasPermissionTrue(t *testing.T) {
	stubHasPermission(t, true, nil)
	w := runRequire(t, &rbacsvc.Service{}, "model:list", map[string]any{"user_id": "u1", "role": "admin"})
	if w.Code != http.StatusOK {
		t.Errorf("granted permission should pass, got %d", w.Code)
	}
}

func TestRequirePermission_HasPermissionFalse(t *testing.T) {
	stubHasPermission(t, false, nil)
	w := runRequire(t, &rbacsvc.Service{}, "model:list", map[string]any{"user_id": "u1", "role": "user"})
	if w.Code != http.StatusForbidden {
		t.Errorf("denied permission should be 403, got %d", w.Code)
	}
}

func TestRequirePermission_HasPermissionError(t *testing.T) {
	stubHasPermission(t, false, errors.New("db down"))
	w := runRequire(t, &rbacsvc.Service{}, "model:list", map[string]any{"user_id": "u1", "role": "admin"})
	if w.Code != http.StatusInternalServerError {
		t.Errorf("permission check error should be 500, got %d", w.Code)
	}
}
