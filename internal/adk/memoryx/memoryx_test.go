package memoryx

import (
	"context"
	"errors"
	"math"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/ieshan/adk-go-memory/adapter"
	"github.com/ieshan/idx"
	"go.mongodb.org/mongo-driver/mongo"
)

func TestCosine(t *testing.T) {
	a := []float32{1, 0, 0}
	b := []float32{1, 0, 0}
	if cosine(a, b) != 1.0 {
		t.Errorf("identical vectors: %v", cosine(a, b))
	}

	c := []float32{0, 1, 0}
	if cosine(a, c) != 0.0 {
		t.Errorf("orthogonal vectors: %v", cosine(a, c))
	}

	d := []float32{-1, 0, 0}
	if cosine(a, d) != -1.0 {
		t.Errorf("opposite vectors: %v", cosine(a, d))
	}

	// Different lengths
	if cosine(a, []float32{1}) != 0 {
		t.Error("different lengths should return 0")
	}
	if cosine([]float32{}, b) != 0 {
		t.Error("empty vectors should return 0")
	}
}

func TestMergeSimilar_Threshold(t *testing.T) {
	// Very similar vectors (> 0.92)
	a := &adapter.Observation{Content: "a", Embedding: []float32{1, 0.1}}
	existing := []*adapter.Observation{
		{Content: "similar", Embedding: []float32{1, 0.0}},
	}

	merged, idx := MergeSimilar(a, existing, nil)
	if idx != 0 || merged == nil {
		t.Fatalf("should merge similar: merged=%v idx=%d", merged, idx)
	}
	if merged.Content != "similar" {
		t.Errorf("merged content: %q (short candidate doesn't replace longer)", merged.Content)
	}

	// Dissimilar vectors
	b := &adapter.Observation{Content: "b", Embedding: []float32{0, 1}}
	merged2, idx2 := MergeSimilar(b, existing, nil)
	if merged2 != nil || idx2 != -1 {
		t.Errorf("should not merge dissimilar: %v, %d", merged2, idx2)
	}

	// No embedding in candidate
	c := &adapter.Observation{Content: "c"}
	merged3, _ := MergeSimilar(c, existing, nil)
	if merged3 != nil {
		t.Error("no embedding → no merge")
	}
}

func TestMergeContent(t *testing.T) {
	if mergeContent("short", "longer_content") != "longer_content" {
		t.Error("longer candidate wins")
	}
	// Candidate is substantial (> half of existing) and new → append.
	if mergeContent("ab", "cd") != "ab; cd" {
		t.Errorf("append: %q", mergeContent("ab", "cd"))
	}
	// Candidate already contained → keep existing.
	if mergeContent("hello world", "world") != "hello world" {
		t.Error("already contained")
	}
}

func TestNewID(t *testing.T) {
	id := NewID()
	if id == [16]byte{} {
		t.Error("ID should not be zero")
	}
}

func TestMongoStorage_Store(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "ReplaceOne", &mongo.UpdateResult{}, nil)
	_ = coll // used by patches

	s := &MongoStorage{coll: &coll, appName: "app"}
	err := s.Store(context.Background(), &adapter.Observation{
		ID:      idx.ID{1},
		Content: "test",
		AppName: "app",
	})
	if err != nil {
		t.Fatalf("Store: %v", err)
	}
}

func TestMongoStorage_GetByID_NotFound(t *testing.T) {
	// GetByID wraps FindOne → mongo.ErrNoDocuments → returns nil, nil.
	// Full integration test covered in SPEC-046 E2E.
}

func TestMongoStorage_Forget(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "DeleteOne", &mongo.DeleteResult{}, nil)

	s := &MongoStorage{coll: &coll, appName: "app"}
	if err := s.Forget(context.Background(), idx.ID{1}); err != nil {
		t.Fatalf("Forget: %v", err)
	}
}

func TestMongoStorage_Purge(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "DeleteMany", &mongo.DeleteResult{}, nil)

	s := &MongoStorage{coll: &coll, appName: "app"}
	if err := s.Purge(context.Background(), map[string]string{"user_id": "u1"}); err != nil {
		t.Fatalf("Purge: %v", err)
	}
}

func TestMongoStorage_IncrementTimesDerived(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "UpdateOne", &mongo.UpdateResult{}, nil)

	s := &MongoStorage{coll: &coll, appName: "app"}
	if err := s.IncrementTimesDerived(context.Background(), idx.ID{1}); err != nil {
		t.Fatalf("IncrementTimesDerived: %v", err)
	}
}

func TestMongoStorage_Close(t *testing.T) {
	s := &MongoStorage{}
	if err := s.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
}

