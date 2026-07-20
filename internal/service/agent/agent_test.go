package agent_svc

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/mongo"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
)

// ── helpers ──

func newTestService() *Service {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()
	mgr := chat.NewManager(mongoinfra.NewSessionRepository(db), 24*time.Hour)
	return NewService(nil, mgr, nil)
}

func newGinContext(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

// ── constructor / wiring ──

func TestNewService(t *testing.T) {
	svc := newTestService()
	if svc.toolLister == nil {
		t.Error("default tool lister should be set")
	}
	if names := svc.toolLister.List(); len(names) != 0 {
		t.Errorf("default lister should be empty: %v", names)
	}
}

func TestWithToolLister(t *testing.T) {
	svc := newTestService()
	svc.WithToolLister(ToolListerFunc(func() []string { return []string{"a", "b"} }))
	if names := svc.toolLister.List(); len(names) != 2 {
		t.Errorf("lister = %v", names)
	}

	// nil lister must not overwrite.
	svc.WithToolLister(nil)
	if names := svc.toolLister.List(); len(names) != 2 {
		t.Errorf("nil lister should be ignored: %v", names)
	}
}

func TestWithTaskService(t *testing.T) {
	svc := newTestService()
	if svc.taskService != nil {
		t.Error("task service should be nil before injection")
	}
	// Injecting a real task service requires mongo; use nil-safe check only.
	svc.WithTaskService(nil)
	if svc.taskService != nil {
		t.Error("nil injection")
	}
}

// ── HandleChat delegation ──

func TestHandleChat_Delegates(t *testing.T) {
	svc := newTestService()
	svc.chatSvc = &chat.Service{}
	patches := gomonkey.ApplyMethod(svc.chatSvc, "HandleChat", func(_ *chat.Service, c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"delegated": true})
	})
	defer patches.Reset()

	c, w := newGinContext("POST", "/chat", `{}`)
	svc.HandleChat(c)
	if w.Code != http.StatusOK {
		t.Errorf("delegation failed: %d", w.Code)
	}
}

// ── CreateAgentTask ──

func TestCreateAgentTask_InvalidJSON(t *testing.T) {
	svc := newTestService()
	c, w := newGinContext("POST", "/agent", "not-json")
	svc.CreateAgentTask(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateAgentTask_NoTaskService(t *testing.T) {
	svc := newTestService()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.sessions, "Create", &chat.Session{ID: "s1", UserID: "u1"}, nil)

	c, w := newGinContext("POST", "/agent", `{"title": "t", "messages": []}`)
	c.Set("user_id", "u1")
	svc.CreateAgentTask(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["task_id"] != "task_memory_fallback" {
		t.Errorf("fallback task id = %v", resp["task_id"])
	}
	if resp["note"] == nil {
		t.Errorf("fallback note missing")
	}
}

func TestCreateAgentTask_SessionError(t *testing.T) {
	svc := newTestService()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.sessions, "Create", (*chat.Session)(nil), fmt.Errorf("db down"))

	c, w := newGinContext("POST", "/agent", `{"title": "t"}`)
	c.Set("user_id", "u1")
	svc.CreateAgentTask(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestCreateAgentTask_WithTaskService(t *testing.T) {
	svc := newTestService()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.sessions, "Create", &chat.Session{ID: "s1", UserID: "u1"}, nil)

	ts := &taskSvcStub{}
	svc.WithTaskService(ts.real(patches))

	c, w := newGinContext("POST", "/agent", `{"title": "t", "skill_chain": ["stats_engine"], "params": {"k": 1}}`)
	c.Set("user_id", "u1")
	svc.CreateAgentTask(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["task_id"] != "task_1" || resp["status"] != "pending" {
		t.Errorf("resp = %v", resp)
	}
}

func TestCreateAgentTask_TaskServiceError(t *testing.T) {
	svc := newTestService()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(svc.sessions, "Create", &chat.Session{ID: "s1", UserID: "u1"}, nil)

	ts := &taskSvcStub{err: fmt.Errorf("redis down")}
	svc.WithTaskService(ts.real(patches))

	c, w := newGinContext("POST", "/agent", `{"title": "t"}`)
	c.Set("user_id", "u1")
	svc.CreateAgentTask(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetAgentTask ──

func TestGetAgentTask_NoService(t *testing.T) {
	svc := newTestService()
	c, w := newGinContext("GET", "/agent/task_1", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	svc.GetAgentTask(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestGetAgentTask_Found(t *testing.T) {
	svc := newTestService()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	ts := &taskSvcStub{}
	svc.WithTaskService(ts.real(patches))

	c, w := newGinContext("GET", "/agent/task_1", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	svc.GetAgentTask(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["task_id"] != "task_1" {
		t.Errorf("resp = %v", resp)
	}
}

func TestGetAgentTask_Error(t *testing.T) {
	svc := newTestService()
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	ts := &taskSvcStub{err: fmt.Errorf("not found")}
	svc.WithTaskService(ts.real(patches))

	c, w := newGinContext("GET", "/agent/missing", "")
	c.Params = gin.Params{{Key: "task_id", Value: "missing"}}
	svc.GetAgentTask(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── ListSkills / SearchSkills ──

func TestListSkills(t *testing.T) {
	svc := newTestService()
	svc.WithToolLister(ToolListerFunc(func() []string {
		return []string{"sql_validate", "stats_compute"}
	}))

	c, w := newGinContext("GET", "/skills", "")
	svc.ListSkills(c)
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

func TestListSkills_NilLister(t *testing.T) {
	svc := newTestService()
	svc.toolLister = ToolListerFunc(func() []string { return nil })
	c, w := newGinContext("GET", "/skills", "")
	svc.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
}

func TestSearchSkills(t *testing.T) {
	svc := newTestService()
	svc.WithToolLister(ToolListerFunc(func() []string {
		return []string{"sql_validate", "stats_compute", "knowledge_search"}
	}))

	c, w := newGinContext("GET", "/skills/search?q=sql", "")
	svc.SearchSkills(c)
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

func TestSearchSkills_MissingQuery(t *testing.T) {
	svc := newTestService()
	c, w := newGinContext("GET", "/skills/search", "")
	svc.SearchSkills(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── contains ──

func TestContains(t *testing.T) {
	cases := []struct {
		s, sub string
		want   bool
	}{
		{"hello", "", true},
		{"hello", "ell", true},
		{"hello", "hello", true},
		{"hi", "hello", false},
		{"hello", "world", false},
	}
	for _, c := range cases {
		if got := contains(c.s, c.sub); got != c.want {
			t.Errorf("contains(%q, %q) = %v, want %v", c.s, c.sub, got, c.want)
		}
	}
}

// ── task service stub (via gomonkey on the real type) ──

type taskSvcStub struct {
	err error
}

func (s *taskSvcStub) real(patches *gomonkey.Patches) *task_svc.Service {
	real := &task_svc.Service{}
	if s.err != nil {
		patches.ApplyMethodReturn(real, "CreateTask", (*task.Task)(nil), s.err)
		patches.ApplyMethodReturn(real, "GetTask", (*task.Task)(nil), s.err)
	} else {
		tk := &task.Task{ID: "task_1", SessionID: "s1", UserID: "u1", Status: task.StatusPending, CreatedAt: time.Now()}
		patches.ApplyMethodReturn(real, "CreateTask", tk, nil)
		patches.ApplyMethodReturn(real, "GetTask", tk, nil)
	}
	return real
}
