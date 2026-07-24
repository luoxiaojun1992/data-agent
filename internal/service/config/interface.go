package config

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name Service --output ./mocks --outpkg mocks

// Service defines the system configuration service contract.
type Service interface {
	GetAll(ctx context.Context, namespace string) ([]model.SystemConfig, error)
	Upsert(ctx context.Context, namespace, key, value string) error
	Delete(ctx context.Context, namespace, key string) error
}
