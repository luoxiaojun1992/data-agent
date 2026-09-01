package mongo

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/modelconfig"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// ModelConfigRepository handles model configuration documents (one per model).
type ModelConfigRepository struct {
	coll *mongo.Collection
}

// NewModelConfigRepository creates a new ModelConfigRepository.
func NewModelConfigRepository(db *mongo.Database) *ModelConfigRepository {
	return &ModelConfigRepository{coll: db.Collection(modelconfig.CollModelConfigs)}
}

// List returns a page of models of the given type plus the total count (DB
// pagination, replaces the old in-memory slicing).
func (r *ModelConfigRepository) List(ctx context.Context, t modelconfig.ModelType, skip, limit int64) ([]modelconfig.ModelEntry, int64, error) {
	filter := bson.M{}
	if t != "" {
		filter["type"] = string(t)
	}
	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count model configs: %w", err)
	}
	opts := options.Find().SetSkip(skip).SetLimit(limit).SetSort(bson.D{{Key: "fallback_order", Value: 1}})
	cursor, err := r.coll.Find(ctx, filter, opts)
	if err != nil {
		return nil, 0, fmt.Errorf("list model configs: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, fmt.Errorf("decode model configs: %w", err)
	}
	entries := make([]modelconfig.ModelEntry, len(docs))
	for i, d := range docs {
		entries[i] = docToModelEntry(d)
	}
	return entries, total, nil
}

// Search returns top-N models of the given type matching q. Models whose IDs
// are in defaultIDs are sorted first (via a computed _defaultOrder field),
// then by name ascending. All filtering ($match), sorting ($sort), and
// truncation ($skip/$limit) happen in MongoDB — never in Go memory (SPEC-074).
// The search term is escaped with regexp.QuoteMeta to prevent regex injection.
func (r *ModelConfigRepository) Search(ctx context.Context, t modelconfig.ModelType, q string, defaultIDs []string, skip, limit int64) ([]modelconfig.ModelEntry, int64, error) {
	filter := bson.M{}
	if t != "" {
		filter["type"] = string(t)
	}
	if q != "" {
		re := bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}
		filter["$or"] = []bson.M{
			{"name": re},
			{"display_name": re},
		}
	}

	total, err := r.coll.CountDocuments(ctx, filter)
	if err != nil {
		return nil, 0, fmt.Errorf("count model configs: %w", err)
	}

	// _defaultOrder: 0 when the model is a default (in defaultIDs), else 1.
	var defaultOrder any = 1
	if len(defaultIDs) > 0 {
		defaultOrder = bson.M{"$cond": bson.A{
			bson.M{"$in": bson.A{"$_id", defaultIDs}},
			0,
			1,
		}}
	}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: filter}},
		{{Key: "$addFields", Value: bson.M{"_defaultOrder": defaultOrder}}},
		{{Key: "$sort", Value: bson.D{{Key: "_defaultOrder", Value: 1}, {Key: "name", Value: 1}}}},
		{{Key: "$skip", Value: skip}},
		{{Key: "$limit", Value: limit}},
	}
	cursor, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, 0, fmt.Errorf("search model configs: %w", err)
	}
	defer cursor.Close(ctx)

	var docs []bson.M
	if err := cursor.All(ctx, &docs); err != nil {
		return nil, 0, fmt.Errorf("decode model configs: %w", err)
	}
	entries := make([]modelconfig.ModelEntry, len(docs))
	for i, d := range docs {
		entries[i] = docToModelEntry(d)
	}
	return entries, total, nil
}

// Get returns the model entry by ID, or nil when not found.
func (r *ModelConfigRepository) Get(ctx context.Context, id string) (*modelconfig.ModelEntry, error) {
	var d bson.M
	err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
	if err != nil {
		if err == mongo.ErrNoDocuments {
			return nil, nil
		}
		return nil, fmt.Errorf("get model config: %w", err)
	}
	e := docToModelEntry(d)
	return &e, nil
}

// Insert inserts a single model document (atomic).
func (r *ModelConfigRepository) Insert(ctx context.Context, entry modelconfig.ModelEntry) error {
	doc := modelEntryToDoc(entry)
	doc["_id"] = entry.ID
	_, err := r.coll.InsertOne(ctx, doc)
	if err != nil {
		if mongo.IsDuplicateKeyError(err) {
			return fmt.Errorf("model ID %q already exists", entry.ID)
		}
		return fmt.Errorf("insert model config: %w", err)
	}
	return nil
}

// Update replaces a single model document by ID (atomic).
func (r *ModelConfigRepository) Update(ctx context.Context, id string, entry modelconfig.ModelEntry) error {
	doc := modelEntryToDoc(entry)
	doc["_id"] = id
	res, err := r.coll.ReplaceOne(ctx, bson.M{"_id": id}, doc)
	if err != nil {
		return fmt.Errorf("update model config: %w", err)
	}
	if res.MatchedCount == 0 {
		return fmt.Errorf("model %q not found", id)
	}
	return nil
}

// Delete removes a model document by ID. Idempotent (delete never 404s).
func (r *ModelConfigRepository) Delete(ctx context.Context, id string) error {
	_, err := r.coll.DeleteOne(ctx, bson.M{"_id": id})
	if err != nil {
		return fmt.Errorf("delete model config: %w", err)
	}
	return nil
}

// ---- bson conversion (converter rule: keep in sync with ModelEntry tags) ----

func modelEntryToDoc(m modelconfig.ModelEntry) bson.M {
	return bson.M{
		"name":             m.Name,
		"base_url":         m.BaseURL,
		"api_key":          m.APIKey,
		"type":             string(m.Type),
		"instruction":      m.Instruction,
		"capability":       m.Capability,
		"use_cases":        m.UseCases,
		"token_multiplier": m.TokenMultiplier,
		"temperature":      m.Temperature,
		"max_tokens":       m.MaxTokens,
		"context_len":      m.ContextLen,
		"fallback_order":   m.FallbackOrder,
		"embedding_dim":    m.EmbeddingDim,
		"updated_at":       time.Now(),
	}
}

func docToModelEntry(d bson.M) modelconfig.ModelEntry {
	return modelconfig.ModelEntry{
		ID:              getStr(d, "_id"),
		Name:            getStr(d, "name"),
		BaseURL:         getStr(d, "base_url"),
		APIKey:          getStr(d, "api_key"),
		Type:            modelconfig.ModelType(getStr(d, "type")),
		Instruction:     getStr(d, "instruction"),
		Capability:      getStr(d, "capability"),
		UseCases:        getStrSlice(d, "use_cases"),
		TokenMultiplier: getFloat(d, "token_multiplier"),
		Temperature:     getFloat(d, "temperature"),
		MaxTokens:       getInt(d, "max_tokens"),
		ContextLen:      getInt(d, "context_len"),
		FallbackOrder:   getInt(d, "fallback_order"),
		EmbeddingDim:    getInt(d, "embedding_dim"),
	}
}
