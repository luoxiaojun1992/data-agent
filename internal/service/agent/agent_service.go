package agent_svc

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
)

// Service is the unified Agent Service entry point.
// It handles Chat, Agent sync/async, and Task management.
type Service struct {
	engine    *agent.Engine
	chatSvc   *chat.Service
	sessions  *chat.Manager
	cbReg     *security.CircuitBreakerRegistry
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

// AgentTask represents an async agent task.
type AgentTask struct {
	TaskID    string    `json:"task_id"`
	SessionID string    `json:"session_id"`
	UserID    string    `json:"user_id"`
	Status    string    `json:"status"` // "pending", "running", "completed", "failed"
	CreatedAt time.Time `json:"created_at"`
}

// tasks is an in-memory task store (will be persisted to MongoDB in SPEC-009).
var tasks = make(map[string]*AgentTask)

// HandleChat delegates to the Chat Service.
func (s *Service) HandleChat(c *gin.Context) {
	s.chatSvc.HandleChat(c)
}

// CreateAgentTask creates an async agent task and returns immediately.
func (s *Service) CreateAgentTask(c *gin.Context) {
	var req struct {
		Model    string          `json:"model"`
		Messages []agent.Message `json:"messages"`
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

	taskID := "task_" + uuid.New().String()[:8]
	task := &AgentTask{
		TaskID:    taskID,
		SessionID: sess.ID,
		UserID:    userIDStr,
		Status:    "pending",
		CreatedAt: time.Now(),
	}

	tasks[taskID] = task

	// Execute in background (full async implementation in SPEC-009)
	go func() {
		task.Status = "running"
		agentReq := agent.ChatRequest{
			Model:    req.Model,
			Messages: req.Messages,
		}
		_, err := s.engine.Run(c.Request.Context(), agentReq)
		if err != nil {
			task.Status = "failed"
		} else {
			task.Status = "completed"
		}
	}()

	c.JSON(http.StatusAccepted, gin.H{
		"task_id":    taskID,
		"session_id": sess.ID,
		"status":     task.Status,
	})
}

// GetAgentTask returns the status of an async agent task.
func (s *Service) GetAgentTask(c *gin.Context) {
	taskID := c.Param("task_id")
	task, exists := tasks[taskID]
	if !exists {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.JSON(http.StatusOK, task)
}

// ListSkills returns all registered skills.
func (s *Service) ListSkills(c *gin.Context) {
	// Skills will be registered via the Registry from SPEC-008
	c.JSON(http.StatusOK, gin.H{"skills": []string{"placeholder"}})
}

// SearchSkills searches for skills by name/description.
func (s *Service) SearchSkills(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' required"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"query": query, "results": []string{}})
}
