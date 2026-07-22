package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewStatsHandler(t *testing.T) {
	h := NewStatsHandler(&llmstats.Recorder{})
	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.recorder == nil {
		t.Error("recorder should be set")
	}
}

func TestGetLLMStats_Success(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&llmstats.Recorder{}, "Aggregate",
		[]llmstats.AggregateResult{
			{CallPoint: "enhance", Count: 5, PromptTokens: 100, CompletionTokens: 50},
			{CallPoint: "chat", Count: 3, PromptTokens: 60, CompletionTokens: 30},
		}, nil)

	h := NewStatsHandler(&llmstats.Recorder{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/stats/llm?call_point=enhance", nil)

	h.GetLLMStats(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	stats, ok := resp["stats"].([]interface{})
	if !ok || len(stats) != 2 {
		t.Fatalf("stats = %+v (type %T)", resp["stats"], resp["stats"])
	}
	first := stats[0].(map[string]interface{})
	// Behaviour assertion 1: call_point filtering passed through.
	if first["call_point"] != "enhance" {
		t.Errorf("call_point = %v, want enhance", first["call_point"])
	}
	// Behaviour assertion 2: count echoed from recorder.
	if first["count"] != float64(5) {
		t.Errorf("count = %v, want 5", first["count"])
	}
	// Behaviour assertion 3: total_tokens = prompt + completion (derived).
	if first["total_tokens"] != float64(150) {
		t.Errorf("total_tokens = %v, want 150", first["total_tokens"])
	}
}

func TestGetLLMStats_NoCallPoint(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&llmstats.Recorder{}, "Aggregate",
		[]llmstats.AggregateResult{{CallPoint: "chat", Count: 1, PromptTokens: 10, CompletionTokens: 5}}, nil)

	h := NewStatsHandler(&llmstats.Recorder{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	// No call_point query — aggregates every call point.
	c.Request = httptest.NewRequest("GET", "/api/v1/stats/llm", nil)

	h.GetLLMStats(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetLLMStats_WithSince(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&llmstats.Recorder{}, "Aggregate",
		[]llmstats.AggregateResult{}, nil)

	h := NewStatsHandler(&llmstats.Recorder{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	since := time.Now().Add(-24 * time.Hour).UTC().Format(time.RFC3339)
	c.Request = httptest.NewRequest("GET", "/api/v1/stats/llm?since="+since, nil)

	h.GetLLMStats(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetLLMStats_InvalidSince(t *testing.T) {
	h := NewStatsHandler(&llmstats.Recorder{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/stats/llm?since=not-a-date", nil)

	h.GetLLMStats(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetLLMStats_EmptyResults(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&llmstats.Recorder{}, "Aggregate",
		[]llmstats.AggregateResult{}, nil)

	h := NewStatsHandler(&llmstats.Recorder{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/stats/llm", nil)

	h.GetLLMStats(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]interface{}
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	stats, _ := resp["stats"].([]interface{})
	if len(stats) != 0 {
		t.Errorf("expected empty stats, got %d items", len(stats))
	}
}

func TestGetLLMStats_RecorderError(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&llmstats.Recorder{}, "Aggregate",
		([]llmstats.AggregateResult)(nil), errStatsTest)

	h := NewStatsHandler(&llmstats.Recorder{})
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/stats/llm", nil)

	h.GetLLMStats(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetLLMStats_NilRecorder(t *testing.T) {
	h := NewStatsHandler(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/stats/llm", nil)

	h.GetLLMStats(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("expected 503, got %d: %s", w.Code, w.Body.String())
	}
}

func TestRegisterStatsRoutes_NilHandler(t *testing.T) {
	// A nil StatsHandler must not panic and must register nothing.
	router := gin.New()
	RegisterStatsRoutes(router, nil, nil)
	if len(router.Routes()) != 0 {
		t.Errorf("expected no routes registered, got %d", len(router.Routes()))
	}
}

func TestGetLLMStats_PermissionDenied(t *testing.T) {
	// Route-level: a non-admin role (user) must be rejected by RequirePermission.
	router := gin.New()
	router.Use(func(c *gin.Context) { c.Set("role", "user") })
	router.GET("/api/v1/stats/llm",
		middleware.RequirePermission(model.PermUserManage),
		func(c *gin.Context) { c.JSON(http.StatusOK, gin.H{"should": "not reach"}) })

	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/stats/llm", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusForbidden {
		t.Fatalf("expected 403 for non-admin, got %d: %s", w.Code, w.Body.String())
	}
}

// errStatsTest is a sentinel error reused by recorder-error test cases.
var errStatsTest = newStatsTestError("recorder unavailable")

type statsTestError struct{ msg string }

func (e *statsTestError) Error() string { return e.msg }

func newStatsTestError(msg string) *statsTestError { return &statsTestError{msg: msg} }
