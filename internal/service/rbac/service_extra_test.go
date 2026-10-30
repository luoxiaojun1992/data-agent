package rbac

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

// ── HasPermission error paths ────────────────────────────────────────

func TestHasPermission_GetUserRoleIDsError(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return nil, errors.New("db") })
	t.Cleanup(patches.Reset)

	if _, err := s.HasPermission(context.Background(), "u1", "kb:view"); err == nil {
		t.Fatal("expected error")
	}
}

func TestHasPermission_DescendantError(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return []string{"r1"}, nil })
	patches.ApplyMethodFunc(repo, "GetAllDescendantRoleIDs",
		func(_ context.Context, roleIDs []string) ([]string, error) { return nil, errors.New("db") })

	if _, err := s.HasPermission(context.Background(), "u1", "kb:view"); err == nil {
		t.Fatal("expected descendant error")
	}
}

func TestGetUserPermissionKeys_Success(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return []string{"r1"}, nil })
	patches.ApplyMethodFunc(repo, "GetAllDescendantRoleIDs",
		func(_ context.Context, roleIDs []string) ([]string, error) { return []string{"r1"}, nil })
	patches.ApplyMethodFunc(repo, "GetEffectivePermissionKeys",
		func(_ context.Context, roleIDs []string) ([]string, error) { return []string{"kb:view"}, nil })

	keys, err := s.GetUserPermissionKeys(context.Background(), "u1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "kb:view" {
		t.Errorf("keys = %v, want [kb:view]", keys)
	}
}

func TestGetUserPermissionKeys_DescendantError(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return []string{"r1"}, nil })
	patches.ApplyMethodFunc(repo, "GetAllDescendantRoleIDs",
		func(_ context.Context, roleIDs []string) ([]string, error) { return nil, errors.New("db") })

	if _, err := s.GetUserPermissionKeys(context.Background(), "u1"); err == nil {
		t.Fatal("expected descendant error")
	}
}

// ── ListRoles skip calculation ───────────────────────────────────────

func TestListRoles_SkipCalculation(t *testing.T) {
	s, repo := newRbacSvc()
	var gotSkip, gotLimit int64
	patches := gomonkey.ApplyMethodFunc(repo, "ListRoles",
		func(_ context.Context, skip, limit int64, parentID, q, excludeUserID string) ([]model.RBACRole, int64, error) {
			gotSkip, gotLimit = skip, limit
			return nil, 0, nil
		})
	t.Cleanup(patches.Reset)

	_, _, err := s.ListRoles(context.Background(), 3, 20, "", "", "")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	// page=3, pageSize=20 → skip=(3-1)*20=40
	if gotSkip != 40 || gotLimit != 20 {
		t.Errorf("skip/limit = %d/%d, want 40/20", gotSkip, gotLimit)
	}
}

// ── CreateRole parent success / not-found ────────────────────────────

func TestCreateRole_ParentSuccess(t *testing.T) {
	s, repo := newRbacSvc()
	var created *model.RBACRole
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Level: 0, ChildCount: 0}, nil
		})
	patches.ApplyMethodFunc(repo, "CreateRole",
		func(_ context.Context, role *model.RBACRole) error { created = role; return nil })

	role, err := s.CreateRole(context.Background(), "r", "R", "d", "parent")
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if role == nil || role.ParentID != "parent" {
		t.Errorf("role.ParentID = %v, want parent", role)
	}
	if created == nil || created.ParentID != "parent" {
		t.Error("created role should carry parentID")
	}
}

func TestCreateRole_ParentNotFound(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) { return nil, errors.New("nf") })
	t.Cleanup(patches.Reset)

	if _, err := s.CreateRole(context.Background(), "r", "R", "d", "parent"); err == nil {
		t.Fatal("expected 'parent role not found' error")
	}
}

// ── UpdateRole parent-change branches ────────────────────────────────

func TestUpdateRole_GetRoleError(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) { return nil, errors.New("nf") })
	t.Cleanup(patches.Reset)

	if _, err := s.UpdateRole(context.Background(), "r1", "", "", ""); err == nil {
		t.Fatal("expected GetRole error")
	}
}

func TestUpdateRole_NewParentMaxChild(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			if id == "r1" {
				return &model.RBACRole{ID: id, Level: 2, ParentID: "old"}, nil
			}
			return &model.RBACRole{ID: id, Level: 1, ChildCount: maxCount}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.UpdateRole(context.Background(), "r1", "", "", "newparent"); err == nil {
		t.Fatal("expected 'new parent has reached max child count' error")
	}
}

