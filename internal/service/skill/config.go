package skill

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"strings"

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
			Name:        "external_api_search",
			DisplayName: "外部 API 搜索",
			Description: "模糊搜索已审批的 API 集合描述",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "external_api_summary",
			DisplayName: "外部 API 概览",
			Description: "查看某个 API 集合下的所有方法列表",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "external_api_method",
			DisplayName: "外部 API 方法详情",
			Description: "查询某个具体 API 方法的入参/出参",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "external_api_call",
			DisplayName: "外部 API 调用",
			Description: "调用外部 API 方法透（参数",
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
			Name:        "knowledge_graph_search",
			DisplayName: "知识图谱搜索",
			Description: "查询知识图谱中与某切片/概念最相关的 topN 节点及关系",
			Enabled:     true,
			ConfigJSON:  `{"top_n":5,"max_top_n":20}`,
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
		{
			Name:        "invoke_subagent",
			DisplayName: "子 Agent 委派",
			Description: "将子任务委派给独立的子 agent 执行（独立 session、同模型、并行委派支持）；子 agent 的 save_artifact/文件操作透明落到父会话上下文，返回即销毁",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "pptx_generator",
			DisplayName: "PPTX 生成",
			Description: "从 markdown 内容生成 .pptx PowerPoint 文件，保存到 session workspace",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "save_artifact",
			DisplayName: "Artifact 保存",
			Description: "将 session workspace 中的文件或目录打包为 artifact 持久化存储（zip + SeaweedFS + DB 记录）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "file_write",
			DisplayName: "文件写入",
			Description: "在 session workspace 内写入/覆盖文本文件（父目录须已存在，不自动创建）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "dir_create",
			DisplayName: "文件夹创建",
			Description: "在 session workspace 内递归创建文件夹",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "file_delete",
			DisplayName: "文件删除",
			Description: "删除 session workspace 内的单个文件（拒绝目录）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "dir_delete",
			DisplayName: "文件夹删除",
			Description: "递归删除 session workspace 内的文件夹（含所有子目录和文件）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "file_read",
			DisplayName: "文件查看",
			Description: "按行数范围读取 session workspace 内的文件（默认前 10 行，禁止一次返回全部）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "dir_list",
			DisplayName: "文件夹查看",
			Description: "列出 session workspace 内文件夹的当前层级内容（不递归）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "ask_user",
			DisplayName: "用户提问",
			Description: "通过人机交互信道向用户提问（可带候选选项），阻塞等待用户回复后继续（选项或自由文本）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "web_search",
			DisplayName: "联网搜索",
			Description: "通过 Bing/Baidu API 进行联网搜索。需在 Config 中配置 bing_api_key 或 baidu_api_key，未配置时降级返回空结果",
			Enabled:     true,
			ConfigJSON:  `{"bing_api_key":"","baidu_api_key":"","top_n":5}`,
		},
		{
			Name:        "web_fetch",
			DisplayName: "网页抓取",
			Description: "抓取指定 URL 的网页内容并提取纯文本（仅支持 GET 请求）",
			Enabled:     true,
			ConfigJSON:  `{"max_chars":8000,"max_body_size":524288,"timeout_sec":10}`,
		},
		{
			Name:        "skill_search",
			DisplayName: "Skill 搜索",
			Description: "按关键词模糊搜索已启用的 skill，只匹配描述字段，返回 skill 名字/显示名/描述的列表（不含详细配置）",
			Enabled:     true,
			ConfigJSON:  "{}",
		},
		{
			Name:        "skill_detail",
			DisplayName: "Skill 详情",
			Description: "根据 skill 名字（非显示名）精确获取某个 skill 的完整详细配置，含 enabled 状态和 config_json",
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

// SearchByDescription returns up to `topN` enabled skills whose description
// matches the keyword. Only the description field is matched (not name or
// display_name).
func (s *ConfigService) SearchByDescription(ctx context.Context, keyword string, topN int) ([]skill.SkillConfig, error) {
	if strings.TrimSpace(keyword) == "" {
		return []skill.SkillConfig{}, nil
	}
	if topN <= 0 {
		topN = 5
	}
	if topN > 20 {
		topN = 20
	}
	return s.repo.SearchByDescription(ctx, keyword, topN)
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
