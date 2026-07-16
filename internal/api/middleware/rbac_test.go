package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestGetRolePermissions(t *testing.T) {
	tests := []struct {
		role    string
		wantLen int
		want    []string
	}{
		{"system_admin", 8, []string{"model:config", "system:config", "user:manage_all"}},
		{"admin", 5, []string{"user:manage", "kb:manage_own", "password:change"}},
		{"user", 2, []string{"kb:manage_own", "password:change"}},
		{"unknown", 0, nil},
	}

	for _, tt := range tests {
		perms := getRolePermissions(tt.role)
		if len(perms) < tt.wantLen {
			t.Errorf("getRolePermissions(%q) len=%d, want >= %d", tt.role, len(perms), tt.wantLen)
		}
		for _, p := range tt.want {
			found := false
			for _, perm := range perms {
				if perm == p {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("getRolePermissions(%q) missing %q", tt.role, p)
			}
		}
	}
}

func TestHasPermission(t *testing.T) {
	tests := []struct {
		perms  []string
		target string
		want   bool
	}{
		{[]string{"a", "b", "c"}, "b", true},
		{[]string{"a", "b"}, "c", false},
		{nil, "a", false},
		{[]string{}, "a", false},
		{[]string{"exact"}, "exac", false},
	}

	for _, tt := range tests {
		got := hasPermission(tt.perms, tt.target)
		if got != tt.want {
			t.Errorf("hasPermission(%v, %q) = %v, want %v", tt.perms, tt.target, got, tt.want)
		}
	}
}

func TestRequirePermission(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("role not in context returns 403", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		handler := RequirePermission("model:config")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("status: got %d, want 403", w.Code)
		}
	})

	t.Run("system_admin has all permissions", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "system_admin")
		handler := RequirePermission("system:config")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("system_admin should pass, got %d", w.Code)
		}
	})

	t.Run("user lacks permission", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "user")
		handler := RequirePermission("user:manage")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("user should be forbidden, got %d", w.Code)
		}
	})

	t.Run("user has own permissions", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "user")
		handler := RequirePermission("password:change")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("user should have password:change, got %d", w.Code)
		}
	})

	t.Run("admin lacks system config", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "admin")
		handler := RequirePermission("system:config")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("admin should lack system:config, got %d", w.Code)
		}
	})
}

func TestRequirePermission_EdgeCases(t *testing.T) {
	gin.SetMode(gin.TestMode)

	t.Run("role is int type", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", 123)
		handler := RequirePermission("model:config")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("non-string role should be 403, got %d", w.Code)
		}
	})

	t.Run("role is nil", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", nil)
		handler := RequirePermission("model:config")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("nil role should be 403, got %d", w.Code)
		}
	})

	t.Run("role is empty string", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "")
		handler := RequirePermission("model:config")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("empty role should be 403, got %d", w.Code)
		}
	})

	t.Run("unknown role", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "super_admin")
		handler := RequirePermission("model:config")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("unknown role should be 403, got %d", w.Code)
		}
	})

	t.Run("admin has user:manage", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "admin")
		handler := RequirePermission("user:manage")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("admin should have user:manage, got %d", w.Code)
		}
	})

	t.Run("admin has kb:manage_own", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "admin")
		handler := RequirePermission("kb:manage_own")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("admin should have kb:manage_own, got %d", w.Code)
		}
	})

	t.Run("admin has api:convert", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "admin")
		handler := RequirePermission("api:convert")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("admin should have api:convert, got %d", w.Code)
		}
	})

	t.Run("admin has notify:group", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "admin")
		handler := RequirePermission("notify:group")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("admin should have notify:group, got %d", w.Code)
		}
	})

	t.Run("system_admin has model:config", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "system_admin")
		handler := RequirePermission("model:config")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("system_admin should have model:config, got %d", w.Code)
		}
	})

	t.Run("system_admin has user:manage_all", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "system_admin")
		handler := RequirePermission("user:manage_all")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("system_admin should have user:manage_all, got %d", w.Code)
		}
	})

	t.Run("system_admin has audit:view", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "system_admin")
		handler := RequirePermission("audit:view")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("system_admin should have audit:view, got %d", w.Code)
		}
	})

	t.Run("system_admin has notify:all", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "system_admin")
		handler := RequirePermission("notify:all")
		handler(c)
		if w.Code != http.StatusOK {
			t.Errorf("system_admin should have notify:all, got %d", w.Code)
		}
	})

	t.Run("system_admin lacks nonexistent permission", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", "system_admin")
		handler := RequirePermission("super:power")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("system_admin should not have super:power, got %d", w.Code)
		}
	})

	t.Run("role is bool type", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Set("role", true)
		handler := RequirePermission("model:config")
		handler(c)
		if w.Code != http.StatusForbidden {
			t.Errorf("bool role should be 403, got %d", w.Code)
		}
	})
}
