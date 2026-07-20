package adktools

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
	"go.mongodb.org/mongo-driver/mongo"
	"google.golang.org/adk/agent"
	"google.golang.org/adk/memory"
	"google.golang.org/adk/session"
	"google.golang.org/adk/tool"
	"google.golang.org/adk/tool/toolconfirmation"
	"google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	knowledgepkg "github.com/luoxiaojun1992/data-agent/internal/service/knowledge"
)

// ---- fake tool.Context ----

type fakeState struct{ m map[string]any }

func (s *fakeState) Get(k string) (any, error) {
	v, ok := s.m[k]
	if !ok {
		return nil, fmt.Errorf("key %q: %w", k, session.ErrStateKeyNotExist)
	}
	return v, nil
}

func (s *fakeState) Set(k string, v any) error { s.m[k] = v; return nil }

func (s *fakeState) All() iter.Seq2[string, any] {
	return func(yield func(string, any) bool) {
		for k, v := range s.m {
			if !yield(k, v) {
				return
			}
		}
	}
}

type fakeToolContext struct {
	context.Context
	state *fakeState
}

func newFakeToolContext(state map[string]any) *fakeToolContext {
	return &fakeToolContext{Context: context.Background(), state: &fakeState{m: state}}
}

func (c *fakeToolContext) UserContent() *genai.Content          { return nil }
func (c *fakeToolContext) InvocationID() string                 { return "inv-1" }
func (c *fakeToolContext) AgentName() string                    { return "test-agent" }
func (c *fakeToolContext) ReadonlyState() session.ReadonlyState { return c.state }
func (c *fakeToolContext) UserID() string                       { return "u1" }
func (c *fakeToolContext) AppName() string                      { return "data-agent" }
func (c *fakeToolContext) SessionID() string                    { return "s1" }
func (c *fakeToolContext) Branch() string                       { return "" }
func (c *fakeToolContext) Artifacts() agent.Artifacts           { return nil }
func (c *fakeToolContext) State() session.State                 { return c.state }
func (c *fakeToolContext) FunctionCallID() string               { return "fc-1" }
func (c *fakeToolContext) Actions() *session.EventActions       { return &session.EventActions{} }
func (c *fakeToolContext) SearchMemory(ctx context.Context, query string) (*memory.SearchResponse, error) {
	return &memory.SearchResponse{}, nil
}
func (c *fakeToolContext) ToolConfirmation() *toolconfirmation.ToolConfirmation { return nil }
func (c *fakeToolContext) RequestConfirmation(hint string, payload any) error   { return nil }

var _ agent.ToolContext = (*fakeToolContext)(nil)

// ---- sql_validate ----

func TestSQLValidate(t *testing.T) {
	tc := newFakeToolContext(nil)

	// Missing query.
	if _, err := sqlValidate(tc, SQLValidateArgs{}); err == nil {
		t.Error("empty query should fail")
	}

	// Valid SELECT.
	res, err := sqlValidate(tc, SQLValidateArgs{Query: "SELECT id FROM users LIMIT 10"})
	if err != nil {
		t.Fatalf("valid query failed: %v", err)
	}
	if res.Status != "validated" || res.Query == "" {
		t.Errorf("result = %+v", res)
	}

	// Dangerous query rejected.
	if _, err := sqlValidate(tc, SQLValidateArgs{Query: "DROP TABLE users"}); err == nil {
		t.Error("DROP should be rejected")
	}
}

// ---- stats_compute ----

func TestStatsCompute(t *testing.T) {
	tc := newFakeToolContext(nil)

	if _, err := statsCompute(tc, StatsComputeArgs{}); err == nil {
		t.Error("missing method should fail")
	}
	if _, err := statsCompute(tc, StatsComputeArgs{Method: "descriptive"}); err == nil {
		t.Error("empty values should fail")
	}
	if _, err := statsCompute(tc, StatsComputeArgs{Method: "unknown", Values: []float64{1}}); err == nil {
		t.Error("unknown method should fail")
	}
	if _, err := statsCompute(tc, StatsComputeArgs{Method: "linear_regression", Values: []float64{1, 2}}); err == nil {
		t.Error("missing x_values should fail")
	}

	res, err := statsCompute(tc, StatsComputeArgs{Method: "descriptive", Values: []float64{1, 2, 3}, Label: "test"})
	if err != nil {
		t.Fatalf("descriptive failed: %v", err)
	}
	if res == nil {
		t.Error("descriptive result nil")
	}

	if _, err := statsCompute(tc, StatsComputeArgs{Method: "linear_regression", Values: []float64{2, 4, 6}, XValues: []float64{1, 2, 3}}); err != nil {
		t.Errorf("linear_regression failed: %v", err)
	}
	if _, err := statsCompute(tc, StatsComputeArgs{Method: "time_series", Values: []float64{1, 2, 3, 4, 5, 6, 7, 8}}); err != nil {
		t.Errorf("time_series failed: %v", err)
	}
}

