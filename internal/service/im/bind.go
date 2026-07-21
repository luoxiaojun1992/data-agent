package im

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// BindService manages IM (Feishu) binding records for users. It is a thin
// service over IMBindRepository, keeping the handler layer free of infra.
type BindService struct {
	repo repository.IMBindRepository
}

// NewBindService creates a new BindService.
func NewBindService(repo repository.IMBindRepository) *BindService {
	return &BindService{repo: repo}
}

// Get returns the IM binding for the given user, or nil if not bound.
func (s *BindService) Get(ctx context.Context, userID string) (map[string]interface{}, error) {
	return s.repo.Get(ctx, userID)
}

// Upsert creates or updates the IM binding for the given user.
func (s *BindService) Upsert(ctx context.Context, userID string, data map[string]interface{}) error {
	return s.repo.Upsert(ctx, userID, data)
}

// Delete removes the IM binding for the given user (idempotent).
func (s *BindService) Delete(ctx context.Context, userID string) error {
	return s.repo.Delete(ctx, userID)
}
