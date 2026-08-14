package feishu

import (
	"context"
	"fmt"
	"time"

	"github.com/google/uuid"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/feishu"
	"github.com/luoxiaojun1992/data-agent/internal/infra/vault"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// ConfigService manages Feishu integration configs.
type ConfigService struct {
	repo    repository.FeishuConfigRepository
	session domainchat.SessionService
	vault   *vault.Client
}

func NewConfigService(repo repository.FeishuConfigRepository, session domainchat.SessionService, vc *vault.Client) *ConfigService {
	return &ConfigService{repo: repo, session: session, vault: vc}
}

// feishuSecretPath builds the Vault KV path for a config's app_secret.
func feishuSecretPath(id string) string {
	return "data-agent/feishu/" + id + "/app_secret"
}

// storeSecret writes the plaintext app_secret to Vault and records the path on
// the config. Fails (returns error) when Vault is unavailable so a caller
// never believes a secret was persisted when it wasn't.
func (s *ConfigService) storeSecret(ctx context.Context, cfg *feishu.Config, plaintext string) error {
	if plaintext == "" {
		return nil
	}
	if s.vault == nil {
		return fmt.Errorf("vault not available, cannot store app_secret")
	}
	path := feishuSecretPath(cfg.ID)
	if err := s.vault.Store(ctx, path, plaintext); err != nil {
		return fmt.Errorf("store app_secret to vault: %w", err)
	}
	cfg.VaultSecretPath = path
	cfg.AppSecret = plaintext // in-memory plaintext for the API response
	return nil
}

// resolveSecret decrypts the config's Vault path into cfg.AppSecret. On any
// failure the field is cleared (empty), never falling back to the path string.
func (s *ConfigService) resolveSecret(ctx context.Context, cfg *feishu.Config) {
	if cfg == nil || cfg.VaultSecretPath == "" {
		return
	}
	if s.vault == nil {
		cfg.AppSecret = ""
		return
	}
	plain, err := s.vault.Retrieve(ctx, cfg.VaultSecretPath)
	if err != nil {
		cfg.AppSecret = ""
		return
	}
	cfg.AppSecret = plain
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
		ModelID:   modelID,
		SessionID: sess.ID,
		Enabled:   true,
		CreatedAt: now,
		UpdatedAt: now,
	}
	if err := s.storeSecret(ctx, cfg, appSecret); err != nil {
		return nil, err
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
	list, total, err := s.repo.ListByUser(ctx, userID, skip, int64(pageSize))
	if err != nil {
		return nil, 0, err
	}
	for _, cfg := range list {
		s.resolveSecret(ctx, cfg)
	}
	return list, total, nil
}

// Get returns a single config with its app_secret decrypted from Vault.
func (s *ConfigService) Get(ctx context.Context, id string) (*feishu.Config, error) {
	cfg, err := s.repo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	s.resolveSecret(ctx, cfg)
	return cfg, nil
}

// Update updates mutable fields on an existing config.
// Allowed fields: name, app_id, app_secret, enabled. ModelID is immutable after creation.
type UpdateConfigRequest struct {
	Name      string `json:"name,omitempty"`
	AppID     string `json:"app_id,omitempty"`
	AppSecret string `json:"app_secret,omitempty"`
	Enabled   *bool  `json:"enabled,omitempty"`
}

func (s *ConfigService) Update(ctx context.Context, id string, req UpdateConfigRequest) error {
	cfg, err := s.repo.Get(ctx, id)
	if err != nil {
		return err
	}
	if req.Name != "" {
		cfg.Name = req.Name
	}
	if req.AppID != "" {
		cfg.AppID = req.AppID
	}
	if req.AppSecret != "" {
		if err := s.storeSecret(ctx, cfg, req.AppSecret); err != nil {
			return err
		}
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
	cfg, err := s.repo.FindBySession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	s.resolveSecret(ctx, cfg)
	return cfg, nil
}

// AllEnabled returns all enabled configs.
func (s *ConfigService) AllEnabled(ctx context.Context) ([]*feishu.Config, error) {
	list, err := s.repo.AllEnabled(ctx)
	if err != nil {
		return nil, err
	}
	for _, cfg := range list {
		s.resolveSecret(ctx, cfg)
	}
	return list, nil
}
