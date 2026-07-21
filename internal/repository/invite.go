package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name InviteRepository --output ./mocks --outpkg mocks

// InviteRepository defines the data access contract for registration invites.
type InviteRepository interface {
	Create(ctx context.Context, invite *model.Invite) error
	FindByInviteID(ctx context.Context, inviteID string) (*model.Invite, error)
	FindByTokenHash(ctx context.Context, hash string) (*model.Invite, error)
	MarkAccepted(ctx context.Context, inviteID string, userID string) error
	Revoke(ctx context.Context, inviteID string) error
	List(ctx context.Context, createdBy string, skip, limit int64) ([]model.Invite, int64, error)
}
