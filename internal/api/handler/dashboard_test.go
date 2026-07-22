package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	domainknowledge "github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	taskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
	kbmocks "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

// fakeAggregator is a test double for TokenTrendAggregator. It returns a
// canned bucket slice (or an error) without touching MongoDB.
type fakeAggregator struct {
	buckets []llmstats.TimeBucketResult
	err     error
}

func (f *fakeAggregator) AggregateByTime(ctx context.Context, since time.Time, bucketMs int64) ([]llmstats.TimeBucketResult, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.buckets, nil
}

func TestDashboardHandler_Get(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	kb := kbmocks.NewKnowledgeService(t)

	tasks.On("ListAllTasks", "u1").Return([]*domaintask.Task{
		{ID: "t1", Status: domaintask.StatusCompleted},
		{ID: "t2", Status: domaintask.StatusPending},
		{ID: "t3", Status: domaintask.StatusFailed},
	}, nil)
	kb.On("ListAllDocs").Return([]*domainknowledge.KnowledgeDoc{{ID: "d1"}, {ID: "d2"}}, nil)

	h := NewDashboardHandler(tasks, kb, nil)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/dashboard", nil)
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	taskStats, ok := resp["task_stats"].(map[string]any)
	if !ok {
		t.Fatalf("missing task_stats field: %+v", resp)
	}
	if taskStats["total"].(float64) != 3 {
		t.Errorf("task_stats.total = %v, want 3", taskStats["total"])
	}
	if taskStats["completed"].(float64) != 1 {
		t.Errorf("task_stats.completed = %v, want 1", taskStats["completed"])
	}
	if taskStats["pending"].(float64) != 1 {
		t.Errorf("task_stats.pending = %v, want 1", taskStats["pending"])
	}
	if taskStats["failed"].(float64) != 1 {
		t.Errorf("task_stats.failed = %v, want 1", taskStats["failed"])
	}
	if resp["kb_docs"].(float64) != 2 {
		t.Errorf("kb_docs = %v, want 2", resp["kb_docs"])
	}
}

func TestDashboardHandler_GetTrends(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	kb := kbmocks.NewKnowledgeService(t)
	tasks.On("ListAllTasks", "u1").Return([]*domaintask.Task{
		{ID: "t1", Status: domaintask.StatusCompleted, CreatedAt: time.Now().Add(-1 * time.Hour)},
	}, nil)

	agg := &fakeAggregator{buckets: []llmstats.TimeBucketResult{
		{BucketStart: time.Now().Add(-1 * time.Hour), TotalTokens: 200},
	}}

	h := NewDashboardHandler(tasks, kb, agg)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/dashboard/trends", nil)
	h.GetTrends(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)

	// All seven trend fields should be present.
	for _, key := range []string{"call_trend", "duration_dist", "req_dist", "success_trend", "token_trend", "output_stats", "roi_trend"} {
		if resp[key] == nil {
			t.Errorf("missing %s in trends response: %+v", key, resp)
		}
	}

	// token_trend should carry the 200 tokens aggregated by the fake recorder.
	tokenTrend, ok := resp["token_trend"].([]any)
	if !ok {
		t.Fatalf("token_trend is not a slice: %T", resp["token_trend"])
	}
	if len(tokenTrend) != 6 {
		t.Errorf("token_trend len = %d, want 6", len(tokenTrend))
	}
	total := 0
	for _, p := range tokenTrend {
		point, _ := p.(map[string]any)
		total += int(point["value"].(float64))
	}
	if total != 200 {
		t.Errorf("token_trend total = %d, want 200", total)
	}
}

func TestDashboardHandler_GetTrends_RecorderError(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	kb := kbmocks.NewKnowledgeService(t)
	tasks.On("ListAllTasks", "u1").Return([]*domaintask.Task{}, nil)

	agg := &fakeAggregator{err: errStr("llmstats down")}

	h := NewDashboardHandler(tasks, kb, agg)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/dashboard/trends", nil)
	h.GetTrends(c)

	// Errors are swallowed so the dashboard still renders with empty trends.
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even when recorder errors, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	tokenTrend, ok := resp["token_trend"].([]any)
	if !ok {
		t.Fatalf("token_trend missing even on recorder error: %+v", resp)
	}
	if len(tokenTrend) != 6 {
		t.Errorf("token_trend len = %d, want 6 (zeroed)", len(tokenTrend))
	}
}

func TestDashboardHandler_GetTrends_NilRecorder(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	kb := kbmocks.NewKnowledgeService(t)
	tasks.On("ListAllTasks", "u1").Return([]*domaintask.Task{}, nil)

	// nil recorder (e.g. when MongoDB is unavailable) must not panic.
	h := NewDashboardHandler(tasks, kb, nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/dashboard/trends", nil)
	h.GetTrends(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 with nil recorder, got %d", w.Code)
	}
}

func TestNewDashboardHandler(t *testing.T) {
	h := NewDashboardHandler(nil, nil, nil)
	if h == nil {
		t.Error("handler should not be nil")
	}
}

// Compile-time guard: *fakeAggregator satisfies TokenTrendAggregator.
var _ TokenTrendAggregator = (*fakeAggregator)(nil)