// ---- knowledge_search ----

func TestKnowledgeSearch(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(db, "Collection", &coll)
	// KB service falls back to empty results on find errors.
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), fmt.Errorf("no docs"))

	kbSvc := knowledgepkg.NewService(mongoinfra.NewKBRepository(db))
	deps := &Deps{KBService: kbSvc, AppName: "data-agent"}
	handler := knowledgeSearch(deps)

	tc := newFakeToolContext(map[string]any{
		"user_id": "u1",
		"role":    "admin",
		"kb_id":   "kb-42",
	})

	// Missing query.
	if _, err := handler(tc, KnowledgeSearchArgs{}); err == nil {
		t.Error("empty query should fail")
	}

	res, err := handler(tc, KnowledgeSearchArgs{Query: "营收", TopK: 0})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if res.KBID != "kb-42" {
		t.Errorf("kb_id should come from session state, got %q", res.KBID)
	}
	if res.Count != 0 || len(res.Results) != 0 {
		t.Errorf("expected empty results, got %+v", res)
	}

	// TopK upper bound.
	if _, err := handler(tc, KnowledgeSearchArgs{Query: "q", TopK: 999}); err != nil {
		t.Errorf("topK bound: %v", err)
	}
}

// ---- save_report ----

func TestSaveReport(t *testing.T) {
	tc := newFakeToolContext(nil)

	if _, err := saveReport(tc, SaveReportArgs{}); err == nil {
		t.Error("missing title should fail")
	}
	if _, err := saveReport(tc, SaveReportArgs{Title: "t"}); err == nil {
		t.Error("missing content should fail")
	}

	// With validation disabled.
	noValidate := false
	res, err := saveReport(tc, SaveReportArgs{Title: "报告", Content: "anything", Validate: &noValidate})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if res.Status != "saved" || res.Valid {
		t.Errorf("validation should be skipped: %+v", res)
	}

	// With validation enabled (default) — content missing mandatory sections.
	res, err = saveReport(tc, SaveReportArgs{Title: "报告", Content: "# 标题\n一些内容"})
	if err != nil {
		t.Fatalf("save failed: %v", err)
	}
	if res.Status == "validation_failed" {
		if len(res.MissingSections) == 0 {
			t.Error("missing sections expected")
		}
	}
}

// ---- memory_search ----

type stubMemoryService struct {
	resp *memory.SearchResponse
	err  error
}

func (s *stubMemoryService) AddSessionToMemory(ctx context.Context, sess session.Session) error {
	return nil
}

func (s *stubMemoryService) SearchMemory(ctx context.Context, req *memory.SearchRequest) (*memory.SearchResponse, error) {
	if s.err != nil {
		return nil, s.err
	}
	return s.resp, nil
}

func TestMemorySearch(t *testing.T) {
	tc := newFakeToolContext(map[string]any{"user_id": "u1"})

	// Missing query.
	deps := &Deps{AppName: "data-agent"}
	if _, err := memorySearch(deps)(tc, MemorySearchArgs{}); err == nil {
		t.Error("empty query should fail")
	}

	// Memory service not configured.
	res, err := memorySearch(deps)(tc, MemorySearchArgs{Query: "q"})
	if err != nil {
		t.Fatalf("nil memory should not error: %v", err)
	}
	if res.Note == "" || res.Count != 0 {
		t.Errorf("expected explanatory note: %+v", res)
	}

	// With memories.
	deps.Memory = &stubMemoryService{resp: &memory.SearchResponse{Memories: []memory.Entry{
		{ID: "m1", Content: genai.NewContentFromText("用户叫张三", "user")},
		{ID: "m2", Content: nil},
		{ID: "m3", Content: genai.NewContentFromText("第三个", "user")},
	}}}
	res, err = memorySearch(deps)(tc, MemorySearchArgs{Query: "张三", Limit: 2})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if res.Count != 2 || len(res.Memories) != 2 {
		t.Errorf("limit=2 truncation: %+v", res)
	}
	if res.Memories[0] != "用户叫张三" {
		t.Errorf("memory text = %q", res.Memories[0])
	}

	// Default limit.
	res, _ = memorySearch(deps)(tc, MemorySearchArgs{Query: "q"})
	if res.Count != 3 {
		t.Errorf("default limit should return all 3: %d", res.Count)
	}

	// Backend error.
	deps.Memory = &stubMemoryService{err: fmt.Errorf("mem down")}
	if _, err := memorySearch(deps)(tc, MemorySearchArgs{Query: "q"}); err == nil {
		t.Error("backend error should propagate")
	}
}

// ---- registry ----

