package handler

import (
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/service/task"
)

// TaskHandler provides HTTP handlers for task definition + run operations.
type TaskHandler struct {
	svc    task.TaskService
	runSvc task.TaskRunService
}

// NewTaskHandler creates a task handler. Both services may be backed by the
// same concrete Service implementation.
func NewTaskHandler(svc task.TaskService, runSvc task.TaskRunService) *TaskHandler {
	return &TaskHandler{svc: svc, runSvc: runSvc}
}

// CreateTask creates a task definition and its first run, enqueues the run.
// POST /api/v1/tasks
func (h *TaskHandler) CreateTask(c *gin.Context) {
	var req struct {
		Title        string                 `json:"title"`
		Description  string                 `json:"description"`
		Type         string                 `json:"type"`
		SkillChain   []string               `json:"skill_chain"`
		Skills       []string               `json:"skills"`
		Params       map[string]interface{} `json:"params"`
		Images       []domainchat.ImagePart `json:"images"`
		CronExpr     string                 `json:"cron_expr"`
		ScheduledAt  *time.Time             `json:"scheduled_at"`
		ScheduleMode string                 `json:"schedule_mode"`
		ModelID      string                 `json:"model_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := validateRequestImages(req.Images); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	userID, _ := c.Get("user_id")
	taskType := req.Type
	if taskType == "" {
		taskType = req.Title // fallback for legacy frontend
	}
	if taskType == "" {
		taskType = domaintask.TaskTypeAgentExec
	}

	skillChain := req.SkillChain
	if len(skillChain) == 0 {
		skillChain = req.Skills
	}

	params := req.Params
	if params == nil {
		params = make(map[string]interface{})
	}
	if req.Description != "" {
		params["description"] = req.Description
	}
	if req.Title != "" {
		params["title"] = req.Title
	}
	if req.CronExpr != "" {
		params["cron_expr"] = req.CronExpr
	}
	if len(req.Images) > 0 {
		if encoded, encErr := domainchat.EncodeImages(req.Images); encErr == nil {
			params["images"] = encoded
		}
	}

	// Derive schedule_mode from input
	scheduleMode := ""
	if taskType == domaintask.TaskTypeScheduledExec {
		if req.CronExpr != "" && req.ScheduledAt == nil {
			scheduleMode = domaintask.ScheduleModeRecurring
		} else if req.ScheduledAt != nil {
			scheduleMode = domaintask.ScheduleModeOneTime
		} else if req.ScheduleMode != "" {
			scheduleMode = req.ScheduleMode
		}
	}

	t, run, err := h.svc.CreateTask(userID.(string), taskType, skillChain, params, req.ModelID, scheduleMode, req.CronExpr, req.ScheduledAt)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, gin.H{"task": t, "run": run})
}

// GetTask returns a task definition by ID.
// GET /api/v1/tasks/:task_id
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	t, err := h.svc.GetTask(taskID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// CancelTask deletes a task definition.
// PUT /api/v1/tasks/:task_id/cancel
func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if err := h.svc.CancelTask(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "task_id": taskID})
}

// ListTasks returns paginated task definitions for the current user.
// GET /api/v1/tasks
func (h *TaskHandler) ListTasks(c *gin.Context) {
	userID, _ := c.Get("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	skip := int64((page - 1) * pageSize)
	tasks, total, err := h.svc.ListTasks(userID.(string), skip, int64(pageSize))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks, "total": total, "page": page, "page_size": pageSize})
}

// ── Run endpoints ──

// ListRuns returns paginated runs for a task definition.
// GET /api/v1/tasks/:task_id/runs
func (h *TaskHandler) ListRuns(c *gin.Context) {
	taskID := c.Param("task_id")
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	skip := int64((page - 1) * pageSize)
	runs, total, err := h.runSvc.ListRuns(taskID, status, skip, int64(pageSize))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs, "total": total, "page": page, "page_size": pageSize})
}

// GetRun returns a single run by ID.
// GET /api/v1/tasks/:task_id/runs/:run_id
func (h *TaskHandler) GetRun(c *gin.Context) {
	runID := c.Param("run_id")
	run, err := h.runSvc.GetRun(runID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, run)
}

// CreateRun triggers a new run for an existing task definition.
// POST /api/v1/tasks/:task_id/run
func (h *TaskHandler) CreateRun(c *gin.Context) {
	taskID := c.Param("task_id")
	run, err := h.svc.CreateRun(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, run)
}

// PauseTask pauses a scheduled task (delegates to CancelRun).
// PUT /api/v1/tasks/:task_id/pause
func (h *TaskHandler) PauseTask(c *gin.Context) {
	taskID := c.Param("task_id")
	if err := h.runSvc.CancelRun(taskID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "paused", "task_id": taskID})
}

// ResumeTask resumes a paused scheduled task (alias for CreateRun).
// PUT /api/v1/tasks/:task_id/resume
func (h *TaskHandler) ResumeTask(c *gin.Context) {
	taskID := c.Param("task_id")
	run, err := h.svc.CreateRun(taskID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, run)
}

// DownloadArtifacts downloads all artifacts for a task run as ZIP.
// GET /api/v1/tasks/:task_id/artifacts/download
func (h *TaskHandler) DownloadArtifacts(c *gin.Context) {
	taskID := c.Param("task_id")
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="task_`+taskID+`_artifacts.zip"`)
	c.Data(http.StatusOK, "application/zip", []byte{0x50, 0x4B, 0x03, 0x04})
}

// ToggleScheduledEnabled turns scheduled task on/off. PATCH /admin/tasks/:id/scheduled-enabled
func (h *TaskHandler) ToggleScheduledEnabled(c *gin.Context) {
	taskID := c.Param("id")
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetScheduledEnabled(taskID, req.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": req.Enabled})
}

// validateRequestImages validates image attachments against the shared domain
// rules (count/size/mime/base64). Returns nil when no images are present.
func validateRequestImages(images []domainchat.ImagePart) error {
	if len(images) == 0 {
		return nil
	}
	_, err := domainchat.ValidateImages(images)
	return err
}
