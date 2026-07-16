package knowledge

import (
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
)

func TestGenShortID(t *testing.T) {
	id := genShortID()
	if id == "" {
		t.Error("genShortID should not return empty")
	}
}

func TestRRFFusion(t *testing.T) {
	t.Run("empty lists return empty", func(t *testing.T) {
		result := rrfFusion(nil, nil, 10, 60.0)
		if len(result) != 0 {
			t.Errorf("got %d results, want 0", len(result))
		}
	})

	t.Run("single list fusion", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "a", Score: 0.9, Content: "A"},
			{ChunkID: "b", Score: 0.7, Content: "B"},
		}
		result := rrfFusion(list1, nil, 5, 60.0)
		if len(result) != 2 {
			t.Errorf("got %d results, want 2", len(result))
		}
	})

	t.Run("two list fusion", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "a", Score: 0.9},
			{ChunkID: "b", Score: 0.5},
		}
		list2 := []knowledge.SearchResult{
			{ChunkID: "a", Score: 0.8}, // same doc in both lists
			{ChunkID: "c", Score: 0.6},
		}
		result := rrfFusion(list1, list2, 10, 60.0)
		// Should get 3 unique results: a, b, c
		if len(result) != 3 {
			t.Errorf("got %d results, want 3", len(result))
		}
		// "a" should be top since it appears in both lists
		if result[0].ChunkID != "a" {
			t.Errorf("top result should be 'a', got %q", result[0].ChunkID)
		}
	})

	t.Run("topK limits results", func(t *testing.T) {
		list1 := []knowledge.SearchResult{
			{ChunkID: "a"}, {ChunkID: "b"}, {ChunkID: "c"},
		}
		result := rrfFusion(list1, nil, 2, 60.0)
		if len(result) != 2 {
			t.Errorf("topK=2: got %d results, want 2", len(result))
		}
	})
}
