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

// Recorder feeds token/call counters into the unified metrics component and,
// when a SessionStatStore is attached, double-writes the session dimension.
type Recorder struct {
	counter      metrics.Counter
	sessionStats SessionStatStore
}

// NewRecorder creates a Recorder that increments the metrics Counter. A nil
// counter makes Record a no-op (defensive; call sites ignore the error).
func NewRecorder(counter metrics.Counter) *Recorder {
	return &Recorder{counter: counter}
}

// SetSessionStats attaches the session-scoped counter store. A nil store (or
// leaving it unset) keeps session accounting a no-op while global accounting
// still runs.
func (r *Recorder) SetSessionStats(store SessionStatStore) *Recorder {
	r.sessionStats = store
	return r
}

// Record increments token_tokens (billed) and llm_calls for one LLM call, and
// — when SessionID is non-empty and a session store is attached — accumulates
// the same call into session_stats. It never returns an error — the counter is
// buffered and failures are swallowed downstream (statistical accounting).
func (r *Recorder) Record(ctx context.Context, rec Record) error {
	if rec.CreatedAt.IsZero() {
		rec.CreatedAt = time.Now()
	}
	billed := rec.PromptTokens + rec.CompletionTokens
	if rec.Multiplier > 0 {
		billed = int(float64(billed) * rec.Multiplier)
	}
	at := rec.CreatedAt
	if r.counter != nil {
		_ = r.counter.Incr(ctx, metrics.MetricTokenTokens, at, int64(billed))
		_ = r.counter.Incr(ctx, metrics.MetricLLMCalls, at, 1)
	}
	// Session dimension (SPEC-100): only when the call carries a session and a
	// store is wired; empty SessionID means no session context (defensive).
	if r.sessionStats != nil && rec.SessionID != "" {
		_ = r.sessionStats.Incr(ctx, rec.SessionID, rec.PromptTokens, rec.CompletionTokens, int64(billed), at)
	}
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
