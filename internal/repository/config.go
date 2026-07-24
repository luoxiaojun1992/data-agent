package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/apireview"
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
	Delete(ctx context.Context, namespace, key string) error
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
	Create(ctx context.Context, review *apireview.APIReview) error
	List(ctx context.Context, skip, limit int64) ([]apireview.APIReview, error)
	FindByID(ctx context.Context, id string) (*apireview.APIReview, error)
	UpdateStatus(ctx context.Context, id string, update map[string]interface{}) error
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
