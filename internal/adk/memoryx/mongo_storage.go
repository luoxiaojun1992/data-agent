// Package memoryx provides the adk-go-memory integration layer, including
// a MongoDB-backed adapter.Storage and cosine-similarity merge logic.
package memoryx

import (
	"context"
	"errors"
	"strings"
	"time"

	"github.com/ieshan/adk-go-memory/adapter"
	"github.com/ieshan/idx"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// MongoStorage implements adapter.Storage backed by MongoDB.
type MongoStorage struct {
	coll    *mongo.Collection
	appName string
}

// NewMongoStorage creates a MongoStorage using the "memories" collection.
func NewMongoStorage(db *mongo.Database, appName string) *MongoStorage {
	return &MongoStorage{
		coll:    db.Collection("memories"),
		appName: appName,
	}
}

type mongoDoc struct {
	ID           idx.ID                   `bson:"_id"`
	Content      string                   `bson:"content"`
	Level        adapter.ObservationLevel `bson:"level"`
	SessionID    string                   `bson:"session_id"`
	UserID       string                   `bson:"user_id"`
	AppName      string                   `bson:"app_name"`
	Tags         []string                 `bson:"tags,omitempty"`
	TimesDerived int                      `bson:"times_derived"`
	CreatedAt    time.Time                `bson:"created_at"`
	Embedding    []float32                `bson:"embedding,omitempty"`
	MergedFrom   []idx.ID                 `bson:"merged_from,omitempty"`
	UpdatedAt    time.Time                `bson:"updated_at"`
}

func docFromObs(obs *adapter.Observation) mongoDoc {
	return mongoDoc{
		ID:           obs.ID,
		Content:      obs.Content,
		Level:        obs.Level,
		SessionID:    obs.SessionID,
		UserID:       obs.UserID,
		AppName:      obs.AppName,
		Tags:         obs.Tags,
		TimesDerived: obs.TimesDerived,
		CreatedAt:    obs.CreatedAt,
		Embedding:    obs.Embedding,
	}
}

func obsFromDoc(d mongoDoc) *adapter.Observation {
	return &adapter.Observation{
		ID:           d.ID,
		Content:      d.Content,
		Level:        d.Level,
		SessionID:    d.SessionID,
		UserID:       d.UserID,
		AppName:      d.AppName,
		Tags:         d.Tags,
		TimesDerived: d.TimesDerived,
		CreatedAt:    d.CreatedAt,
		Embedding:    d.Embedding,
	}
}

// Store saves (upserts) an observation by ID.
func (s *MongoStorage) Store(ctx context.Context, obs *adapter.Observation) error {
	now := time.Now()
	if obs.CreatedAt.IsZero() {
		obs.CreatedAt = now
	}
	doc := docFromObs(obs)
	doc.UpdatedAt = now

	_, err := s.coll.ReplaceOne(ctx,
		bson.M{"_id": obs.ID, "app_name": s.appName},
		doc,
		options.Replace().SetUpsert(true),
	)
	return err
}

// GetByID retrieves an observation by ID.
func (s *MongoStorage) GetByID(ctx context.Context, id idx.ID) (*adapter.Observation, error) {
	var doc mongoDoc
	err := s.coll.FindOne(ctx, bson.M{"_id": id, "app_name": s.appName}).Decode(&doc)
	if errors.Is(err, mongo.ErrNoDocuments) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return obsFromDoc(doc), nil
}

// Search performs vector search over embedding or falls back to keyword match.
func (s *MongoStorage) Search(ctx context.Context, opts *adapter.SearchOptions) ([]adapter.SearchResult, error) {
	filter := bson.M{"app_name": s.appName}
	if opts.UserID != "" {
		filter["user_id"] = opts.UserID
	}

	findOpts := options.Find()
	if opts.MaxResults > 0 {
		findOpts.SetLimit(int64(opts.MaxResults))
	}
	findOpts.SetSort(bson.D{{Key: "updated_at", Value: -1}})

	cur, err := s.coll.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var results []adapter.SearchResult
	for cur.Next(ctx) {
		var doc mongoDoc
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		obs := obsFromDoc(doc)
		score := computeScore(opts, obs)
		results = append(results, adapter.SearchResult{
			Observation: *obs,
			Score:       score,
			Source:      "keyword",
		})
	}
	if err := cur.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// computeScore calculates relevance by keyword match + level score.
func computeScore(opts *adapter.SearchOptions, obs *adapter.Observation) float64 {
	base := obs.Score()
	if opts.Query != "" && strings.Contains(strings.ToLower(obs.Content), strings.ToLower(opts.Query)) {
		base += 0.2
	}
	return base
}

// Forget deletes an observation by ID. Not-found is a no-op.
func (s *MongoStorage) Forget(ctx context.Context, id idx.ID) error {
	_, err := s.coll.DeleteOne(ctx, bson.M{"_id": id, "app_name": s.appName})
	return err
}

// Purge deletes observations matching the given filter.
func (s *MongoStorage) Purge(ctx context.Context, filter map[string]string) error {
	f := bson.M{"app_name": s.appName}
	for k, v := range filter {
		f[k] = v
	}
	_, err := s.coll.DeleteMany(ctx, f)
	return err
}

// IncrementTimesDerived increments the times_derived counter.
func (s *MongoStorage) IncrementTimesDerived(ctx context.Context, id idx.ID) error {
	_, err := s.coll.UpdateOne(ctx,
		bson.M{"_id": id, "app_name": s.appName},
		bson.M{"$inc": bson.M{"times_derived": 1}},
	)
	return err
}

// QueryMostDerived returns the most-derived observations (sorted by times_derived DESC).
func (s *MongoStorage) QueryMostDerived(ctx context.Context, sessionID, userID, appName string, limit int) ([]adapter.Observation, error) {
	return s.queryRanked(ctx, sessionID, userID, appName, limit, "times_derived")
}

// QueryRecent returns the most recent observations.
func (s *MongoStorage) QueryRecent(ctx context.Context, sessionID, userID, appName string, limit int) ([]adapter.Observation, error) {
	return s.queryRanked(ctx, sessionID, userID, appName, limit, "created_at")
}

func (s *MongoStorage) queryRanked(ctx context.Context, sessionID, userID, appName string, limit int, sortField string) ([]adapter.Observation, error) {
	filter := bson.M{"app_name": s.appName}
	if userID != "" {
		filter["user_id"] = userID
	}
	if sessionID != "" {
		filter["session_id"] = sessionID
	}

	findOpts := options.Find().SetSort(bson.D{{Key: sortField, Value: -1}})
	if limit > 0 {
		findOpts.SetLimit(int64(limit))
	}

	cur, err := s.coll.Find(ctx, filter, findOpts)
	if err != nil {
		return nil, err
	}
	defer cur.Close(ctx)

	var obs []adapter.Observation
	for cur.Next(ctx) {
		var doc mongoDoc
		if err := cur.Decode(&doc); err != nil {
			continue
		}
		obs = append(obs, *obsFromDoc(doc))
	}
	return obs, cur.Err()
}

// Close is a no-op for MongoDB (connection pool managed externally).
func (s *MongoStorage) Close() error { return nil }
