// Package llmstats provides unified LLM token recording (MongoDB llm_usage
// collection + Redis counters) for all LLM call points in the system.
package llmstats

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
)

// Record represents one LLM invocation's token consumption.
type Record struct {
	CallPoint        string    `bson:"call_point"`
	Model            string    `bson:"model"`
	PromptTokens     int       `bson:"prompt_tokens"`
	CompletionTokens int       `bson:"completion_tokens"`
	Multiplier       float64   `bson:"multiplier"`
	BilledTokens     int       `bson:"billed_tokens"`
	Estimated        bool      `bson:"estimated"`
	UserID           string    `bson:"user_id,omitempty"`
	SessionID        string    `bson:"session_id,omitempty"`
	CacheHit         bool      `bson:"cache_hit"`
	CreatedAt        time.Time `bson:"created_at"`
}

// Recorder persists token usage to MongoDB with optional Redis counters.
type Recorder struct {
	coll *mongo.Collection
}

// NewRecorder creates a Recorder using the llm_usage collection.
func NewRecorder(db *mongo.Database) *Recorder {
	coll := db.Collection("llm_usage")
	// Ensure index for querying by time / user.
	_, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "created_at", Value: -1}},
	})
	_, _ = coll.Indexes().CreateOne(context.Background(), mongo.IndexModel{
		Keys: bson.D{{Key: "user_id", Value: 1}, {Key: "created_at", Value: -1}},
	})
	return &Recorder{coll: coll}
}

// Record writes one token usage record to MongoDB.
func (r *Recorder) Record(ctx context.Context, rec Record) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	rec.BilledTokens = int(float64(rec.PromptTokens+rec.CompletionTokens) * rec.Multiplier)
	if rec.Multiplier == 0 {
		rec.Multiplier = 1.0
		rec.BilledTokens = rec.PromptTokens + rec.CompletionTokens
	}
	_, err := r.coll.InsertOne(ctx, rec)
	return err
}

// EstimateTokens estimates token count from text length (4 chars ≈ 1 token).
func EstimateTokens(text string) int {
	if text == "" {
		return 0
	}
	return (len([]rune(text)) + 3) / 4
}

// CacheKey builds a deterministic cache key from content and prefix.
func CacheKey(prefix, model, content string) string {
	h := sha256.Sum256([]byte(content))
	return fmt.Sprintf("%s:%s:%x", prefix, model, h[:8])
}
