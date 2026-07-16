package knowledge

import (
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
)

func TestGenShortID(t *testing.T) {
	id := genShortID()
	if id == "" {
		t.Error("genShortID should not be empty")
	}
}

func TestRRFFusion_Empty(t *testing.T) {
	result := rrfFusion(nil, nil, 10, 60.0)
	if len(result) != 0 {
		t.Errorf("got %d, want 0", len(result))
	}
}

func TestRRFFusion_SingleList(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.9},
		{ChunkID: "b", Score: 0.7},
	}
	result := rrfFusion(list1, nil, 5, 60.0)
	if len(result) != 2 {
		t.Errorf("got %d, want 2", len(result))
	}
}

func TestRRFFusion_TwoLists(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.9},
		{ChunkID: "b", Score: 0.5},
	}
	list2 := []knowledge.SearchResult{
		{ChunkID: "a", Score: 0.8},
		{ChunkID: "c", Score: 0.6},
	}
	result := rrfFusion(list1, list2, 10, 60.0)
	if len(result) != 3 {
		t.Errorf("got %d, want 3", len(result))
	}
	if result[0].ChunkID != "a" {
		t.Errorf("top should be 'a', got %q", result[0].ChunkID)
	}
}

func TestRRFFusion_TopK(t *testing.T) {
	list1 := []knowledge.SearchResult{
		{ChunkID: "a"}, {ChunkID: "b"}, {ChunkID: "c"},
	}
	result := rrfFusion(list1, nil, 2, 60.0)
	if len(result) != 2 {
		t.Errorf("got %d, want 2", len(result))
	}
}
