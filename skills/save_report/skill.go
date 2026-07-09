package skill

import (
	"fmt"

	reportpkg "github.com/luoxiaojun1992/data-agent/internal/logic/report"
	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

// SaveReport implements skill.Skill for saving and validating analysis reports.
type SaveReport struct{}

func (s *SaveReport) Name() string        { return "save_report" }
func (s *SaveReport) Description() string { return "Validates and saves analysis reports, ensuring mandatory sections are present" }

func (s *SaveReport) Parameters() []skilldomain.Parameter {
	return []skilldomain.Parameter{
		{Name: "title", Type: "string", Description: "Report title", Required: true},
		{Name: "content", Type: "string", Description: "Report content in markdown format", Required: true},
		{Name: "validate", Type: "boolean", Description: "Whether to validate mandatory sections (default: true)", Required: false},
	}
}

func (s *SaveReport) Permissions() []string {
	return []string{"skill:save_report"}
}

func (s *SaveReport) RateLimit() *skilldomain.RateLimitConfig {
	return &skilldomain.RateLimitConfig{MaxRequests: 20, WindowSec: 60}
}

func (s *SaveReport) Execute(ctx skilldomain.SkillContext, params map[string]any) (any, error) {
	title, ok := params["title"].(string)
	if !ok || title == "" {
		return nil, fmt.Errorf("save_report: missing required parameter 'title'")
	}

	content, ok := params["content"].(string)
	if !ok || content == "" {
		return nil, fmt.Errorf("save_report: missing required parameter 'content'")
	}

	shouldValidate := true
	if raw, ok := params["validate"].(bool); ok {
		shouldValidate = raw
	}

	result := map[string]any{
		"title":  title,
		"status": "saved",
	}

	if shouldValidate {
		validation := reportpkg.Validate(content)
		result["valid"] = validation.Valid
		result["detected_sections"] = validation.DetectedSections

		if !validation.Valid {
			result["missing_sections"] = validation.MissingSections
			result["feedback"] = validation.Feedback
			result["status"] = "validation_failed"
		}
	}

	return result, nil
}
