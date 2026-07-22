package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	taskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	agentlogic "github.com/luoxiaojun1992/data-agent/internal/logic/agent"
)

// newAgentErrGin is a tiny helper local to the agent error tests so we can
// construct contexts without colliding with the existing newAgentGinContext.
func newAgentErrGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// TestAgentHandler_CreateAgentTask_SessionCreateError wires a real
// agentlogic.Orchestrator to a SessionService mock that fails Create. The
// handler should translate the orchestrator error into a 500.
func TestAgentHandler_CreateAgentTask_SessionCreateError(t *testing.T) {
	sessions := chatmocks.NewSessionService(t)
	sessions.On("Create", "u1", "agent").
		Return((*domainchat.Session)(nil), errStr("session create failed"))
	orch := agentlogic.NewOrchestrator(sessions, nil)
	h := NewAgentHandler(orch, nil, nil)

	c, w := newAgentErrGin("POST", "/agent/tasks", `{"title":"t","messages":[]}`)
	c.Set("user_id", "u1")
	h.CreateAgentTask(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "error") {
		t.Errorf("expected error field in body, got %s", w.Body.String())
	}
}

// TestAgentHandler_CreateAgentTask_TaskCreateError wires a real orchestrator
// with a SessionService that succeeds and a TaskService that fails CreateTask.
// The orchestrator returns an error and the handler should respond 500.
func TestAgentHandler_CreateAgentTask_TaskCreateError(t *testing.T) {
	sessions := chatmocks.NewSessionService(t)
	sessions.On("Create", "u1", "agent").
		Return(&domainchat.Session{ID: "s1"}, nil)
	tasks := taskmocks.NewTaskService(t)
	tasks.On("CreateTask", "s1", "u1", "agent", []string{}, (map[string]interface{})(nil)).
		Return((*domaintask.Task)(nil), errStr("task create failed"))
	orch := agentlogic.NewOrchestrator(sessions, tasks)
	h := NewAgentHandler(orch, tasks, nil)

	c, w := newAgentErrGin("POST", "/agent/tasks", `{"title":"t","messages":[]}`)
	c.Set("user_id", "u1")
	h.CreateAgentTask(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "error") {
		t.Errorf("expected error field in body, got %s", w.Body.String())
	}
}

// TestAgentHandler_CreateAgentTask_FallbackSuccess exercises the memory
// fallback path inside the orchestrator: sessions succeed, task service is
// nil, and the handler should respond 202 with the fallback response.
func TestAgentHandler_CreateAgentTask_FallbackSuccess(t *testing.T) {
	sessions := chatmocks.NewSessionService(t)
	sessions.On("Create", "u1", "agent").
		Return(&domainchat.Session{ID: "s1"}, nil)
	orch := agentlogic.NewOrchestrator(sessions, nil)
	h := NewAgentHandler(orch, nil, nil)

	c, w := newAgentErrGin("POST", "/agent/tasks", `{"title":"t","messages":[]}`)
	c.Set("user_id", "u1")
	h.CreateAgentTask(c)

	if w.Code != http.StatusAccepted {
		t.Errorf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "task_memory_fallback") {
		t.Errorf("expected fallback task id, got %s", w.Body.String())
	}
}

// TestAgentHandler_SearchSkills_EmptyResults exercises the no-match branch of
// SearchSkills: a valid query that matches no tool names returns 200 with an
// empty (nil) results slice.
func TestAgentHandler_SearchSkills_EmptyResults(t *testing.T) {
	h := NewAgentHandler(nil, nil, ToolListerFunc(func() []string {
		return []string{"sql_validate", "stats_compute"}
	}))
	c, w := newAgentErrGin("GET", "/skills/search?q=nomatch", "")
	h.SearchSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "nomatch") {
		t.Errorf("expected query echoed in body, got %s", w.Body.String())
	}
	// results should be absent/empty (JSON marshal of nil slice → null).
	if !strings.Contains(w.Body.String(), `"results":null`) {
		// Some encoders emit "[]"; accept either by checking the body does not
		// contain a skill name.
		if strings.Contains(w.Body.String(), "sql_validate") {
			t.Errorf("expected no matches, got %s", w.Body.String())
		}
	}
}

// TestAgentHandler_ListSkills_DefaultEmptyLister covers the branch inside
// ListSkills where the configured lister returns nil (the handler coerces to
// an empty slice before serializing).
func TestAgentHandler_ListSkills_DefaultEmptyLister(t *testing.T) {
	h := NewAgentHandler(nil, nil, ToolListerFunc(func() []string { return nil }))
	c, w := newAgentErrGin("GET", "/skills", "")
	h.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"skills":[]`) {
		t.Errorf("expected empty skills array, got %s", w.Body.String())
	}
}

// ensure mock package referenced.
var _ = mock.Anything

// TestRegisterAgentRoutes verifies that RegisterAgentRoutes wires the agent
// task and skills routes on the given router group. This exercises the
// previously uncovered RegisterAgentRoutes function.
func TestRegisterAgentRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewAgentHandler(nil, nil, ToolListerFunc(func() []string {
		return []string{"sql_validate"}
	}))
	api := r.Group("/api/v1")
	RegisterAgentRoutes(api, h)

	// GET /api/v1/skills → 200 (ListSkills)
	req := httptest.NewRequest("GET", "/api/v1/skills", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("ListSkills expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sql_validate") {
		t.Errorf("expected skill in body, got %s", w.Body.String())
	}

	// GET /api/v1/skills/search?q=sql → 200 (SearchSkills)
	req = httptest.NewRequest("GET", "/api/v1/skills/search?q=sql", nil)
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("SearchSkills expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sql_validate") {
		t.Errorf("expected match in body, got %s", w.Body.String())
	}
}
