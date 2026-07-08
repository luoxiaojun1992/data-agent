package skill

import (
	"fmt"
	"sync"
)

// Registry manages skill registration and discovery.
// Skills are loaded at startup and cannot be hot-reloaded.
type Registry struct {
	mu     sync.RWMutex
	skills map[string]Skill
}

// NewRegistry creates an empty skill registry.
func NewRegistry() *Registry {
	return &Registry{
		skills: make(map[string]Skill),
	}
}

// Register adds a skill to the registry. Returns error if skill name is already taken.
func (r *Registry) Register(s Skill) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	name := s.Name()
	if _, exists := r.skills[name]; exists {
		return fmt.Errorf("skill %q already registered", name)
	}
	r.skills[name] = s
	return nil
}

// Get retrieves a skill by name.
func (r *Registry) Get(name string) (Skill, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	s, exists := r.skills[name]
	if !exists {
		return nil, fmt.Errorf("skill %q not found", name)
	}
	return s, nil
}

// List returns all registered skill names.
func (r *Registry) List() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	names := make([]string, 0, len(r.skills))
	for name := range r.skills {
		names = append(names, name)
	}
	return names
}

// Search finds skills whose name or description contains the query.
func (r *Registry) Search(query string) []Skill {
	r.mu.RLock()
	defer r.mu.RUnlock()

	var results []Skill
	for _, s := range r.skills {
		if contains(s.Name(), query) || contains(s.Description(), query) {
			results = append(results, s)
		}
	}
	return results
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && searchSubstring(s, substr)
}

func searchSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
