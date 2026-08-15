package mongo

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// UserRepository handles user data access.
type UserRepository struct {
	coll *mongo.Collection
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db *mongo.Database) *UserRepository {
	return &UserRepository{coll: db.Collection(model.CollUsers)}
}

// Create inserts a new user.
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
	user.ID = NewDomainID()
	user.CreatedAt = time.Now()
	user.UpdatedAt = time.Now()
	_, err := r.coll.InsertOne(ctx, userToDoc(user))
	if err != nil {
		return fmt.Errorf("create user: %w", err)
	}
	return nil
}

// FindByUsername looks up a user by username.
func (r *UserRepository) FindByUsername(ctx context.Context, username string) (*model.User, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"username": username}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("find user by username: %w", err)
	}
	return docToUser(d), nil
}

// FindByID looks up a user by ID.
func (r *UserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("find user by id: %w", err)
	}
	return docToUser(d), nil
}

// FindByIDs looks up users by a batch of IDs (single $in query).
func (r *UserRepository) FindByIDs(ctx context.Context, ids []string) ([]model.User, error) {
	if len(ids) == 0 {
		return nil, nil
	}
	cursor, err := r.coll.Find(ctx, bson.M{"_id": bson.M{"$in": ids}})
	if err != nil {
		return nil, fmt.Errorf("find users by ids: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode users by ids: %w", err)
	}
	users := make([]model.User, len(docs))
	for i, d := range docs {
		users[i] = *docToUser(d)
	}
	return users, nil
}

// SearchByEmail finds up to `limit` users whose username (the login email)
// contains the given substring, case-insensitively.
func (r *UserRepository) SearchByEmail(ctx context.Context, email string, limit int) ([]model.User, error) {
	if email == "" {
		return nil, nil
	}
	if limit <= 0 {
		limit = 10
	}
	filter := bson.M{
		"username": bson.M{"$regex": regexp.QuoteMeta(email), "$options": "i"},
	}
	opts := options.Find().SetLimit(int64(limit)).SetSort(bson.D{{Key: "created_at", Value: 1}})
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, fmt.Errorf("search users by email: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode users by email: %w", err)
	}
	users := make([]model.User, len(docs))
	for i, d := range docs {
		users[i] = *docToUser(d)
	}
	return users, nil
}

// HasSystemAdmin checks if a system_admin user exists.
func (r *UserRepository) HasSystemAdmin(ctx context.Context) (bool, error) {
	count, err := r.coll.CountDocuments(ctx, bson.M{"role": model.RoleSystemAdmin})
	if err != nil {
		return false, fmt.Errorf("count system admin: %w", err)
	}
	return count > 0, nil
}

// UpdatePassword updates a user's password hash and marks password as changed.
func (r *UserRepository) UpdatePassword(ctx context.Context, userID string, passwordHash string) error {
	_, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{
			"$set": bson.M{
				"password_hash":    passwordHash,
				"password_changed": true,
				"updated_at":       time.Now(),
			},
		},
	)
	if err != nil {
		return fmt.Errorf("update password: %w", err)
	}
	return nil
}

// UpdateRole updates a user's role.
func (r *UserRepository) UpdateRole(ctx context.Context, userID string, newRole model.UserRole) error {
	result, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"role": newRole, "updated_at": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("update user role: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// UpdateStatus enables or disables a user.
func (r *UserRepository) UpdateStatus(ctx context.Context, userID string, status model.UserStatus) error {
	result, err := r.coll.UpdateOne(ctx,
		bson.M{"_id": userID},
		bson.M{"$set": bson.M{"status": status, "updated_at": time.Now()}},
	)
	if err != nil {
		return fmt.Errorf("update user status: %w", err)
	}
	if result.MatchedCount == 0 {
		return fmt.Errorf("user not found")
	}
	return nil
}

// Delete removes a user by ID.
func (r *UserRepository) Delete(ctx context.Context, userID string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": userID})
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	return nil
}

// List returns paginated users. Admins can only see non-system_admin users.
func (r *UserRepository) List(ctx context.Context, role string, skip, limit int64) ([]model.User, int64, error) {
	return r.ListSorted(ctx, role, skip, limit, "", "")
}

func (r *UserRepository) ListSorted(ctx context.Context, role string, skip, limit int64, sortBy, sortOrder string) ([]model.User, int64, error) {
	filter := bson.M{}
	if role == "admin" {
		// Admins cannot see system_admin users
		filter["role"] = bson.M{"$ne": model.RoleSystemAdmin}
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count users: %w", err)
	}

	opts := options.Find()
	if skip > 0 {
		opts.SetSkip(skip)
	}
	if limit > 0 {
		opts.SetLimit(limit)
	}
	if sortBy != "" {
		order := -1 // desc
		if sortOrder == "asc" {
			order = 1
		}
		opts.SetSort(bson.D{{Key: sortBy, Value: order}})
	}

	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list users: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, fmt.Errorf("decode users: %w", err)
	}

	users := make([]model.User, len(docs))
	for i, d := range docs {
		users[i] = *docToUser(d)
	}

	return users, total, nil
}
