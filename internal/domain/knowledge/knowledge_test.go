package knowledge

import (
	"testing"
)

func TestDocStatusConstants(t *testing.T) {
	if StatusUploaded != "uploaded" {
		t.Errorf("StatusUploaded = %q", StatusUploaded)
	}
	if StatusReady != "ready" {
		t.Errorf("StatusReady = %q", StatusReady)
	}
	if StatusFailed != "failed" {
		t.Errorf("StatusFailed = %q", StatusFailed)
	}
}

func TestSearchResultZeroValue(t *testing.T) {
	r := SearchResult{}
	if r.Score != 0 {
		t.Errorf("Score default: got %f, want 0", r.Score)
	}
}

func TestKnowledgeDocFieldAssignment(t *testing.T) {
	doc := KnowledgeDoc{
		ID:        "doc-1",
		Title:     "Test Doc",
		Status:    StatusReady,
		SizeBytes: 1024,
	}
	if doc.ID != "doc-1" {
		t.Errorf("ID: got %s", doc.ID)
	}
	if doc.Status != StatusReady {
		t.Errorf("Status: got %s", doc.Status)
	}
	if doc.SizeBytes != 1024 {
		t.Errorf("SizeBytes: got %d", doc.SizeBytes)
	}
}

func TestChunkFieldAssignment(t *testing.T) {
	c := Chunk{
		DocID:    "doc-1",
		ChunkIdx: 0,
		Content:  "hello",
	}
	if c.ChunkIdx != 0 {
		t.Errorf("ChunkIdx: got %d", c.ChunkIdx)
	}
	if c.Content != "hello" {
		t.Errorf("Content: got %q", c.Content)
	}
}
