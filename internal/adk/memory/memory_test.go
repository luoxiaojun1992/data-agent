package adkmemory

import (
	"context"
	"fmt"
	"iter"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func newServiceWithMockColl(embed EmbeddingFunc) (*Service, *mongo.Collection) {
	var coll mongo.Collection
	return &Service{coll: &coll, embed: embed, maxChars: 1000}, &coll
}

func fixedEmbed(vec []float32) EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		return vec, nil
	}
}

func failEmbed() EmbeddingFunc {
	return func(ctx context.Context, text string) ([]float32, error) {
		return nil, fmt.Errorf("embedding backend down")
	}
}

// fakeSession builds a session.Session snapshot with the given texts.
func fakeSession(userID string, texts ...string) session.Session {
	events := make([]*session.Event, 0, len(texts))
	for i, txt := range texts {
		events = append(events, &session.Event{
			ID:        fmt.Sprintf("e%d", i),
			Timestamp: time.Now(),
			Author:    "user",
			LLMResponse: model.LLMResponse{
				Content: genai.NewContentFromText(txt, "user"),
			},
		})
	}
	return &testSession{id: "s1", app: "data-agent", user: userID, events: events}
}

type testSession struct {
	id     string
	app    string
	user   string
	events []*session.Event
}

func (s *testSession) ID() string                { return s.id }
func (s *testSession) AppName() string           { return s.app }
func (s *testSession) UserID() string            { return s.user }
func (s *testSession) LastUpdateTime() time.Time { return time.Now() }
func (s *testSession) State() session.State      { return nil }
func (s *testSession) Events() session.Events    { return testEvents(s.events) }

type testEvents []*session.Event

func (e testEvents) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, ev := range e {
			if !yield(ev) {
				return
			}
		}
	}
}

func (e testEvents) Len() int                { return len(e) }
func (e testEvents) At(i int) *session.Event { return e[i] }

// patchFind wires cursor mocking for Find queries returning docs.
func patchFind(patches *gomonkey.Patches, coll *mongo.Collection, docs []memoryDoc) {
	cur := &mongo.Cursor{}
	idx := 0
	patches.ApplyMethodReturn(coll, "Find", cur, nil)
	patches.ApplyMethod(cur, "Next", func(_ *mongo.Cursor, ctx context.Context) bool {
		return idx < len(docs)
	})
	patches.ApplyMethod(cur, "Decode", func(_ *mongo.Cursor, v any) error {
		*v.(*memoryDoc) = docs[idx]
		idx++
		return nil
	})
	patches.ApplyMethodReturn(cur, "Close", nil)
	patches.ApplyMethodReturn(cur, "Err", nil)
}

func TestAddSessionToMemory(t *testing.T) {
	svc, coll := newServiceWithMockColl(fixedEmbed([]float32{1, 0, 0}))
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// No existing memories for the session.
	patchFind(patches, coll, nil)

	inserted := 0
	patches.ApplyMethod(coll, "InsertOne", func(_ *mongo.Collection, ctx context.Context, doc any, _ ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
		inserted++
		d := doc.(memoryDoc)
		if d.UserID != "u1" || d.SessionID != "s1" || d.AppName != "data-agent" {
			t.Errorf("doc scoping wrong: %+v", d)
		}
		if len(d.Embedding) != 3 {
			t.Errorf("embedding not stored: %v", d.Embedding)
		}
		return &mongo.InsertOneResult{}, nil
	})

	err := svc.AddSessionToMemory(context.Background(), fakeSession("u1", "我叫张三", "营收增长 20%"))
	if err != nil {
		t.Fatalf("AddSessionToMemory failed: %v", err)
	}
	if inserted != 2 {
		t.Errorf("expected 2 memories stored, got %d", inserted)
	}
}

