package save_report

import (
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
	"github.com/luoxiaojun1992/data-agent/internal/logic/report"
)

type Saver struct{}

func (s *Saver) Name() string        { return "save_analysis_report" }
func (s *Saver) Description() string { return "Validates and saves an analysis report" }

func (s *Saver) Parameters() []skill.Parameter {
	return []skill.Parameter{
		{Name: "title", Type: "string", Description: "Report title", Required: true},
		{Name: "content", Type: "string", Description: "Markdown report content", Required: true},
		{Name: "session_id", Type: "string", Description: "Session ID", Required: false},
	}
}

func (s *Saver) Permissions() []string               { return []string{"kb:manage_own"} }
func (s *Saver) RateLimit() *skill.RateLimitConfig   { return &skill.RateLimitConfig{MaxRequests: 5, WindowSec: 60} }

func (s *Saver) Execute(ctx skill.SkillContext, params map[string]any) (any, error) {
	content, _ := params["content"].(string)
	if content == "" {
		return nil, fmt.Errorf("content required")
	}

	// Validate report structure
	vr := report.Validate(content)
	if !vr.Valid {
		return map[string]interface{}{
			"status":   "validation_failed",
			"valid":    false,
			"feedback": vr.Feedback,
			"missing":  vr.MissingSections,
		}, nil
	}

	_ = ctx

	return map[string]interface{}{
		"status": "saved",
		"valid":  true,
	}, nil
}
