package redis

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/redis/go-redis/v9"
)

// DefaultLockTTL is the uniform lock lifetime for the SPEC-104 exclusive
// locks. A 24h TTL is the safety net that auto-releases a lock held by a
// crashed/cancelled holder so the opposing side (delete/archive) is never
// blocked forever.
const DefaultLockTTL = 24 * time.Hour

// Locker is the cross-process exclusive-lock primitive (SPEC-104 §3). It is
// an interface so service/executor layers can be tested with a mock; the
// production implementation is RedisLocker. The lock is "抢占式" (try-acquire):
// Acquire returns ok=false when another holder already owns the key — the
// caller must then reject with a 409 rather than wait.
type Locker interface {
	// Acquire atomically sets key=value with the given TTL only when the key
	// does not already exist (SET key value NX EX ttl). Returns true when the
	// lock was acquired. value is the holder token used by Release to guard
	// against deleting another holder's lock.
	Acquire(ctx context.Context, key, value string, ttl time.Duration) (bool, error)
	// Release deletes key only when its current value still equals the holder
	// token (Lua compare-and-delete, atomic), preventing a stale holder from
	// deleting a lock re-acquired by someone else after TTL expiry.
	Release(ctx context.Context, key, value string) error
}

// RedisLocker implements Locker over *redis.Client using SET NX EX + a Lua
// compare-and-delete release script.
type RedisLocker struct {
	client *redis.Client
}

// NewLocker creates a Redis-backed distributed lock.
func NewLocker(client *redis.Client) *RedisLocker {
	return &RedisLocker{client: client}
}

// RunLockKey returns the Redis key guarding a task run's lifetime
// ("占用方=executor ↔ 销毁方=DeleteRun" 双向互斥, SPEC-104 D2).
func RunLockKey(runID string) string {
	return "lock:run:" + runID
}

// SessionLockKey returns the Redis key guarding a chat session's lifetime
// ("占用方=chat 流 ↔ 销毁方=归档" 双向互斥, SPEC-104 D4).
func SessionLockKey(sessionID string) string {
	return "lock:session:" + sessionID
}

// NewLockToken returns a fresh unique holder token for a single Acquire.
// A unique per-acquisition token is what makes the release-time
// compare-and-delete meaningful: after a 24h TTL expiry + re-acquire, the old
// holder's token no longer matches and its Release becomes a no-op.
func NewLockToken() string {
	return uuid.NewString()
}

// releaseScript compares the key's value against the holder token and deletes
// only on match (atomic). It returns the number of deleted keys (0 or 1), which
// callers can ignore — the important property is that a mismatched token never
// deletes someone else's lock.
var releaseScript = redis.NewScript(`
if redis.call("get", KEYS[1]) == ARGV[1] then
    return redis.call("del", KEYS[1])
else
    return 0
end`)

// Acquire implements Locker via SET key value NX EX ttl.
func (l *RedisLocker) Acquire(ctx context.Context, key, value string, ttl time.Duration) (bool, error) {
	if l == nil || l.client == nil {
		return false, fmt.Errorf("redis locker not initialized")
	}
	ok, err := l.client.SetNX(ctx, key, value, ttl).Result()
	if err != nil {
		return false, fmt.Errorf("lock acquire %s: %w", key, err)
	}
	return ok, nil
}

// Release implements Locker via the atomic compare-and-delete Lua script.
func (l *RedisLocker) Release(ctx context.Context, key, value string) error {
	if l == nil || l.client == nil {
		return fmt.Errorf("redis locker not initialized")
	}
	if _, err := releaseScript.Run(ctx, l.client, []string{key}, value).Result(); err != nil {
		return fmt.Errorf("lock release %s: %w", key, err)
	}
	return nil
}
