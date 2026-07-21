package repository

import (
	"context"
	"time"

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
	Count(ctx context.Context, filter map[string]interface{}) (int64, error)
	List(ctx context.Context, filter map[string]interface{}, skip, limit int64) ([]map[string]interface{}, error)
}

//go:generate mockery --name SessionRepository --output ./mocks --outpkg mocks

// SessionRepository defines the data access contract for chat sessions.
type SessionRepository interface {
	Create(ctx context.Context, s SessionRecord) error
	Get(ctx context.Context, id string) (*SessionRecord, error)
	Renew(ctx context.Context, id string, newExpiry time.Time) error
	ListByUser(ctx context.Context, userID string) ([]*SessionRecord, error)
	Cleanup(ctx context.Context, before time.Time) (int64, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	ListDeleted(ctx context.Context, before time.Time, limit int64) ([]*SessionRecord, error)
	SetRecoveryHours(ctx context.Context, hours int) error
}

// SessionRecord is the session data record used by the repository.
type SessionRecord struct {
	ID          string     `bson:"_id"`
	UserID      string     `bson:"user_id"`
	Title       string     `bson:"title"`
	CreatedAt   time.Time  `bson:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at"`
	ExpiresAt   time.Time  `bson:"expires_at"`
	DeletedAt   *time.Time `bson:"deleted_at,omitempty"`
	Recoverable bool       `bson:"recoverable"`
	RecoveryHrs int        `bson:"recovery_hours"`
}
