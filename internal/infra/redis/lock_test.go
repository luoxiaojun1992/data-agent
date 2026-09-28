package redis

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRunLockKey(t *testing.T) {
	assert.Equal(t, "lock:run:run_1", RunLockKey("run_1"))
}

func TestSessionLockKey(t *testing.T) {
	assert.Equal(t, "lock:session:sess_1", SessionLockKey("sess_1"))
}

func TestNewLockToken_Unique(t *testing.T) {
	a, b := NewLockToken(), NewLockToken()
	assert.NotEmpty(t, a)
	assert.NotEmpty(t, b)
	assert.NotEqual(t, a, b, "lock tokens must be unique per acquisition")
}

func TestRedisLocker_Acquire_Success(t *testing.T) {
	_, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	ok, err := l.Acquire(ctx, "lock:run:r1", "token-1", DefaultLockTTL)
	require.NoError(t, err)
	assert.True(t, ok, "first acquire must succeed")

	// The key now exists with the holder token.
	val, err := client.Get(ctx, "lock:run:r1").Result()
	require.NoError(t, err)
	assert.Equal(t, "token-1", val)
}

func TestRedisLocker_Acquire_Conflict(t *testing.T) {
	_, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	ok, err := l.Acquire(ctx, "lock:run:r1", "token-1", DefaultLockTTL)
	require.NoError(t, err)
	require.True(t, ok)

	// A second holder must fail to acquire the same key.
	ok2, err := l.Acquire(ctx, "lock:run:r1", "token-2", DefaultLockTTL)
	require.NoError(t, err)
	assert.False(t, ok2, "second acquire must fail while the lock is held")
}

func TestRedisLocker_Acquire_TTL(t *testing.T) {
	mr, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	ok, err := l.Acquire(ctx, "lock:run:r1", "token-1", 100*time.Millisecond)
	require.NoError(t, err)
	require.True(t, ok)

	// After the TTL passes the lock auto-releases.
	mr.FastForward(200 * time.Millisecond)
	ok2, err := l.Acquire(ctx, "lock:run:r1", "token-2", DefaultLockTTL)
	require.NoError(t, err)
	assert.True(t, ok2, "lock must be re-acquirable after TTL expiry")
}

func TestRedisLocker_Release_Success(t *testing.T) {
	_, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	ok, err := l.Acquire(ctx, "lock:run:r1", "token-1", DefaultLockTTL)
	require.NoError(t, err)
	require.True(t, ok)

	require.NoError(t, l.Release(ctx, "lock:run:r1", "token-1"))

	// Released → re-acquirable.
	ok2, err := l.Acquire(ctx, "lock:run:r1", "token-2", DefaultLockTTL)
	require.NoError(t, err)
	assert.True(t, ok2)
}

func TestRedisLocker_Release_NonOwnerTokenRejected(t *testing.T) {
	_, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	ok, err := l.Acquire(ctx, "lock:run:r1", "owner-token", DefaultLockTTL)
	require.NoError(t, err)
	require.True(t, ok)

	// A different token must NOT delete the owner's lock.
	require.NoError(t, l.Release(ctx, "lock:run:r1", "other-token"))

	val, err := client.Get(ctx, "lock:run:r1").Result()
	require.NoError(t, err)
	assert.Equal(t, "owner-token", val, "non-owner release must not delete the lock")
}

func TestRedisLocker_Release_Idempotent(t *testing.T) {
	_, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	// Releasing a non-existent key is a no-op, not an error.
	require.NoError(t, l.Release(ctx, "lock:run:ghost", "token-1"))
}

func TestRedisLocker_NilClient_Acquire(t *testing.T) {
	l := &RedisLocker{} // client is nil
	ok, err := l.Acquire(context.Background(), "lock:run:r1", "token", DefaultLockTTL)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestRedisLocker_NilClient_Release(t *testing.T) {
	l := &RedisLocker{} // client is nil
	assert.Error(t, l.Release(context.Background(), "lock:run:r1", "token"))
}

func TestNewLocker_NonNil(t *testing.T) {
	_, client := startMiniRedis(t)
	l := NewLocker(client)
	require.NotNil(t, l)
	require.NotNil(t, l.client)
}

func TestRedisLocker_Acquire_ServerDown(t *testing.T) {
	mr, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	// Shut the server down so SetNX returns a real connection error.
	mr.Close()

	ok, err := l.Acquire(ctx, "lock:run:r1", "token", DefaultLockTTL)
	assert.False(t, ok)
	assert.Error(t, err)
}

func TestRedisLocker_Release_ServerDown(t *testing.T) {
	mr, client := startMiniRedis(t)
	l := NewLocker(client)
	ctx := context.Background()

	// Shut the server down so the release Lua script returns a real error.
	mr.Close()

	assert.Error(t, l.Release(ctx, "lock:run:r1", "token"))
}
