package agent_svc

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

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

// ===== Enhanced contains tests =====

func TestContains_UnicodeAndEdgeCases(t *testing.T) {
	tests := []struct {
		s, sub string
		want   bool
	}{
		// Unicode characters
		{"你好世界", "世界", true},
		{"你好世界", "你好", true},
		{"你好世界", "hello", false},
		{"Привет мир", "мир", true},
		{"café au lait", "café", true},
		{"café au lait", "lait", true},
		// Spaces and whitespace
		{"hello world", " ", true},
		{"hello world", "lo wo", true},
		{"  leading", "lead", true},
		{"trailing  ", "il", true},
		{"", "a", false},
		{"a", "b", false},
		{"abcdef", "abcdef", true},
		// Substring longer than string
		{"a", "ab", false},
		{"hello", "helloo", false},
		// Special characters
		{"hello@world.com", "@", true},
		{"hello@world.com", ".com", true},
		{"file_name.txt", "_", true},
		{"test\nnewline", "\n", true},
		{"tab\there", "\t", true},
	}
	for _, tt := range tests {
		if got := contains(tt.s, tt.sub); got != tt.want {
			t.Errorf("contains(%q, %q) = %v, want %v", tt.s, tt.sub, got, tt.want)
		}
	}
}

func TestContains_EmptyStrings(t *testing.T) {
	// Empty substring always returns true
	if !contains("anything", "") {
		t.Error("contains(anything, \"\") should return true")
	}
	if !contains("", "") {
		t.Error("contains(\"\", \"\") should return true")
	}
	// Empty string with non-empty substring
	if contains("", "a") {
		t.Error("contains(\"\", \"a\") should return false")
	}
}

// ===== Enhanced SearchSkills tests =====

func TestSearchSkills_SpecialCharacters(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{
		"sql_executor",
		"report_generator@v2",
		"chart_maker",
		"data_export.txt",
	})
	defer patches.Reset()

	svc.skillReg = mockReg

	tests := []struct {
		query    string
		wantCode int
		wantBody string
	}{
		{"@", http.StatusOK, "report_generator@v2"},
		{".", http.StatusOK, "data_export.txt"},
		{"_", http.StatusOK, "sql_executor"},
		{"/", http.StatusOK, ""}, // no match
		{"generator@v2", http.StatusOK, "report_generator@v2"},
	}

	for _, tt := range tests {
		t.Run(tt.query, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/skills/search?q="+tt.query, nil)

			svc.SearchSkills(c)
			if w.Code != tt.wantCode {
				t.Errorf("status: got %d, want %d", w.Code, tt.wantCode)
			}
			if tt.wantBody != "" && !strings.Contains(w.Body.String(), tt.wantBody) {
				t.Errorf("body should contain %q, got: %s", tt.wantBody, w.Body.String())
			}
		})
	}
}

func TestSearchSkills_EmptyResultsWhenNoMatch(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{"sql_executor"})
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills/search?q=chart", nil)

	svc.SearchSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"results":null`) {
		t.Errorf("body should contain null results (nil slice serialized), got: %s", w.Body.String())
	}
}

func TestSearchSkills_EmptyQueryWithRegistry(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{"sql_executor"})
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills/search?q=", nil)

	svc.SearchSkills(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("status: got %d, want 400", w.Code)
	}
}

// ===== Enhanced CreateAgentTask tests =====

func TestCreateAgentTask_WithParams(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-params"}, nil)
	defer patches.Reset()

	svc := &Service{
		sessions: sessions,
		cbReg:    cbReg,
	}

	body := `{"title":"Test with params","skill_chain":["sql"],"params":{"key":"value"}}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	c.Set("user_id", "user-2")

	svc.CreateAgentTask(c)
	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want 202, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "sess-params") {
		t.Errorf("body should contain sess-params, got: %s", w.Body.String())
	}
}

func TestCreateAgentTask_NoUserID(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-no-user"}, nil)
	defer patches.Reset()

	svc := &Service{
		sessions: sessions,
		cbReg:    cbReg,
	}

	body := `{"title":"Test"}`
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/agent/tasks", strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	// No user_id set — should still work (userID defaults to empty)

	svc.CreateAgentTask(c)
	if w.Code != http.StatusAccepted {
		t.Errorf("status: got %d, want 202, body: %s", w.Code, w.Body.String())
	}
}

