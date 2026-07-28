package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name NotificationRepository --output ./mocks --outpkg mocks

// NotificationRepository defines the data access contract for system notifications.
type NotificationRepository interface {
	Create(ctx context.Context, n *model.Notification) error
	ListForUser(ctx context.Context, userID string, skip, limit int64) ([]*model.Notification, int64, error)
	CountUnread(ctx context.Context, userID string) (int64, error)
	MarkRead(ctx context.Context, id string, userID string) error
	MarkAllRead(ctx context.Context, userID string) error
}

//go:generate mockery --name AuditRepository --output ./mocks --outpkg mocks

// AuditRepository defines the data access contract for audit logs.
type AuditRepository interface {
	Create(ctx context.Context, log *model.AuditLog) error
	Count(ctx context.Context, filter map[string]interface{}) (int64, error)
	List(ctx context.Context, filter map[string]interface{}, skip, limit int64) ([]model.AuditLog, error)
}

