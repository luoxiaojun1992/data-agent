package notification

import (
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

//go:generate mockery --name NotificationService --output ./mocks --outpkg mocks

// NotificationService defines the notification management service contract.
type NotificationService interface {
	Send(title, content, nType string, targetIDs []string) (*model.Notification, error)
	Broadcast(title, content, nType string) (*model.Notification, error)
	ListForUser(userID string, limit int64) ([]model.Notification, error)
	UnreadCount(userID string) (int64, error)
	MarkRead(id string, userID string) error
	MarkAllRead(userID string) error
}

var _ NotificationService = (*Service)(nil)
