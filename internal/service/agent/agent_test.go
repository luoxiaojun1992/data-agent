package agent_svc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
)

func init() { gin.SetMode(gin.TestMode) }

func TestNewAgentService(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	if svc == nil {
		t.Error("NewService should not return nil")
	}
}

func TestWithTaskService(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	result := svc.WithTaskService(nil)
	if result != svc {
		t.Error("WithTaskService should return self")
	}
}

func TestWithSkillRegistry(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	result := svc.WithSkillRegistry(nil)
	if result != svc {
		t.Error("WithSkillRegistry should return self")
	}
}

func TestContains(t *testing.T) {
	tests := []struct {
		s, sub string
		want   bool
	}{
		{"hello", "ell", true},
		{"hello", "hello", true},
		{"hello", "world", false},
		{"hello", "", true},
		{"hi", "hello", false},
	}
	for _, tt := range tests {
		if got := contains(tt.s, tt.sub); got != tt.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.sub, got, tt.want)
		}
	}
}

func TestListSkills_Empty(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	svc := NewService(nil, nil, nil, cbReg)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills", nil)

	svc.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d", w.Code)
	}
}

func TestSearchSkills_EmptyQuery(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills/search", nil)

	svc.SearchSkills(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

func TestSearchSkills_WithQuery(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills/search?q=sql", nil)

	svc.SearchSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, body: %s", w.Code, w.Body.String())
	}
}

func TestCreateAgentTask(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	svc := NewService(nil, nil, nil, cbReg)
	sessions := &chat.Manager{}
	svc.sessions = sessions

	patches := gomonkey.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-1"}, nil)
	defer patches.Reset()

	t.Run("invalid body returns 400", func(t *testing.T) {
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(`{bad`))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", "user-1")

		svc.CreateAgentTask(c)
		if w.Code != http.StatusBadRequest {
			t.Errorf("status: got %d, want 400", w.Code)
		}
	})

	t.Run("creates task via memory fallback", func(t *testing.T) {
		body := `{"title":"Test","skill_chain":["sql"]}`
		w := httptest.NewRecorder()
		c, _ := gin.CreateTestContext(w)
		c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(body))
		c.Request.Header.Set("Content-Type", "application/json")
		c.Set("user_id", "user-1")

		svc.CreateAgentTask(c)
		if w.Code != http.StatusAccepted {
			t.Errorf("status: got %d", w.Code)
		}
	})
}

func TestGetAgentTask_NoTaskService(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/agent/tasks/task-1", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task-1"}}

	svc.GetAgentTask(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

// ===== Gomonkey-based tests =====

func TestHandleChat(t *testing.T) {
	chatSvc := &chat.Service{}
	svc := &Service{chatSvc: chatSvc}

	var called bool
	patches := gomonkey.ApplyMethodFunc(chatSvc, "HandleChat", func(c *gin.Context) {
		called = true
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	defer patches.Reset()

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	svc.HandleChat(c)
	if !called {
		t.Error("HandleChat should delegate to chatSvc.HandleChat")
	}
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestListSkills_WithRegistry(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{"sql_executor", "report_generator"})
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills", nil)

	svc.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sql_executor") {
		t.Errorf("body should contain sql_executor, got: %s", w.Body.String())
	}
}

func TestListSkills_WithRegistryNilList(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", nil)
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills", nil)

	svc.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestSearchSkills_WithRegistry(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{"sql_executor", "report_generator", "chart_maker"})
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills/search?q=sql", nil)

	svc.SearchSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "sql_executor") {
		t.Errorf("body should contain sql_executor, got: %s", w.Body.String())
	}
}

func TestSearchSkills_WithRegistryNoMatch(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{"sql_executor", "report_generator"})
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills/search?q=nonexistent", nil)

	svc.SearchSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
}

func TestCreateAgentTask_WithTaskService(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{
		ID:        "task-1",
		SessionID: "sess-1",
		Status:    task.StatusQueued,
	}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-1"}, nil)
	patches.ApplyMethodReturn(taskSvc, "CreateTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{
		sessions: sessions,
		cbReg:    cbReg,
	}
	svc.taskService = taskSvc

	body := `{"title":"Test","skill_chain":["sql"]}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "user-1")

	svc.CreateAgentTask(c)
	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want 202, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "task-1") {
		t.Errorf("body should contain task-1, got: %s", w.Body.String())
	}
}

func TestCreateAgentTask_TaskServiceError(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	taskSvc := &task_svc.Service{}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-1"}, nil)
	patches.ApplyMethodReturn(taskSvc, "CreateTask", nil, fmt.Errorf("db error"))
	defer patches.Reset()

	svc := &Service{
		sessions: sessions,
		cbReg:    cbReg,
	}
	svc.taskService = taskSvc

	body := `{"title":"Test","skill_chain":["sql"]}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "user-1")

	svc.CreateAgentTask(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
}

func TestCreateAgentTask_SessionError(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	patches := gomonkey.ApplyMethodReturn(sessions, "Create", nil, fmt.Errorf("db error"))
	defer patches.Reset()

	svc := &Service{
		sessions: sessions,
		cbReg:    cbReg,
	}

	body := `{"title":"Test","skill_chain":["sql"]}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "user-1")

	svc.CreateAgentTask(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("status: got %d, want 500", w.Code)
	}
}

func TestCreateAgentTask_WithTaskServiceNilSkillChain(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{
		ID:        "task-1",
		SessionID: "sess-1",
		Status:    task.StatusQueued,
	}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-1"}, nil)
	patches.ApplyMethodReturn(taskSvc, "CreateTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{
		sessions: sessions,
		cbReg:    cbReg,
	}
	svc.taskService = taskSvc

	body := `{"title":"Test"}` // no skill_chain
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "user-1")

	svc.CreateAgentTask(c)
	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want 202, body: %s", w.Code, w.Body.String())
	}
}

func TestGetAgentTask_WithTaskService(t *testing.T) {
	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{
		ID:        "task-1",
		SessionID: "sess-1",
		UserID:    "user-1",
		Status:    task.StatusCompleted,
	}

	patches := gomonkey.ApplyMethodReturn(taskSvc, "GetTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{}
	svc.taskService = taskSvc

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/agent/tasks/task-1", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task-1"}}

	svc.GetAgentTask(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "task-1") {
		t.Errorf("body should contain task-1, got: %s", w.Body.String())
	}
}

func TestGetAgentTask_WithTaskService_NotFound(t *testing.T) {
	taskSvc := &task_svc.Service{}

	patches := gomonkey.ApplyMethodReturn(taskSvc, "GetTask", nil, fmt.Errorf("task not found"))
	defer patches.Reset()

	svc := &Service{}
	svc.taskService = taskSvc

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/agent/tasks/task-1", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task-1"}}

	svc.GetAgentTask(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}
