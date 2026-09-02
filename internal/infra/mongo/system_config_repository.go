package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

// SystemConfigRepository handles system configuration data.
//
// Schema:
//   - `_id`           : UUID (string) — primary key
//   - `key`           : business key (string) — unique index
//   - `value`         : current user value (string)
//   - `description`   : human-readable label (string)
//   - `updated_at`    : last write timestamp
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

// Upsert writes the (key, value, description) document. The `_id` is a UUID:
// when the key already exists we reuse its existing _id; otherwise we mint a
// fresh UUID. Writes use the `_id` as the filter so the primary key drives
// updates.
func (r *SystemConfigRepository) Upsert(ctx context.Context, key, value, description string) error {
	id, err := r.resolveID(ctx, key)
	if err != nil {
		return err
	}
	_, err = r.coll.UpdateOne(ctx,
		bson.M{"_id": id},
		bson.M{"$set": bson.M{
			"key":         key,
			"value":       value,
			"description": description,
			"updated_at":  time.Now(),
		}},
		options.Update().SetUpsert(true),
	)
	if err != nil {
		return fmt.Errorf("upsert config: %w", err)
	}
	return nil
}

// resolveID returns the existing _id for the given key, or a freshly minted
// UUID when the key is not yet persisted.
func (r *SystemConfigRepository) resolveID(ctx context.Context, key string) (string, error) {
	existing, err := r.Get(ctx, key)
	if err != nil {
		return "", err
	}
	if existing != nil && existing.ID != "" {
		return existing.ID, nil
	}
	return uuid.NewString(), nil
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
