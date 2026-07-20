package chat

import (
	"time"
)

//go:generate mockery --name SessionService --output ./mocks --outpkg mocks

// SessionService defines the session management service contract.
type SessionService interface {
	Create(userID, sessionType string) (*Session, error)
	Get(id string) (*Session, error)
	Renew(id string) error
	Cleanup() (int64, error)
	ListByUser(userID string) ([]*Session, error)
	Delete(id string) error
	Restore(id string) error
	ListDeleted(before time.Time, limit int64) ([]*Session, error)
	SetRecoveryHours(hours int) error
}

var _ SessionService = (*Manager)(nil)
