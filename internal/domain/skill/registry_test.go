package skill

import (
	"context"
	"sync"
	"testing"
)

// mockSkill implements Skill for testing.
type mockSkill struct {
	nameVal        string
	descVal        string
	paramsVal      []Parameter
	permsVal       []string
	rateLimitVal   *RateLimitConfig
}

func (m *mockSkill) Name() string                           { return m.nameVal }
func (m *mockSkill) Description() string                    { return m.descVal }
func (m *mockSkill) Parameters() []Parameter                { return m.paramsVal }
func (m *mockSkill) Permissions() []string                  { return m.permsVal }
func (m *mockSkill) RateLimit() *RateLimitConfig            { return m.rateLimitVal }
func (m *mockSkill) Execute(ctx SkillContext, params map[string]any) (any, error) { return nil, nil }

func newMockSkill(name, desc string) *mockSkill {
	return &mockSkill{nameVal: name, descVal: desc}
}

func TestNewRegistry(t *testing.T) {
	r := NewRegistry()
	if r == nil {
		t.Fatal("NewRegistry() returned nil")
	}
}

func TestRegistry_Register(t *testing.T) {
	r := NewRegistry()

	t.Run("register success", func(t *testing.T) {
		s := newMockSkill("skill-1", "first skill")
		if err := r.Register(s); err != nil {
			t.Fatalf("Register() unexpected error: %v", err)
		}
	})

	t.Run("duplicate name returns error", func(t *testing.T) {
		s1 := newMockSkill("dup-skill", "first")
		s2 := newMockSkill("dup-skill", "second")
		if err := r.Register(s1); err != nil {
			t.Fatalf("Register(s1): %v", err)
		}
		err := r.Register(s2)
		if err == nil {
			t.Error("Register() should return error for duplicate name")
		}
	})
}

func TestRegistry_Get(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(newMockSkill("get-test", "test desc")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	t.Run("found", func(t *testing.T) {
		s, err := r.Get("get-test")
		if err != nil {
			t.Fatalf("Get() unexpected error: %v", err)
		}
		if s.Name() != "get-test" {
			t.Errorf("Get() wrong name: got %s", s.Name())
		}
	})

	t.Run("not found", func(t *testing.T) {
		_, err := r.Get("nonexistent")
		if err == nil {
			t.Error("Get() should return error for nonexistent skill")
		}
	})
}

func TestRegistry_List(t *testing.T) {
	r := NewRegistry()

	t.Run("empty", func(t *testing.T) {
		names := r.List()
		if len(names) != 0 {
			t.Errorf("List() on empty: got %d names, want 0", len(names))
		}
	})

	t.Run("with skills", func(t *testing.T) {
		r2 := NewRegistry()
		if err := r2.Register(newMockSkill("a", "Skill A")); err != nil {
			t.Fatalf("Register a: %v", err)
		}
		if err := r2.Register(newMockSkill("b", "Skill B")); err != nil {
			t.Fatalf("Register b: %v", err)
		}
		names := r2.List()
		if len(names) != 2 {
			t.Errorf("List() len: got %d, want 2", len(names))
		}
	})
}

func TestRegistry_Search(t *testing.T) {
	r := NewRegistry()
	if err := r.Register(newMockSkill("sql_executor", "Execute SQL queries")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(newMockSkill("stats_engine", "Statistical analysis")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	if err := r.Register(newMockSkill("email_sender", "Send emails")); err != nil {
		t.Fatalf("Register: %v", err)
	}

	t.Run("exact name match", func(t *testing.T) {
		results := r.Search("sql_executor")
		if len(results) != 1 {
			t.Fatalf("Search exact: got %d results, want 1", len(results))
		}
		if results[0].Name() != "sql_executor" {
			t.Errorf("Search exact: wrong result: %s", results[0].Name())
		}
	})

	t.Run("partial name match", func(t *testing.T) {
		results := r.Search("engine")
		if len(results) != 1 {
			t.Fatalf("Search partial: got %d results, want 1", len(results))
		}
	})

	t.Run("partial description match", func(t *testing.T) {
		results := r.Search("SQL")
		if len(results) != 1 {
			t.Fatalf("Search desc: got %d results, want 1", len(results))
		}
	})

	t.Run("no match", func(t *testing.T) {
		results := r.Search("nonexistent")
		if len(results) != 0 {
			t.Errorf("Search no match: got %d results, want 0", len(results))
		}
	})
}

func TestRegistry_ConcurrentRegistration(t *testing.T) {
	r := NewRegistry()
	var wg sync.WaitGroup
	names := []string{"a", "b", "c", "d", "e"}

	for _, name := range names {
		wg.Add(1)
		go func(n string) {
			defer wg.Done()
			_ = r.Register(newMockSkill(n, "skill "+n))
		}(name)
	}
	wg.Wait()

	listed := r.List()
	if len(listed) != len(names) {
		t.Errorf("concurrent registration: got %d names, want %d", len(listed), len(names))
	}
}

func TestNewSkillContext(t *testing.T) {
	ctx := NewSkillContext(context.Background(), "sess-1", "user-1", "task-1", "trace-1", "admin")
	if ctx.SessionID != "sess-1" {
		t.Errorf("SessionID: got %s, want 'sess-1'", ctx.SessionID)
	}
	if ctx.UserID != "user-1" {
		t.Errorf("UserID: got %s, want 'user-1'", ctx.UserID)
	}
	if ctx.TaskID != "task-1" {
		t.Errorf("TaskID: got %s, want 'task-1'", ctx.TaskID)
	}
	if ctx.TraceID != "trace-1" {
		t.Errorf("TraceID: got %s, want 'trace-1'", ctx.TraceID)
	}
	if ctx.Role != "admin" {
		t.Errorf("Role: got %s, want 'admin'", ctx.Role)
	}
}
