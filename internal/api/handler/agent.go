package handler

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	agentlogic "github.com/luoxiaojun1992/data-agent/internal/logic/agent"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

// ToolLister provides the names of registered ADK tools for the skills API.
type ToolLister interface {
	List() []string
}

// ToolListerFunc adapts a function to ToolLister.
type ToolListerFunc func() []string

// List returns the tool names.
func (f ToolListerFunc) List() []string { return f() }

// AgentHandler exposes the agent task and skills endpoints. Async task
// creation is coordinated by the logic/agent Orchestrator; skill listing
// delegates to the ADK ToolLister.
type AgentHandler struct {
	orch       *agentlogic.Orchestrator
	tasks      domaintask.TaskService
	toolLister ToolLister
}

// NewAgentHandler creates an agent handler wired to the orchestrator, task
// service, and ADK tool lister.
func NewAgentHandler(orch *agentlogic.Orchestrator, tasks domaintask.TaskService, lister ToolLister) *AgentHandler {
	if lister == nil {
		lister = ToolListerFunc(func() []string { return []string{} })
	}
	return &AgentHandler{orch: orch, tasks: tasks, toolLister: lister}
}

// RegisterAgentRoutes registers the agent task and skills routes on an
// authenticated router group.
func RegisterAgentRoutes(rg *gin.RouterGroup, h *AgentHandler) {
	rg.POST("/tasks", h.CreateAgentTask)
	rg.GET("/tasks/:task_id", h.GetAgentTask)
	rg.GET("/skills", h.ListSkills)
	rg.GET("/skills/search", h.SearchSkills)
}

// CreateAgentTask creates an async agent task via the orchestrator.
func (h *AgentHandler) CreateAgentTask(c *gin.Context) {
	var req agentlogic.CreateAgentTaskRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	userID := c.GetString("user_id")
	resp, err := h.orch.CreateAgentTask(c.Request.Context(), userID, req)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, resp)
}

// GetAgentTask returns the status of an async agent task.
func (h *AgentHandler) GetAgentTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if h.tasks == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task service not available"})
		return
	}
	t, err := h.tasks.GetTask(taskID)
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
}

// ListSkills returns all registered ADK tools.
func (h *AgentHandler) ListSkills(c *gin.Context) {
	names := h.toolLister.List()
	if names == nil {
		names = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"skills": names})
}

// SearchSkills searches for tools by name substring.
func (h *AgentHandler) SearchSkills(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' required"})
		return
	}
	var results []string
	for _, name := range h.toolLister.List() {
		if strings.Contains(name, query) {
			results = append(results, name)
		}
	}
	c.JSON(http.StatusOK, gin.H{"query": query, "results": results})
}
