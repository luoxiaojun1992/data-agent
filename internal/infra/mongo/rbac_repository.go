package mongo

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

type RBACRepository struct {
	db *mongo.Database
}

func NewRBACRepository(db *mongo.Database) *RBACRepository {
	return &RBACRepository{db: db}
}

// ── Roles ────────────────────────────────────────────────────────────

func (r *RBACRepository) ListRoles(ctx context.Context, skip, limit int64, parentID, q, excludeUserID string) ([]model.RBACRole, int64, error) {
	coll := r.db.Collection("rbac_roles")
	filter := bson.M{}
	if parentID != "" {
		filter["parent_id"] = parentID
	}
	if q != "" {
		re := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		filter["$or"] = []bson.M{
			{"name": re},
			{"display_name": re},
		}
	}
	if excludeUserID != "" {
		roleIDs, err := r.GetUserRoleIDs(ctx, excludeUserID)
		if err != nil {
			return nil, 0, err
		}
		if len(roleIDs) > 0 {
			filter["_id"] = bson.M{"$nin": roleIDs}
		}
	}
	total, _ := coll.CountDocuments(ctx, filter)
	cur, err := coll.Find(ctx, filter, options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.M{"level": 1, "display_name": 1}))
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)
	var roles []model.RBACRole
	if err := cur.All(ctx, &roles); err != nil {
		return nil, 0, err
	}
	return roles, total, nil
}

func (r *RBACRepository) GetRole(ctx context.Context, id string) (*model.RBACRole, error) {
	var role model.RBACRole
	err := r.db.Collection("rbac_roles").FindOne(ctx, bson.M{"_id": id}).Decode(&role)
	if err != nil {
		return nil, err
	}
	return &role, nil
}

func (r *RBACRepository) CreateRole(ctx context.Context, role *model.RBACRole) error {
	role.ID = "rbac_role_" + uuid.New().String()[:8]
	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()
	if role.ParentID != "" {
		parent, err := r.GetRole(ctx, role.ParentID)
		if err != nil {
			return fmt.Errorf("parent role not found: %w", err)
		}
		role.Level = parent.Level + 1
		// increment parent's child_count
		r.db.Collection("rbac_roles").UpdateOne(ctx,
			bson.M{"_id": role.ParentID},
			bson.M{"$inc": bson.M{"child_count": 1}},
		)
	}
	if _, err := r.db.Collection("rbac_roles").InsertOne(ctx, role); err != nil {
		return err
	}
	return nil
}

func (r *RBACRepository) UpdateRole(ctx context.Context, id string, updates bson.M) error {
	updates["updated_at"] = time.Now()
	res, err := r.db.Collection("rbac_roles").UpdateOne(ctx, bson.M{"_id": id, "type": "custom"}, bson.M{"$set": updates})
	if err != nil {
		return err
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("role not found or not custom")
	}
	return nil
}

func (r *RBACRepository) DeleteRole(ctx context.Context, id string) error {
	res, err := r.db.Collection("rbac_roles").DeleteOne(ctx, bson.M{"_id": id, "type": "custom"})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return fmt.Errorf("role not found or not custom")
	}
	return nil
}

