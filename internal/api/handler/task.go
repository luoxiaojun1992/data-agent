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
		Title       string                 `json:"title"`
		Description string                 `json:"description"`
		SessionID   string                 `json:"session_id"`
		Type        string                 `json:"type"`
		Skills      []string               `json:"skills"`
		SkillChain  []string               `json:"skill_chain"`
		Params      map[string]interface{} `json:"params"`
		CronExpr    string                 `json:"cron_expr"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")

	// Normalize: accept both "title" (frontend agent page) and "type" (API)
	taskType := req.Type
	if taskType == "" {
		taskType = req.Title
	}
	if taskType == "" {
		taskType = "agent_exec" // default
	}

	// Normalize: accept both "skills" (frontend) and "skill_chain" (API)
	skillChain := req.SkillChain
	if len(skillChain) == 0 {
		skillChain = req.Skills
	}

	// Build params from request
	params := req.Params
	if params == nil {
		params = make(map[string]interface{})
	}
	if req.Description != "" {
		params["description"] = req.Description
	}
	if req.CronExpr != "" {
		params["cron_expr"] = req.CronExpr
	}

	t, err := h.svc.CreateTask(req.SessionID, userID.(string), taskType, skillChain, params)
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
	tasks, _, err := h.svc.ListTasks(userID.(string), "", 0, 50)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks})
}

// PauseTask pauses a scheduled task.
func (h *TaskHandler) PauseTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if err := h.svc.UpdateStatus(taskID, "paused"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "paused", "task_id": taskID})
}

// ResumeTask resumes a paused scheduled task.
func (h *TaskHandler) ResumeTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if err := h.svc.UpdateStatus(taskID, "active"); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "active", "task_id": taskID})
}

// DownloadArtifacts downloads all artifacts for a task as ZIP.
func (h *TaskHandler) DownloadArtifacts(c *gin.Context) {
	taskID := c.Param("task_id")
	t, err := h.svc.GetTask(taskID)
	if err != nil || t == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "task not found"})
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="task_`+taskID+`_artifacts.zip"`)
	c.Data(http.StatusOK, "application/zip", []byte{0x50, 0x4B, 0x03, 0x04}) // minimal ZIP stub
}

// ListAllTasks returns all tasks globally (admin only, filter by ?status=).
func (h *TaskHandler) ListAllTasks(c *gin.Context) {
	status := c.Query("status")
	tasks, err := h.svc.ListAllTasks(status)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, tasks)
}

// RetryTask retries a failed task.
func (h *TaskHandler) RetryTask(c *gin.Context) {
	taskID := c.Param("task_id")
	t, err := h.svc.RetryTask(taskID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "retried", "task_id": taskID, "task": t})
}

// BatchCancelTasks cancels multiple tasks.
func (h *TaskHandler) BatchCancelTasks(c *gin.Context) {
	var req struct {
		TaskIDs []string `json:"task_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.BatchCancelTasks(req.TaskIDs); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"cancelled": len(req.TaskIDs)})
}
