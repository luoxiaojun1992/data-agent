package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// ── GetRole ──────────────────────────────────────────────────────────

func TestRBACHandler_GetRole_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotID string
	patches := gomonkey.ApplyMethodFunc(svc, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			gotID = id
			return &model.RBACRole{ID: id, DisplayName: "Admin"}, nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.GetRole(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotID != "r1" {
		t.Errorf("id = %q, want r1", gotID)
	}
}

func TestRBACHandler_GetRole_NotFound(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) { return nil, errors.New("nf") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.GetRole(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── CreateRole ───────────────────────────────────────────────────────

func TestRBACHandler_CreateRole_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "CreateRole",
		func(_ context.Context, name, displayName, desc, parentID string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: "new", Name: name, DisplayName: displayName}, nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"name":"r","display_name":"R","description":"d","parent_id":"p"}`)
	h.CreateRole(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRBACHandler_CreateRole_MissingRequired(t *testing.T) {
	svc := &rbacsvc.Service{}
	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"description":"no name"}`)
	h.CreateRole(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRBACHandler_CreateRole_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "CreateRole",
		func(_ context.Context, name, displayName, desc, parentID string) (*model.RBACRole, error) {
			return nil, errors.New("duplicate")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"name":"r","display_name":"R"}`)
	h.CreateRole(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── UpdateRole ───────────────────────────────────────────────────────

func TestRBACHandler_UpdateRole_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotID string
	patches := gomonkey.ApplyMethodFunc(svc, "UpdateRole",
		func(_ context.Context, id, displayName, desc, parentID string) (*model.RBACRole, error) {
			gotID = id
			return &model.RBACRole{ID: id, DisplayName: displayName}, nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("PUT", "/x", `{"display_name":"new","description":"d","parent_id":"p"}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.UpdateRole(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if gotID != "r1" {
		t.Errorf("id = %q, want r1", gotID)
	}
}

func TestRBACHandler_UpdateRole_BadJSON(t *testing.T) {
	svc := &rbacsvc.Service{}
	h := NewRBACHandler(svc)
	c, w := newGinContext("PUT", "/x", `{invalid`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.UpdateRole(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRBACHandler_UpdateRole_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "UpdateRole",
		func(_ context.Context, id, displayName, desc, parentID string) (*model.RBACRole, error) {
			return nil, errors.New("bad")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("PUT", "/x", `{}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.UpdateRole(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── DeleteRole ───────────────────────────────────────────────────────

func TestRBACHandler_DeleteRole_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "DeleteRole",
		func(_ context.Context, id string) error { return nil })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.DeleteRole(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRBACHandler_DeleteRole_Forbidden(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "DeleteRole",
		func(_ context.Context, id string) error { return errors.New("builtin") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.DeleteRole(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

// ── CreatePermission ─────────────────────────────────────────────────

func TestRBACHandler_CreatePermission_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	var created *model.RBACPermission
	patches := gomonkey.ApplyMethodFunc(svc, "CreatePermission",
		func(_ context.Context, p *model.RBACPermission) error { created = p; return nil })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"key":"kb:view","name":"View","module":"kb","description":"d"}`)
	h.CreatePermission(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if created == nil || !strings.HasPrefix(created.ID, "rbac_perm_") {
		t.Errorf("created perm ID should have rbac_perm_ prefix, got %v", created)
	}
	if created.Type != model.RBACPermTypeCustom {
		t.Errorf("Type = %q, want custom", created.Type)
	}
}

func TestRBACHandler_CreatePermission_BadJSON(t *testing.T) {
	svc := &rbacsvc.Service{}
	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{invalid`)
	h.CreatePermission(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRBACHandler_CreatePermission_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "CreatePermission",
		func(_ context.Context, p *model.RBACPermission) error { return errors.New("dup") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"key":"k","name":"n","module":"m"}`)
	h.CreatePermission(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetPermission / DeletePermission ─────────────────────────────────

func TestRBACHandler_GetPermission_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) {
			return &model.RBACPermission{ID: id, Key: "kb:view"}, nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "p1"}}
	h.GetPermission(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRBACHandler_GetPermission_NotFound(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) { return nil, errors.New("nf") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "p1"}}
	h.GetPermission(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRBACHandler_DeletePermission_Forbidden(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "DeletePermission",
		func(_ context.Context, id string) error { return errors.New("builtin") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "id", Value: "p1"}}
	h.DeletePermission(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d", w.Code)
	}
}

func TestRBACHandler_DeletePermission_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "DeletePermission",
		func(_ context.Context, id string) error { return nil })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "id", Value: "p1"}}
	h.DeletePermission(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── ListRolePermissions / AddRolePermission / RemoveRolePermission ───

func TestRBACHandler_ListRolePermissions_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "ListRolePermissions",
		func(_ context.Context, roleID string, page, pageSize int) ([]model.RBACPermission, int64, error) {
			return []model.RBACPermission{{ID: "p1"}}, int64(1), nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x?page=1&page_size=10")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.ListRolePermissions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRBACHandler_ListRolePermissions_Error(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "ListRolePermissions",
		func(_ context.Context, roleID string, page, pageSize int) ([]model.RBACPermission, int64, error) {
			return nil, 0, errors.New("boom")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.ListRolePermissions(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRBACHandler_AddRolePermission_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "AddRolePermission",
		func(_ context.Context, roleID, permissionID string) error { return nil })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"permission_id":"p1"}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.AddRolePermission(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRBACHandler_AddRolePermission_MissingField(t *testing.T) {
	svc := &rbacsvc.Service{}
	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.AddRolePermission(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRBACHandler_RemoveRolePermission_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotRole, gotPerm string
	patches := gomonkey.ApplyMethodFunc(svc, "RemoveRolePermission",
		func(_ context.Context, roleID, permissionID string) error {
			gotRole, gotPerm = roleID, permissionID
			return nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}, {Key: "permId", Value: "p1"}}
	h.RemoveRolePermission(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if gotRole != "r1" || gotPerm != "p1" {
		t.Errorf("args = (%q,%q), want (r1,p1)", gotRole, gotPerm)
	}
}

func TestRBACHandler_RemoveRolePermission_Error(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "RemoveRolePermission",
		func(_ context.Context, roleID, permissionID string) error { return errors.New("builtin") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}, {Key: "permId", Value: "p1"}}
	h.RemoveRolePermission(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── EffectivePermissions / MyPermissions ─────────────────────────────

func TestRBACHandler_EffectivePermissions_NilBecomesEmpty(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetEffectivePermissions",
		func(_ context.Context, roleID string) ([]string, error) { return nil, nil })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.EffectivePermissions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"permissions":[]`) {
		t.Errorf("expected permissions:[] (not null), got %s", w.Body.String())
	}
}

func TestRBACHandler_EffectivePermissions_Error(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetEffectivePermissions",
		func(_ context.Context, roleID string) ([]string, error) { return nil, errors.New("boom") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.EffectivePermissions(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRBACHandler_MyPermissions_NilBecomesEmpty(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetUserPermissionKeys",
		func(_ context.Context, userID string) ([]string, error) { return nil, nil })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Set("user_id", "u1")
	h.MyPermissions(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"permissions":[]`) {
		t.Errorf("expected permissions:[] (not null), got %s", w.Body.String())
	}
}

func TestRBACHandler_MyPermissions_Error(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetUserPermissionKeys",
		func(_ context.Context, userID string) ([]string, error) { return nil, errors.New("boom") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Set("user_id", "u1")
	h.MyPermissions(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ListUserRoles / AddUserRole / RemoveUserRole ─────────────────────

func TestRBACHandler_ListUserRoles_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "ListUserRoles",
		func(_ context.Context, userID string, page, pageSize int) ([]model.RBACRole, int64, error) {
			return []model.RBACRole{{ID: "r1"}}, int64(1), nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "userId", Value: "u1"}}
	h.ListUserRoles(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRBACHandler_ListUserRoles_Error(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "ListUserRoles",
		func(_ context.Context, userID string, page, pageSize int) ([]model.RBACRole, int64, error) {
			return nil, 0, errors.New("boom")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "userId", Value: "u1"}}
	h.ListUserRoles(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRBACHandler_AddUserRole_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	var gotUser, gotRole string
	patches := gomonkey.ApplyMethodFunc(svc, "AddUserRole",
		func(_ context.Context, userID, roleID string) error {
			gotUser, gotRole = userID, roleID
			return nil
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"role_id":"rbac_role_viewer"}`)
	c.Params = gin.Params{{Key: "userId", Value: "u1"}}
	h.AddUserRole(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
	if gotUser != "u1" || gotRole != "rbac_role_viewer" {
		t.Errorf("args = (%q,%q)", gotUser, gotRole)
	}
}

func TestRBACHandler_AddUserRole_MissingField(t *testing.T) {
	svc := &rbacsvc.Service{}
	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{}`)
	c.Params = gin.Params{{Key: "userId", Value: "u1"}}
	h.AddUserRole(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRBACHandler_AddUserRole_AdminEscalationBlocked(t *testing.T) {
	svc := &rbacsvc.Service{}
	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"role_id":"rbac_role_system_admin"}`)
	c.Set("role", "admin")
	c.Params = gin.Params{{Key: "userId", Value: "u1"}}
	h.AddUserRole(c)

	if w.Code != http.StatusForbidden {
		t.Errorf("expected 403, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRBACHandler_AddUserRole_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "AddUserRole",
		func(_ context.Context, userID, roleID string) error { return errors.New("max") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"role_id":"rbac_role_viewer"}`)
	c.Params = gin.Params{{Key: "userId", Value: "u1"}}
	h.AddUserRole(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestRBACHandler_RemoveUserRole_OK(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "RemoveUserRole",
		func(_ context.Context, userID, roleID string) error { return nil })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "userId", Value: "u1"}, {Key: "id", Value: "r1"}}
	h.RemoveUserRole(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestRBACHandler_RemoveUserRole_Error(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "RemoveUserRole",
		func(_ context.Context, userID, roleID string) error { return errors.New("protected") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("DELETE", "/x")
	c.Params = gin.Params{{Key: "userId", Value: "u1"}, {Key: "id", Value: "r1"}}
	h.RemoveUserRole(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── error branches for dropdown/permission handlers ──────────────────

func TestRBACHandler_AvailableParents_GetRoleError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetRole",
		func(_ context.Context, _ string) (*model.RBACRole, error) {
			return nil, errors.New("missing")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r9"}}
	h.AvailableParents(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestRBACHandler_AvailableParents_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "GetRole",
		func(_ context.Context, _ string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: "r2", Level: 2}, nil
		})
	patches.ApplyMethodFunc(svc, "AvailableParents",
		func(_ context.Context, _ int, _ string, _ int) ([]model.RBACRole, error) {
			return nil, errors.New("db down")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	c.Params = gin.Params{{Key: "id", Value: "r2"}}
	h.AvailableParents(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRBACHandler_ListParentCandidates_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "ListParentCandidates",
		func(_ context.Context, _ string, _ int) ([]model.RBACRole, error) {
			return nil, errors.New("db down")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	h.ListParentCandidates(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRBACHandler_ListPermissions_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "ListPermissions",
		func(_ context.Context, _, _ int, _, _ string) ([]model.RBACPermission, int64, error) {
			return nil, 0, errors.New("db down")
		})
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newRBACGin("GET", "/x")
	h.ListPermissions(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestRBACHandler_AddRolePermission_ServiceError(t *testing.T) {
	svc := &rbacsvc.Service{}
	patches := gomonkey.ApplyMethodFunc(svc, "AddRolePermission",
		func(_ context.Context, _, _ string) error { return errors.New("conflict") })
	t.Cleanup(patches.Reset)

	h := NewRBACHandler(svc)
	c, w := newGinContext("POST", "/x", `{"permission_id":"p1"}`)
	c.Params = gin.Params{{Key: "id", Value: "r1"}}
	h.AddRolePermission(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// Ensure json import stays referenced even if assertions above change shape.
var _ = json.Valid
