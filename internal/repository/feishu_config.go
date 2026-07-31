package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/feishu"
)

//go:generate mockery --name FeishuConfigRepository --output ./mocks --outpkg mocks

// FeishuConfigRepository defines the data access contract for Feishu configs.
type FeishuConfigRepository interface {
	Create(ctx context.Context, cfg *feishu.Config) error
	Get(ctx context.Context, id string) (*feishu.Config, error)
	Update(ctx context.Context, cfg *feishu.Config) error
	Delete(ctx context.Context, id string) error
	ListByUser(ctx context.Context, userID string, skip, limit int64) ([]*feishu.Config, int64, error)
	// FindBySession returns the config associated with a session.
	FindBySession(ctx context.Context, sessionID string) (*feishu.Config, error)
	// AllEnabled returns all enabled configs (for websocket connection on startup).
	AllEnabled(ctx context.Context) ([]*feishu.Config, error)
}