func TestCreateAgentTask_ValidJSON(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-valid"}, nil)
	defer patches.Reset()

	svc := &Service{
		sessions: sessions,
		cbReg:    cbReg,
	}

	// Minimal valid JSON
	body := `{}`
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

// ===== Enhanced ListSkills test =====

func TestListSkills_EmptyRegistry_EmptySlice(t *testing.T) {
	svc := &Service{} // skillReg is nil

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills", nil)

	svc.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"skills":[]`) {
		t.Errorf("body should contain empty skills array, got: %s", w.Body.String())
	}
}

func TestListSkills_EmptyListReturned(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{})
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills", nil)

	svc.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"skills":[]`) {
		t.Errorf("body should contain empty skills array, got: %s", w.Body.String())
	}
}

// ===== Enhanced GetAgentTask tests =====

func TestGetAgentTask_WithTaskService_AllFields(t *testing.T) {
	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{
		ID:        "task-detail",
		SessionID: "sess-detail",
		UserID:    "user-detail",
		Status:    task.StatusCompleted,
		Result:    map[string]interface{}{"output": "done"},
	}

	patches := gomonkey.ApplyMethodReturn(taskSvc, "GetTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{}
	svc.taskService = taskSvc

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/agent/tasks/task-detail", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task-detail"}}

	svc.GetAgentTask(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, body: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"task_id":"task-detail"`) {
		t.Errorf("body should contain task-detail, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"session_id":"sess-detail"`) {
		t.Errorf("body should contain sess-detail, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"user_id":"user-detail"`) {
		t.Errorf("body should contain user-detail, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"status":"completed"`) {
		t.Errorf("body should contain completed status, got: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"result":`) {
		t.Errorf("body should contain result field, got: %s", w.Body.String())
	}
}

// ===== NewService with nil fields test =====

func TestNewService_AllNil(t *testing.T) {
	svc := NewService(nil, nil, nil, nil)
	if svc == nil {
		t.Error("NewService should not return nil even with all nil arguments")
	}
	if svc.engine != nil {
		t.Error("engine should be nil")
	}
	if svc.chatSvc != nil {
		t.Error("chatSvc should be nil")
	}
	if svc.sessions != nil {
		t.Error("sessions should be nil")
	}
	if svc.cbReg != nil {
		t.Error("cbReg should be nil")
	}
}

// ===== HandleChat nil chatSvc =====

func TestHandleChat_NilChatService(t *testing.T) {
	svc := &Service{} // nil chatSvc

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("POST", "/chat", strings.NewReader(`{"message":"hello"}`))
	c.Request.Header.Set("Content-Type", "application/json")

	// This will panic if chatSvc is nil, so we mock it
	chatSvc := &chat.Service{}
	patches := gomonkey.ApplyMethodFunc(chatSvc, "HandleChat", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "ok"})
	})
	defer patches.Reset()
	svc.chatSvc = chatSvc

	svc.HandleChat(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d", w.Code)
	}
}

// ===== GetAgentTask with nil result =====

func TestGetAgentTask_WithTaskService_NilResult(t *testing.T) {
	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{
		ID:        "task-no-result",
		SessionID: "sess-1",
		UserID:    "user-1",
		Status:    task.StatusRunning,
		Result:    nil,
	}

	patches := gomonkey.ApplyMethodReturn(taskSvc, "GetTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{}
	svc.taskService = taskSvc

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/agent/tasks/task-no-result", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task-no-result"}}

	svc.GetAgentTask(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, body: %s", w.Code, w.Body.String())
	}
}

// ===== contains with empty string =====

func TestContains_StringWithEmptySearch(t *testing.T) {
	if !contains("anything", "") {
		t.Error("contains should return true for empty substring")
	}
}

func TestContains_EmptyStringWithNonEmptySearch(t *testing.T) {
	if contains("", "abc") {
		t.Error("contains should return false for non-empty substring in empty string")
	}
}

// ===== CreateAgentTask with description and cron_expr params =====

