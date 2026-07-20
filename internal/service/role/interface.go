package role

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name Service --output ./mocks --outpkg mocks

// Service defines the role management service contract.
type Service interface {
	List(ctx context.Context) ([]model.Role, error)
	ListPermissions() []model.PermissionInfo
	Create(ctx context.Context, name string, permissions []string) (*model.Role, error)
	Update(ctx context.Context, id string, permissions []string) error
	Delete(ctx context.Context, id string) error
}