func TestUpdateRole_ParentChangeSuccess(t *testing.T) {
	s, repo := newRbacSvc()
	var changedID, changedParent string
	var updateCalled bool
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			if id == "newparent" {
				return &model.RBACRole{ID: id, Level: 1, ChildCount: 0}, nil
			}
			return &model.RBACRole{ID: id, Level: 2, ParentID: "old"}, nil
		})
	patches.ApplyMethodFunc(repo, "ChangeRoleParent",
		func(_ context.Context, id, newParentID string) error {
			changedID, changedParent = id, newParentID
			return nil
		})
	patches.ApplyMethodFunc(repo, "UpdateRole",
		func(_ context.Context, id string, updates map[string]interface{}) error {
			updateCalled = true
			return nil
		})

	if _, err := s.UpdateRole(context.Background(), "r1", "dn", "desc", "newparent"); err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}
	if changedID != "r1" || changedParent != "newparent" {
		t.Errorf("ChangeRoleParent(%q,%q), want (r1,newparent)", changedID, changedParent)
	}
	if !updateCalled {
		t.Error("UpdateRole should be called when display_name/description changed")
	}
}

// ── DeleteRole has-user-links / success ──────────────────────────────

func TestDeleteRole_HasUserLinks(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Type: model.RBACRoleTypeCustom}, nil
		})
	patches.ApplyMethodFunc(repo, "RoleHasChildren",
		func(_ context.Context, id string) (bool, error) { return false, nil })
	patches.ApplyMethodFunc(repo, "RoleHasPermissionLinks",
		func(_ context.Context, id string) (bool, error) { return false, nil })
	patches.ApplyMethodFunc(repo, "RoleHasUserLinks",
		func(_ context.Context, id string) (bool, error) { return true, nil })

	if err := s.DeleteRole(context.Background(), "r1"); err == nil {
		t.Fatal("expected 'role has user associations' error")
	}
}

func TestDeleteRole_Success(t *testing.T) {
	s, repo := newRbacSvc()
	var deletedID string
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Type: model.RBACRoleTypeCustom}, nil
		})
	patches.ApplyMethodFunc(repo, "RoleHasChildren",
		func(_ context.Context, id string) (bool, error) { return false, nil })
	patches.ApplyMethodFunc(repo, "RoleHasPermissionLinks",
		func(_ context.Context, id string) (bool, error) { return false, nil })
	patches.ApplyMethodFunc(repo, "RoleHasUserLinks",
		func(_ context.Context, id string) (bool, error) { return false, nil })
	patches.ApplyMethodFunc(repo, "DeleteRole",
		func(_ context.Context, id string) error { deletedID = id; return nil })

	if err := s.DeleteRole(context.Background(), "r1"); err != nil {
		t.Fatalf("DeleteRole() error = %v", err)
	}
	if deletedID != "r1" {
		t.Errorf("DeleteRole id = %q, want r1", deletedID)
	}
}

// ── DeletePermission branches ────────────────────────────────────────

func TestDeletePermission_Builtin(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) {
			return &model.RBACPermission{ID: id, Type: model.RBACPermTypeBuiltin}, nil
		})
	t.Cleanup(patches.Reset)

	if err := s.DeletePermission(context.Background(), "p1"); err == nil {
		t.Fatal("expected 'cannot delete built-in permission' error")
	}
}

func TestDeletePermission_HasRoleLinks(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) {
			return &model.RBACPermission{ID: id, Type: model.RBACPermTypeCustom}, nil
		})
	patches.ApplyMethodFunc(repo, "PermissionHasRoleLinks",
		func(_ context.Context, id string) (bool, error) { return true, nil })

	if err := s.DeletePermission(context.Background(), "p1"); err == nil {
		t.Fatal("expected 'permission has role associations' error")
	}
}

func TestDeletePermission_Success(t *testing.T) {
	s, repo := newRbacSvc()
	var deletedID string
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) {
			return &model.RBACPermission{ID: id, Type: model.RBACPermTypeCustom}, nil
		})
	patches.ApplyMethodFunc(repo, "PermissionHasRoleLinks",
		func(_ context.Context, id string) (bool, error) { return false, nil })
	patches.ApplyMethodFunc(repo, "DeletePermission",
		func(_ context.Context, id string) error { deletedID = id; return nil })

	if err := s.DeletePermission(context.Background(), "p1"); err != nil {
		t.Fatalf("DeletePermission() error = %v", err)
	}
	if deletedID != "p1" {
		t.Errorf("DeletePermission id = %q, want p1", deletedID)
	}
}

