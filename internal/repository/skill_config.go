package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

// SkillConfigRepository persists and retrieves skill configurations.
type SkillConfigRepository interface {
	List(ctx context.Context, skip, limit int64) ([]skill.SkillConfig, error)
	Count(ctx context.Context) (int64, error)
	Get(ctx context.Context, name string) (*skill.SkillConfig, error)
	// SearchByDescription returns up to `limit` enabled skills whose
	// description matches the keyword (regex, case-insensitive). Only the
	// description field is matched — name and display_name are ignored.
	SearchByDescription(ctx context.Context, keyword string, limit int) ([]skill.SkillConfig, error)
	Upsert(ctx context.Context, cfg skill.SkillConfig) error
}
