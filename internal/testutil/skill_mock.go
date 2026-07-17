package testutil

import "github.com/luoxiaojun1992/data-agent/internal/domain/skill"

// MockSkill is a simple mock implementation of skill.Skill for testing.
type MockSkill struct {
	NameVal        string
	DescVal        string
	ParamsVal      []skill.Parameter
	PermsVal       []string
	RateLimitVal   *skill.RateLimitConfig
	ExecuteFn      func(ctx skill.SkillContext, params map[string]any) (any, error)
}

func (m *MockSkill) Name() string                           { return m.NameVal }
func (m *MockSkill) Description() string                    { return m.DescVal }
func (m *MockSkill) Parameters() []skill.Parameter          { return m.ParamsVal }
func (m *MockSkill) Permissions() []string                  { return m.PermsVal }
func (m *MockSkill) RateLimit() *skill.RateLimitConfig      { return m.RateLimitVal }
func (m *MockSkill) Execute(ctx skill.SkillContext, params map[string]any) (any, error) {
	if m.ExecuteFn != nil {
		return m.ExecuteFn(ctx, params)
	}
	return nil, nil
}