func TestAddSessionToMemory_Idempotent(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	// Existing memory with the same text → skip.
	patchFind(patches, coll, []memoryDoc{{ID: "m1", SessionID: "s1", UserID: "u1", Text: "我叫张三"}})

	inserted := 0
	patches.ApplyMethod(coll, "InsertOne", func(_ *mongo.Collection, ctx context.Context, doc any, _ ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
		inserted++
		return &mongo.InsertOneResult{}, nil
	})

	if err := svc.AddSessionToMemory(context.Background(), fakeSession("u1", "我叫张三", "新信息")); err != nil {
		t.Fatalf("failed: %v", err)
	}
	if inserted != 1 {
		t.Errorf("duplicate should be skipped, inserted=%d", inserted)
	}
}

func TestAddSessionToMemory_NoEvents(t *testing.T) {
	svc, _ := newServiceWithMockColl(nil)
	if err := svc.AddSessionToMemory(context.Background(), fakeSession("u1")); err != nil {
		t.Errorf("empty session should be no-op: %v", err)
	}
}

func TestAddSessionToMemory_EmbeddingFailure(t *testing.T) {
	svc, coll := newServiceWithMockColl(failEmbed())
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchFind(patches, coll, nil)

	inserted := 0
	patches.ApplyMethod(coll, "InsertOne", func(_ *mongo.Collection, ctx context.Context, doc any, _ ...*options.InsertOneOptions) (*mongo.InsertOneResult, error) {
		inserted++
		if doc.(memoryDoc).Embedding != nil {
			t.Error("embedding failure should store without vector")
		}
		return &mongo.InsertOneResult{}, nil
	})

	if err := svc.AddSessionToMemory(context.Background(), fakeSession("u1", "text")); err != nil {
		t.Fatalf("embedding failure must not fail the write: %v", err)
	}
	if inserted != 1 {
		t.Errorf("inserted=%d", inserted)
	}
}

func TestAddSessionToMemory_InsertError(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchFind(patches, coll, nil)
	patches.ApplyMethodReturn(coll, "InsertOne", (*mongo.InsertOneResult)(nil), fmt.Errorf("db down"))

	if err := svc.AddSessionToMemory(context.Background(), fakeSession("u1", "text")); err == nil {
		t.Error("expected insert error")
	}
}

func TestAddSessionToMemory_FindError(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "Find", (*mongo.Cursor)(nil), fmt.Errorf("db down"))

	if err := svc.AddSessionToMemory(context.Background(), fakeSession("u1", "text")); err == nil {
		t.Error("expected find error")
	}
}

func TestSearchMemory_VectorRanked(t *testing.T) {
	svc, coll := newServiceWithMockColl(fixedEmbed([]float32{1, 0, 0}))
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	docs := []memoryDoc{
		{ID: "m1", UserID: "u1", Text: "营收增长", Embedding: []float64{0.9, 0.1, 0}, CreatedAt: time.Now()},
		{ID: "m2", UserID: "u1", Text: "无关内容", Embedding: []float64{0, 1, 0}, CreatedAt: time.Now()},
		{ID: "m3", UserID: "u1", Text: "零向量", Embedding: []float64{0, 0, 0}, CreatedAt: time.Now()},
	}
	patchFind(patches, coll, docs)

	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{AppName: "data-agent", UserID: "u1", Query: "营收"})
	if err != nil {
		t.Fatalf("SearchMemory failed: %v", err)
	}
	if len(resp.Memories) == 0 || resp.Memories[0].ID != "m1" {
		t.Fatalf("highest-similarity memory should rank first: %+v", resp.Memories)
	}
	if resp.Memories[0].Content.Parts[0].Text != "营收增长" {
		t.Errorf("content = %v", resp.Memories[0].Content.Parts[0].Text)
	}
	if resp.Memories[0].CustomMetadata["session_id"] == nil && resp.Memories[0].CustomMetadata["score"] == nil {
		t.Errorf("metadata missing: %v", resp.Memories[0].CustomMetadata)
	}
}

