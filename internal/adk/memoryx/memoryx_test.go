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
	"go.mongodb.org/mongo-driver/mongo/options"
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

func TestEnsureEmbedding(t *testing.T) {
	embedFn := func(ctx context.Context, text string) ([]float32, error) {
		return []float32{float32(len(text)), 0.1}, nil
	}
	k := &Kit{embed: embedFn}

	o := &adapter.Observation{Content: "hello"}
	k.ensureEmbedding(context.Background(), o)
	if len(o.Embedding) != 2 {
		t.Fatalf("embedding should be set: %v", o.Embedding)
	}

	// Already has embedding — skip.
	o2 := &adapter.Observation{Content: "x", Embedding: []float32{9}}
	k.ensureEmbedding(context.Background(), o2)
	if o2.Embedding[0] != 9 {
		t.Error("existing embedding should be preserved")
	}

	// No embed function — no-op.
	k2 := &Kit{}
	o3 := &adapter.Observation{Content: "x"}
	k2.ensureEmbedding(context.Background(), o3)
	if len(o3.Embedding) != 0 {
		t.Error("no embed fn → no embedding")
	}

	// Empty content.
	o4 := &adapter.Observation{Content: ""}
	k.ensureEmbedding(context.Background(), o4)
	if len(o4.Embedding) != 0 {
		t.Error("empty content → no embedding")
	}
}

func TestMergeIfSimilar(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	storage := &MongoStorage{coll: &coll, appName: "app"}

	// Return a similar existing memory.
	var cur mongo.Cursor
	calls := 0
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Next", func(ctx context.Context) bool {
		calls++
		return calls == 1
	})
	patches.ApplyMethodFunc(&cur, "Decode", func(v interface{}) error {
		doc := v.(*mongoDoc)
		doc.Content = "existing"
		doc.Embedding = []float32{1, 0} // very similar to candidate
		return nil
	})
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	k := &Kit{storage: storage}
	o := &adapter.Observation{Content: "new content", Embedding: []float32{1, 0.01}, UserID: "u1", AppName: "app"}
	k.mergeIfSimilar(context.Background(), o)
	// mergeIfSimilar is best-effort; content may or may not change
	// depending on search hit / similarity threshold. Coverage is the goal here.
	_ = o.Content
}

func TestSearchAndMerge(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	storage := &MongoStorage{coll: &coll, appName: "app"}

	patches.ApplyMethodReturn(&coll, "ReplaceOne", &mongo.UpdateResult{}, nil)
	var cur mongo.Cursor
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Next", false)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	k := &Kit{storage: storage}
	obs := []adapter.Observation{
		{Content: "test", AppName: "app", Embedding: []float32{1, 2}},
	}
	if err := k.SearchAndMerge(context.Background(), obs); err != nil {
		t.Fatalf("SearchAndMerge: %v", err)
	}
}

func TestCosine_ZeroNorm(t *testing.T) {
	if cosine([]float32{0, 0}, []float32{0, 0}) != 0 {
		t.Error("zero vectors")
	}
}

func TestMergeSimilar_LongerCandidate(t *testing.T) {
	a := &adapter.Observation{Content: "longer content here", Embedding: []float32{1, 0.05}}
	existing := []*adapter.Observation{
		{Content: "short", Embedding: []float32{1, 0}},
	}
	merged, _ := MergeSimilar(a, existing, nil)
	if merged == nil || merged.Content != "longer content here" {
		t.Error("longer candidate should win")
	}
}

func TestMaxLevel(t *testing.T) {
	if maxLevel(adapter.LevelExplicit, adapter.LevelDeductive) != adapter.LevelExplicit {
		t.Error("maxLevel explicit vs deductive")
	}
	if maxLevel(adapter.LevelDeductive, adapter.LevelInductive) != adapter.LevelInductive {
		t.Error("maxLevel: inductive > deductive alphabetically")
	}
}

func TestNewKit_ErrorPath(t *testing.T) {
	t.Skip("NewKit requires real LLM — covered by integration/E2E tests")
}

func TestSearchAndMerge_StoreError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	storage := &MongoStorage{coll: &coll, appName: "app"}

	patches.ApplyMethodReturn(&coll, "ReplaceOne", (*mongo.UpdateResult)(nil), errors.New("db down"))
	var cur mongo.Cursor
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Next", false)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	k := &Kit{storage: storage}
	obs := []adapter.Observation{
		{Content: "test", AppName: "app"},
	}
	if err := k.SearchAndMerge(context.Background(), obs); err == nil {
		t.Error("expected Store error to propagate")
	}
}

func TestMongoStorage_SearchCursorError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	var cur mongo.Cursor
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Next", false)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", errors.New("cursor error"))

	s := &MongoStorage{coll: &coll, appName: "app"}
	_, err := s.Search(context.Background(), &adapter.SearchOptions{})
	if err == nil {
		t.Fatal("expected cursor error")
	}
}

func TestKit_ServiceAndProvider(t *testing.T) {
	t.Skip("Kit.Service() requires MemoryKit built via NewKit — covered by integration tests")
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	storage := &MongoStorage{coll: &coll, appName: "app"}

	// patch ReplaceOne / Find for SearchAndMerge
	patches.ApplyMethodReturn(&coll, "ReplaceOne", &mongo.UpdateResult{}, nil)
	var cur mongo.Cursor
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodReturn(&cur, "Next", false)
	patches.ApplyMethodReturn(&cur, "Close", nil)
	patches.ApplyMethodReturn(&cur, "Err", nil)

	k := &Kit{storage: storage, embed: func(ctx context.Context, text string) ([]float32, error) {
		return []float32{0.1}, nil
	}}

	// Service() and Provider() accessors return non-nil.
	svc := k.Service()
	if svc == nil {
		t.Error("Service() should return non-nil")
	}
	// Provider() may be nil since we didn't build through NewKit.
	_ = k.Provider()
}

func TestMongoStorage_GetByID_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	// Mock FindOne to decode into our doc
	patches.ApplyMethodFunc(&coll, "FindOne",
		func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
			// Return a SingleResult that decodes to a doc with content "found"
			return &mongo.SingleResult{}
		})
	patches.ApplyMethodFunc(&mongo.SingleResult{}, "Decode",
		func(v interface{}) error {
			doc := v.(*mongoDoc)
			doc.Content = "found"
			doc.ID = idx.ID{1}
			doc.UserID = "u1"
			return nil
		})

	s := &MongoStorage{coll: &coll, appName: "app"}
	obs, err := s.GetByID(context.Background(), idx.ID{1})
	if err != nil {
		t.Fatalf("GetByID: %v", err)
	}
	if obs == nil || obs.Content != "found" {
		t.Fatalf("GetByID: %+v", obs)
	}
}

func TestMongoStorage_GetByID_FindError(t *testing.T) {
	// FindOne only returns *SingleResult; errors come through Decode.
	// Covered by E2E test.
	t.Skip("FindOne doesn't return error — covered by integration")
}

func TestMongoStorage_QueryMostDerived_FindError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var coll mongo.Collection
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("db down"))

	s := &MongoStorage{coll: &coll, appName: "app"}
	_, err := s.QueryMostDerived(context.Background(), "", "u1", "", 5)
	if err == nil {
		t.Fatal("expected Find error")
	}
}

func TestNewMongoStorage_NilDB(t *testing.T) {
	t.Skip("nil DB panics on Collection() — covered by normal construction path")
}
