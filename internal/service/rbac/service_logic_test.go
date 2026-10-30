package rbac

import (
	"context"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// fakeUserRepo is a minimal UserRepository double; only FindByID is
// customizable (used by RemoveUserRole's system_admin guard).
type fakeUserRepo struct {
	findByIDFn func(ctx context.Context, id string) (*model.User, error)
}

func (f *fakeUserRepo) Create(context.Context, *model.User) error                     { return nil }
func (f *fakeUserRepo) FindByUsername(context.Context, string) (*model.User, error)   { return nil, nil }
func (f *fakeUserRepo) FindByIDs(context.Context, []string) ([]model.User, error)     { return nil, nil }
func (f *fakeUserRepo) SearchByEmail(context.Context, string, int) ([]model.User, error) {
	return nil, nil
}
func (f *fakeUserRepo) HasSystemAdmin(context.Context) (bool, error) { return false, nil }
func (f *fakeUserRepo) UpdatePassword(context.Context, string, string) error {
	return nil
}
func (f *fakeUserRepo) UpdateRole(context.Context, string, model.UserRole) error { return nil }
func (f *fakeUserRepo) UpdateStatus(context.Context, string, model.UserStatus) error {
	return nil
}
func (f *fakeUserRepo) Delete(context.Context, string) error { return nil }
func (f *fakeUserRepo) List(context.Context, string, int64, int64) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUserRepo) ListSorted(context.Context, string, int64, int64, string, string) ([]model.User, int64, error) {
	return nil, 0, nil
}
func (f *fakeUserRepo) FindByID(ctx context.Context, id string) (*model.User, error) {
	if f.findByIDFn != nil {
		return f.findByIDFn(ctx, id)
	}
	return nil, nil
}

var _ repository.UserRepository = (*fakeUserRepo)(nil)

func newRbacSvc() (*Service, *mongo.RBACRepository) {
	repo := &mongo.RBACRepository{}
	return &Service{repo: repo}, repo
}

func TestCanSeeAllData(t *testing.T) {
	s := &Service{}
	if !s.CanSeeAllData("system_admin") {
		t.Error("system_admin should see all data")
	}
	if s.CanSeeAllData("admin") {
		t.Error("admin should not see all data")
	}
	if s.CanSeeAllData("") {
		t.Error("empty role should not see all data")
	}
}

func TestHasPermission_NoRoles(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return nil, nil })
	t.Cleanup(patches.Reset)

	ok, err := s.HasPermission(context.Background(), "u1", "kb:view")
	if err != nil {
		t.Fatalf("HasPermission() error = %v", err)
	}
	if ok {
		t.Error("no roles should mean no permission")
	}
}

func TestHasPermission_RoleChainResolved(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return []string{"r1"}, nil })
	patches.ApplyMethodFunc(repo, "GetAllDescendantRoleIDs",
		func(_ context.Context, roleIDs []string) ([]string, error) { return []string{"r1", "r1_child"}, nil })
	patches.ApplyMethodFunc(repo, "RolesHavePermission",
		func(_ context.Context, roleIDs []string, perm string) (bool, error) { return true, nil })

	ok, err := s.HasPermission(context.Background(), "u1", "kb:view")
	if err != nil {
		t.Fatalf("HasPermission() error = %v", err)
	}
	if !ok {
		t.Error("expected permission granted")
	}
}

func TestGetUserPermissionKeys_NoRoles(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return nil, nil })
	t.Cleanup(patches.Reset)

	keys, err := s.GetUserPermissionKeys(context.Background(), "u1")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if keys != nil {
		t.Errorf("keys = %v, want nil", keys)
	}
}

func TestCreateRole_NoParentDefaultsToCustom(t *testing.T) {
	s, repo := newRbacSvc()
	var created *model.RBACRole
	patches := gomonkey.ApplyMethodFunc(repo, "CreateRole",
		func(_ context.Context, role *model.RBACRole) error { created = role; return nil })
	t.Cleanup(patches.Reset)

	role, err := s.CreateRole(context.Background(), "r", "R", "d", "")
	if err != nil {
		t.Fatalf("CreateRole() error = %v", err)
	}
	if role == nil || created == nil {
		t.Fatal("CreateRole() returned nil")
	}
	if role.Type != model.RBACRoleTypeCustom {
		t.Errorf("Type = %q, want custom", role.Type)
	}
}

func TestCreateRole_ParentAtMaxLevel(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Level: model.MaxRoleLevel}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.CreateRole(context.Background(), "r", "R", "d", "p"); err == nil {
		t.Fatal("expected error when parent is at max level")
	}
}

func TestCreateRole_ParentMaxChildCount(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Level: 0, ChildCount: maxCount}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.CreateRole(context.Background(), "r", "R", "d", "p"); err == nil {
		t.Fatal("expected error when parent reached max child count")
	}
}

func TestUpdateRole_SelfParent(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Level: 1}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := s.UpdateRole(context.Background(), "r1", "", "", "r1"); err == nil {
		t.Fatal("expected 'cannot be its own parent' error")
	}
}

