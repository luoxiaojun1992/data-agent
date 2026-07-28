package repository

import (
	"context"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

// SkillConfigRepository persists and retrieves skill configurations.
type SkillConfigRepository interface {
	List(ctx context.Context) ([]skill.SkillConfig, error)
	Get(ctx context.Context, name string) (*skill.SkillConfig, error)
	Upsert(ctx context.Context, cfg skill.SkillConfig) error
}