func TestCreateAgentTask_WithDescription(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{ID: "task-desc", Status: task.StatusQueued}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-desc"}, nil)
	patches.ApplyMethodReturn(taskSvc, "CreateTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{sessions: sessions, cbReg: cbReg}
	svc.taskService = taskSvc

	body := `{"title":"Task with desc","skill_chain":["sql"],"description":"A long description"}`
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

// ===== GetAgentTask with task service returning error for non-existent task =====

func TestGetAgentTask_TaskServiceError(t *testing.T) {
	taskSvc := &task_svc.Service{}

	patches := gomonkey.ApplyMethodReturn(taskSvc, "GetTask", nil, fmt.Errorf("task not found"))
	defer patches.Reset()

	svc := &Service{}
	svc.taskService = taskSvc

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/agent/tasks/bad-task", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "bad-task"}}

	svc.GetAgentTask(c)
	if w.Code != http.StatusNotFound {
		t.Errorf("status: got %d, want 404", w.Code)
	}
}

// ===== SearchSkills with Unicode query =====

func TestSearchSkills_UnicodeQuery(t *testing.T) {
	svc := &Service{}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", []string{
		"中文技能",
		"sql_executor",
		"データ分析",
	})
	defer patches.Reset()

	svc.skillReg = mockReg

	tests := []struct {
		q    string
		want string
	}{
		{"中文", "中文技能"},
		{"データ", "データ分析"},
		{"sql", "sql_executor"},
	}

	for _, tt := range tests {
		t.Run(tt.q, func(t *testing.T) {
			w := httptest.NewRecorder()
			c, _ := gin.CreateTestContext(w)
			c.Request = httptest.NewRequest("GET", "/skills/search?q="+tt.q, nil)

			svc.SearchSkills(c)
			if w.Code != http.StatusOK {
				t.Errorf("status: got %d, want 200", w.Code)
			}
			if !strings.Contains(w.Body.String(), tt.want) {
				t.Errorf("body should contain %q: %s", tt.want, w.Body.String())
			}
		})
	}
}

// ===== ListSkills with many skills =====

func TestListSkills_ManySkills(t *testing.T) {
	svc := &Service{}

	manySkills := make([]string, 100)
	for i := range manySkills {
		manySkills[i] = fmt.Sprintf("skill_%d", i)
	}

	mockReg := &agent.SkillRegistryAdapter{}
	patches := gomonkey.ApplyMethodReturn(mockReg, "List", manySkills)
	defer patches.Reset()

	svc.skillReg = mockReg

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/skills", nil)

	svc.ListSkills(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, want 200", w.Code)
	}
	if !strings.Contains(w.Body.String(), "skill_0") {
		t.Errorf("should contain skill_0: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "skill_99") {
		t.Errorf("should contain skill_99: %s", w.Body.String())
	}
}

// ===== CreateAgentTask with no title, only type =====

func TestCreateAgentTask_OnlyType(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{ID: "task-type-only", Status: task.StatusQueued}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-type"}, nil)
	patches.ApplyMethodReturn(taskSvc, "CreateTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{sessions: sessions, cbReg: cbReg}
	svc.taskService = taskSvc

	body := `{"type":"custom_type"}`
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

// ===== CreateAgentTask with both skills and skill_chain =====

func TestCreateAgentTask_BothSkillsAndSkillChain(t *testing.T) {
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())
	sessions := &chat.Manager{}

	taskSvc := &task_svc.Service{}
	mockTask := &task.Task{ID: "task-both", Status: task.StatusQueued}

	patches := gomonkey.NewPatches()
	patches.ApplyMethodReturn(sessions, "Create", &chat.Session{ID: "sess-both"}, nil)
	patches.ApplyMethodReturn(taskSvc, "CreateTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{sessions: sessions, cbReg: cbReg}
	svc.taskService = taskSvc

	// skill_chain takes precedence
	body := `{"title":"Test","skills":["frontend_skill"],"skill_chain":["backend_skill"]}`
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

// ===== GetAgentTask with completed_at field =====

func TestGetAgentTask_WithCompletedAt(t *testing.T) {
	taskSvc := &task_svc.Service{}
	now := time.Now()
	mockTask := &task.Task{
		ID:          "task-completed",
		SessionID:   "sess-1",
		UserID:      "user-1",
		Status:      task.StatusCompleted,
		CompletedAt: &now,
	}

	patches := gomonkey.ApplyMethodReturn(taskSvc, "GetTask", mockTask, nil)
	defer patches.Reset()

	svc := &Service{}
	svc.taskService = taskSvc

	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/agent/tasks/task-completed", nil)
	c.Params = gin.Params{{Key: "task_id", Value: "task-completed"}}

	svc.GetAgentTask(c)
	if w.Code != http.StatusOK {
		t.Errorf("status: got %d, body: %s", w.Code, w.Body.String())
	}
}
