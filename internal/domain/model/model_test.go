package model

import (
	"slices"
	"testing"
)

func TestGetDefaultPermissions(t *testing.T) {
	tests := []struct {
		name       string
		role       UserRole
		wantMinLen int
		wantPerms  []string
	}{
		{"system_admin", RoleSystemAdmin, 8, []string{PermModelConfig, PermSystemConfig, PermUserManageAll}},
		{"admin", RoleAdmin, 5, []string{PermUserManage, PermKBManageOwn, PermPasswordChange}},
		{"user", RoleUser, 2, []string{PermKBManageOwn, PermPasswordChange}},
		{"unknown", UserRole("unknown"), 0, nil},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := GetDefaultPermissions(tt.role)
			if len(got) < tt.wantMinLen {
				t.Errorf("GetDefaultPermissions(%q) len=%d, want >= %d", tt.role, len(got), tt.wantMinLen)
			}
			for _, p := range tt.wantPerms {
				if !slices.Contains(got, p) {
					t.Errorf("GetDefaultPermissions(%q) missing %q", tt.role, p)
				}
			}
		})
	}
}

func TestGetAllPermissions(t *testing.T) {
	perms := GetAllPermissions()
	if len(perms) != 12 {
		t.Errorf("GetAllPermissions() len=%d, want 12", len(perms))
	}

	keys := make(map[string]bool)
	for _, p := range perms {
		if p.Key == "" || p.Name == "" {
			t.Errorf("permission has empty Key or Name: %+v", p)
		}
		if keys[p.Key] {
			t.Errorf("duplicate permission key: %s", p.Key)
		}
		keys[p.Key] = true
	}
}

func TestGetAllPermissionKeys(t *testing.T) {
	keys := GetAllPermissionKeys()
	if len(keys) != 12 {
		t.Errorf("GetAllPermissionKeys() len=%d, want 12", len(keys))
	}
}

func TestFixedRoles(t *testing.T) {
	roles := FixedRoles()
	if len(roles) != 4 {
		t.Errorf("FixedRoles() len=%d, want 4", len(roles))
	}

	expectedNames := map[string]string{
		"system_admin":  "系统管理员",
		"data_analyst":  "数据分析师",
		"kb_admin":      "知识管理员",
		"auditor":       "审计员",
	}

	for _, r := range roles {
		expectedDisplay, ok := expectedNames[r.Name]
		if !ok {
			t.Errorf("unexpected fixed role: %s", r.Name)
			continue
		}
		if r.DisplayName != expectedDisplay {
			t.Errorf("role %s DisplayName=%q, want %q", r.Name, r.DisplayName, expectedDisplay)
		}
		if r.Type != "fixed" {
			t.Errorf("role %s Type=%q, want fixed", r.Name, r.Type)
		}
	}
}
