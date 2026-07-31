package feishu

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/feishu"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// ConfigService manages Feishu integration configs.
type ConfigService struct {
	repo    repository.FeishuConfigRepository
	session domainchat.SessionService
}

func NewConfigService(repo repository.FeishuConfigRepository, session domainchat.SessionService) *ConfigService {
	return &ConfigService{repo: repo, session: session}
}

// Create creates a Feishu config and associates it with a new session.
func (s *ConfigService) Create(ctx context.Context, userID, name, appID, appSecret, modelID string) (*feishu.Config, error) {
	// Create a Feishu-flagged session first.
	sess, err := s.session.CreateFeishuSession(userID, modelID)
	if err != nil {
		return nil, fmt.Errorf("create feishu session: %w", err)
	}

	now := time.Now()
	cfg := &feishu.Config{
		ID:        "feishu_" + uuid.New().String(),
		UserID:    userID,
		Name:      name,
		AppID:     appID,
		AppSecret: appSecret,
		ModelID:   modelID,
		SessionID: sess.ID,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.repo.Create(ctx, cfg); err != nil {
		return nil, fmt.Errorf("create feishu config: %w", err)
	}
	return cfg, nil
}

// ListByUser returns paginated configs for a user.
func (s *ConfigService) ListByUser(ctx context.Context, userID string, page, pageSize int) ([]*feishu.Config, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 50 {
		pageSize = 10
	}
	skip := int64((page - 1) * pageSize)
	return s.repo.ListByUser(ctx, userID, skip, int64(pageSize))
}

// Get returns a single config.
func (s *ConfigService) Get(ctx context.Context, id string) (*feishu.Config, error) {
	return s.repo.Get(ctx, id)
}

// Update updates mutable fields on an existing config.
// Allowed fields: app_id, app_secret, enabled. ModelID is immutable after creation.
type UpdateConfigRequest struct {
	AppID     string `json:"app_id,omitempty"`
	AppSecret string `json:"app_secret,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

func (s *ConfigService) Update(ctx context.Context, id string, req UpdateConfigRequest) error {
	cfg, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if req.AppID != "" {
		cfg.AppID = req.AppID
	}
	if req.AppSecret != "" {
		cfg.AppSecret = req.AppSecret
	}
	if req.Enabled != nil {
		cfg.Enabled = *req.Enabled
	}
	cfg.UpdatedAt = time.Now()
	return s.repo.Update(ctx, cfg)
}

// Delete removes a config (idempotent).
func (s *ConfigService) Delete(ctx context.Context, id string) error {
	return s.repo.Delete(ctx, id)
}

// FindBySession returns the config associated with a session.
func (s *ConfigService) FindBySession(ctx context.Context, sessionID string) (*feishu.Config, error) {
	return s.repo.FindBySession(ctx, sessionID)
}

// AllEnabled returns all enabled configs.
func (s *ConfigService) AllEnabled(ctx context.Context) ([]*feishu.Config, error) {
	return s.repo.AllEnabled(ctx)
}