func TestSearchMemory_KeywordFallback(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil) // no embed → keyword matching
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	docs := []memoryDoc{
		{ID: "m1", UserID: "u1", Text: "Alpha 项目营收翻倍", CreatedAt: time.Now()},
		{ID: "m2", UserID: "u1", Text: "无关内容", CreatedAt: time.Now()},
	}
	patchFind(patches, coll, docs)

	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{UserID: "u1", Query: "Alpha"})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(resp.Memories) != 1 || resp.Memories[0].ID != "m1" {
		t.Errorf("keyword search should hit m1: %+v", resp.Memories)
	}
}

func TestSearchMemory_Empty(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchFind(patches, coll, nil)

	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{UserID: "u1", Query: "q"})
	if err != nil || len(resp.Memories) != 0 {
		t.Errorf("empty store = %+v, %v", resp, err)
	}

	// Empty query short-circuits.
	resp, err = svc.SearchMemory(context.Background(), &memory.SearchRequest{UserID: "u1", Query: ""})
	if err != nil || len(resp.Memories) != 0 {
		t.Errorf("empty query = %+v, %v", resp, err)
	}
}

func TestSearchMemory_QueryEmbeddingFails(t *testing.T) {
	// embed fails for the query → falls back to keyword matching.
	svc, coll := newServiceWithMockColl(failEmbed())
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchFind(patches, coll, []memoryDoc{{ID: "m1", UserID: "u1", Text: "Alpha 项目", CreatedAt: time.Now()}})

	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{UserID: "u1", Query: "Alpha"})
	if err != nil || len(resp.Memories) != 1 {
		t.Errorf("keyword fallback after embed failure = %+v, %v", resp, err)
	}
}

func TestSearchMemory_FindError(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "Find", (*mongo.Cursor)(nil), fmt.Errorf("db down"))

	_, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{UserID: "u1", Query: "q"})
	if err == nil {
		t.Error("expected find error")
	}
}

func TestAdminSearch(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchFind(patches, coll, []memoryDoc{{ID: "m1", UserID: "u1", Text: "Alpha 项目营收", SessionID: "s1", CreatedAt: time.Now()}})

	out, err := svc.AdminSearch(context.Background(), "data-agent", "u1", "Alpha")
	if err != nil {
		t.Fatalf("AdminSearch failed: %v", err)
	}
	if len(out) != 1 || out[0]["memory"] != "Alpha 项目营收" {
		t.Errorf("admin search = %+v", out)
	}
	if out[0]["id"] != "m1" || out[0]["metadata"] == nil {
		t.Errorf("admin search fields missing: %+v", out[0])
	}
}

func TestDeleteBySession(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "DeleteMany", &mongo.DeleteResult{DeletedCount: 2}, nil)

	if err := svc.DeleteBySession(context.Background(), "s1"); err != nil {
		t.Errorf("delete failed: %v", err)
	}

	patches.ApplyMethodReturn(coll, "DeleteMany", (*mongo.DeleteResult)(nil), fmt.Errorf("db down"))
	if err := svc.DeleteBySession(context.Background(), "s1"); err == nil {
		t.Error("expected db error")
	}
}

func TestExtractTexts(t *testing.T) {
	long := make([]byte, 1500)
	for i := range long {
		long[i] = 'a'
	}
	sess := fakeSession("u1", "short", string(long))
	texts := extractTexts(sess)
	if len(texts) != 2 {
		t.Fatalf("expected 2 texts, got %d", len(texts))
	}
	if len(texts[1]) != 1000 {
		t.Errorf("long text should be truncated to 1000, got %d", len(texts[1]))
	}

	// compaction events are skipped.
	events := []*session.Event{
		{Author: "compaction", LLMResponse: model.LLMResponse{Content: genai.NewContentFromText("summary", "model")}},
		{Author: "user", LLMResponse: model.LLMResponse{Content: genai.NewContentFromText("real", "user")}},
		{Author: "user", LLMResponse: model.LLMResponse{Content: genai.NewContentFromText("   ", "user")}},
	}
	texts = extractTexts(&testSession{events: events})
	if len(texts) != 1 || texts[0] != "real" {
		t.Errorf("extractTexts = %v", texts)
	}
}

