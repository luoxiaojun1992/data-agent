package agent_svc

import (
	"testing"
)

func TestNewService(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	if s == nil {
		t.Fatal("NewService() should not return nil")
	}
}

func TestNewService_ReturnsNonNil(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	if s == nil {
		t.Fatal("NewService returned nil")
	}
	_ = s
}

func TestWithTaskService(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	s = s.WithTaskService(nil)
	if s == nil {
		t.Fatal("WithTaskService() should return non-nil")
	}
	// taskService should be nil since we passed nil
	if s.taskService != nil {
		t.Error("taskService should be nil after WithTaskService(nil)")
	}
}

func TestWithSkillRegistry(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	s = s.WithSkillRegistry(nil)
	if s == nil {
		t.Fatal("WithSkillRegistry() should return non-nil")
	}
	if s.skillReg != nil {
		t.Error("skillReg should be nil after WithSkillRegistry(nil)")
	}
}

func TestWithTaskService_Chaining(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	s2 := s.WithTaskService(nil)
	// Should return same instance for chaining
	if s != s2 {
		t.Error("WithTaskService should return same Service instance")
	}
}

func TestContains(t *testing.T) {
	t.Run("exact match", func(t *testing.T) {
		if !contains("hello world", "hello") {
			t.Error("contains('hello world', 'hello') should be true")
		}
	})

	t.Run("substring", func(t *testing.T) {
		if !contains("sql_executor", "sql") {
			t.Error("contains('sql_executor', 'sql') should be true")
		}
	})

	t.Run("no match", func(t *testing.T) {
		if contains("hello", "xyz") {
			t.Error("contains('hello', 'xyz') should be false")
		}
	})

	t.Run("empty search", func(t *testing.T) {
		if !contains("anything", "") {
			t.Error("contains('anything', '') should be true")
		}
	})

	t.Run("empty string", func(t *testing.T) {
		if contains("", "hello") {
			t.Error("contains('', 'hello') should be false")
		}
	})

	t.Run("same strings", func(t *testing.T) {
		if !contains("hello", "hello") {
			t.Error("contains('hello', 'hello') should be true")
		}
	})

	t.Run("substr longer than s", func(t *testing.T) {
		if contains("hi", "hello") {
			t.Error("contains('hi', 'hello') should be false")
		}
	})

	t.Run("unicode text", func(t *testing.T) {
		if !contains("你好世界", "世界") {
			t.Error("contains('你好世界', '世界') should be true")
		}
	})
}
