package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SystemConfigRepository handles system configuration data (namespace removed —
// each key is a standalone document).
type SystemConfigRepository struct {
	coll *mongo.Collection
}

// NewSystemConfigRepository creates a new SystemConfigRepository.
func NewSystemConfigRepository(db *mongo.Database) *SystemConfigRepository {
	return &SystemConfigRepository{coll: db.Collection(model.CollSystemConfigs)}
}

// Get retrieves a config value by key.
func (r *SystemConfigRepository) Get(ctx context.Context, key string) (*model.SystemConfig, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"key": key}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("get config: %w", err)
	}
	return docToSystemConfig(d), nil
}

// GetAll returns all configs.
func (r *SystemConfigRepository) GetAll(ctx context.Context) ([]model.SystemConfig, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("list configs: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode configs: %w", err)
	}
	configs := make([]model.SystemConfig, len(docs))
	for i, d := range docs {
		configs[i] = *docToSystemConfig(d)
	}
	return configs, nil
}

// List returns a page of configs.
func (r *SystemConfigRepository) List(ctx context.Context, skip, limit int64) ([]model.SystemConfig, error) {
	opts := options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.D{{Key: "key", Value: 1}})
	cursor, err := r.coll.Find(ctx, bson.M{}, opts)
	if err != nil {
		return nil, fmt.Errorf("list configs: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode configs: %w", err)
	}
	configs := make([]model.SystemConfig, len(docs))
	for i, d := range docs {
		configs[i] = *docToSystemConfig(d)
	}
	return configs, nil
}

// Count returns the number of configs.
func (r *SystemConfigRepository) Count(ctx context.Context) (int64, error) {
	n, err := r.coll.CountDocuments(ctx, bson.M{})
	if err != nil {
		return 0, fmt.Errorf("count configs: %w", err)
	}
	return n, nil
}

// Upsert creates or updates a config value.
func (r *SystemConfigRepository) Upsert(ctx context.Context, key, value string) error {
	filter := bson.M{"key": key}
	update := bson.M{"$set": bson.M{"value": value, "updated_at": time.Now()}}
	opts := options.Update().SetUpsert(true)

	_, err := r.coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("upsert config: %w", err)
	}
	return nil
}

// Delete removes a config value by key. Idempotent: returns nil if the
// document does not exist (project convention: delete never 404s).
func (r *SystemConfigRepository) Delete(ctx context.Context, key string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"key": key})
	if err != nil {
		return fmt.Errorf("delete config: %w", err)
	}
	return nil
}
