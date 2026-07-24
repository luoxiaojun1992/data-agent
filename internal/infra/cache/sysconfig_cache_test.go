package cache

import (
	"context"
	"encoding/json"
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	repomocks "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

// newMocks creates a fresh pair of SysConfigRepository and CacheRepository
// mocks for a single test case. The SysConfigRepository mock asserts
// expectations on cleanup; the CacheRepository mock does not (some cache
// calls are best-effort and should not be strictly asserted).
func newMocks(t *testing.T) (*repomocks.SysConfigRepository, *repomocks.CacheRepository) {
	t.Helper()
	repoMock := repomocks.NewSysConfigRepository(t)
	cacheMock := &repomocks.CacheRepository{}
	cacheMock.Mock.Test(t)
	return repoMock, cacheMock
}

// ── Get ────────────────────────────────────────────────────────────────

func TestGet_CacheHit(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	// Cache returns a non-empty value → should NOT touch mongo.
	cacheMock.On("Get", mock.Anything, "syscfg:model:models").Return("cached-value", nil)

	cfg, err := dec.Get(context.Background(), "model", "models")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "cached-value", cfg.Value)
	assert.Equal(t, "model", cfg.Namespace)
	assert.Equal(t, "models", cfg.Key)
}

func TestGet_CacheMiss_Backfills(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	// Cache miss (empty string) → falls through to mongo → backfills cache.
	cacheMock.On("Get", mock.Anything, "syscfg:model:models").Return("", nil)
	repoMock.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{
		Namespace: "model", Key: "models", Value: "db-value",
	}, nil)
	cacheMock.On("Set", mock.Anything, "syscfg:model:models", "db-value", 600).Return(nil)

	cfg, err := dec.Get(context.Background(), "model", "models")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "db-value", cfg.Value)
}

func TestGet_CacheNil_DegradesToMongo(t *testing.T) {
	repoMock, _ := newMocks(t)
	// cache is nil → degrade mode, no cache interaction.
	dec := NewSysConfigCacheRepo(repoMock, nil, 0)

	repoMock.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{
		Namespace: "model", Key: "models", Value: "direct-value",
	}, nil)

	cfg, err := dec.Get(context.Background(), "model", "models")
	require.NoError(t, err)
	require.NotNil(t, cfg)
	assert.Equal(t, "direct-value", cfg.Value)
}

func TestGet_CacheError_FallsThroughToMongo(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	// Cache returns error → should fall through to mongo (degrade on error).
	cacheMock.On("Get", mock.Anything, "syscfg:model:models").Return("", errors.New("redis down"))
	repoMock.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{
		Namespace: "model", Key: "models", Value: "fallback",
	}, nil)
	cacheMock.On("Set", mock.Anything, "syscfg:model:models", "fallback", 600).Return(nil)

	cfg, err := dec.Get(context.Background(), "model", "models")
	require.NoError(t, err)
	assert.Equal(t, "fallback", cfg.Value)
}

// ── GetAll ─────────────────────────────────────────────────────────────

func TestGetAll_CacheHit(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	cached := []model.SystemConfig{{Key: "k1", Value: "v1"}, {Key: "k2", Value: "v2"}}
	data, _ := json.Marshal(cached)
	cacheMock.On("Get", mock.Anything, "syscfg:ns:model:all").Return(string(data), nil)

	cfgs, err := dec.GetAll(context.Background(), "model")
	require.NoError(t, err)
	assert.Len(t, cfgs, 2)
	assert.Equal(t, "v1", cfgs[0].Value)
}

func TestGetAll_CacheMiss_Backfills(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	dbCfgs := []model.SystemConfig{{Key: "k", Value: "v"}}
	cacheMock.On("Get", mock.Anything, "syscfg:ns:model:all").Return("", nil)
	repoMock.On("GetAll", mock.Anything, "model").Return(dbCfgs, nil)
	cacheMock.On("Set", mock.Anything, "syscfg:ns:model:all", mock.Anything, 600).Return(nil)

	cfgs, err := dec.GetAll(context.Background(), "model")
	require.NoError(t, err)
	assert.Len(t, cfgs, 1)
}

