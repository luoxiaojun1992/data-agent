package rbac

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

const maxCount = 10

type Service struct {
	repo     *mongo.RBACRepository
	userRepo repository.UserRepository
}

func NewService(repo *mongo.RBACRepository, userRepo repository.UserRepository) *Service {
	return &Service{repo: repo, userRepo: userRepo}
}

// CanSeeAllData returns true for system_admin users (user role attribute, not RBAC).
func (s *Service) CanSeeAllData(userRole string) bool {
	return userRole == "system_admin"
}

// HasPermission checks if a user has the given permission via their RBAC roles.
func (s *Service) HasPermission(ctx context.Context, userID string, perm string) (bool, error) {
	roleIDs, err := s.repo.GetUserRoleIDs(ctx, userID)
	if err != nil {
		return false, err
	}
	if len(roleIDs) == 0 {
		return false, nil
	}
	allRoleIDs, err := s.repo.GetAllDescendantRoleIDs(ctx, roleIDs)
	if err != nil {
		return false, err
	}
	return s.repo.RolesHavePermission(ctx, allRoleIDs, perm)
}

// GetUserPermissionKeys returns all effective permission keys for a user.
func (s *Service) GetUserPermissionKeys(ctx context.Context, userID string) ([]string, error) {
	roleIDs, err := s.repo.GetUserRoleIDs(ctx, userID)
	if err != nil {
		return nil, err
	}
	if len(roleIDs) == 0 {
		return nil, nil
	}
	allRoleIDs, err := s.repo.GetAllDescendantRoleIDs(ctx, roleIDs)
	if err != nil {
		return nil, err
	}
	return s.repo.GetEffectivePermissionKeys(ctx, allRoleIDs)
}

// ── Role CRUD ────────────────────────────────────────────────────────

func (s *Service) ListRoles(ctx context.Context, page, pageSize int, parentID, q, excludeUserID string) ([]model.RBACRole, int64, error) {
	skip := int64((page - 1) * pageSize)
	return s.repo.ListRoles(ctx, skip, int64(pageSize), parentID, q, excludeUserID)
}

func (s *Service) GetRole(ctx context.Context, id string) (*model.RBACRole, error) {
	return s.repo.GetRole(ctx, id)
}

func (s *Service) CreateRole(ctx context.Context, name, displayName, description, parentID string) (*model.RBACRole, error) {
	role := &model.RBACRole{
		Name:        name,
		DisplayName: displayName,
		Description: description,
		ParentID:    parentID,
		Type:        model.RBACRoleTypeCustom,
	}
	if parentID != "" {
		parent, err := s.repo.GetRole(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("parent role not found: %w", err)
		}
		if parent.Level >= model.MaxRoleLevel {
			return nil, fmt.Errorf("parent role is at max level (%d), cannot create child", model.MaxRoleLevel)
		}
		if parent.ChildCount >= maxCount {
			return nil, fmt.Errorf("parent role has reached max child count (%d)", maxCount)
		}
	}
	if err := s.repo.CreateRole(ctx, role); err != nil {
		return nil, err
	}
	return role, nil
}

