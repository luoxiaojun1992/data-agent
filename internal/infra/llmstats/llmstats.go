// Package llmstats provides unified LLM token recording for all LLM call
// points in the system. SPEC-072: recording now feeds the unified metrics
// Counter (token_tokens + llm_calls hourly counts) instead of writing the
// legacy llm_usage detail collection.
package llmstats

import (
	"context"
	"crypto/sha256"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
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

// Recorder feeds token/call counters into the unified metrics component.
type Recorder struct {
	counter metrics.Counter
}

// NewRecorder creates a Recorder that increments the metrics Counter. A nil
// counter makes Record a no-op (defensive; call sites ignore the error).
func NewRecorder(counter metrics.Counter) *Recorder {
	return &Recorder{counter: counter}
}

// Record increments token_tokens (billed) and llm_calls for one LLM call.
// It never returns an error — the counter is buffered and failures are
// swallowed downstream (statistical accounting).
func (r *Recorder) Record(ctx context.Context, rec Record) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	billed := rec.PromptTokens + rec.CompletionTokens
	if rec.Multiplier > 0 {
		billed = int(float64(billed) * rec.Multiplier)
	}
	if r.counter == nil {
		return nil
	}
	at := rec.CreatedAt
	_ = r.counter.Incr(ctx, metrics.MetricTokenTokens, at, int64(billed))
	_ = r.counter.Incr(ctx, metrics.MetricLLMCalls, at, 1)
	return nil
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
