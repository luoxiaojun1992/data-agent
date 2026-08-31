package llmstats

import (
	"context"
	"testing"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
)

func TestEstimateTokens(t *testing.T) {
	if EstimateTokens("") != 0 {
		t.Error("empty")
	}
	if EstimateTokens("abcd") != 1 {
		t.Error("4 chars = 1 token")
	}
	if EstimateTokens("abcde") != 2 {
		t.Error("5 chars = 2 tokens")
	}
	if EstimateTokens("hello world") > 10 {
		t.Error("reasonable")
	}
}

func TestCacheKey(t *testing.T) {
	k1 := CacheKey("emb", "m", "hello")
	k2 := CacheKey("emb", "m", "hello")
	if k1 != k2 {
		t.Error("deterministic")
	}
	if CacheKey("emb", "m", "hello") == CacheKey("emb", "m", "world") {
		t.Error("different inputs")
	}
}

// fakeCounter captures Incr calls for asserting metric/delta/at.
type fakeCounter struct {
	incrs []struct {
		m     metrics.Metric
		at    time.Time
		delta int64
	}
}

func (f *fakeCounter) Incr(_ context.Context, m metrics.Metric, at time.Time, delta int64) error {
	f.incrs = append(f.incrs, struct {
		m     metrics.Metric
		at    time.Time
		delta int64
	}{m, at, delta})
	return nil
}

func (f *fakeCounter) Stop() {}

func TestRecorder_Record(t *testing.T) {
	fc := &fakeCounter{}
	r := NewRecorder(fc)
	at := time.Date(2026, 8, 31, 13, 20, 0, 0, time.UTC)
	err := r.Record(context.Background(), Record{
		CallPoint: "chat", Model: "gpt-4",
		PromptTokens: 100, CompletionTokens: 50, Multiplier: 2.0,
		CreatedAt: at,
	})
	if err != nil {
		t.Fatalf("Record: %v", err)
	}
	if len(fc.incrs) != 2 {
		t.Fatalf("incr count = %d, want 2 (token + call)", len(fc.incrs))
	}
	if fc.incrs[0].m != metrics.MetricTokenTokens || fc.incrs[0].delta != 300 {
		t.Errorf("token incr = %s/%d, want token_tokens/300", fc.incrs[0].m, fc.incrs[0].delta)
	}
	if fc.incrs[1].m != metrics.MetricLLMCalls || fc.incrs[1].delta != 1 {
		t.Errorf("call incr = %s/%d, want llm_calls/1", fc.incrs[1].m, fc.incrs[1].delta)
	}
	if !fc.incrs[0].at.Equal(at) {
		t.Errorf("token incr at = %v, want %v", fc.incrs[0].at, at)
	}
}

func TestRecorder_ZeroMultiplier(t *testing.T) {
	fc := &fakeCounter{}
	r := NewRecorder(fc)
	_ = r.Record(context.Background(), Record{PromptTokens: 10, CompletionTokens: 5, Multiplier: 0})
	// Multiplier 0 → billed = prompt + completion = 15 (no multiplier).
	if fc.incrs[0].delta != 15 {
		t.Errorf("billed = %d, want 15", fc.incrs[0].delta)
	}
}

func TestRecorder_NilCounter(t *testing.T) {
	r := NewRecorder(nil)
	if err := r.Record(context.Background(), Record{PromptTokens: 1}); err != nil {
		t.Fatalf("nil counter should be a no-op success, got %v", err)
	}
}

func TestRecorder_DefaultCreatedAt(t *testing.T) {
	fc := &fakeCounter{}
	r := NewRecorder(fc)
	_ = r.Record(context.Background(), Record{PromptTokens: 2})
	if fc.incrs[0].at.IsZero() {
		t.Error("CreatedAt should default to now")
	}
}
