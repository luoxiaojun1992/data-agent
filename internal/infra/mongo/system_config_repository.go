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

// SystemConfigRepository handles system configuration data.
type SystemConfigRepository struct {
	coll *mongo.Collection
}

// NewSystemConfigRepository creates a new SystemConfigRepository.
func NewSystemConfigRepository(db *mongo.Database) *SystemConfigRepository {
	return &SystemConfigRepository{coll: db.Collection(model.CollSystemConfigs)}
}

// Get retrieves a config value by namespace and key.
func (r *SystemConfigRepository) Get(ctx context.Context, namespace, key string) (*model.SystemConfig, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"namespace": namespace, "key": key}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("get config: %w", err)
	}
	return docToSystemConfig(d), nil
}

// GetAll returns all configs in a namespace.
func (r *SystemConfigRepository) GetAll(ctx context.Context, namespace string) ([]model.SystemConfig, error) {
	cursor, err := r.coll.Find(ctx, bson.M{"namespace": namespace})
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

// Upsert creates or updates a config value.
func (r *SystemConfigRepository) Upsert(ctx context.Context, namespace, key, value string) error {
	filter := bson.M{"namespace": namespace, "key": key}
	update := bson.M{"$set": bson.M{"value": value, "updated_at": time.Now()}}
	opts := options.Update().SetUpsert(true)

	_, err := r.coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		return fmt.Errorf("upsert config: %w", err)
	}
	return nil
}
