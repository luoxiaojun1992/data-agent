// Package llmcache provides Redis caching for LLM embedding and prompt-enhance
// results, avoiding redundant LLM calls for identical inputs.
package llmcache

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// Cache wraps a Redis client for LLM result caching.
type Cache struct {
	client *redis.Client
	// embedTTL returns the current embedding-cache TTL; nil means the default
	// (defaultEmbedTTL). Injected by the wiring layer so the TTL reads a live
	// system-config value on every Set (hot-reload), keeping this infra package
	// decoupled from the config service.
	embedTTL func(ctx context.Context) time.Duration
}

// defaultEmbedTTL is the fallback TTL when no provider is set (or it returns a
// non-positive value). Embeddings are deterministic, so a modest TTL is enough
// to dedupe bursts while bounding Redis memory growth.
const defaultEmbedTTL = 1 * time.Hour

// New creates a Cache backed by the given Redis client.
func New(client *redis.Client) *Cache {
	return &Cache{client: client}
}

// SetEmbeddingTTLProvider injects a function returning the current embedding
// cache TTL. When nil (default), SetEmbedding uses defaultEmbedTTL.
func (c *Cache) SetEmbeddingTTLProvider(f func(ctx context.Context) time.Duration) {
	c.embedTTL = f
}

// CacheEntry is the serialized cached value.
type CacheEntry struct {
	Result string `json:"r"`
}

func embedKey(model, text string) string {
	h := sha256.Sum256([]byte(text))
	return fmt.Sprintf("emb:%s:%x", model, h[:8])
}

func enhanceKey(model, input string) string {
	h := sha256.Sum256([]byte(input))
	return fmt.Sprintf("enh:%s:%x", model, h[:8])
}

// GetEmbedding returns a cached embedding result, or (false, nil) on miss.
func (c *Cache) GetEmbedding(ctx context.Context, model, text string) (string, bool) {
	return c.get(ctx, embedKey(model, text))
}

// SetEmbedding caches an embedding result with a TTL (default 1h, configurable
// via SetEmbeddingTTLProvider). Bounds Redis memory growth.
func (c *Cache) SetEmbedding(ctx context.Context, model, text, result string) {
	ttl := defaultEmbedTTL
	if c.embedTTL != nil {
		if v := c.embedTTL(ctx); v > 0 {
			ttl = v
		}
	}
	_ = c.client.Set(ctx, embedKey(model, text), marshalEntry(result), ttl).Err()
}

// GetEnhance returns a cached enhance result, or (false, nil) on miss.
func (c *Cache) GetEnhance(ctx context.Context, model, input string) (string, bool) {
	return c.get(ctx, enhanceKey(model, input))
}

// SetEnhance caches an enhance result with 1-hour TTL.
func (c *Cache) SetEnhance(ctx context.Context, model, input, result string) {
	_ = c.client.Set(ctx, enhanceKey(model, input), marshalEntry(result), 1*time.Hour).Err()
}

func (c *Cache) get(ctx context.Context, key string) (string, bool) {
	val, err := c.client.Get(ctx, key).Result()
	if err != nil {
		return "", false
	}
	var entry CacheEntry
	if err := json.Unmarshal([]byte(val), &entry); err != nil {
		return "", false
	}
	return entry.Result, true
}

func marshalEntry(result string) string {
	b, _ := json.Marshal(CacheEntry{Result: result})
	return string(b)
}
