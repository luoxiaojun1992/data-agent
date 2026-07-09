package agent

import (
	"context"
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

// skillRegistryAdapter bridges skill.Registry to agent.SkillRegistry.
// It wraps the domain skill registry so the agent engine can discover
// and execute skills registered in the system.
type skillRegistryAdapter struct {
	registry *skill.Registry
}

// NewSkillRegistryFromDomain creates a SkillRegistry backed by the domain skill registry.
func NewSkillRegistryFromDomain(reg *skill.Registry) SkillRegistry {
	return &skillRegistryAdapter{registry: reg}
}

// Get retrieves a skill by name, wrapping it as a SkillExecutor.
func (a *skillRegistryAdapter) Get(name string) (SkillExecutor, error) {
	s, err := a.registry.Get(name)
	if err != nil {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return &skillExecutorWrapper{skill: s}, nil
}

// List returns all registered skill names.
func (a *skillRegistryAdapter) List() []string {
	return a.registry.List()
}

// skillExecutorWrapper adapts a skill.Skill to the agent.SkillExecutor interface.
type skillExecutorWrapper struct {
	skill skill.Skill
}

func (w *skillExecutorWrapper) Name() string {
	return w.skill.Name()
}

func (w *skillExecutorWrapper) Execute(ctx context.Context, params map[string]any) (any, error) {
	// skill.Skill.Execute requires a skill.SkillContext, but agent.SkillExecutor
	// receives a context.Context. We create a minimal SkillContext.
	sc := skill.NewSkillContext(ctx, "", "", "", "", "")
	return w.skill.Execute(sc, params)
}
