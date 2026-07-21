package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// RoleRepository handles role data access.
type RoleRepository struct {
	coll *mongo.Collection
}

// NewRoleRepository creates a new RoleRepository.
func NewRoleRepository(db *mongo.Database) *RoleRepository {
	return &RoleRepository{coll: db.Collection(model.CollRoles)}
}

// Create inserts a new role.
func (r *RoleRepository) Create(ctx context.Context, role *model.Role) error {
	role.ID = NewDomainID()
	role.CreatedAt = time.Now()
	role.UpdatedAt = time.Now()
	_, err := r.coll.InsertOne(ctx, roleToDoc(role))
	if err != nil {
		return fmt.Errorf("create role: %w", err)
	}
	return nil
}

// List returns all roles. Fixed roles are auto-generated, custom roles from DB.
func (r *RoleRepository) List(ctx context.Context) ([]model.Role, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("list roles: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode roles: %w", err)
	}

	roles := make([]model.Role, len(docs))
	for i, d := range docs {
		roles[i] = *docToRole(d)
	}

	return roles, nil
}

// FindByID looks up a role by ID.
func (r *RoleRepository) FindByID(ctx context.Context, id string) (*model.Role, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("find role by id: %w", err)
	}
	return docToRole(d), nil
}

// Update updates a role's permissions.
func (r *RoleRepository) Update(ctx context.Context, roleID string, permissions []string) error {
	result, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": roleID},
		bson.M{"$set": bson.M{"permissions": permissions, "updated_at": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("update role: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("role not found")
	}
	return nil
}

// Delete removes a custom role. Fixed roles cannot be deleted via repository.
func (r *RoleRepository) Delete(ctx context.Context, roleID string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": roleID, "type": "custom"})
	if err != nil {
		return fmt.Errorf("delete role: %w", err)
	}
	return nil
}