// ── AddRolePermission / RemoveRolePermission success ─────────────────

func TestAddRolePermission_Success(t *testing.T) {
	s, repo := newRbacSvc()
	var gotRole, gotPerm string
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, PermissionCount: 0}, nil
		})
	patches.ApplyMethodFunc(repo, "AddRolePermission",
		func(_ context.Context, roleID, permissionID string) error {
			gotRole, gotPerm = roleID, permissionID
			return nil
		})

	if err := s.AddRolePermission(context.Background(), "r1", "p1"); err != nil {
		t.Fatalf("AddRolePermission() error = %v", err)
	}
	if gotRole != "r1" || gotPerm != "p1" {
		t.Errorf("args = (%q,%q)", gotRole, gotPerm)
	}
}

func TestRemoveRolePermission_Success(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Name: "custom_role"}, nil
		})
	patches.ApplyMethodFunc(repo, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) {
			return &model.RBACPermission{ID: id, Type: model.RBACPermTypeCustom}, nil
		})
	patches.ApplyMethodFunc(repo, "RemoveRolePermission",
		func(_ context.Context, roleID, permissionID string) error { return nil })

	if err := s.RemoveRolePermission(context.Background(), "r1", "p1"); err != nil {
		t.Fatalf("RemoveRolePermission() error = %v", err)
	}
}

func TestRemoveRolePermission_GetPermissionError(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Name: "custom_role"}, nil
		})
	patches.ApplyMethodFunc(repo, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) { return nil, errors.New("nf") })

	if err := s.RemoveRolePermission(context.Background(), "r1", "p1"); err == nil {
		t.Fatal("expected GetPermission error")
	}
}

// ── AddUserRole success / error ──────────────────────────────────────

func TestAddUserRole_Success(t *testing.T) {
	s, repo := newRbacSvc()
	var gotUser, gotRole string
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return []string{"r0"}, nil })
	patches.ApplyMethodFunc(repo, "AddUserRole",
		func(_ context.Context, userID, roleID string) error {
			gotUser, gotRole = userID, roleID
			return nil
		})

	if err := s.AddUserRole(context.Background(), "u1", "r1"); err != nil {
		t.Fatalf("AddUserRole() error = %v", err)
	}
	if gotUser != "u1" || gotRole != "r1" {
		t.Errorf("args = (%q,%q)", gotUser, gotRole)
	}
}

func TestAddUserRole_GetUserRoleIDsError(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return nil, errors.New("db") })
	t.Cleanup(patches.Reset)

	if err := s.AddUserRole(context.Background(), "u1", "r1"); err == nil {
		t.Fatal("expected GetUserRoleIDs error")
	}
}

// ── RemoveUserRole FindByID error ────────────────────────────────────

func TestRemoveUserRole_FindByIDError(t *testing.T) {
	repo := &mongo.RBACRepository{}
	userRepo := &fakeUserRepo{findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
		return nil, errors.New("db")
	}}
	s := &Service{repo: repo, userRepo: userRepo}

	if err := s.RemoveUserRole(context.Background(), "u1", "rbac_role_system_admin"); err == nil {
		t.Fatal("expected 'find user' error")
	}
}

// ── GetEffectivePermissions ──────────────────────────────────────────

func TestGetEffectivePermissions_Success(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetAllDescendantRoleIDs",
		func(_ context.Context, roleIDs []string) ([]string, error) { return []string{"r1", "r2"}, nil })
	patches.ApplyMethodFunc(repo, "GetEffectivePermissionKeys",
		func(_ context.Context, roleIDs []string) ([]string, error) { return []string{"kb:view"}, nil })

	keys, err := s.GetEffectivePermissions(context.Background(), "r1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if len(keys) != 1 || keys[0] != "kb:view" {
		t.Errorf("keys = %v, want [kb:view]", keys)
	}
}

func TestGetEffectivePermissions_DescendantError(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetAllDescendantRoleIDs",
		func(_ context.Context, roleIDs []string) ([]string, error) { return nil, errors.New("db") })
	t.Cleanup(patches.Reset)

	if _, err := s.GetEffectivePermissions(context.Background(), "r1"); err == nil {
		t.Fatal("expected descendant error")
	}
}
