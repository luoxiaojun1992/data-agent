package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	domainknowledge "github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	taskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	kbmocks "github.com/luoxiaojun1992/data-agent/internal/service/knowledge/mocks"
)

// TestDashboardHandler_Get_AllServicesError verifies the Get endpoint still
// returns 200 with nil fields when every underlying service fails — the
// dashboard intentionally swallows errors so partial outages do not break the
// admin UI.
func TestDashboardHandler_Get_AllServicesError(t *testing.T) {
	tasks := taskmocks.NewTaskService(t)
	sessions := chatmocks.NewSessionService(t)
	kb := kbmocks.NewKnowledgeService(t)

	tasks.On("ListAllTasks", "u1").Return(([]*domaintask.Task)(nil), errStr("task db down"))
	sessions.On("ListByUser", "u1").Return(([]*domainchat.Session)(nil), errStr("session db down"))
	kb.On("ListAllDocs").Return(([]*domainknowledge.KnowledgeDoc)(nil), errStr("kb db down"))

	h := NewDashboardHandler(tasks, sessions, kb)
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
	// Errors are ignored; fields are still present (nil values) so the UI can
	// render an empty state.
	if _, ok := resp["tasks"]; !ok {
		t.Errorf("missing tasks field: %+v", resp)
	}
	if _, ok := resp["sessions"]; !ok {
		t.Errorf("missing sessions field: %+v", resp)
	}
	if _, ok := resp["docs"]; !ok {
		t.Errorf("missing docs field: %+v", resp)
	}
}

// ensure mock package referenced.
var _ = mock.Anything

// TestRegisterDashboardRoutes verifies that RegisterDashboardRoutes wires the
// /api/v1/admin/dashboard route with the given middleware. This exercises the
// previously uncovered RegisterDashboardRoutes function.
func TestRegisterDashboardRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	tasks := taskmocks.NewTaskService(t)
	sessions := chatmocks.NewSessionService(t)
	kb := kbmocks.NewKnowledgeService(t)
	tasks.On("ListAllTasks", "u1").Return([]*domaintask.Task{{ID: "t1"}}, nil)
	sessions.On("ListByUser", "u1").Return([]*domainchat.Session{{ID: "s1"}}, nil)
	kb.On("ListAllDocs").Return([]*domainknowledge.KnowledgeDoc{{ID: "d1"}}, nil)
	h := NewDashboardHandler(tasks, sessions, kb)
	midd := func(c *gin.Context) { c.Set("user_id", "u1"); c.Next() }
	RegisterDashboardRoutes(r, midd, h)

	req := httptest.NewRequest("GET", "/api/v1/admin/dashboard", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "tasks") {
		t.Errorf("expected tasks field in body, got %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sessions") {
		t.Errorf("expected sessions field in body, got %s", w.Body.String())
	}
}
