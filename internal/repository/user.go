// Package repository defines data access interfaces for the DataAgent system.
// All interfaces use domain types and standard Go types only — zero infra SDK imports.
package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name UserRepository --output ./mocks --outpkg mocks

// UserRepository defines the data access contract for users.
// Matches the concrete implementation in internal/infra/mongo/.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id string) (*model.User, error)
	HasSystemAdmin(ctx context.Context) (bool, error)
	UpdatePassword(ctx context.Context, userID string, passwordHash string) error
	UpdateRole(ctx context.Context, userID string, newRole model.UserRole) error
	UpdateStatus(ctx context.Context, userID string, status model.UserStatus) error
	Delete(ctx context.Context, userID string) error
	List(ctx context.Context, role string, skip, limit int64) ([]model.User, int64, error)
	ListSorted(ctx context.Context, role string, skip, limit int64, sortBy, sortOrder string) ([]model.User, int64, error)
}

// UserFilter carries optional filter parameters for listing users.
type UserFilter struct {
	Role   string
	Active *bool
}
