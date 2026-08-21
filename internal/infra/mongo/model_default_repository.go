package mongo

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/modelconfig"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// ModelDefaultRepository maps each use case to its default model. A unique
// index on use_case guarantees at most one default per use case.
type ModelDefaultRepository struct {
	coll *mongo.Collection
}

// NewModelDefaultRepository creates a new ModelDefaultRepository.
func NewModelDefaultRepository(db *mongo.Database) *ModelDefaultRepository {
	return &ModelDefaultRepository{coll: db.Collection(modelconfig.CollModelDefaults)}
}

// List returns all default records.
func (r *ModelDefaultRepository) List(ctx context.Context) ([]modelconfig.ModelDefault, error) {
	cursor, err := r.coll.Find(ctx, bson.M{})
	if err != nil {
		return nil, fmt.Errorf("list model defaults: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, fmt.Errorf("decode model defaults: %w", err)
	}
	out := make([]modelconfig.ModelDefault, len(docs))
	for i, d := range docs {
		out[i] = docToModelDefault(d)
	}
	return out, nil
}

// Get returns the default for a use case, or nil when not found.
func (r *ModelDefaultRepository) Get(ctx context.Context, useCase string) (*modelconfig.ModelDefault, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"use_case": useCase}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("get model default: %w", err)
	}
	md := docToModelDefault(d)
	return &md, nil
}

// Set sets the default model for a use case: deleteOne + insertOne. The
// unique index on use_case guarantees at most one default; concurrent Set
// may surface a duplicate-key error (E11000), which the caller surfaces to the
// user for retry (low-frequency operation, no auto-retry).
func (r *ModelDefaultRepository) Set(ctx context.Context, useCase, modelID string) error {
	if _, err := r.coll.DeleteOne(ctx, bson.M{"use_case": useCase}); err != nil {
		return fmt.Errorf("clear default for %q: %w", useCase, err)
	}
	doc := bson.M{
		"_id":        uuid.New().String(),
		"use_case":   useCase,
		"model_id":   modelID,
		"updated_at": time.Now(),
	}
	if _, err := r.coll.InsertOne(ctx, doc); err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("default for use case %q already set; please retry", useCase)
		}
		return fmt.Errorf("set default for %q: %w", useCase, err)
	}
	return nil
}

// Delete removes the default for a use case (cancel default). Idempotent.
func (r *ModelDefaultRepository) Delete(ctx context.Context, useCase string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"use_case": useCase})
	if err != nil {
		return fmt.Errorf("delete model default: %w", err)
	}
	return nil
}

func docToModelDefault(d bson.M) modelconfig.ModelDefault {
	return modelconfig.ModelDefault{
		ID:      getStr(d, "_id"),
		UseCase: getStr(d, "use_case"),
		ModelID: getStr(d, "model_id"),
	}
}