func (s *Service) UpdateRole(ctx context.Context, id string, displayName, description, parentID string) (*model.RBACRole, error) {
	role, err := s.repo.GetRole(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := bson.M{}
	if displayName != "" {
		updates["display_name"] = displayName
	}
	if description != "" {
		updates["description"] = description
	}

	// Handle parent change
	if parentID != "" && parentID != role.ParentID {
		if parentID == id {
			return nil, fmt.Errorf("role cannot be its own parent")
		}
		newParent, err := s.repo.GetRole(ctx, parentID)
		if err != nil {
			return nil, fmt.Errorf("parent role not found: %w", err)
		}
		// Level must stay the same: new parent's level must equal current level - 1
		expectedParentLevel := role.Level - 1
		if newParent.Level != expectedParentLevel {
			return nil, fmt.Errorf("new parent level (%d) would change role level (currently %d, parent must be %d)",
				newParent.Level, role.Level, expectedParentLevel)
		}
		if newParent.ChildCount >= maxCount {
			return nil, fmt.Errorf("new parent has reached max child count (%d)", maxCount)
		}
		if err := s.repo.ChangeRoleParent(ctx, id, parentID); err != nil {
			return nil, err
		}
	}

	if len(updates) > 0 {
		if err := s.repo.UpdateRole(ctx, id, updates); err != nil {
			return nil, err
		}
	}

	return s.repo.GetRole(ctx, id)
}

func (s *Service) DeleteRole(ctx context.Context, id string) error {
	role, err := s.repo.GetRole(ctx, id)
	if err != nil {
		return err
	}
	if role.Type != model.RBACRoleTypeCustom {
		return fmt.Errorf("cannot delete built-in role")
	}

	if has, _ := s.repo.RoleHasChildren(ctx, id); has {
		return fmt.Errorf("role has children, cannot delete")
	}
	if has, _ := s.repo.RoleHasPermissionLinks(ctx, id); has {
		return fmt.Errorf("role has permission associations, cannot delete")
	}
	if has, _ := s.repo.RoleHasUserLinks(ctx, id); has {
		return fmt.Errorf("role has user associations, cannot delete")
	}
	return s.repo.DeleteRole(ctx, id)
}

func (s *Service) AvailableParents(ctx context.Context, level int, q string, limit int) ([]model.RBACRole, error) {
	return s.repo.AvailableParents(ctx, level, q, 0, int64(limit))
}

// ListParentCandidates returns roles eligible as the parent of a new role.
func (s *Service) ListParentCandidates(ctx context.Context, q string, limit int) ([]model.RBACRole, error) {
	return s.repo.ListParentCandidates(ctx, q, 0, int64(limit))
}

// ── Permission ───────────────────────────────────────────────────────

func (s *Service) ListPermissions(ctx context.Context, page, pageSize int, q, excludeRoleID string) ([]model.RBACPermission, int64, error) {
	skip := int64((page - 1) * pageSize)
	return s.repo.ListPermissions(ctx, skip, int64(pageSize), q, excludeRoleID)
}

func (s *Service) CreatePermission(ctx context.Context, p *model.RBACPermission) error {
	return s.repo.CreatePermission(ctx, p)
}

func (s *Service) GetPermission(ctx context.Context, id string) (*model.RBACPermission, error) {
	return s.repo.GetPermission(ctx, id)
}

func (s *Service) DeletePermission(ctx context.Context, id string) error {
	perm, err := s.repo.GetPermission(ctx, id)
	if err != nil {
		return err
	}
	if perm.Type != model.RBACPermTypeCustom {
		return fmt.Errorf("cannot delete built-in permission")
	}
	if has, _ := s.repo.PermissionHasRoleLinks(ctx, id); has {
		return fmt.Errorf("permission has role associations, cannot delete")
	}
	return s.repo.DeletePermission(ctx, id)
}

// ── Role-Permission Association ──────────────────────────────────────

func (s *Service) ListRolePermissions(ctx context.Context, roleID string, page, pageSize int) ([]model.RBACPermission, int64, error) {
	skip := int64((page - 1) * pageSize)
	return s.repo.ListRolePermissions(ctx, roleID, skip, int64(pageSize))
}

func (s *Service) AddRolePermission(ctx context.Context, roleID, permissionID string) error {
	role, err := s.repo.GetRole(ctx, roleID)
	if err != nil {
		return err
	}
	if role.PermissionCount >= maxCount {
		return fmt.Errorf("role has reached max permission count (%d)", maxCount)
	}
	return s.repo.AddRolePermission(ctx, roleID, permissionID)
}

func (s *Service) RemoveRolePermission(ctx context.Context, roleID, permissionID string) error {
	role, err := s.repo.GetRole(ctx, roleID)
	if err != nil {
		return err
	}
	perm, err := s.repo.GetPermission(ctx, permissionID)
	if err != nil {
		return err
	}
	// Prevent removing builtin permissions from system_admin or admin builtin roles.
	if perm != nil && perm.Type == model.RBACPermTypeBuiltin &&
		(role.Name == "system_admin_role" || role.Name == "admin_role") {
		return fmt.Errorf("cannot remove builtin permission from builtin role")
	}
	return s.repo.RemoveRolePermission(ctx, roleID, permissionID)
}

func (s *Service) GetEffectivePermissions(ctx context.Context, roleID string) ([]string, error) {
	allIDs, err := s.repo.GetAllDescendantRoleIDs(ctx, []string{roleID})
	if err != nil {
		return nil, err
	}
	return s.repo.GetEffectivePermissionKeys(ctx, allIDs)
}

// ── User-Role Association ────────────────────────────────────────────

func (s *Service) ListUserRoles(ctx context.Context, userID string, page, pageSize int) ([]model.RBACRole, int64, error) {
	skip := int64((page - 1) * pageSize)
	return s.repo.ListUserRoles(ctx, userID, skip, int64(pageSize))
}

func (s *Service) AddUserRole(ctx context.Context, userID, roleID string) error {
	ids, err := s.repo.GetUserRoleIDs(ctx, userID)
	if err != nil {
		return err
	}
	if len(ids) >= maxCount {
		return fmt.Errorf("user has reached max role count (%d)", maxCount)
	}
	return s.repo.AddUserRole(ctx, userID, roleID)
}

func (s *Service) RemoveUserRole(ctx context.Context, userID, roleID string) error {
	// Only reject removing system_admin_role from actual system_admin users.
	if roleID == "rbac_role_system_admin" {
		user, err := s.userRepo.FindByID(ctx, userID)
		if err != nil {
			return fmt.Errorf("find user: %w", err)
		}
		if user != nil && user.Role == model.RoleSystemAdmin {
			return fmt.Errorf("cannot remove system_admin_role from system_admin user")
		}
	}
	return s.repo.RemoveUserRole(ctx, userID, roleID)
}

// UserHasRoleLinks checks if user has RBAC roles (for delete safety).
func (s *Service) UserHasRoleLinks(ctx context.Context, userID string) (bool, error) {
	return s.repo.UserHasRoleLinks(ctx, userID)
}
