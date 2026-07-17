package report

import (
	"strings"
	"testing"
)

func TestMandatorySections(t *testing.T) {
	sections := MandatorySections()
	if len(sections) != 5 {
		t.Errorf("MandatorySections() len=%d, want 5", len(sections))
	}

	expected := map[string]int{
		"摘要": 1,
		"数据来源": 2,
		"分析方法": 2,
		"关键指标": 2,
		"结论": 1,
	}

	for _, s := range sections {
		level, ok := expected[s.Name]
		if !ok {
			t.Errorf("unexpected section: %s", s.Name)
			continue
		}
		if s.Level != level {
			t.Errorf("section %s: level=%d, want %d", s.Name, s.Level, level)
		}
	}
}

func TestValidate(t *testing.T) {
	tests := []struct {
		name        string
		content     string
		wantValid   bool
		wantMissing []string
	}{
		{
			name: "all sections present",
			content: `# 摘要
这是摘要内容。
## 数据来源
数据来自内部系统。
## 分析方法
使用回归分析。
## 关键指标
- 指标1: 100
- 指标2: 200
# 结论
总体趋势向好。`,
			wantValid: true,
		},
		{
			name: "missing 结论",
			content: `# 摘要
简短摘要。
## 数据来源
来源说明。
## 分析方法
方法说明。
## 关键指标
一些指标。`,
			wantValid:   false,
			wantMissing: []string{"结论"},
		},
		{
			name: "missing multiple sections",
			content: `# 摘要
只有摘要。`,
			wantValid:   false,
			wantMissing: []string{"数据来源", "分析方法", "关键指标", "结论"},
		},
		{
			name:      "empty content",
			content:   "",
			wantValid: false,
		},
		{
			name: "partial name match still finds sections",
			content: `# 摘要
内容。
## 数据来源说明
来源。
## 分析方法详细说明
方法。
## 关键指标列表
指标。
# 结论
完了。`,
			wantValid: true,
		},
		{
			name: "sections detected at different heading levels",
			content: `## 摘要
## 数据来源
### 分析方法
#### 关键指标
# 结论`,
			wantValid: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := Validate(tt.content)
			if result.Valid != tt.wantValid {
				t.Errorf("Validate().Valid = %v, want %v (missing: %v)", result.Valid, tt.wantValid, result.MissingSections)
			}
			if !tt.wantValid && tt.wantMissing != nil {
				for _, want := range tt.wantMissing {
					found := false
					for _, missing := range result.MissingSections {
						if strings.Contains(missing, want) {
							found = true
							break
						}
					}
					if !found {
						t.Errorf("expected missing section %q not found in %v", want, result.MissingSections)
					}
				}
			}
		})
	}
}

func TestValidate_FeedbackNotEmptyWhenInvalid(t *testing.T) {
	result := Validate("empty")
	if result.Valid {
		t.Fatal("should be invalid for missing all sections")
	}
	if result.Feedback == "" {
		t.Error("Feedback should not be empty when invalid")
	}
}

func TestValidate_DetectedSectionsNotEmpty(t *testing.T) {
	content := `# 摘要
内容。
## 数据来源
来源。`
	result := Validate(content)
	if len(result.DetectedSections) == 0 {
		t.Error("DetectedSections should not be empty")
	}
}