func TestCosine(t *testing.T) {
	if got := cosine([]float32{1, 0}, []float32{1, 0}); got != 1 {
		t.Errorf("identical = %f", got)
	}
	if got := cosine([]float32{1, 0}, []float32{0, 1}); got != 0 {
		t.Errorf("orthogonal = %f", got)
	}
	if got := cosine(nil, []float32{1}); got != 0 {
		t.Errorf("nil = %f", got)
	}
	if got := cosine([]float32{1}, []float32{1, 2}); got != 0 {
		t.Errorf("length mismatch = %f", got)
	}
	if got := cosine([]float32{0, 0}, []float32{1, 0}); got != 0 {
		t.Errorf("zero norm = %f", got)
	}
}

func TestVectorConversions(t *testing.T) {
	if float32To64(nil) != nil {
		t.Error("nil 32→64")
	}
	if float64To32(nil) != nil {
		t.Error("nil 64→32")
	}
	v := float32To64([]float32{1.5, 2.5})
	if len(v) != 2 || v[0] != 1.5 {
		t.Errorf("32→64 = %v", v)
	}
	v2 := float64To32(v)
	if len(v2) != 2 || v2[1] != 2.5 {
		t.Errorf("64→32 = %v", v2)
	}
}

// ---- embedding endpoint ----

func TestNewOpenAIEmbedding(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/embeddings" {
			t.Errorf("path = %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer key" {
			t.Errorf("missing auth header")
		}
		fmt.Fprint(w, `{"data":[{"embedding":[0.1,0.2,0.3]}]}`)
	}))
	defer srv.Close()

	embed := NewOpenAIEmbedding(OpenAIEmbeddingConfig{BaseURL: srv.URL + "/v1", Model: "m", APIKey: "key"})
	vec, err := embed(context.Background(), "hello")
	if err != nil {
		t.Fatalf("embedding failed: %v", err)
	}
	if len(vec) != 3 || vec[0] != 0.1 {
		t.Errorf("vec = %v", vec)
	}
}

