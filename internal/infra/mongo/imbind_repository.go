package mongo

import (
	"context"
	"fmt"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// IMBindRepository implements repository.IMBindRepository backed by MongoDB.
type IMBindRepository struct {
	coll *mongo.Collection
}

// NewIMBindRepository creates a new IMBindRepository.
func NewIMBindRepository(db *mongo.Database) *IMBindRepository {
	return &IMBindRepository{coll: db.Collection("im_binds")}
}

// Get returns the IM binding record for the given user, or nil if none exists.
func (r *IMBindRepository) Get(ctx context.Context, userID string) (map[string]interface{}, error) {
	var result map[string]interface{}
	err := r.coll.FindOne(ctx, bson.M{"user_id": userID}).Decode(&result)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("get im bind: %w", err)
	}
	return result, nil
}

// Upsert creates or replaces the IM binding record for the given user.
func (r *IMBindRepository) Upsert(ctx context.Context, userID string, data map[string]interface{}) error {
	if data == nil {
		data = map[string]interface{}{}
	}
	data["user_id"] = userID
	_, err := r.coll.UpdateOne(ctx,
		bson.M{"user_id": userID},
		bson.M{"$set": data},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("upsert im bind: %w", err)
	}
	return nil
}

// Delete removes the IM binding record for the given user (idempotent).
func (r *IMBindRepository) Delete(ctx context.Context, userID string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"user_id": userID})
	if err != nil {
		return fmt.Errorf("delete im bind: %w", err)
	}
	return nil
}
