package repository

import (
	"context"
	"time"
)

//go:generate mockery --name SessionRepository --output ./mocks --outpkg mocks

// SessionRepository defines the data access contract for chat sessions.
type SessionRepository interface {
	Create(ctx context.Context, s SessionRecord) error
	Get(ctx context.Context, id string) (*SessionRecord, error)
	Renew(ctx context.Context, id string, newExpiry time.Time) error
	ListByUser(ctx context.Context, userID string) ([]*SessionRecord, error)
	ListByUserPaged(ctx context.Context, userID string, skip, limit int64) ([]*SessionRecord, int64, error)
	Cleanup(ctx context.Context, before time.Time) (int64, error)
	Delete(ctx context.Context, id string) error
	Restore(ctx context.Context, id string) error
	ListDeleted(ctx context.Context, before time.Time, limit int64) ([]*SessionRecord, error)
	SetRecoveryHours(ctx context.Context, hours int) error
	SetTitle(ctx context.Context, id, title string) error
}

// SessionRecord is the session data record used by the repository.
type SessionRecord struct {
	ID          string     `bson:"_id"`
	UserID      string     `bson:"user_id"`
	Title       string     `bson:"title"`
	ModelID     string     `bson:"model_id"`
	// IsTask marks sessions created by the async/scheduled task executor.
	// These ride the same ADK chat-session infrastructure but represent an
	// autonomous analysis run rather than a real-time user conversation.
	IsTask      bool       `bson:"is_task,omitempty"`
	CreatedAt   time.Time  `bson:"created_at"`
	UpdatedAt   time.Time  `bson:"updated_at"`
	ExpiresAt   time.Time  `bson:"expires_at"`
	DeletedAt   *time.Time `bson:"deleted_at,omitempty"`
	Recoverable bool       `bson:"recoverable"`
	RecoveryHrs int        `bson:"recovery_hours"`
}