func (r *RBACRepository) GetRoleChildren(ctx context.Context, parentID string) ([]model.RBACRole, error) {
	cur, err := r.db.Collection("rbac_roles").Find(ctx, bson.M{"parent_id": parentID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var roles []model.RBACRole
	if err := cur.All(ctx, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

func (r *RBACRepository) ChangeRoleParent(ctx context.Context, id, newParentID string) error {
	// Atomically update parent_id and decrement old parent / increment new parent
	role, err := r.GetRole(ctx, id)
	if err != nil {
		return err
	}
	oldParentID := role.ParentID

	ses, err := r.db.Client().StartSession()
	if err != nil {
		return err
	}
	defer ses.EndSession(ctx)

	_, err = ses.WithTransaction(ctx, func(sc mongo.SessionContext) (interface{}, error) {
		if oldParentID != "" {
			r.db.Collection("rbac_roles").UpdateOne(sc, bson.M{"_id": oldParentID}, bson.M{"$inc": bson.M{"child_count": -1}})
		}
		if newParentID != "" {
			r.db.Collection("rbac_roles").UpdateOne(sc, bson.M{"_id": newParentID}, bson.M{"$inc": bson.M{"child_count": 1}})
		}
		_, err := r.db.Collection("rbac_roles").UpdateOne(sc,
			bson.M{"_id": id, "type": "custom"},
			bson.M{"$set": bson.M{"parent_id": newParentID, "updated_at": time.Now()}},
		)
		return nil, err
	})
	return err
}

func (r *RBACRepository) AvailableParents(ctx context.Context, level int, q string, skip, limit int64) ([]model.RBACRole, error) {
	// Parents must be exactly one level below the child (RBAC levels are
	// adjacent) and have spare child capacity. Optional q search on
	// name/display_name. Filtering/sorting/truncation all happen in DB.
	parentLevel := level - 1
	filter := bson.M{"level": parentLevel, "child_count": bson.M{"$lt": 10}}
	if q != "" {
		re := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		filter["$or"] = []bson.M{
			{"name": re},
			{"display_name": re},
		}
	}
	cur, err := r.db.Collection("rbac_roles").Find(ctx, filter,
		options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.M{"level": 1, "display_name": 1}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var roles []model.RBACRole
	if err := cur.All(ctx, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// ListParentCandidates returns roles eligible as the parent of a NEW role:
// any role below the max level (L0/L1) with spare child capacity. Used by the
// "新建角色" modal's parent dropdown. Filtering/sorting/truncation in DB.
func (r *RBACRepository) ListParentCandidates(ctx context.Context, q string, skip, limit int64) ([]model.RBACRole, error) {
	filter := bson.M{"level": bson.M{"$lt": model.MaxRoleLevel}, "child_count": bson.M{"$lt": 10}}
	if q != "" {
		re := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		filter["$or"] = []bson.M{
			{"name": re},
			{"display_name": re},
		}
	}
	cur, err := r.db.Collection("rbac_roles").Find(ctx, filter,
		options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.M{"level": 1, "display_name": 1}),
	)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var roles []model.RBACRole
	if err := cur.All(ctx, &roles); err != nil {
		return nil, err
	}
	return roles, nil
}

// ── Permissions ──────────────────────────────────────────────────────

func (r *RBACRepository) ListPermissions(ctx context.Context, skip, limit int64, q, excludeRoleID string) ([]model.RBACPermission, int64, error) {
	coll := r.db.Collection("rbac_permissions")
	filter := bson.M{}
	if q != "" {
		re := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		filter["$or"] = []bson.M{
			{"name": re},
			{"key": re},
		}
	}
	if excludeRoleID != "" {
		permIDs, err := r.GetRolePermissionIDs(ctx, excludeRoleID)
		if err != nil {
			return nil, 0, err
		}
		if len(permIDs) > 0 {
			filter["_id"] = bson.M{"$nin": permIDs}
		}
	}
	total, _ := coll.CountDocuments(ctx, filter)
	cur, err := coll.Find(ctx, filter, options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.M{"module": 1, "name": 1}))
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)
	var perms []model.RBACPermission
	if err := cur.All(ctx, &perms); err != nil {
		return nil, 0, err
	}
	return perms, total, nil
}

// GetRolePermissionIDs returns the permission IDs currently linked to a role.
func (r *RBACRepository) GetRolePermissionIDs(ctx context.Context, roleID string) ([]string, error) {
	cur, err := r.db.Collection("rbac_role_permissions").Find(ctx, bson.M{"role_id": roleID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)
	var links []model.RBACRolePermission
	if err := cur.All(ctx, &links); err != nil {
		return nil, err
	}
	ids := make([]string, len(links))
	for i, l := range links {
		ids[i] = l.PermissionID
	}
	return ids, nil
}

func (r *RBACRepository) CreatePermission(ctx context.Context, p *model.RBACPermission) error {
	_, err := r.db.Collection("rbac_permissions").InsertOne(ctx, p)
	return err
}

func (r *RBACRepository) GetPermission(ctx context.Context, id string) (*model.RBACPermission, error) {
	var p model.RBACPermission
	err := r.db.Collection("rbac_permissions").FindOne(ctx, bson.M{"_id": id}).Decode(&p)
	if err != nil {
		return nil, err
	}
	return &p, nil
}

func (r *RBACRepository) DeletePermission(ctx context.Context, id string) error {
	res, err := r.db.Collection("rbac_permissions").DeleteOne(ctx, bson.M{"_id": id, "type": "custom"})
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return fmt.Errorf("permission not found or not custom")
	}
	return nil
}

// ── Role-Permission Association ──────────────────────────────────────

func (r *RBACRepository) ListRolePermissions(ctx context.Context, roleID string, skip, limit int64) ([]model.RBACPermission, int64, error) {
	coll := r.db.Collection("rbac_role_permissions")
	filter := bson.M{"role_id": roleID}
	total, _ := coll.CountDocuments(ctx, filter)

	cur, err := coll.Find(ctx, filter, options.Find().SetSkip(skip).SetLimit(limit))
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	var links []model.RBACRolePermission
	if err := cur.All(ctx, &links); err != nil {
		return nil, 0, err
	}

	permIDs := make([]string, len(links))
	for i, l := range links {
		permIDs[i] = l.PermissionID
	}
	if len(permIDs) == 0 {
		return nil, 0, nil
	}

	permCur, err := r.db.Collection("rbac_permissions").Find(ctx, bson.M{"_id": bson.M{"$in": permIDs}})
	if err != nil {
		return nil, 0, err
	}
	defer permCur.Close(ctx)

	var perms []model.RBACPermission
	if err := permCur.All(ctx, &perms); err != nil {
		return nil, 0, err
	}
	return perms, total, nil
}

func (r *RBACRepository) AddRolePermission(ctx context.Context, roleID, permissionID string) error {
	link := &model.RBACRolePermission{
		ID:           "rbac_rp_" + uuid.New().String()[:8],
		RoleID:       roleID,
		PermissionID: permissionID,
		CreatedAt:    time.Now(),
	}
	if _, err := r.db.Collection("rbac_role_permissions").InsertOne(ctx, link); err != nil {
		return err
	}
	// atomic increment
	r.db.Collection("rbac_roles").UpdateOne(ctx,
		bson.M{"_id": roleID},
		bson.M{"$inc": bson.M{"permission_count": 1}},
	)
	return nil
}

func (r *RBACRepository) RemoveRolePermission(ctx context.Context, roleID, permissionID string) error {
	res, err := r.db.Collection("rbac_role_permissions").DeleteOne(ctx,
		bson.M{"role_id": roleID, "permission_id": permissionID},
	)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return fmt.Errorf("association not found")
	}
	// atomic decrement
	r.db.Collection("rbac_roles").UpdateOne(ctx,
		bson.M{"_id": roleID},
		bson.M{"$inc": bson.M{"permission_count": -1}},
	)
	return nil
}

// ── Permission Lookup ────────────────────────────────────────────────


// GetAllRoleIDsWithAncestors returns the given role IDs plus all ancestor role IDs
// traversed via parent_id. Used for perm inheritance: a role inherits perms
// from its parent chain (admin inherits from user, sysAdmin inherits from admin).
func (r *RBACRepository) GetAllRoleIDsWithAncestors(ctx context.Context, roleIDs []string) ([]string, error) {
	result := make(map[string]bool)
	for _, rid := range roleIDs {
		result[rid] = true
	}
	// Load all roles once
	allRoles := make(map[string]string) // id -> parent_id
	cur, err := r.db.Collection("rbac_roles").Find(ctx, bson.M{})
	if err != nil {
		return nil, err
	}
	var roles []model.RBACRole
	if err := cur.All(ctx, &roles); err != nil {
		return nil, err
	}
	for _, role := range roles {
		allRoles[role.ID] = role.ParentID
	}
	// BFS upwards
	queue := append([]string{}, roleIDs...)
	for len(queue) > 0 {
		rid := queue[0]
		queue = queue[1:]
		parent, ok := allRoles[rid]
		if !ok || parent == "" {
			continue
		}
		if !result[parent] {
			result[parent] = true
			queue = append(queue, parent)
		}
	}
	out := make([]string, 0, len(result))
	for id := range result {
		out = append(out, id)
	}
	return out, nil
}

func (r *RBACRepository) GetAllDescendantRoleIDs(ctx context.Context, roleIDs []string) ([]string, error) {
	result := make(map[string]bool)
	for _, rid := range roleIDs {
		result[rid] = true
	}
	queue := append([]string{}, roleIDs...)

	for len(queue) > 0 {
		rid := queue[0]
		queue = queue[1:]

		children, err := r.GetRoleChildren(ctx, rid)
		if err != nil {
			return nil, err
		}
		for _, child := range children {
			if !result[child.ID] {
				result[child.ID] = true
				queue = append(queue, child.ID)
			}
		}
	}

	ids := make([]string, 0, len(result))
	for id := range result {
		ids = append(ids, id)
	}
	return ids, nil
}

func (r *RBACRepository) RolesHavePermission(ctx context.Context, roleIDs []string, perm string) (bool, error) {
	// Find all rbac_role_permissions for these roles
	cur, err := r.db.Collection("rbac_role_permissions").Find(ctx, bson.M{"role_id": bson.M{"$in": roleIDs}})
	if err != nil {
		return false, err
	}
	defer cur.Close(ctx)

	var links []model.RBACRolePermission
	if err := cur.All(ctx, &links); err != nil {
		return false, err
	}

	permIDs := make([]string, len(links))
	for i, l := range links {
		permIDs[i] = l.PermissionID
	}
	if len(permIDs) == 0 {
		return false, nil
	}

	// Check if any of those permissions match the target key
	count, err := r.db.Collection("rbac_permissions").CountDocuments(ctx,
		bson.M{"_id": bson.M{"$in": permIDs}, "key": perm},
	)
	if err != nil {
		return false, err
	}
	return count > 0, nil
}

func (r *RBACRepository) GetEffectivePermissionKeys(ctx context.Context, roleIDs []string) ([]string, error) {
	cur, err := r.db.Collection("rbac_role_permissions").Find(ctx, bson.M{"role_id": bson.M{"$in": roleIDs}})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var links []model.RBACRolePermission
	if err := cur.All(ctx, &links); err != nil {
		return nil, err
	}

	permIDs := make([]string, len(links))
	for i, l := range links {
		permIDs[i] = l.PermissionID
	}
	if len(permIDs) == 0 {
		return nil, nil
	}

	permCur, err := r.db.Collection("rbac_permissions").Find(ctx, bson.M{"_id": bson.M{"$in": permIDs}})
	if err != nil {
		return nil, err
	}
	defer permCur.Close(ctx)

	var perms []model.RBACPermission
	if err := permCur.All(ctx, &perms); err != nil {
		return nil, err
	}

	keys := make([]string, len(perms))
	for i, p := range perms {
		keys[i] = p.Key
	}
	return keys, nil
}

// ── User-Role Association ────────────────────────────────────────────

func (r *RBACRepository) ListUserRoles(ctx context.Context, userID string, skip, limit int64) ([]model.RBACRole, int64, error) {
	coll := r.db.Collection("user_rbac_roles")
	filter := bson.M{"user_id": userID}
	total, _ := coll.CountDocuments(ctx, filter)

	cur, err := coll.Find(ctx, filter, options.Find().SetSkip(skip).SetLimit(limit))
	if err != nil {
		return nil, 0, err
	}
	defer cur.Close(ctx)

	var links []model.UserRBACRole
	if err := cur.All(ctx, &links); err != nil {
		return nil, 0, err
	}
	if len(links) == 0 {
		return nil, 0, nil
	}

	roleIDs := make([]string, len(links))
	for i, l := range links {
		roleIDs[i] = l.RoleID
	}

	roleCur, err := r.db.Collection("rbac_roles").Find(ctx, bson.M{"_id": bson.M{"$in": roleIDs}})
	if err != nil {
		return nil, 0, err
	}
	defer roleCur.Close(ctx)

	var roles []model.RBACRole
	if err := roleCur.All(ctx, &roles); err != nil {
		return nil, 0, err
	}
	return roles, total, nil
}

func (r *RBACRepository) GetUserRoleIDs(ctx context.Context, userID string) ([]string, error) {
	cur, err := r.db.Collection("user_rbac_roles").Find(ctx, bson.M{"user_id": userID})
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var links []model.UserRBACRole
	if err := cur.All(ctx, &links); err != nil {
		return nil, err
	}
	ids := make([]string, len(links))
	for i, l := range links {
		ids[i] = l.RoleID
	}
	return ids, nil
}

func (r *RBACRepository) AddUserRole(ctx context.Context, userID, roleID string) error {
	link := &model.UserRBACRole{
		ID:        "rbac_ur_" + uuid.New().String()[:8],
		UserID:    userID,
		RoleID:    roleID,
		CreatedAt: time.Now(),
	}
	if _, err := r.db.Collection("user_rbac_roles").InsertOne(ctx, link); err != nil {
		return err
	}
	r.db.Collection("users").UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$inc": bson.M{"rbac_role_count": 1}},
	)
	return nil
}

func (r *RBACRepository) RemoveUserRole(ctx context.Context, userID, roleID string) error {
	res, err := r.db.Collection("user_rbac_roles").DeleteOne(ctx,
		bson.M{"user_id": userID, "role_id": roleID},
	)
	if err != nil {
		return err
	}
	if res.DeletedCount == 0 {
		return fmt.Errorf("association not found")
	}
	r.db.Collection("users").UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$inc": bson.M{"rbac_role_count": -1}},
	)
	return nil
}

// ── Count helpers for delete safety ──────────────────────────────────

func (r *RBACRepository) RoleHasChildren(ctx context.Context, roleID string) (bool, error) {
	n, err := r.db.Collection("rbac_roles").CountDocuments(ctx, bson.M{"parent_id": roleID})
	return n > 0, err
}

func (r *RBACRepository) RoleHasPermissionLinks(ctx context.Context, roleID string) (bool, error) {
	n, err := r.db.Collection("rbac_role_permissions").CountDocuments(ctx, bson.M{"role_id": roleID})
	return n > 0, err
}

func (r *RBACRepository) RoleHasUserLinks(ctx context.Context, roleID string) (bool, error) {
	n, err := r.db.Collection("user_rbac_roles").CountDocuments(ctx, bson.M{"role_id": roleID})
	return n > 0, err
}

func (r *RBACRepository) PermissionHasRoleLinks(ctx context.Context, permID string) (bool, error) {
	n, err := r.db.Collection("rbac_role_permissions").CountDocuments(ctx, bson.M{"permission_id": permID})
	return n > 0, err
}

func (r *RBACRepository) UserHasRoleLinks(ctx context.Context, userID string) (bool, error) {
	n, err := r.db.Collection("user_rbac_roles").CountDocuments(ctx, bson.M{"user_id": userID})
	return n > 0, err
}
