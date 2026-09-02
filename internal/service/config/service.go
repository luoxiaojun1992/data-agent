package config

import (
	"context"
	"fmt"
	"log"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// BuiltinConfig defines a built-in system configuration key with its
// default value. These are seeded into MongoDB on startup so the settings
// page has data even after a fresh deployment.
type BuiltinConfig struct {
	Key         string
	Description string
	Default     string
}

// SystemBuiltins returns all built-in system configuration entries.
// Adding a key here makes it appear in the admin settings page.
func SystemBuiltins() []BuiltinConfig {
	return []BuiltinConfig{
		{Key: "MONGO_URI", Description: "MongoDB 连接 URI", Default: ""},
		{Key: "REDIS_ADDR", Description: "Redis 地址 (host:port)", Default: ""},
		{Key: "QDRANT_URL", Description: "Qdrant HTTP URL", Default: ""},
		{Key: "INVITE_HMAC_SECRET", Description: "邀请 HMAC 签名密钥", Default: ""},
		{Key: "INVITE_BASE_URL", Description: "邀请链接对外基地址", Default: ""},
		{Key: "VAULT_ADDR", Description: "HashiCorp Vault 地址", Default: ""},
		{Key: "JWT_SECRET", Description: "JWT 签名密钥", Default: ""},
		{Key: "SESSION_TIMEOUT", Description: "登录 Session 超时（小时）", Default: "24"},
		{Key: "SERVER_READ_TIMEOUT", Description: "HTTP 读超时（秒）", Default: "600"},
		{Key: "SERVER_WRITE_TIMEOUT", Description: "HTTP 写超时（秒）", Default: "600"},
		{Key: "WORKER_POOL_SIZE", Description: "Worker 协程池大小", Default: "10"},
		{Key: "pii_redaction_enabled", Description: "PII 脱敏开关（Presidio，知识库/模型输入输出）", Default: "true"},
		{Key: "embedding_cache_ttl", Description: "Embedding 缓存 TTL（秒）", Default: "3600"},
	}
}

// service implements Service.
type service struct {
	sysConfig repository.SysConfigRepository
}

// NewService creates a system configuration service.
func NewService(sysConfig repository.SysConfigRepository) Service {
	return &service{sysConfig: sysConfig}
}

var _ Service = (*service)(nil)

func (s *service) GetAll(ctx context.Context) ([]model.SystemConfig, error) {
	cfgs, err := s.sysConfig.GetAll(ctx)
	if err != nil {
		return nil, err
	}
	if cfgs == nil {
		return []model.SystemConfig{}, nil
	}
	return cfgs, nil
}

// List returns a paginated page of configs. All built-ins are seeded into DB
// at startup so no runtime merge is needed — the response is pure DB data.
func (s *service) List(ctx context.Context, page, pageSize int) ([]model.SystemConfig, int64, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	cfgs, err := s.sysConfig.List(ctx, skip, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.sysConfig.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	return cfgs, total, nil
}

// SeedBuiltins ensures every built-in config key exists in the database with
// its description, without overwriting any user-modified values. Safe to call
// on every startup.
func (s *service) SeedBuiltins(ctx context.Context) error {
	existing, err := s.sysConfig.GetAll(ctx)
	if err != nil {
		return fmt.Errorf("seed: list system configs: %w", err)
	}
	existMap := make(map[string]string, len(existing))
	for _, c := range existing {
		// Skip legacy empty-key rows (no business key → can't link to a builtin).
		if c.Key == "" {
			log.Printf("[config] seed: dropping legacy empty-key row id=%s", c.ID)
			if err := s.sysConfig.Delete(ctx, c.Key); err != nil {
				log.Printf("[config] seed: drop empty-key row: %v", err)
			}
			continue
		}
		existMap[c.Key] = c.Value
	}
	for _, b := range SystemBuiltins() {
		if val, ok := existMap[b.Key]; ok {
			// Already exists — keep user-modified value, sync description.
			if err := s.sysConfig.Upsert(ctx, b.Key, val, b.Description); err != nil {
				log.Printf("[config] seed %s: %v", b.Key, err)
			}
			continue
		}
		if err := s.sysConfig.Upsert(ctx, b.Key, b.Default, b.Description); err != nil {
			log.Printf("[config] seed %s: %v", b.Key, err)
		}
	}
	return nil
}

func (s *service) Upsert(ctx context.Context, key, value, description string) error {
	return s.sysConfig.Upsert(ctx, key, value, description)
}

func (s *service) Delete(ctx context.Context, key string) error {
	return s.sysConfig.Delete(ctx, key)
}
