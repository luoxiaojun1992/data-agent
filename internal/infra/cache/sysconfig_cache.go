// Package cache provides Cache-Aside decorators that wrap repository
// implementations with a Redis caching layer. Consumers depend on the same
// repository interface and are unaware of the caching — this keeps the
// caching logic centralized in the infra layer (SPEC-061).
package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"sync"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

const (
	// defaultTTL is the fallback cache TTL in seconds. Applies when no explicit
	// TTL is provided to the constructor. Ensures eventual consistency even if
	// a cache invalidation is missed (e.g. process crash between DB write and
	// cache flush).
	defaultTTL = 600

	// keyPrefix is the root namespace for all sysconfig cache keys.
	keyPrefix = "syscfg"
)

// SysConfigCacheRepo is a Cache-Aside decorator implementing
// repository.SysConfigRepository. It wraps a mongo repository and a Redis
// cache (repository.CacheRepository). When the cache is nil (Redis
// unavailable), all operations transparently degrade to direct mongo access.
//
// Cache rules (SPEC-061 §5.2):
//   - Get:  cache hit → return; miss → mongo → backfill cache
//   - Upsert: DB first → on success, update single cache + invalidate aggregate
//   - Delete:  DB first → on success, invalidate single + aggregate caches
//
// The cache field is mutable via SetCache so it can be injected after Redis
// connects (Redis is initialised later than the mongo repo in the startup
// sequence). Reads are guarded by RWMutex for concurrent safety.
type SysConfigCacheRepo struct {
	mongo repository.SysConfigRepository
	mu    sync.RWMutex
	cache repository.CacheRepository
	ttl   int
}

// NewSysConfigCacheRepo creates a Cache-Aside decorator. Passing nil cache
// means "no caching" — all calls pass through to mongo. Use SetCache to inject
// a Redis-backed cache later (e.g. after Redis connects at startup).
func NewSysConfigCacheRepo(mongo repository.SysConfigRepository, cache repository.CacheRepository, ttlSec int) *SysConfigCacheRepo {
	if ttlSec <= 0 {
		ttlSec = defaultTTL
	}
	return &SysConfigCacheRepo{mongo: mongo, cache: cache, ttl: ttlSec}
}

// SetCache injects or replaces the cache backend. Safe to call after the
// decorator is already in use (e.g. injecting Redis after startup). Passing
// nil disables caching (degrade to mongo passthrough).
func (r *SysConfigCacheRepo) SetCache(cache repository.CacheRepository) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.cache = cache
}

func (r *SysConfigCacheRepo) getCache() repository.CacheRepository {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.cache
}

// singleKey builds the cache key for a single (namespace, key) entry.
func singleKey(namespace, key string) string {
	return fmt.Sprintf("%s:%s:%s", keyPrefix, namespace, key)
}

// allKey builds the cache key for the aggregate (all entries in a namespace).
func allKey(namespace string) string {
	return fmt.Sprintf("%s:ns:%s:all", keyPrefix, namespace)
}

// Get retrieves a config entry. Cache hit returns immediately; cache miss
// falls through to mongo and backfills the cache.
func (r *SysConfigCacheRepo) Get(ctx context.Context, namespace, key string) (*model.SystemConfig, error) {
	c := r.getCache()
	if c != nil {
		cached, err := c.Get(ctx, singleKey(namespace, key))
		if err == nil && cached != "" {
			// Cache hit — reconstruct a SystemConfig with the cached value.
			return &model.SystemConfig{
				Namespace: namespace,
				Key:       key,
				Value:     cached,
			}, nil
		}
		// miss or cache error → fall through to mongo
	}

	cfg, err := r.mongo.Get(ctx, namespace, key)
	if err != nil {
		return nil, err
	}
	if cfg != nil && c != nil {
		// Backfill cache (best-effort; failure is non-fatal, TTL self-heals).
		_ = c.Set(ctx, singleKey(namespace, key), cfg.Value, r.ttl)
	}
	return cfg, nil
}

// GetAll retrieves all configs in a namespace. Uses an aggregate cache key
// storing a JSON array of SystemConfig entries.
func (r *SysConfigCacheRepo) GetAll(ctx context.Context, namespace string) ([]model.SystemConfig, error) {
	c := r.getCache()
	if c != nil {
		cached, err := c.Get(ctx, allKey(namespace))
		if err == nil && cached != "" {
			var configs []model.SystemConfig
			if json.Unmarshal([]byte(cached), &configs) == nil {
				return configs, nil
			}
			// malformed cache → fall through to mongo
		}
	}

	configs, err := r.mongo.GetAll(ctx, namespace)
	if err != nil {
		return nil, err
	}
	if configs != nil && c != nil {
		if data, mErr := json.Marshal(configs); mErr == nil {
			_ = c.Set(ctx, allKey(namespace), string(data), r.ttl)
		}
	}
	return configs, nil
}

// Upsert writes to DB first; on success, updates the single-entry cache and
// invalidates the aggregate cache so the next GetAll re-fetches from DB.
func (r *SysConfigCacheRepo) Upsert(ctx context.Context, namespace, key, value string) error {
	if err := r.mongo.Upsert(ctx, namespace, key, value); err != nil {
		return err // DB is SSOT — don't touch cache on DB failure
	}
	c := r.getCache()
	if c != nil {
		// Best-effort cache update; failures are non-fatal (TTL self-heals).
		_ = c.Set(ctx, singleKey(namespace, key), value, r.ttl)
		_ = c.Delete(ctx, allKey(namespace))
	}
	return nil
}

// Delete removes from DB first; on success, invalidates both the single-entry
// and aggregate caches.
func (r *SysConfigCacheRepo) Delete(ctx context.Context, namespace, key string) error {
	if err := r.mongo.Delete(ctx, namespace, key); err != nil {
		return err
	}
	c := r.getCache()
	if c != nil {
		_ = c.Delete(ctx, singleKey(namespace, key), allKey(namespace))
	}
	return nil
}

// Compile-time interface conformance check.
var _ repository.SysConfigRepository = (*SysConfigCacheRepo)(nil)
