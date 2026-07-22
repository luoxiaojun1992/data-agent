package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	domainknowledge "github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	taskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	kbmocks "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

// TestDashboardHandler_Get_AllServicesError verifies the Get endpoint still
// returns 200 with zeroed fields when every underlying service fails — the
// dashboard intentionally swallows errors so partial outages do not break the
// admin UI.
func TestDashboardHandler_Get_AllServicesError(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	kb := kbmocks.NewKnowledgeService(t)

	tasks.On("ListAllTasks", "u1").Return(([]*domaintask.Task)(nil), errStr("task db down"))
	kb.On("ListAllDocs").Return(([]*domainknowledge.KnowledgeDoc)(nil), errStr("kb db down"))

	h := NewDashboardHandler(tasks, kb, nil)
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Set("user_id", "u1")
	c.Request = httptest.NewRequest("GET", "/dashboard", nil)
	h.Get(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 even when all services fail, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	// Errors are ignored; task_stats is still present (zeroed) so the UI can
	// render an empty state.
	taskStats, ok := resp["task_stats"].(map[string]any)
	if !ok {
		t.Errorf("missing task_stats field: %+v", resp)
	} else {
		if taskStats["total"].(float64) != 0 {
			t.Errorf("task_stats.total = %v, want 0 on error", taskStats["total"])
		}
	}
	if _, ok := resp["kb_docs"]; !ok {
		t.Errorf("missing kb_docs field: %+v", resp)
	}
}

// TestRegisterDashboardRoutes verifies that RegisterDashboardRoutes wires the
// /api/v1/dashboard route (renamed from the legacy admin-scoped path in
// SPEC-060) with the given middleware. This exercises the
// RegisterDashboardRoutes function and the new /api/v1/dashboard/trends route.
func TestRegisterDashboardRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	tasks := taskmocks.NewTaskService(t)
	kb := kbmocks.NewKnowledgeService(t)
	tasks.On("ListAllTasks", "u1").Return([]*domaintask.Task{{ID: "t1", Status: domaintask.StatusCompleted}}, nil)
	kb.On("ListAllDocs").Return([]*domainknowledge.KnowledgeDoc{{ID: "d1"}}, nil)
	h := NewDashboardHandler(tasks, kb, nil)
	midd := func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() }
	RegisterDashboardRoutes(r, midd, h)

	// /api/v1/dashboard (stats)
	req := httptest.NewRequest("GET", "/api/v1/dashboard", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "task_stats") {
		t.Errorf("expected task_stats field in body, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "kb_docs") {
		t.Errorf("expected kb_docs field in body, got %s", w.Body.String())
	}

	// /api/v1/dashboard/trends
	reqTrends := httptest.NewRequest("GET", "/api/v1/dashboard/trends", nil)
	wTrends := httptest.NewRecorder()
	r.ServeHTTP(wTrends, reqTrends)
	if wTrends.Code != http.StatusOK {
		t.Errorf("expected 200 for trends, got %d: %s", wTrends.Code, wTrends.Body.String())
	}
	if !strings.Contains(wTrends.Body.String(), "call_trend") {
		t.Errorf("expected call_trend field in trends body, got %s", wTrends.Body.String())
	}
	if !strings.Contains(wTrends.Body.String(), "token_trend") {
		t.Errorf("expected token_trend field in trends body, got %s", wTrends.Body.String())
	}
}
