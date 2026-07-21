package user

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name Service --output ./mocks --outpkg mocks

// Service defines the user management service contract.
type Service interface {
	List(ctx context.Context, role string, skip, limit int64, sortBy, sortOrder string) ([]model.User, int64, error)
	Get(ctx context.Context, id string) (*model.User, error)
	Create(ctx context.Context, username, password, role string) (*model.User, error)
	UpdateRole(ctx context.Context, id string, role model.UserRole) error
	ToggleStatus(ctx context.Context, id string) error
	Delete(ctx context.Context, id string) error
}
