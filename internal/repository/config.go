package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
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
type SysConfigRepository interface {
	Get(ctx context.Context, namespace, key string) (*model.SystemConfig, error)
	GetAll(ctx context.Context, namespace string) ([]model.SystemConfig, error)
	Upsert(ctx context.Context, namespace, key, value string) error
}

//go:generate mockery --name ModelConfigRepository --output ./mocks --outpkg mocks

// ModelConfigRepository defines the data access contract for model configurations.
type ModelConfigRepository interface {
	GetAll(ctx context.Context) ([]map[string]interface{}, error)
	Upsert(ctx context.Context, key string, config map[string]interface{}) error
	Delete(ctx context.Context, key string) error
}

//go:generate mockery --name APIReviewRepository --output ./mocks --outpkg mocks

// APIReviewRepository defines the data access contract for API review records.
type APIReviewRepository interface {
	Create(ctx context.Context, review map[string]interface{}) error
	List(ctx context.Context, skip, limit int64) ([]map[string]interface{}, error)
	FindByID(ctx context.Context, id string) (map[string]interface{}, error)
	Approve(ctx context.Context, id string) error
	Reject(ctx context.Context, id string, reason string) error
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
