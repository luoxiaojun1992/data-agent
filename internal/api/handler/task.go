package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/task"
)

// TaskHandler provides HTTP handlers for task operations.
type TaskHandler struct {
	svc *task.Service
}

// NewTaskHandler creates a task handler.
func NewTaskHandler(svc *task.Service) *TaskHandler {
	return &TaskHandler{svc: svc}
}

// CreateTask creates and enqueues a new async task.
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var req struct {
		SessionID  string                 `json:"session_id"`
		Type       string                 `json:"type"`
		SkillChain []string               `json:"skill_chain"`
		Params     map[string]interface{} `json:"params"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	t, err := h.svc.CreateTask(req.SessionID, userID.(string), req.Type, req.SkillChain, req.Params)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, t)
}

// GetTask returns a task by ID.
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	t, err := h.svc.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// CancelTask cancels a running or queued task.
func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if err := h.svc.CancelTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "task_id": taskID})
}

// ListTasks returns recent tasks for the user.
func (h *TaskHandler) ListTasks(c *gin.Context) {
	userID, _ := c.Get("user_id")
	tasks, err := h.svc.ListTasks(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tasks)
}
