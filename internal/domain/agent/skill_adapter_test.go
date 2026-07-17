package agent

import (
	"context"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

func TestNewSkillRegistryFromDomain(t *testing.T) {
	reg := skill.NewRegistry()
	a := NewSkillRegistryFromDomain(reg)
	if a == nil {
		t.Fatal("should return non-nil adapter")
	}
}

func TestSkillRegistryAdapter_Get_NotFound(t *testing.T) {
	reg := skill.NewRegistry()
	a := NewSkillRegistryFromDomain(reg)
	_, err := a.Get("nonexistent")
	if err == nil {
		t.Fatal("should error for nonexistent skill")
	}
}

func TestSkillRegistryAdapter_Get_Found(t *testing.T) {
	reg := skill.NewRegistry()
	_ = reg.Register(&testSkill{name: "sql_executor"})
	a := NewSkillRegistryFromDomain(reg)

	exec, err := a.Get("sql_executor")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if exec.Name() != "sql_executor" {
		t.Errorf("Name: got %s", exec.Name())
	}
}

func TestSkillRegistryAdapter_List(t *testing.T) {
	reg := skill.NewRegistry()
	_ = reg.Register(&testSkill{name: "sql"})
	_ = reg.Register(&testSkill{name: "stats"})
	a := NewSkillRegistryFromDomain(reg)

	list := a.List()
	if len(list) != 2 {
		t.Errorf("got %d skills, want 2", len(list))
	}
}

func TestSkillRegistryAdapter_List_Empty(t *testing.T) {
	reg := skill.NewRegistry()
	a := NewSkillRegistryFromDomain(reg)

	list := a.List()
	if len(list) != 0 {
		t.Errorf("got %d skills, want 0", len(list))
	}
}

func TestSkillExecutorWrapper_Execute(t *testing.T) {
	reg := skill.NewRegistry()
	_ = reg.Register(&testSkill{name: "test_skill"})
	a := NewSkillRegistryFromDomain(reg)

	exec, err := a.Get("test_skill")
	if err != nil {
		t.Fatalf("Get: %v", err)
	}

	result, err := exec.Execute(context.Background(), map[string]any{"key": "value"})
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result != nil {
		t.Logf("Execute result: %v", result)
	}
}

// testSkill implements skill.Skill for testing
type testSkill struct {
	name string
}

func (s *testSkill) Name() string                         { return s.name }
func (s *testSkill) Description() string                   { return "test skill" }
func (s *testSkill) Parameters() []skill.Parameter         { return nil }
func (s *testSkill) Execute(ctx skill.SkillContext, params map[string]any) (any, error) {
	return nil, nil
}
func (s *testSkill) Permissions() []string     { return nil }
func (s *testSkill) RateLimit() *skill.RateLimitConfig { return nil }
