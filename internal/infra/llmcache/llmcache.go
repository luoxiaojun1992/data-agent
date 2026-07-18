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
}

// New creates a Cache backed by the given Redis client.
func New(client *redis.Client) *Cache {
	return &Cache{client: client}
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

// SetEmbedding caches an embedding result (no TTL — embeddings are deterministic).
func (c *Cache) SetEmbedding(ctx context.Context, model, text, result string) {
	_ = c.client.Set(ctx, embedKey(model, text), marshalEntry(result), 0).Err()
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