func TestAllAndNames(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	deps := &Deps{KBService: knowledgepkg.NewService(mongoinfra.NewKBRepository(db)), AppName: "data-agent"}
	tools, err := All(deps)
	if err != nil {
		t.Fatalf("All failed: %v", err)
	}
	if len(tools) != 6 {
		t.Errorf("expected 6 tools, got %d", len(tools))
	}

	names, err := Names(deps)
	if err != nil {
		t.Fatalf("Names failed: %v", err)
	}
	want := map[string]bool{
		"sql_validate": true, "stats_compute": true, "save_report": true,
		"knowledge_search": true, "memory_search": true, "memory_write": true,
	}
	for _, n := range names {
		if !want[n] {
			t.Errorf("unexpected tool %q", n)
		}
		delete(want, n)
	}
	if len(want) != 0 {
		t.Errorf("missing tools: %v", want)
	}

	// Without KB service → knowledge_search skipped.
	tools, err = All(&Deps{AppName: "data-agent"})
	if err != nil {
		t.Fatalf("All failed: %v", err)
	}
	if len(tools) != 5 {
		t.Errorf("expected 5 tools without KB, got %d", len(tools))
	}
}

func TestTruncateContent(t *testing.T) {
	if got := truncateContent("short", 10); got != "short" {
		t.Errorf("short = %q", got)
	}
	long := "this is a fairly long string for truncation"
	got := truncateContent(long, 10)
	if len(got) > 14 || got[len(got)-3:] != "..." {
		t.Errorf("truncated = %q", got)
	}
}

func TestStateString(t *testing.T) {
	tc := newFakeToolContext(map[string]any{"user_id": "u1", "n": 42})
	if got := stateString(tc, "user_id"); got != "u1" {
		t.Errorf("user_id = %q", got)
	}
	if got := stateString(tc, "missing"); got != "" {
		t.Errorf("missing key = %q", got)
	}
	if got := stateString(tc, "n"); got != "" {
		t.Errorf("non-string value = %q", got)
	}
}

// ensure tool.Tool interface compliance at compile time.

func TestKnowledgeSearch_HitsAndError(t *testing.T) {
	kbSvc := &knowledgepkg.Service{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	patches.ApplyMethodReturn(kbSvc, "Search", []knowledge.SearchResult{
		{DocID: "d1", DocTitle: "财报", Content: "很长很长的内容", Score: 0.9},
		{DocID: "d2", DocTitle: "年报", Content: strings.Repeat("x", 600), Score: 0.8},
	}, nil)

	deps := &Deps{KBService: kbSvc, AppName: "data-agent"}
	handler := knowledgeSearch(deps)
	tc := newFakeToolContext(map[string]any{"user_id": "u1"})

	res, err := handler(tc, KnowledgeSearchArgs{Query: "营收", TopK: 5})
	if err != nil {
		t.Fatalf("search failed: %v", err)
	}
	if res.Count != 2 {
		t.Fatalf("expected 2 hits, got %d", res.Count)
	}
	if res.Results[0].DocID != "d1" || res.Results[0].Score != 0.9 {
		t.Errorf("hit mapping wrong: %+v", res.Results[0])
	}
	if len(res.Results[1].Content) > 510 {
		t.Errorf("content should be truncated: len=%d", len(res.Results[1].Content))
	}

	// KB error propagates.
	patches.ApplyMethodReturn(kbSvc, "Search", ([]knowledge.SearchResult)(nil), fmt.Errorf("kb down"))
	if _, err := handler(tc, KnowledgeSearchArgs{Query: "q"}); err == nil {
		t.Error("kb error should propagate")
	}
}

func TestAll_BuildError(t *testing.T) {
	// Inject a failing spec via gomonkey on the package-level specs function.
	patches := gomonkey.ApplyFuncReturn(specs, []toolSpec{
		{name: "bad_tool", build: func() (tool.Tool, error) { return nil, fmt.Errorf("boom") }},
	})
	defer patches.Reset()

	if _, err := All(&Deps{}); err == nil {
		t.Error("build error should propagate")
	}

	if _, err := Names(&Deps{}); err == nil {
		t.Error("Names should propagate All error")
	}
}

func TestSpecs_Content(t *testing.T) {
	// Without KB: 5 tools (memory_write + memory_search + sql + stats + report).
	if got := specs(&Deps{}); len(got) != 5 {
		t.Errorf("specs without KB = %d", len(got))
	}
	// With KB: 5 tools and knowledge_search present.
	kbSvc := &knowledgepkg.Service{}
	found := false
	for _, s := range specs(&Deps{KBService: kbSvc}) {
		if s.name == "knowledge_search" {
			found = true
		}
		if s.description == "" {
			t.Errorf("spec %q missing description", s.name)
		}
	}
	if !found {
		t.Error("knowledge_search should be present with KB service")
	}
}
