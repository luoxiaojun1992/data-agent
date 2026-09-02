package config

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name Service --output ./mocks --outpkg mocks

// Service defines the system configuration service contract.
type Service interface {
	GetAll(ctx context.Context) ([]model.SystemConfig, error)
	List(ctx context.Context, page, pageSize int) ([]model.SystemConfig, int64, error)
	Upsert(ctx context.Context, key, value, description string) error
	Delete(ctx context.Context, key string) error
	SeedBuiltins(ctx context.Context) error
}