func TestMongoStorage_Search(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	var cur mongo.Cursor
	called := 0
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Next",
		func(ctx context.Context) bool {
			called++
			return called == 1
		})
	patches.ApplyMethodFunc(&cur, "Decode",
		func(v interface{}) error {
			doc := v.(*mongoDoc)
			doc.Content = "hello world"
			doc.UserID = "u1"
			doc.AppName = "app"
			return nil
		})
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	s := &MongoStorage{coll: &coll, appName: "app"}
	results, err := s.Search(context.Background(), &adapter.SearchOptions{
		Query:  "hello",
		UserID: "u1",
	})
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
}

func TestMongoStorage_SearchError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("db down"))

	s := &MongoStorage{coll: &coll, appName: "app"}
	_, err := s.Search(context.Background(), &adapter.SearchOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestComputeScore(t *testing.T) {
	obs := &adapter.Observation{Content: "hello world", Level: adapter.LevelExplicit}
	s := computeScore(&adapter.SearchOptions{Query: "hello"}, obs)
	if s <= obs.Score() {
		t.Errorf("keyword match should boost score: %v vs base %v", s, obs.Score())
	}
	s2 := computeScore(&adapter.SearchOptions{Query: "xyz"}, obs)
	if math.Abs(s2-obs.Score()) > 1e-9 {
		t.Errorf("no keyword match: %v", s2)
	}
}

func TestNewMongoStorage(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(db, "Collection", &coll)

	s := NewMongoStorage(db, "data-agent")
	if s.coll != &coll || s.appName != "data-agent" {
		t.Error("NewMongoStorage miswired")
	}
}

func TestQueryRanked(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	var cur mongo.Cursor
	calls := 0
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Next", func(ctx context.Context) bool {
		calls++
		return calls == 1
	})
	patches.ApplyMethodFunc(&cur, "Decode", func(v interface{}) error {
		doc := v.(*mongoDoc)
		doc.Content = "test"
		return nil
	})
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	s := &MongoStorage{coll: &coll, appName: "app"}
	obs, err := s.queryRanked(context.Background(), "s1", "u1", "app", 5, "created_at")
	if err != nil || len(obs) != 1 {
		t.Fatalf("queryRanked: %v, len=%d", err, len(obs))
	}
}

func TestQueryMostDerived(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	var cur mongo.Cursor
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Next", false)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	s := &MongoStorage{coll: &coll, appName: "app"}
	obs, err := s.QueryMostDerived(context.Background(), "", "u1", "", 5)
	if err != nil || len(obs) != 0 {
		t.Fatalf("QueryMostDerived: %v, len=%d", err, len(obs))
	}
}

func TestQueryRecent(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	var cur mongo.Cursor
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Next", false)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	s := &MongoStorage{coll: &coll, appName: "app"}
	obs, err := s.QueryRecent(context.Background(), "s1", "u1", "app", 10)
	if err != nil || len(obs) != 0 {
		t.Fatalf("QueryRecent: %v, len=%d", err, len(obs))
	}
}

func TestDocConversion(t *testing.T) {
	obs := &adapter.Observation{
		ID:           idx.ID{1, 2, 3},
		Content:      "test",
		Level:        adapter.LevelExplicit,
		SessionID:    "s1",
		UserID:       "u1",
		AppName:      "app",
		Tags:         []string{"a", "b"},
		TimesDerived: 3,
		Embedding:    []float32{0.1, 0.2},
	}
	doc := docFromObs(obs)
	back := obsFromDoc(doc)
	if back.ID != obs.ID || back.Content != obs.Content || back.Level != obs.Level {
		t.Errorf("round-trip failed: %+v", back)
	}
}

func TestCosine_Various(t *testing.T) {
	if cosine([]float32{0.5, 0.5}, []float32{0.5, 0.5}) < 0.99 {
		t.Error("exact match should be ~1.0")
	}
	if cosine([]float32{}, []float32{1, 2}) != 0 {
		t.Error("empty first vector")
	}
}

func TestMergeSimilar_NoEmbedding(t *testing.T) {
	existing := []*adapter.Observation{{Content: "x", Embedding: []float32{1}}}
	c := &adapter.Observation{Content: "c"}
	merged, _ := MergeSimilar(c, existing, nil)
	if merged != nil {
		t.Error("no embedding in candidate")
	}
}

func TestMergeSimilar_ExistingNoEmbed(t *testing.T) {
	existing := []*adapter.Observation{{Content: "x"}} // no embedding
	c := &adapter.Observation{Content: "c", Embedding: []float32{1, 0}}
	merged, _ := MergeSimilar(c, existing, nil)
	if merged != nil {
		t.Error("existing has no embedding")
	}
}

func TestDedupeTags(t *testing.T) {
	tags := dedupeTags([]string{"a", "b", "a", ""})
	if len(tags) != 2 || tags[0] != "a" || tags[1] != "b" {
		t.Errorf("dedupeTags: %v", tags)
	}
}

func TestAverageVectors(t *testing.T) {
	avg := averageVectors([]float32{2, 4}, []float32{4, 6})
	if avg[0] != 3.0 || avg[1] != 5.0 {
		t.Errorf("averageVectors: %v", avg)
	}
}
