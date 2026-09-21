// Package llmstats — session-scoped token accounting (SPEC-100).
//
// SessionStatStore accumulates per-session LLM usage into a dedicated
// `session_stats` collection keyed by session_id, so chat/task pages can
// surface single-session token consumption with an O(1) point lookup. It
// deliberately does NOT store user_id: all lookups/cascades go through
// session_id, and ownership/RBAC is enforced upstream on the sessions table.
package llmstats

import (
	"context"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// SessionStatCollectionName is the MongoDB collection holding per-session
// token counters.
const SessionStatCollectionName = "session_stats"

// SessionStatStore is the session-scoped token counter abstraction. The
// recorder double-writes here (session dimension) alongside the global
// metrics Counter (stats_hourly); the session manager cascades DeleteBySession
// on hard delete.
type SessionStatStore interface {
	// Incr idempotently accumulates one LLM call's tokens into the session's
	// running totals (implemented as $inc + upsert; never overwrites the
	// first-write fields).
	Incr(ctx context.Context, sessionID string, promptTokens, completionTokens int, billedTokens int64, at time.Time) error
	// DeleteBySession idempotently removes the session's counter document.
	// Deleting a non-existent document (deletedCount=0, err==nil) is success.
	DeleteBySession(ctx context.Context, sessionID string) error
	// GetBySession returns the accumulated billed_tokens for a session; a
	// missing document returns (0, nil).
	GetBySession(ctx context.Context, sessionID string) (int64, error)
}

// sessionStatDoc mirrors one session_stats document (no user_id — SPEC-100 D3).
type sessionStatDoc struct {
	ID               string    `bson:"_id"`
	SessionID        string    `bson:"session_id"`
	BilledTokens     int64     `bson:"billed_tokens"`
	PromptTokens     int64     `bson:"prompt_tokens"`
	CompletionTokens int64     `bson:"completion_tokens"`
	LLMCalls         int64     `bson:"llm_calls"`
	CreatedAt        time.Time `bson:"created_at"`
	UpdatedAt        time.Time `bson:"updated_at"`
}

// MongoSessionStatStore is the MongoDB-backed SessionStatStore.
type MongoSessionStatStore struct {
	coll *mongo.Collection
}

// NewSessionStatStore creates a store backed by the session_stats collection.
func NewSessionStatStore(db *mongo.Database) *MongoSessionStatStore {
	return &MongoSessionStatStore{coll: db.Collection(SessionStatCollectionName)}
}

// ensure MongoSessionStatStore satisfies SessionStatStore.
var _ SessionStatStore = (*MongoSessionStatStore)(nil)

// Incr implements SessionStatStore.
func (s *MongoSessionStatStore) Incr(ctx context.Context, sessionID string, promptTokens, completionTokens int, billedTokens int64, at time.Time) error {
	filter := bson.M{"_id": sessionID}
	update := bson.M{
		"$inc": bson.M{
			"billed_tokens":     billedTokens,
			"prompt_tokens":     int64(promptTokens),
			"completion_tokens": int64(completionTokens),
			"llm_calls":         int64(1),
		},
		"$set": bson.M{"updated_at": at},
		"$setOnInsert": bson.M{
			"_id":        sessionID,
			"session_id": sessionID,
			"created_at": at,
		},
	}
	_, err := s.coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
	return err
}

// DeleteBySession implements SessionStatStore (idempotent).
func (s *MongoSessionStatStore) DeleteBySession(ctx context.Context, sessionID string) error {
	_, err := s.coll.DeleteOne(ctx, bson.M{"_id": sessionID})
	return err
}

// GetBySession implements SessionStatStore.
func (s *MongoSessionStatStore) GetBySession(ctx context.Context, sessionID string) (int64, error) {
	var doc sessionStatDoc
	err := s.coll.FindOne(ctx, bson.M{"_id": sessionID}).Decode(&doc)
	if err == mongo.ErrNoDocuments {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return doc.BilledTokens, nil
}
