package redis

import (
	"context"
	"testing"
	"time"

	"github.com/alicebob/miniredis/v2"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// startMiniRedis starts an in-memory Redis server for testing.
func startMiniRedis(t *testing.T) (*miniredis.Miniredis, *redis.Client) {
	t.Helper()
	mr, err := miniredis.Run()
	require.NoError(t, err)
	t.Cleanup(mr.Close)

	client := redis.NewClient(&redis.Options{Addr: mr.Addr()})
	t.Cleanup(func() { _ = client.Close() })

	return mr, client
}

func TestCacheRepo_SetAndGet(t *testing.T) {
	_, client := startMiniRedis(t)
	c := NewCacheRepo(client)
	ctx := context.Background()

	// Set a value and retrieve it.
	require.NoError(t, c.Set(ctx, "foo", "bar", 0))
	val, err := c.Get(ctx, "foo")
	require.NoError(t, err)
	assert.Equal(t, "bar", val)
}

func TestCacheRepo_Get_Miss(t *testing.T) {
	_, client := startMiniRedis(t)
	c := NewCacheRepo(client)
	ctx := context.Background()

	// Non-existent key → ("", nil) not an error.
	val, err := c.Get(ctx, "nonexistent")
	require.NoError(t, err)
	assert.Equal(t, "", val)
}

func TestCacheRepo_Set_WithTTL(t *testing.T) {
	mr, client := startMiniRedis(t)
	c := NewCacheRepo(client)
	ctx := context.Background()

	// Set with 1-second TTL.
	require.NoError(t, c.Set(ctx, "temp", "val", 1))

	// Immediately available.
	val, err := c.Get(ctx, "temp")
	require.NoError(t, err)
	assert.Equal(t, "val", val)

	// Advance miniredis clock past TTL → key expires.
	mr.FastForward(2 * time.Second)

	val2, err := c.Get(ctx, "temp")
	require.NoError(t, err)
	assert.Equal(t, "", val2) // expired → miss
}

func TestCacheRepo_Set_NoTTL_Persists(t *testing.T) {
	mr, client := startMiniRedis(t)
	c := NewCacheRepo(client)
	ctx := context.Background()

	// TTL 0 or negative → no expiration.
	require.NoError(t, c.Set(ctx, "persistent", "data", 0))

	mr.FastForward(10 * time.Minute)

	val, err := c.Get(ctx, "persistent")
	require.NoError(t, err)
	assert.Equal(t, "data", val)
}

func TestCacheRepo_Delete(t *testing.T) {
	_, client := startMiniRedis(t)
	c := NewCacheRepo(client)
	ctx := context.Background()

	// Set two keys, delete both.
	require.NoError(t, c.Set(ctx, "k1", "v1", 0))
	require.NoError(t, c.Set(ctx, "k2", "v2", 0))

	require.NoError(t, c.Delete(ctx, "k1", "k2"))

	// Both should be gone.
	v1, _ := c.Get(ctx, "k1")
	v2, _ := c.Get(ctx, "k2")
	assert.Equal(t, "", v1)
	assert.Equal(t, "", v2)
}

func TestCacheRepo_Delete_Idempotent(t *testing.T) {
	_, client := startMiniRedis(t)
	c := NewCacheRepo(client)
	ctx := context.Background()

	// Deleting non-existent keys should not error.
	require.NoError(t, c.Delete(ctx, "ghost1", "ghost2"))
}

func TestCacheRepo_Delete_EmptyKeys(t *testing.T) {
	_, client := startMiniRedis(t)
	c := NewCacheRepo(client)
	ctx := context.Background()

	// No keys → no-op, no error.
	require.NoError(t, c.Delete(ctx))
}
