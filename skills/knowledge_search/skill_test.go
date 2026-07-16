package skill

import (
	"testing"

	skilldomain "github.com/luoxiaojun1992/data-agent/internal/domain/skill"
)

func TestKnowledgeSearch_Name(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	if got := s.Name(); got != "knowledge_search" {
		t.Errorf("Name() = %q, want %q", got, "knowledge_search")
	}
}

func TestKnowledgeSearch_Description(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	desc := s.Description()
	if desc == "" {
		t.Error("Description() should not be empty")
	}
}

func TestKnowledgeSearch_Parameters(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	params := s.Parameters()
	if len(params) < 1 {
		t.Fatal("Parameters() should return at least 1 parameter")
	}
	if params[0].Name != "query" {
		t.Errorf("first param name = %q, want %q", params[0].Name, "query")
	}
	if !params[0].Required {
		t.Error("'query' parameter should be required")
	}
}

func TestKnowledgeSearch_Permissions(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	perms := s.Permissions()
	if len(perms) == 0 {
		t.Error("Permissions() should not be empty")
	}
	found := false
	for _, p := range perms {
		if p == "skill:knowledge_search" {
			found = true
			break
		}
	}
	if !found {
		t.Error("Permissions() should contain 'skill:knowledge_search'")
	}
}

func TestKnowledgeSearch_RateLimit(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	rl := s.RateLimit()
	if rl == nil {
		t.Fatal("RateLimit() should not be nil")
	}
	if rl.MaxRequests != 30 {
		t.Errorf("MaxRequests = %d, want 30", rl.MaxRequests)
	}
	if rl.WindowSec != 60 {
		t.Errorf("WindowSec = %d, want 60", rl.WindowSec)
	}
}

func TestNewKnowledgeSearch(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	if s == nil {
		t.Fatal("NewKnowledgeSearch() should not return nil")
	}
}

func TestKnowledgeSearch_Execute_MissingQuery(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{})
	if err == nil {
		t.Error("Execute() should return error for missing 'query'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestKnowledgeSearch_Execute_EmptyQuery(t *testing.T) {
	s := NewKnowledgeSearch(nil)
	ctx := skilldomain.SkillContext{}
	result, err := s.Execute(ctx, map[string]any{"query": ""})
	if err == nil {
		t.Error("Execute() should return error for empty 'query'")
	}
	if result != nil {
		t.Error("Execute() should return nil result on error")
	}
}

func TestTruncateContent(t *testing.T) {
	t.Run("shorter than max", func(t *testing.T) {
		result := truncateContent("hello", 10)
		if result != "hello" {
			t.Errorf("truncateContent() = %q, want %q", result, "hello")
		}
	})

	t.Run("exactly max length", func(t *testing.T) {
		result := truncateContent("hello", 5)
		if result != "hello" {
			t.Errorf("truncateContent() = %q, want %q", result, "hello")
		}
	})

	t.Run("longer than max", func(t *testing.T) {
		result := truncateContent("hello world this is long", 10)
		if len(result) != 13 { // 10 chars + "..."
			t.Errorf("len(truncateContent()) = %d, want 13", len(result))
		}
		if result[len(result)-3:] != "..." {
			t.Errorf("truncated content should end with '...'")
		}
	})

	t.Run("empty content", func(t *testing.T) {
		result := truncateContent("", 10)
		if result != "" {
			t.Errorf("truncateContent() = %q, want %q", result, "")
		}
	})

	t.Run("zero maxLen", func(t *testing.T) {
		result := truncateContent("hello", 0)
		if result != "..." {
			t.Errorf("truncateContent() with maxLen=0 = %q, want %q", result, "...")
		}
	})

	t.Run("trim whitespace in truncated", func(t *testing.T) {
		result := truncateContent("hello world   ", 10)
		// Should trim trailing spaces before appending ...
		if len(result) <= 3+3 {
			t.Errorf("truncateContent() = %q, expected trimmed content + '...'", result)
		}
	})
}
