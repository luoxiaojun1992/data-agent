package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"log"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	sqlpkg "github.com/luoxiaojun1992/data-agent/internal/logic/sql"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// ConfigService manages skill configurations with per-skill validation.
type ConfigService struct {
	repo repository.SkillConfigRepository
}

// NewConfigService creates a skill config service.
func NewConfigService(repo repository.SkillConfigRepository) *ConfigService {
	return &ConfigService{repo: repo}
}

// predefinedSkills returns the built-in skill definitions.
func predefinedSkills() []skill.SkillConfig {
	return []skill.SkillConfig{
		{
			Name:        "sql_executor",
			DisplayName: "SQL 执行器",
			Description: "校验并执行安全的 SQL SELECT 查询，支持参数化查询",
			Enabled:     true,
			ConfigJSON:  `{"dsn":"","max_rows":100,"query_timeout":30000}`,
		},
		{
			Name:        "stats_compute",
			DisplayName: "统计分析",
			Description: "对数值数组进行描述性统计、线性回归、时间序列分解",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "knowledge_search",
			DisplayName: "知识库搜索",
			Description: "结合全文索引和语义向量的混合搜索",
			Enabled:     true,
			ConfigJSON:  `{"max_results":50}`,
		},
		{
			Name:        "memory_search",
			DisplayName: "记忆搜索",
			Description: "搜索长期记忆中的历史对话和分析结果",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "memory_write",
			DisplayName: "记忆写入",
			Description: "将重要信息写入长期记忆供后续检索",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "save_task_result",
			DisplayName: "任务结果保存",
			Description: "异步/定时任务结束时强制调用以保存分析结果（task_id 从 session 自动注入）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
	}
}

// SeedSkills ensures every predefined skill exists in the database.
// If a skill already exists (e.g. user-modified), it is left untouched.
// Safe to call on every startup.
func (s *ConfigService) SeedSkills(ctx context.Context) error {
	saved, err := s.repo.List(ctx, 0, 0) // 0,0 = no pagination, fetch all
	if err != nil {
		return fmt.Errorf("seed: list skills: %w", err)
	}
	existMap := make(map[string]bool, len(saved))
	for _, sk := range saved {
		existMap[sk.Name] = true
	}
	for _, sk := range predefinedSkills() {
		if existMap[sk.Name] {
			continue
		}
		if err := s.repo.Upsert(ctx, sk); err != nil {
			log.Printf("[skill] seed %s: %v", sk.Name, err)
		}
	}
	return nil
}

// List returns paginated skill configs from the database.
// Predefined defaults are seeded on startup via SeedSkills.
func (s *ConfigService) List(ctx context.Context, page, pageSize int) ([]skill.SkillConfig, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 20
	}
	skip := int64((page - 1) * pageSize)
	limit := int64(pageSize)

	saved, err := s.repo.List(ctx, skip, limit)
	if err != nil {
		return nil, 0, err
	}
	total, err := s.repo.Count(ctx)
	if err != nil {
		return nil, 0, err
	}
	return saved, int(total), nil
}

// Get returns a single skill config directly from the database.
func (s *ConfigService) Get(ctx context.Context, name string) (*skill.SkillConfig, error) {
	cfg, err := s.repo.Get(ctx, name)
	if err != nil {
		return nil, err
	}
	if cfg == nil {
		return nil, fmt.Errorf("unknown skill: %s", name)
	}
	return cfg, nil
}

// Upsert validates and saves a skill config.
func (s *ConfigService) Upsert(ctx context.Context, cfg skill.SkillConfig) error {
	if err := validateConfig(cfg.Name, cfg.ConfigJSON); err != nil {
		return fmt.Errorf("invalid config for %s: %w", cfg.Name, err)
	}
	return s.repo.Upsert(ctx, cfg)
}

// GetConfig unmarshals the JSON config for a skill into the given struct.
func (s *ConfigService) GetConfig(ctx context.Context, name string, target interface{}) error {
	cfg, err := s.Get(ctx, name)
	if err != nil {
		return err
	}
	if cfg.ConfigJSON == "" || cfg.ConfigJSON == "{}" {
		return nil // no custom config
	}
	return json.Unmarshal([]byte(cfg.ConfigJSON), target)
}

// IsEnabled returns true if the skill is enabled.
func (s *ConfigService) IsEnabled(ctx context.Context, name string) bool {
	cfg, err := s.Get(ctx, name)
	if err != nil {
		return false
	}
	return cfg.Enabled
}

// validateConfig validates a skill's JSON config against its schema.
func validateConfig(name string, configJSON string) error {
	if configJSON == "" || configJSON == "{}" {
		return nil // empty is always valid
	}
	switch name {
	case "sql_executor":
		var cfg sqlpkg.ExecConfig
		if err := json.Unmarshal([]byte(configJSON), &cfg); err != nil {
			return fmt.Errorf("json parse error: %w", err)
		}
		// Validate DSN format
		if cfg.MaxRows < 0 {
			return fmt.Errorf("max_rows must be >= 0")
		}
		if cfg.QueryTimeout < 0 {
			return fmt.Errorf("query_timeout must be >= 0")
		}
		return nil
	default:
		// Generic: just validate it's valid JSON
		var m map[string]interface{}
		return json.Unmarshal([]byte(configJSON), &m)
	}
}
