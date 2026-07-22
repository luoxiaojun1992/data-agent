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

// AggregateResult is one row of the per-call_point token aggregation.
type AggregateResult struct {
	CallPoint        string `bson:"_id" json:"call_point"`
	Count            int64  `bson:"count" json:"count"`
	PromptTokens     int64  `bson:"prompt_tokens" json:"prompt_tokens"`
	CompletionTokens int64  `bson:"completion_tokens" json:"completion_tokens"`
}

// TimeBucketResult is one row of the time-bucketed token aggregation. The
// bucket boundary is aligned to bucketMs; TotalTokens sums prompt+completion.
type TimeBucketResult struct {
	BucketStart time.Time `bson:"_id" json:"bucket_start"`
	TotalTokens int64     `bson:"total_tokens" json:"total_tokens"`
}

// Aggregate groups token usage by call_point. An empty callPoint aggregates
// every call point; a zero since skips the time filter.
func (r *Recorder) Aggregate(ctx context.Context, callPoint string, since time.Time) ([]AggregateResult, error) {
	match := bson.D{}
	if callPoint != "" {
		match = append(match, bson.E{Key: "call_point", Value: callPoint})
	}
	if !since.IsZero() {
		match = append(match, bson.E{Key: "created_at", Value: bson.D{{Key: "$gte", Value: since}}})
	}
	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: "$call_point"},
			{Key: "count", Value: bson.D{{Key: "$sum", Value: 1}}},
			{Key: "prompt_tokens", Value: bson.D{{Key: "$sum", Value: "$prompt_tokens"}}},
			{Key: "completion_tokens", Value: bson.D{{Key: "$sum", Value: "$completion_tokens"}}},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var results []AggregateResult
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}

// AggregateByTime buckets token usage into time windows of bucketMs
// milliseconds, aligned to the bucket boundary. A zero since aggregates all
// records; bucketMs <= 0 defaults to one hour. Results are sorted ascending by
// bucket start.
func (r *Recorder) AggregateByTime(ctx context.Context, since time.Time, bucketMs int64) ([]TimeBucketResult, error) {
	if bucketMs <= 0 {
		bucketMs = int64(time.Hour / time.Millisecond)
	}
	match := bson.D{}
	if !since.IsZero() {
		match = append(match, bson.E{Key: "created_at", Value: bson.D{{Key: "$gte", Value: since}}})
	}
	// Bucket key = floor(created_at_ms / bucketMs) * bucketMs, converted back
	// to a Date so the result decodes into time.Time. Built as named stages so
	// the pipeline reads top-down instead of deeply nested composite literals.
	toLong := bson.D{{Key: "$toLong", Value: "$created_at"}}
	divideMs := bson.D{{Key: "$divide", Value: bson.A{toLong, bucketMs}}}
	floorDiv := bson.D{{Key: "$floor", Value: divideMs}}
	mulBucket := bson.D{{Key: "$multiply", Value: bson.A{floorDiv, bucketMs}}}
	bucketKey := bson.D{{Key: "$toDate", Value: mulBucket}}
	addTokens := bson.D{{Key: "$add", Value: bson.A{"$prompt_tokens", "$completion_tokens"}}}
	sumTokens := bson.D{{Key: "$sum", Value: addTokens}}

	pipeline := mongo.Pipeline{
		{{Key: "$match", Value: match}},
		{{Key: "$group", Value: bson.D{
			{Key: "_id", Value: bucketKey},
			{Key: "total_tokens", Value: sumTokens},
		}}},
		{{Key: "$sort", Value: bson.D{{Key: "_id", Value: 1}}}},
	}
	cur, err := r.coll.Aggregate(ctx, pipeline)
	if err != nil {
		return nil, err
	}
	var results []TimeBucketResult
	if err := cur.All(ctx, &results); err != nil {
		return nil, err
	}
	return results, nil
}
