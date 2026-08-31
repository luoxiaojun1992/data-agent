package llmcache

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
)

func TestCacheKey(t *testing.T) {
	k1 := embedKey("m", "hello")
	k2 := embedKey("m", "hello")
	if k1 != k2 {
		t.Error("deterministic embedKey")
	}
	if embedKey("m1", "hello") == embedKey("m2", "hello") {
		t.Error("different models produce different keys")
	}
	if enhanceKey("m", "hi") == enhanceKey("m", "hello") {
		t.Error("different inputs produce different keys")
	}
}

func TestMarshalEntry(t *testing.T) {
	entry := marshalEntry("test result")
	var ce CacheEntry
	if err := json.Unmarshal([]byte(entry), &ce); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if ce.Result != "test result" {
		t.Errorf("Result = %q", ce.Result)
	}
}

func TestNew(t *testing.T) {
	c := New(nil)
	if c == nil {
		t.Error("nil client should still return Cache")
	}
}

func TestSetEmbedding_TTL(t *testing.T) {
	mr := miniredis.RunT(t)
	c := New(redis.NewClient(&redis.Options{Addr: mr.Addr()}))
	ctx := context.Background()

	// No provider → default 1h.
	c.SetEmbedding(ctx, "m", "default", "v")
	if got := mr.TTL(embedKey("m", "default")); got != defaultEmbedTTL {
		t.Errorf("default TTL = %v, want %v", got, defaultEmbedTTL)
	}

	// Custom provider → exact TTL applied.
	c.SetEmbeddingTTLProvider(func(context.Context) time.Duration { return 30 * time.Second })
	c.SetEmbedding(ctx, "m", "custom", "v")
	if got := mr.TTL(embedKey("m", "custom")); got != 30*time.Second {
		t.Errorf("custom TTL = %v, want 30s", got)
	}

	// Provider returning 0 → fallback to default.
	c.SetEmbeddingTTLProvider(func(context.Context) time.Duration { return 0 })
	c.SetEmbedding(ctx, "m", "zero", "v")
	if got := mr.TTL(embedKey("m", "zero")); got != defaultEmbedTTL {
		t.Errorf("zero-provider TTL = %v, want default %v", got, defaultEmbedTTL)
	}
}
