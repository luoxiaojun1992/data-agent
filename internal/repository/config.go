package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/domain/modelconfig"
)

//go:generate mockery --name RoleRepository --output ./mocks --outpkg mocks

// RoleRepository defines the data access contract for roles.
type RoleRepository interface {
	Create(ctx context.Context, role *model.Role) error
	List(ctx context.Context) ([]model.Role, error)
	FindByID(ctx context.Context, id string) (*model.Role, error)
	Update(ctx context.Context, roleID string, permissions []string) error
	Delete(ctx context.Context, roleID string) error
}

//go:generate mockery --name SysConfigRepository --output ./mocks --outpkg mocks

// SysConfigRepository defines the data access contract for system configuration.
// Namespace is removed — each key is a standalone document in system_configs.
type SysConfigRepository interface {
	Get(ctx context.Context, key string) (*model.SystemConfig, error)
	GetAll(ctx context.Context) ([]model.SystemConfig, error)
	List(ctx context.Context, skip, limit int64) ([]model.SystemConfig, error)
	Count(ctx context.Context) (int64, error)
	Upsert(ctx context.Context, key, value string) error
	Delete(ctx context.Context, key string) error
}

//go:generate mockery --name ModelConfigRepository --output ./mocks --outpkg mocks

// ModelConfigRepository defines structured CRUD + DB pagination for model
// configurations (one document per model).
type ModelConfigRepository interface {
	List(ctx context.Context, t modelconfig.ModelType, skip, limit int64) ([]modelconfig.ModelEntry, int64, error)
	// Search returns top-N models of the given type matching q, with models
	// whose IDs are in defaultIDs sorted first. Filtering, sorting and
	// truncation all happen in MongoDB (SPEC-074).
	Search(ctx context.Context, t modelconfig.ModelType, q string, defaultIDs []string, skip, limit int64) ([]modelconfig.ModelEntry, int64, error)
	Get(ctx context.Context, id string) (*modelconfig.ModelEntry, error)
	Insert(ctx context.Context, entry modelconfig.ModelEntry) error
	Update(ctx context.Context, id string, entry modelconfig.ModelEntry) error
	Delete(ctx context.Context, id string) error
}

//go:generate mockery --name ModelDefaultRepository --output ./mocks --outpkg mocks

// ModelDefaultRepository maps each use case to its default model, with a
// unique index on use_case guaranteeing at most one default per use case.
type ModelDefaultRepository interface {
	List(ctx context.Context) ([]modelconfig.ModelDefault, error)
	Get(ctx context.Context, useCase string) (*modelconfig.ModelDefault, error)
	Set(ctx context.Context, useCase, modelID string) error // deleteOne + insertOne
	Delete(ctx context.Context, useCase string) error       // cancel default
}

//go:generate mockery --name IMBindRepository --output ./mocks --outpkg mocks

// IMBindRepository defines the data access contract for IM binding records.
type IMBindRepository interface {
	Get(ctx context.Context, userID string) (map[string]interface{}, error)
	Upsert(ctx context.Context, userID string, data map[string]interface{}) error
	Delete(ctx context.Context, userID string) error
}

//go:generate mockery --name CacheRepository --output ./mocks --outpkg mocks

// CacheRepository defines the data access contract for Redis caching.
type CacheRepository interface {
	Get(ctx context.Context, key string) (string, error)
	Set(ctx context.Context, key string, value string, ttlSeconds int) error
	Delete(ctx context.Context, keys ...string) error
}
