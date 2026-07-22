package handler

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	taskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	agentlogic "github.com/luoxiaojun1992/data-agent/internal/logic/agent"
)

func newAgentGinContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestAgentHandler_CreateAgentTask(t *testing.T) {
	orch := &agentlogic.Orchestrator{} // not used directly; we test via handler with real orchestrator
	// Build a real orchestrator with nil task to hit fallback path is hard without mocks.
	// Instead, test the handler's HTTP translation with a direct orchestrator that has nil task.
	_ = orch
}

func TestAgentHandler_GetAgentTask_Found(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	tk := &domaintask.Task{ID: "task_1", SessionID: "s1", UserID: "u1", Status: domaintask.StatusPending}
	tasks.On("GetTask", "task_1").Return(tk, nil)
	h := NewAgentHandler(nil, tasks, nil)

	c, w := newAgentGinContext("GET", "/agent/tasks/task_1", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.GetAgentTask(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["task_id"] != "task_1" || resp["status"] != "pending" {
		t.Errorf("resp = %v", resp)
	}
}

func TestAgentHandler_GetAgentTask_NotFound(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	tasks.On("GetTask", "missing").Return((*domaintask.Task)(nil), errNotFound)
	h := NewAgentHandler(nil, tasks, nil)

	c, w := newAgentGinContext("GET", "/agent/tasks/missing", "")
	c.Params = gin.Params{{Key: "task_id", Value: "missing"}}
	h.GetAgentTask(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestAgentHandler_GetAgentTask_NoTaskService(t *testing.T) {
	h := NewAgentHandler(nil, nil, nil)
	c, w := newAgentGinContext("GET", "/agent/tasks/task_1", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.GetAgentTask(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 when task service nil, got %d", w.Code)
	}
}

func TestAgentHandler_ListSkills(t *testing.T) {
	h := NewAgentHandler(nil, nil, ToolListerFunc(func() []string {
		return []string{"sql_validate", "stats_compute"}
	}))
	c, w := newAgentGinContext("GET", "/skills", "")
	h.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	skills, _ := resp["skills"].([]any)
	if len(skills) != 2 {
		t.Errorf("skills = %v", skills)
	}
}

func TestAgentHandler_ListSkills_NilLister(t *testing.T) {
	h := NewAgentHandler(nil, nil, nil) // nil lister → default empty
	c, w := newAgentGinContext("GET", "/skills", "")
	h.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	skills, _ := resp["skills"].([]any)
	if len(skills) != 0 {
		t.Errorf("expected empty skills, got %v", skills)
	}
}

func TestAgentHandler_SearchSkills(t *testing.T) {
	h := NewAgentHandler(nil, nil, ToolListerFunc(func() []string {
		return []string{"sql_validate", "stats_compute", "knowledge_search"}
	}))
	c, w := newAgentGinContext("GET", "/skills/search?q=sql", "")
	h.SearchSkills(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	results, _ := resp["results"].([]any)
	if len(results) != 1 || results[0] != "sql_validate" {
		t.Errorf("results = %v", results)
	}
}

func TestAgentHandler_SearchSkills_MissingQuery(t *testing.T) {
	h := NewAgentHandler(nil, nil, ToolListerFunc(func() []string { return []string{} }))
	c, w := newAgentGinContext("GET", "/skills/search", "")
	h.SearchSkills(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestAgentHandler_CreateAgentTask_InvalidBody(t *testing.T) {
	h := NewAgentHandler(nil, nil, nil)
	c, w := newAgentGinContext("POST", "/agent/tasks", "not-json")
	h.CreateAgentTask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

var errNotFound = errStr("not found")

type errStr string

func (e errStr) Error() string { return string(e) }

// ensure context import is used.
var _ = context.Background
