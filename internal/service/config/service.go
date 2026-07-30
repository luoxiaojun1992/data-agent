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

func (s *service) GetAll(ctx context.Context, namespace string) ([]model.SystemConfig, error) {
	cfgs, err := s.sysConfig.GetAll(ctx, namespace)
	if err != nil {
		return nil, err
	}
	if cfgs == nil {
		cfgs = []model.SystemConfig{}
	}
	// Merge built-in defaults for the "system" namespace only.
	if namespace == "system" {
		savedMap := make(map[string]bool, len(cfgs))
		for _, c := range cfgs {
			savedMap[c.Key] = true
		}
		for _, b := range SystemBuiltins() {
			if !savedMap[b.Key] {
				cfgs = append(cfgs, model.SystemConfig{
					Namespace: namespace,
					Key:       b.Key,
					Value:     b.Default,
				})
			}
		}
	}
	return cfgs, nil
}

// SeedBuiltins ensures every built-in config key exists in the database
// without overwriting any user-modified values. Safe to call on every startup.
func (s *service) SeedBuiltins(ctx context.Context) error {
	existing, err := s.sysConfig.GetAll(ctx, "system")
	if err != nil {
		return fmt.Errorf("seed: list system configs: %w", err)
	}
	existMap := make(map[string]bool, len(existing))
	for _, c := range existing {
		existMap[c.Key] = true
	}
	for _, b := range SystemBuiltins() {
		if existMap[b.Key] {
			continue // already exists — never overwrite user-modified values
		}
		if err := s.sysConfig.Upsert(ctx, "system", b.Key, b.Default); err != nil {
			log.Printf("[config] seed %s: %v", b.Key, err)
		}
	}
	return nil
}

func (s *service) Upsert(ctx context.Context, namespace, key, value string) error {
	return s.sysConfig.Upsert(ctx, namespace, key, value)
}

func (s *service) Delete(ctx context.Context, namespace, key string) error {
	return s.sysConfig.Delete(ctx, namespace, key)
}
