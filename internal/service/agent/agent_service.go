package agent_svc

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
)

// Service is the unified Agent Service entry point.
// It handles Chat, Agent sync/async, and Task management.
type Service struct {
	engine      *agent.Engine
	chatSvc     *chat.Service
	sessions    *chat.Manager
	cbReg       *security.CircuitBreakerRegistry
	taskService *task_svc.Service      // optional — requires Redis
	skillReg    agent.SkillRegistry    // real skill registry
}

// NewService creates a new Agent Service.
func NewService(engine *agent.Engine, chatSvc *chat.Service, sessions *chat.Manager, cbReg *security.CircuitBreakerRegistry) *Service {
	return &Service{
		engine:   engine,
		chatSvc:  chatSvc,
		sessions: sessions,
		cbReg:    cbReg,
	}
}

// WithTaskService injects the task service for Redis Stream-based async tasks.
func (s *Service) WithTaskService(ts *task_svc.Service) *Service {
	s.taskService = ts
	return s
}

// WithSkillRegistry injects the real skill registry.
func (s *Service) WithSkillRegistry(reg agent.SkillRegistry) *Service {
	s.skillReg = reg
	return s
}

// HandleChat delegates to the Chat Service.
func (s *Service) HandleChat(c *gin.Context) {
	s.chatSvc.HandleChat(c)
}

// CreateAgentTask creates an async agent task via Redis Stream and returns immediately.
func (s *Service) CreateAgentTask(c *gin.Context) {
	var req struct {
		Title      string          `json:"title"`
		Model      string          `json:"model"`
		Messages   []agent.Message `json:"messages"`
		SkillChain []string        `json:"skill_chain"`
		Params     map[string]interface{} `json:"params"`
	}

	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request"})
		return
	}

	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	// Create session
	sess, err := s.sessions.Create(userIDStr, "agent")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
		return
	}

	// Use task service if available (Redis Stream), otherwise fallback to memory
	if s.taskService != nil {
		taskType := "agent"
		skillChain := req.SkillChain
		if skillChain == nil {
			skillChain = []string{}
		}
		t, err := s.taskService.CreateTask(sess.ID, userIDStr, taskType, skillChain, req.Params)
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create task"})
			return
		}

		c.JSON(http.StatusAccepted, gin.H{
			"task_id":    t.ID,
			"session_id": t.SessionID,
			"status":     string(t.Status),
		})
		return
	}

	// Fallback memory-based execution (no Redis available)
	c.JSON(http.StatusAccepted, gin.H{
		"task_id":    "task_memory_fallback",
		"session_id": sess.ID,
		"status":     "queued",
		"note":       "Redis not available — task will not be executed",
	})
}

// GetAgentTask returns the status of an async agent task.
func (s *Service) GetAgentTask(c *gin.Context) {
	taskID := c.Param("task_id")

	if s.taskService != nil {
		t, err := s.taskService.GetTask(taskID)
		if err != nil {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"task_id":    t.ID,
			"session_id": t.SessionID,
			"user_id":    t.UserID,
			"status":     string(t.Status),
			"created_at": t.CreatedAt,
			"result":     t.Result,
		})
		return
	}

	c.JSON(http.StatusNotFound, gin.H{"error": "task service not available"})
}

// ListSkills returns all registered skills.
func (s *Service) ListSkills(c *gin.Context) {
	if s.skillReg != nil {
		names := s.skillReg.List()
		if names == nil {
			names = []string{}
		}
		c.JSON(http.StatusOK, gin.H{"skills": names})
		return
	}
	c.JSON(http.StatusOK, gin.H{"skills": []string{}})
}

// SearchSkills searches for skills by name/description.
func (s *Service) SearchSkills(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' required"})
		return
	}

	if s.skillReg != nil {
		// Check if registry supports search via strings.Contains on List()
		names := s.skillReg.List()
		var results []string
		for _, name := range names {
			if contains(name, query) {
				results = append(results, name)
			}
		}
		c.JSON(http.StatusOK, gin.H{"query": query, "results": results})
		return
	}

	c.JSON(http.StatusOK, gin.H{"query": query, "results": []string{}})
}

func contains(s, substr string) bool {
	if len(substr) == 0 {
		return true
	}
	if len(s) < len(substr) {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
