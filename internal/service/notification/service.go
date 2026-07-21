package notification

import (
	"context"
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

type Service struct {
	repo repository.NotificationRepository
}

func NewService(repo repository.NotificationRepository) *Service {
	return &Service{repo: repo}
}

func (s *Service) Send(title, content, nType string, targetIDs []string) (*model.Notification, error) {
	n := &model.Notification{
		Title:     title,
		Content:   content,
		Type:      nType,
		TargetIDs: targetIDs,
	}
	if err := s.repo.Create(context.Background(), n); err != nil {
		return nil, fmt.Errorf("insert notification: %w", err)
	}
	return n, nil
}

func (s *Service) Broadcast(title, content, nType string) (*model.Notification, error) {
	n := &model.Notification{
		Title:     title,
		Content:   content,
		Type:      nType,
		TargetAll: true,
	}
	if err := s.repo.Create(context.Background(), n); err != nil {
		return nil, fmt.Errorf("broadcast: %w", err)
	}
	return n, nil
}

func (s *Service) ListForUser(userID string, limit int64) ([]model.Notification, error) {
	ns, _, err := s.repo.ListForUser(context.Background(), userID, 0, limit)
	if err != nil {
		return nil, err
	}
	result := make([]model.Notification, len(ns))
	for i, n := range ns {
		if n != nil {
			result[i] = *n
		}
	}
	return result, nil
}

func (s *Service) UnreadCount(userID string) (int64, error) {
	return s.repo.CountUnread(context.Background(), userID)
}

func (s *Service) MarkRead(id string, userID string) error {
	return s.repo.MarkRead(context.Background(), id, userID)
}

func (s *Service) MarkAllRead(userID string) error {
	return s.repo.MarkAllRead(context.Background(), userID)
}
