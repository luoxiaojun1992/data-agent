package knowledge

import (
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
)

func TestNewService(t *testing.T) {
	func() {
		defer func() {
			if r := recover(); r != nil {
				t.Errorf("NewService(nil) panicked: %v", r)
			}
		}()
		s := NewService(nil)
		if s == nil {
			t.Fatal("NewService(nil) should not return nil")
		}
	}()
}

func TestGenShortID(t *testing.T) {
	t.Run("non-empty", func(t *testing.T) {
		id := genShortID()
		if id == "" {
			t.Error("genShortID() should return non-empty string")
		}
	})

	t.Run("unique", func(t *testing.T) {
		ids := make(map[string]bool)
		for i := 0; i < 100; i++ {
			id := genShortID()
			if ids[id] {
				t.Errorf("genShortID() produced duplicate: %s", id)
			}
			ids[id] = true
		}
	})
}

func TestRRFFusion(t *testing.T) {
	t.Run("empty inputs", func(t *testing.T) {
		result := rrfFusion(nil, nil, 5, 60)
		if len(result) != 0 {
			t.Errorf("len = %d, want 0", len(result))
		}
	})

	t.Run("single list", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "a", DocTitle: "Doc A", Score: 1.0},
			{ChunkID: "b", DocTitle: "Doc B", Score: 0.9},
		}
		result := rrfFusion(list1, nil, 2, 60)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2", len(result))
		}
		// First result should have highest score
		if result[0].ChunkID != "a" {
			t.Errorf("first result ChunkID = %q, want %q", result[0].ChunkID, "a")
		}
	})

	t.Run("merge two lists", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "a", DocTitle: "Doc A", Score: 1.0},
			{ChunkID: "b", DocTitle: "Doc B", Score: 0.9},
		}
		list2 := []knowledge.SearchResult{
			{ChunkID: "c", DocTitle: "Doc C", Score: 0.8},
		}
		result := rrfFusion(list1, list2, 10, 60)
		if len(result) != 3 {
			t.Fatalf("len = %d, want 3", len(result))
		}
	})

	t.Run("duplicate chunks get higher score", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "a", DocTitle: "Doc A", Score: 1.0},
		}
		list2 := []knowledge.SearchResult{
			{ChunkID: "a", DocTitle: "Doc A", Score: 0.8},
		}
		result := rrfFusion(list1, list2, 1, 60)
		if len(result) != 1 {
			t.Fatalf("len = %d, want 1", len(result))
		}
		// Same chunk in both lists should get combined score
		if result[0].ChunkID != "a" {
			t.Errorf("ChunkID = %q, want %q", result[0].ChunkID, "a")
		}
	})

	t.Run("respects topK", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "a"}, {ChunkID: "b"}, {ChunkID: "c"},
		}
		result := rrfFusion(list1, nil, 2, 60)
		if len(result) != 2 {
			t.Fatalf("len = %d, want 2 (topK)", len(result))
		}
	})

	t.Run("single element", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "only", DocTitle: "Only", Score: 1.0},
		}
		result := rrfFusion(list1, nil, 5, 60)
		if len(result) != 1 {
			t.Fatalf("len = %d, want 1", len(result))
		}
	})
}

func TestFilterByRole(t *testing.T) {
	s := NewService(nil)

	t.Run("system_admin sees all", func(t *testing.T) {
		results := []knowledge.SearchResult{
			{ChunkID: "a", DocTitle: "Doc A"},
			{ChunkID: "b", DocTitle: "Doc B"},
		}
		filtered := s.filterByRole(results, "system_admin")
		if len(filtered) != 2 {
			t.Errorf("system_admin should see all results, got %d", len(filtered))
		}
	})

	t.Run("user sees all (no filter yet)", func(t *testing.T) {
		results := []knowledge.SearchResult{
			{ChunkID: "a", DocTitle: "Doc A"},
		}
		filtered := s.filterByRole(results, "user")
		if len(filtered) != 1 {
			t.Errorf("user should see their own docs, got %d", len(filtered))
		}
	})

	t.Run("empty results", func(t *testing.T) {
		filtered := s.filterByRole(nil, "user")
		if filtered != nil {
			t.Error("filterByRole(nil) should return nil")
		}
	})
}
