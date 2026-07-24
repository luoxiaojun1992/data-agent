package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

// CacheRepo implements repository.CacheRepository over *redis.Client.
// It supports TTL on Set (unlike the legacy Client.Set wrapper which had no
// TTL parameter) and returns ("", nil) on cache miss so callers can detect
// misses by checking for an empty string.
type CacheRepo struct {
	client *redis.Client
}

// NewCacheRepo creates a CacheRepo backed by the given Redis client.
func NewCacheRepo(client *redis.Client) *CacheRepo {
	return &CacheRepo{client: client}
}

// Get retrieves a value by key. Returns ("", nil) on cache miss (redis.Nil).
func (c *CacheRepo) Get(ctx context.Context, key string) (string, error) {
	v, err := c.client.Get(ctx, key).Result()
	if err == redis.Nil {
		return "", nil // miss — empty string signals miss to caller
	}
	if err != nil {
		return "", fmt.Errorf("cache get %s: %w", key, err)
	}
	return v, nil
}

// Set stores a key-value pair with the given TTL in seconds. A ttlSeconds of
// 0 or less means no expiration (persistent until explicitly deleted).
func (c *CacheRepo) Set(ctx context.Context, key, value string, ttlSeconds int) error {
	var ttl time.Duration
	if ttlSeconds > 0 {
		ttl = time.Duration(ttlSeconds) * time.Second
	}
	if err := c.client.Set(ctx, key, value, ttl).Err(); err != nil {
		return fmt.Errorf("cache set %s: %w", key, err)
	}
	return nil
}

// Delete removes one or more keys. Idempotent: returns nil even if keys do not
// exist.
func (c *CacheRepo) Delete(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	if err := c.client.Del(ctx, keys...).Err(); err != nil {
		return fmt.Errorf("cache delete %v: %w", keys, err)
	}
	return nil
}