func TestUpdateRole_ParentLevelMismatch(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			if id == "r1" {
				return &model.RBACRole{ID: id, Level: 2, ParentID: "old"}, nil
			}
			return &model.RBACRole{ID: id, Level: 0}, nil // want level 1
		})
	t.Cleanup(patches.Reset)

	if _, err := s.UpdateRole(context.Background(), "r1", "", "", "newparent"); err == nil {
		t.Fatal("expected level-mismatch error")
	}
}

func TestUpdateRole_UpdatesFields(t *testing.T) {
	s, repo := newRbacSvc()
	var gotID string
	var gotUpdates map[string]interface{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Level: 1}, nil
		})
	patches.ApplyMethodFunc(repo, "UpdateRole",
		func(_ context.Context, id string, updates map[string]interface{}) error {
			gotID, gotUpdates = id, updates
			return nil
		})

	if _, err := s.UpdateRole(context.Background(), "r1", "new-dn", "new-desc", ""); err != nil {
		t.Fatalf("UpdateRole() error = %v", err)
	}
	if gotID != "r1" {
		t.Errorf("UpdateRole id = %q, want r1", gotID)
	}
	if gotUpdates["display_name"] != "new-dn" || gotUpdates["description"] != "new-desc" {
		t.Errorf("updates = %v", gotUpdates)
	}
}

func TestDeleteRole_Builtin(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Type: model.RBACRoleTypeBuiltin}, nil
		})
	t.Cleanup(patches.Reset)

	if err := s.DeleteRole(context.Background(), "r1"); err == nil {
		t.Fatal("expected 'cannot delete built-in role' error")
	}
}

func TestDeleteRole_HasChildren(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Type: model.RBACRoleTypeCustom}, nil
		})
	patches.ApplyMethodFunc(repo, "RoleHasChildren",
		func(_ context.Context, id string) (bool, error) { return true, nil })

	if err := s.DeleteRole(context.Background(), "r1"); err == nil {
		t.Fatal("expected 'role has children' error")
	}
}

func TestDeleteRole_HasPermissionLinks(t *testing.T) {
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
		func(_ context.Context, id string) (bool, error) { return true, nil })

	if err := s.DeleteRole(context.Background(), "r1"); err == nil {
		t.Fatal("expected 'role has permission associations' error")
	}
}

func TestAddRolePermission_MaxCount(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, PermissionCount: maxCount}, nil
		})
	t.Cleanup(patches.Reset)

	if err := s.AddRolePermission(context.Background(), "r1", "p1"); err == nil {
		t.Fatal("expected max permission count error")
	}
}

func TestRemoveRolePermission_BuiltinProtection(t *testing.T) {
	s, repo := newRbacSvc()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(repo, "GetRole",
		func(_ context.Context, id string) (*model.RBACRole, error) {
			return &model.RBACRole{ID: id, Name: "system_admin_role"}, nil
		})
	patches.ApplyMethodFunc(repo, "GetPermission",
		func(_ context.Context, id string) (*model.RBACPermission, error) {
			return &model.RBACPermission{ID: id, Type: model.RBACPermTypeBuiltin}, nil
		})

	if err := s.RemoveRolePermission(context.Background(), "r1", "p1"); err == nil {
		t.Fatal("expected builtin-protection error")
	}
}

func TestAddUserRole_MaxCount(t *testing.T) {
	s, repo := newRbacSvc()
	ids := make([]string, maxCount)
	patches := gomonkey.ApplyMethodFunc(repo, "GetUserRoleIDs",
		func(_ context.Context, userID string) ([]string, error) { return ids, nil })
	t.Cleanup(patches.Reset)

	if err := s.AddUserRole(context.Background(), "u1", "r1"); err == nil {
		t.Fatal("expected max role count error")
	}
}

func TestRemoveUserRole_SystemAdminProtected(t *testing.T) {
	repo := &mongo.RBACRepository{}
	userRepo := &fakeUserRepo{findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
		return &model.User{Role: model.RoleSystemAdmin}, nil
	}}
	s := &Service{repo: repo, userRepo: userRepo}

	if err := s.RemoveUserRole(context.Background(), "u1", "rbac_role_system_admin"); err == nil {
		t.Fatal("expected system_admin protection error")
	}
}

func TestRemoveUserRole_NonAdminProceeds(t *testing.T) {
	repo := &mongo.RBACRepository{}
	userRepo := &fakeUserRepo{findByIDFn: func(ctx context.Context, id string) (*model.User, error) {
		return &model.User{Role: model.RoleAdmin}, nil
	}}
	s := &Service{repo: repo, userRepo: userRepo}

	var removedUser, removedRole string
	patches := gomonkey.ApplyMethodFunc(repo, "RemoveUserRole",
		func(_ context.Context, userID, roleID string) error {
			removedUser, removedRole = userID, roleID
			return nil
		})
	t.Cleanup(patches.Reset)

	if err := s.RemoveUserRole(context.Background(), "u1", "rbac_role_system_admin"); err != nil {
		t.Fatalf("RemoveUserRole() error = %v", err)
	}
	if removedUser != "u1" || removedRole != "rbac_role_system_admin" {
		t.Errorf("RemoveUserRole args = (%q,%q)", removedUser, removedRole)
	}
}