func TestNewOpenAIEmbedding_Errors(t *testing.T) {
	// HTTP error status.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		fmt.Fprint(w, `{"error":"boom"}`)
	}))
	defer srv.Close()
	embed := NewOpenAIEmbedding(OpenAIEmbeddingConfig{BaseURL: srv.URL, Model: "m"})
	if _, err := embed(context.Background(), "x"); err == nil {
		t.Error("expected 500 error")
	}

	// Bad JSON.
	srv2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	}))
	defer srv2.Close()
	embed2 := NewOpenAIEmbedding(OpenAIEmbeddingConfig{BaseURL: srv2.URL, Model: "m"})
	if _, err := embed2(context.Background(), "x"); err == nil {
		t.Error("expected parse error")
	}

	// Empty data.
	srv3 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[]}`)
	}))
	defer srv3.Close()
	embed3 := NewOpenAIEmbedding(OpenAIEmbeddingConfig{BaseURL: srv3.URL, Model: "m"})
	if _, err := embed3(context.Background(), "x"); err == nil {
		t.Error("expected empty data error")
	}

	// Unreachable.
	embed4 := NewOpenAIEmbedding(OpenAIEmbeddingConfig{BaseURL: "http://127.0.0.1:1", Model: "m"})
	if _, err := embed4(context.Background(), "x"); err == nil {
		t.Error("expected connection error")
	}
}

func TestNewService(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	svc := NewService(db, nil)
	if svc.maxChars != 1000 {
		t.Errorf("maxChars = %d", svc.maxChars)
	}
	if svc.coll != &coll {
		t.Error("collection should come from db.Collection")
	}
}

func TestTextsForSession_DecodeError(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	cur := &mongo.Cursor{}
	calls := 0
	patches.ApplyMethodReturn(coll, "Find", cur, nil)
	patches.ApplyMethod(cur, "Next", func(_ *mongo.Cursor, ctx context.Context) bool {
		calls++
		return calls == 1
	})
	patches.ApplyMethod(cur, "Decode", func(_ *mongo.Cursor, v any) error {
		return fmt.Errorf("bad doc")
	})
	patches.ApplyMethodReturn(cur, "Close", nil)
	patches.ApplyMethodReturn(cur, "Err", nil)

	err := svc.AddSessionToMemory(context.Background(), fakeSession("u1", "text"))
	if err == nil {
		t.Error("expected decode error")
	}
}

func TestDocsForUser_AppNameFilter(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	var capturedFilter any
	cur := &mongo.Cursor{}
	patches.ApplyMethod(coll, "Find", func(_ *mongo.Collection, ctx context.Context, filter any, _ ...*options.FindOptions) (*mongo.Cursor, error) {
		capturedFilter = filter
		return cur, nil
	})
	patches.ApplyMethod(cur, "Next", func(_ *mongo.Cursor, ctx context.Context) bool { return false })
	patches.ApplyMethodReturn(cur, "Close", nil)
	patches.ApplyMethodReturn(cur, "Err", nil)

	if _, err := svc.docsForUser(context.Background(), "data-agent", "u1"); err != nil {
		t.Fatalf("failed: %v", err)
	}
	f := capturedFilter.(bson.M)
	if f["app_name"] != "data-agent" || f["user_id"] != "u1" {
		t.Errorf("filter = %v", f)
	}

	// Decode error path.
	calls := 0
	patches.ApplyMethod(cur, "Next", func(_ *mongo.Cursor, ctx context.Context) bool {
		calls++
		return calls == 1
	})
	patches.ApplyMethod(cur, "Decode", func(_ *mongo.Cursor, v any) error {
		return fmt.Errorf("bad doc")
	})
	if _, err := svc.docsForUser(context.Background(), "", "u1"); err == nil {
		t.Error("expected decode error")
	}
}

func TestSearchMemory_TopKTruncation(t *testing.T) {
	svc, coll := newServiceWithMockColl(fixedEmbed([]float32{1, 0, 0}))
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	docs := make([]memoryDoc, 8)
	for i := range docs {
		docs[i] = memoryDoc{
			ID:        fmt.Sprintf("m%d", i),
			UserID:    "u1",
			Text:      fmt.Sprintf("mem %d", i),
			Embedding: []float64{float64(8 - i), 0, 0},
			CreatedAt: time.Now(),
		}
	}
	patchFind(patches, coll, docs)

	resp, err := svc.SearchMemory(context.Background(), &memory.SearchRequest{UserID: "u1", Query: "q"})
	if err != nil {
		t.Fatalf("failed: %v", err)
	}
	if len(resp.Memories) != 5 {
		t.Errorf("topK=5 truncation, got %d", len(resp.Memories))
	}
	if resp.Memories[0].ID != "m0" {
		t.Errorf("highest score first: %v", resp.Memories[0].ID)
	}
}

func TestAdminSearch_Error(t *testing.T) {
	svc, coll := newServiceWithMockColl(nil)
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(coll, "Find", (*mongo.Cursor)(nil), fmt.Errorf("db down"))

	if _, err := svc.AdminSearch(context.Background(), "app", "u1", "q"); err == nil {
		t.Error("expected error passthrough")
	}
}

func TestEmbedding_BadRequestBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"data":[{"embedding":[]}]}`)
	}))
	defer srv.Close()
	embed := NewOpenAIEmbedding(OpenAIEmbeddingConfig{BaseURL: srv.URL, Model: "m"})
	if _, err := embed(context.Background(), "x"); err == nil {
		t.Error("empty embedding vector should error")
	}
}
