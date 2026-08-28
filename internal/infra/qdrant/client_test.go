package qdrant

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

func TestClientSearch_FilterIncludedInPayload(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if err := json.NewDecoder(r.Body).Decode(&gotBody); err != nil {
			t.Errorf("decode request body: %v", err)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"id":42,"score":0.9,"payload":{"doc_id":"doc1"}}]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	filter := map[string]any{
		"must": []any{
			map[string]any{"key": "creator_id", "match": map[string]any{"value": "u1"}},
		},
	}
	hits, err := c.Search("kb_chunks", []float32{0.1, 0.2}, 5, filter)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}

	// 断言1: filter 被序列化进 payload 且结构完整
	gotFilter, ok := gotBody["filter"].(map[string]any)
	if !ok {
		t.Fatalf("filter not in request payload: %v", gotBody)
	}
	must, ok := gotFilter["must"].([]any)
	if !ok || len(must) != 1 {
		t.Fatalf("filter.must = %v, want single condition", gotFilter["must"])
	}
	cond := must[0].(map[string]any)
	if cond["key"] != "creator_id" {
		t.Errorf("filter condition key = %v, want creator_id", cond["key"])
	}
	// 断言2: 结果正确解析
	if len(hits) != 1 || hits[0].ID != 42 || hits[0].Score != 0.9 {
		t.Errorf("hits = %+v, want 1 hit id=42 score=0.9", hits)
	}
	if hits[0].Payload["doc_id"] != "doc1" {
		t.Errorf("payload doc_id = %v, want doc1", hits[0].Payload["doc_id"])
	}
}

func TestClientSearch_NoFilterOmitsField(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[]}`))
	}))
	defer srv.Close()

	c := NewClient(srv.URL)
	hits, err := c.Search("kb_chunks", []float32{0.1}, 3, nil)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// 断言1: 空 filter 不写入 payload
	if _, ok := gotBody["filter"]; ok {
		t.Errorf("filter should be omitted when empty, got %v", gotBody)
	}
	// 断言2: limit 正确且结果为空
	if gotBody["limit"] != float64(3) {
		t.Errorf("limit = %v, want 3", gotBody["limit"])
	}
	if len(hits) != 0 {
		t.Errorf("hits = %+v, want empty", hits)
	}
}

func TestVectorStoreSearch_ForwardsFilterAndMapsHits(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"result":[{"id":7,"score":0.88,"payload":{"doc_id":"d","content":"hello"}}]}`))
	}))
	defer srv.Close()

	vs := NewVectorStore(NewClient(srv.URL))
	filter := map[string]interface{}{
		"must": []interface{}{
			map[string]interface{}{"key": "is_public", "match": map[string]interface{}{"value": true}},
		},
	}
	hits, err := vs.Search(context.Background(), "kb_chunks", []float32{0.5}, 5, filter)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	// 断言1: filter 透传至 Qdrant（VectorStore 不再静默丢弃）
	if _, ok := gotBody["filter"]; !ok {
		t.Fatalf("filter not forwarded to client: %v", gotBody)
	}
	// 断言2: hits 正确映射（ID int64→string, Score, Metadata）
	if len(hits) != 1 {
		t.Fatalf("hits count = %d, want 1", len(hits))
	}
	if hits[0].ID != "7" || hits[0].Score != 0.88 {
		t.Errorf("hit = %+v, want ID=7 score=0.88", hits[0])
	}
	if hits[0].Metadata["doc_id"] != "d" || hits[0].Metadata["content"] != "hello" {
		t.Errorf("metadata = %v, want doc_id=d content=hello", hits[0].Metadata)
	}
}

var _ repository.VectorRepository = (*VectorStore)(nil)
