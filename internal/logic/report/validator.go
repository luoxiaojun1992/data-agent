package report

import (
	"strings"
)

// RequiredSection defines a mandatory report section.
type RequiredSection struct {
	Name        string `json:"name"`
	Level       int    `json:"level"` // Markdown heading level (1-3)
	Description string `json:"description"`
}

// ValidationResult holds the result of report validation.
type ValidationResult struct {
	Valid            bool     `json:"valid"`
	MissingSections  []string `json:"missing_sections"`
	Feedback         string   `json:"feedback"`
	DetectedSections []string `json:"detected_sections"`
}

// MandatorySections returns the list of required report sections.
func MandatorySections() []RequiredSection {
	return []RequiredSection{
		{Name: "摘要", Level: 1, Description: "报告核心发现和结论的简要总结"},
		{Name: "数据来源", Level: 2, Description: "分析所用数据的来源、时间范围、数据量"},
		{Name: "分析方法", Level: 2, Description: "使用的统计方法、模型和分析工具"},
		{Name: "关键指标", Level: 2, Description: "核心分析指标和数值结果"},
		{Name: "结论", Level: 1, Description: "分析结论和建议"},
	}
}

// Validate checks if a markdown report contains all mandatory sections.
func Validate(content string) *ValidationResult {
	detected := extractSections(content)
	required := MandatorySections()

	result := &ValidationResult{
		Valid:            true,
		DetectedSections: detected,
	}

	var missing []string
	for _, rs := range required {
		found := false
		for _, ds := range detected {
			if strings.Contains(strings.ToLower(ds), strings.ToLower(rs.Name)) {
				found = true
				break
			}
		}
		if !found {
			missing = append(missing, rs.Name)
		}
	}

	if len(missing) > 0 {
		result.Valid = false
		result.MissingSections = missing
		result.Feedback = generateFeedback(missing, required)
	}

	return result
}

// extractSections parses markdown headings (H1-H4) from content.
func extractSections(content string) []string {
	var headings []string
	lines := strings.Split(content, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			// Remove # prefix and trim
			for strings.HasPrefix(trimmed, "#") {
				trimmed = strings.TrimPrefix(trimmed, "#")
			}
			section := strings.TrimSpace(trimmed)
			if section != "" {
				headings = append(headings, section)
			}
		}
	}
	return headings
}

// generateFeedback produces user-friendly feedback for missing sections.
func generateFeedback(missing []string, required []RequiredSection) string {
	var sb strings.Builder
	sb.WriteString("报告校验失败，缺少以下必含章节：\n\n")

	for _, m := range missing {
		sb.WriteString("- **" + m + "**")
		for _, r := range required {
			if r.Name == m {
				sb.WriteString("： " + r.Description)
				break
			}
		}
		sb.WriteString("\n")
	}

	sb.WriteString("\n示例格式：\n")
	sb.WriteString("```markdown\n")
	for _, r := range required {
		prefix := strings.Repeat("#", r.Level)
		sb.WriteString(prefix + " " + r.Name + "\n\n" + r.Description + "\n\n")
	}
	sb.WriteString("```\n")

	return sb.String()
}
