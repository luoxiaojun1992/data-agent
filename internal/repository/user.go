// Package repository defines data access interfaces for the DataAgent system.
// All interfaces use domain types and standard Go types only — zero infra SDK imports.
package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name UserRepository --output ./mocks --outpkg mocks

// UserRepository defines the data access contract for users.
type UserRepository interface {
	Create(ctx context.Context, user *model.User) error
	FindByUsername(ctx context.Context, username string) (*model.User, error)
	FindByID(ctx context.Context, id string) (*model.User, error)
	HasSystemAdmin(ctx context.Context) (bool, error)
	UpdatePassword(ctx context.Context, id string, hashedPassword string) error
	UpdateRole(ctx context.Context, id string, role string) error
	UpdateStatus(ctx context.Context, id string, active bool) error
	Delete(ctx context.Context, id string) error
	List(ctx context.Context, skip, limit int64) ([]*model.User, error)
	ListSorted(ctx context.Context, skip, limit int64, sortField string, sortDesc bool) ([]*model.User, error)
}

// UserFilter carries optional filter parameters for listing users.
type UserFilter struct {
	Role   string
	Active *bool
}
