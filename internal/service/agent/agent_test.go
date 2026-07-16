package agent_svc

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
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
