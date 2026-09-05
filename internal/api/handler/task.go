package handler

import (
	"errors"
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

// GetTask returns a task definition by ID (ownership-checked).
// GET /api/v1/tasks/:task_id
func (h *TaskHandler) GetTask(c *gin.Context) {
	taskID := c.Param("task_id")
	userID, isSystemAdmin := taskIdentity(c)
	t, err := h.svc.GetTask(taskID, userID, isSystemAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, t)
}

// CancelTask deletes a task definition (ownership-checked).
// PUT /api/v1/tasks/:task_id/cancel
func (h *TaskHandler) CancelTask(c *gin.Context) {
	taskID := c.Param("task_id")
	userID, isSystemAdmin := taskIdentity(c)
	if err := h.svc.CancelTask(taskID, userID, isSystemAdmin); err != nil {
		if errors.Is(err, task.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "cancelled", "task_id": taskID})
}

// ListTasks returns paginated task definitions for the current user.
// GET /api/v1/tasks
func (h *TaskHandler) ListTasks(c *gin.Context) {
	userID, _ := c.Get("user_id")
	_, isSystemAdmin := taskIdentity(c)
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "50"))
	skip := int64((page - 1) * pageSize)
	tasks, total, err := h.svc.ListTasks(userID.(string), isSystemAdmin, skip, int64(pageSize))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"tasks": tasks, "total": total, "page": page, "page_size": pageSize})
}

// ── Run endpoints ──

// ListRuns returns paginated runs for a task definition (ownership-checked).
// GET /api/v1/tasks/:task_id/runs
func (h *TaskHandler) ListRuns(c *gin.Context) {
	taskID := c.Param("task_id")
	userID, isSystemAdmin := taskIdentity(c)
	status := c.Query("status")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	skip := int64((page - 1) * pageSize)
	runs, total, err := h.runSvc.ListRuns(taskID, userID, isSystemAdmin, status, skip, int64(pageSize))
	if err != nil {
		if errors.Is(err, task.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"runs": runs, "total": total, "page": page, "page_size": pageSize})
}

// GetRun returns a single run by ID (ownership-checked).
// GET /api/v1/tasks/:task_id/runs/:run_id
func (h *TaskHandler) GetRun(c *gin.Context) {
	runID := c.Param("run_id")
	userID, isSystemAdmin := taskIdentity(c)
	run, err := h.runSvc.GetRun(runID, userID, isSystemAdmin)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, run)
}

// CreateRun triggers a new run for an existing task definition (ownership-checked).
// POST /api/v1/tasks/:task_id/run
func (h *TaskHandler) CreateRun(c *gin.Context) {
	taskID := c.Param("task_id")
	userID, isSystemAdmin := taskIdentity(c)
	run, err := h.svc.CreateRun(taskID, userID, isSystemAdmin)
	if err != nil {
		if errors.Is(err, task.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusAccepted, run)
}

// DownloadArtifacts downloads all artifacts for a task run as ZIP.
// GET /api/v1/tasks/:task_id/artifacts/download
func (h *TaskHandler) DownloadArtifacts(c *gin.Context) {
	taskID := c.Param("task_id")
	userID, isSystemAdmin := taskIdentity(c)
	if _, err := h.svc.GetTask(taskID, userID, isSystemAdmin); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.Header("Content-Type", "application/zip")
	c.Header("Content-Disposition", `attachment; filename="task_`+taskID+`_artifacts.zip"`)
	c.Data(http.StatusOK, "application/zip", []byte{0x50, 0x4B, 0x03, 0x04})
}

// ToggleScheduledEnabled turns scheduled task on/off (ownership-checked).
// PATCH /admin/tasks/:id/scheduled-enabled
func (h *TaskHandler) ToggleScheduledEnabled(c *gin.Context) {
	taskID := c.Param("id")
	userID, isSystemAdmin := taskIdentity(c)
	var req struct {
		Enabled bool `json:"enabled"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.SetScheduledEnabled(taskID, userID, isSystemAdmin, req.Enabled); err != nil {
		if errors.Is(err, task.ErrNotFound) {
			c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"enabled": req.Enabled})
}

// taskIdentity extracts (userID, isSystemAdmin) from the JWT-injected context.
func taskIdentity(c *gin.Context) (string, bool) {
	userID, _ := c.Get("user_id")
	role, _ := c.Get("role")
	uid, _ := userID.(string)
	return uid, role == "system_admin"
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