func TestGetAll_CacheNil_DegradesToMongo(t *testing.T) {
	repoMock, _ := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, nil, 0)

	repoMock.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{{Key: "k", Value: "v"}}, nil)

	cfgs, err := dec.GetAll(context.Background(), "model")
	require.NoError(t, err)
	assert.Len(t, cfgs, 1)
}

// ── Upsert ─────────────────────────────────────────────────────────────

func TestUpsert_Success_UpdatesCache(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	// DB upsert succeeds → cache.Set (single) + cache.Delete (aggregate).
	repoMock.On("Upsert", mock.Anything, "model", "models", "new-val").Return(nil)
	cacheMock.On("Set", mock.Anything, "syscfg:model:models", "new-val", 600).Return(nil)
	cacheMock.On("Delete", mock.Anything, "syscfg:ns:model:all").Return(nil)

	err := dec.Upsert(context.Background(), "model", "models", "new-val")
	require.NoError(t, err)
}

func TestUpsert_DBError_NoCacheTouch(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	// DB fails → cache must NOT be touched.
	repoMock.On("Upsert", mock.Anything, "model", "models", "val").Return(errors.New("db down"))

	err := dec.Upsert(context.Background(), "model", "models", "val")
	require.Error(t, err)
}

func TestUpsert_CacheNil_PassesThrough(t *testing.T) {
	repoMock, _ := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, nil, 0)

	repoMock.On("Upsert", mock.Anything, "model", "models", "val").Return(nil)

	err := dec.Upsert(context.Background(), "model", "models", "val")
	require.NoError(t, err)
}

// ── Delete ─────────────────────────────────────────────────────────────

func TestDelete_Success_InvalidatesCache(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	repoMock.On("Delete", mock.Anything, "model", "models").Return(nil)
	// Both single and aggregate keys are invalidated.
	cacheMock.On("Delete", mock.Anything, "syscfg:model:models", "syscfg:ns:model:all").Return(nil)

	err := dec.Delete(context.Background(), "model", "models")
	require.NoError(t, err)
}

func TestDelete_DBError_NoCacheTouch(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, cacheMock, 0)

	repoMock.On("Delete", mock.Anything, "model", "models").Return(errors.New("db down"))

	err := dec.Delete(context.Background(), "model", "models")
	require.Error(t, err)
}

func TestDelete_CacheNil_PassesThrough(t *testing.T) {
	repoMock, _ := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, nil, 0)

	repoMock.On("Delete", mock.Anything, "model", "models").Return(nil)

	err := dec.Delete(context.Background(), "model", "models")
	require.NoError(t, err)
}

// ── SetCache (deferred injection) ──────────────────────────────────────

func TestSetCache_InjectsAfterConstruction(t *testing.T) {
	repoMock, cacheMock := newMocks(t)
	// Construct with nil cache (simulates startup before Redis connects).
	dec := NewSysConfigCacheRepo(repoMock, nil, 0)

	// First call: degrade to mongo (no cache).
	repoMock.On("Get", mock.Anything, "model", "k").Return(&model.SystemConfig{
		Namespace: "model", Key: "k", Value: "before-inject",
	}, nil).Once()

	cfg, err := dec.Get(context.Background(), "model", "k")
	require.NoError(t, err)
	assert.Equal(t, "before-inject", cfg.Value)

	// Inject cache → subsequent calls should use it.
	dec.SetCache(cacheMock)
	cacheMock.On("Get", mock.Anything, "syscfg:model:k").Return("after-inject", nil)

	cfg2, err := dec.Get(context.Background(), "model", "k")
	require.NoError(t, err)
	assert.Equal(t, "after-inject", cfg2.Value)
}

// ── TTL ─────────────────────────────────────────────────────────────────

func TestNewSysConfigCacheRepo_DefaultTTL(t *testing.T) {
	repoMock, _ := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, nil, 0)
	assert.Equal(t, defaultTTL, dec.ttl)
}

func TestNewSysConfigCacheRepo_CustomTTL(t *testing.T) {
	repoMock, _ := newMocks(t)
	dec := NewSysConfigCacheRepo(repoMock, nil, 300)
	assert.Equal(t, 300, dec.ttl)
}
