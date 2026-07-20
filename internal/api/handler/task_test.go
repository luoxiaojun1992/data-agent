package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	mocktasksvc "github.com/luoxiaojun1992/data-agent/internal/service/task/mocks"
)

func init() { gin.SetMode(gin.TestMode) }

// ── NewTaskHandler ──

func TestNewTaskHandler(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)
	if h == nil {
		t.Fatal("NewTaskHandler returned nil")
	}
	if h.svc == nil {
		t.Error("svc not set correctly")
	}
}

// ── CreateTask ──

func TestCreateTask_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	now := time.Now()
	mockTask := &task.Task{
		ID:        "task_1",
		SessionID: "sess-1",
		UserID:    "user-1",
		Type:      "agent_exec",
		Status:    task.StatusQueued,
		CreatedAt: now,
		UpdatedAt: now,
	}

	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( mockTask, nil)

	body := `{"title":"agent_exec","session_id":"sess-1","skill_chain":["sql","report"]}`
	c, w := newGinContext("POST", "/tasks", body)
	c.Set("user_id", "user-1")
	h.CreateTask(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTask_DefaultType(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	mockTask := &task.Task{ID: "task_2", Type: "agent_exec"}

	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( mockTask, nil)

	// Empty title and type → defaults to "agent_exec"
	body := `{}`
	c, w := newGinContext("POST", "/tasks", body)
	c.Set("user_id", "user-1")
	h.CreateTask(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTask_FromFrontend(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	mockTask := &task.Task{ID: "task_3", Type: "agent_exec"}

	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( mockTask, nil)

	// Frontend sends "title" and "skills" (not "type" and "skill_chain")
	body := `{"title":"My Task","skills":["sql","chart"],"description":"Do something"}`
	c, w := newGinContext("POST", "/tasks", body)
	c.Set("user_id", "user-1")
	h.CreateTask(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTask_WithParams(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	mockTask := &task.Task{ID: "task_4", Type: "agent_exec"}

	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( mockTask, nil)

	body := `{"title":"Task","skill_chain":["sql"],"params":{"key":"value"}}`
	c, w := newGinContext("POST", "/tasks", body)
	c.Set("user_id", "user-1")
	h.CreateTask(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTask_WithCronExpr(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	mockTask := &task.Task{ID: "task_5", Type: "agent_exec"}

	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( mockTask, nil)

	body := `{"title":"Scheduled","skill_chain":["sql"],"cron_expr":"0 0 * * *"}`
	c, w := newGinContext("POST", "/tasks", body)
	c.Set("user_id", "user-1")
	h.CreateTask(c)

	if w.Code != http.StatusAccepted {
		t.Fatalf("expected 202, got %d: %s", w.Code, w.Body.String())
	}
}

func TestCreateTask_InvalidJSON(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	c, w := newGinContext("POST", "/tasks", "not-json")
	c.Set("user_id", "user-1")
	h.CreateTask(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestCreateTask_ServiceError(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("queue full"))

	body := `{"title":"agent_exec"}`
	c, w := newGinContext("POST", "/tasks", body)
	c.Set("user_id", "user-1")
	h.CreateTask(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── GetTask ──

func TestGetTask_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	mockTask := &task.Task{
		ID:        "task_1",
		SessionID: "sess-1",
		UserID:    "user-1",
		Status:    task.StatusCompleted,
	}

	svc.On("GetTask", mock.Anything).Return(mock.Anything).Return( mockTask, nil)

	c, w := newGinContext("GET", "/tasks/task_1", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.GetTask(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestGetTask_NotFound(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("GetTask", mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("not found"))

	c, w := newGinContext("GET", "/tasks/missing", "")
	c.Params = gin.Params{{Key: "task_id", Value: "missing"}}
	h.GetTask(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── CancelTask ──

func TestCancelTask_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("CancelTask", mock.Anything).Return(mock.Anything).Return( nil)

	c, w := newGinContext("POST", "/tasks/task_1/cancel", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.CancelTask(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "cancelled") {
		t.Errorf("body should contain cancelled: %s", w.Body.String())
	}
}

func TestCancelTask_Error(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("CancelTask", mock.Anything).Return(mock.Anything).Return( fmt.Errorf("cannot cancel completed"))

	c, w := newGinContext("POST", "/tasks/task_1/cancel", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.CancelTask(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ListTasks ──

func TestListTasks_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	tasks := []*task.Task{
		{ID: "task_1", UserID: "user-1", Status: task.StatusCompleted},
		{ID: "task_2", UserID: "user-1", Status: task.StatusRunning},
	}

	svc.On("ListTasks", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( tasks, int64(len(tasks)), nil)

	c, w := newGinContext("GET", "/tasks", "")
	c.Set("user_id", "user-1")
	h.ListTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListTasks_Error(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("ListTasks", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( ([]*task.Task)(nil), int64(0), fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/tasks", "")
	c.Set("user_id", "user-1")
	h.ListTasks(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListTasks_Empty(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("ListTasks", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( ([]*task.Task)(nil), int64(0), nil)

	c, w := newGinContext("GET", "/tasks", "")
	c.Set("user_id", "user-1")
	h.ListTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── PauseTask ──

func TestPauseTask_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("UpdateStatus", mock.Anything, mock.Anything).Return(mock.Anything).Return( nil)

	c, w := newGinContext("POST", "/tasks/task_1/pause", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.PauseTask(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "paused") {
		t.Errorf("body should contain paused: %s", w.Body.String())
	}
}

func TestPauseTask_Error(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("UpdateStatus", mock.Anything, mock.Anything).Return(mock.Anything).Return( fmt.Errorf("invalid status transition"))

	c, w := newGinContext("POST", "/tasks/task_1/pause", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.PauseTask(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ResumeTask ──

func TestResumeTask_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("UpdateStatus", mock.Anything, mock.Anything).Return(mock.Anything).Return( nil)

	c, w := newGinContext("POST", "/tasks/task_1/resume", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.ResumeTask(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "active") {
		t.Errorf("body should contain active: %s", w.Body.String())
	}
}

func TestResumeTask_Error(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("UpdateStatus", mock.Anything, mock.Anything).Return(mock.Anything).Return( fmt.Errorf("task not found"))

	c, w := newGinContext("POST", "/tasks/task_1/resume", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.ResumeTask(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── DownloadArtifacts ──

func TestDownloadArtifacts_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	mockTask := &task.Task{ID: "task_1", UserID: "user-1", Status: task.StatusCompleted}

	svc.On("GetTask", mock.Anything).Return(mock.Anything).Return( mockTask, nil)

	c, w := newGinContext("GET", "/tasks/task_1/artifacts", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.DownloadArtifacts(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	contentType := w.Header().Get("Content-Type")
	if contentType != "application/zip" {
		t.Errorf("Content-Type should be application/zip, got: %s", contentType)
	}
}

func TestDownloadArtifacts_TaskNotFound(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("GetTask", mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("not found"))

	c, w := newGinContext("GET", "/tasks/missing/artifacts", "")
	c.Params = gin.Params{{Key: "task_id", Value: "missing"}}
	h.DownloadArtifacts(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestDownloadArtifacts_NilTask(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("GetTask", mock.Anything).Return(mock.Anything).Return( (*task.Task)(nil), nil)

	c, w := newGinContext("GET", "/tasks/task_1/artifacts", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.DownloadArtifacts(c)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

// ── ListAllTasks ──

func TestListAllTasks_All(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	tasks := []*task.Task{
		{ID: "task_1", UserID: "user-1", Status: task.StatusCompleted},
		{ID: "task_2", UserID: "user-2", Status: task.StatusRunning},
		{ID: "task_3", UserID: "user-3", Status: task.StatusFailed},
	}

	svc.On("ListAllTasks", mock.Anything).Return(mock.Anything).Return( tasks, nil)

	c, w := newGinContext("GET", "/admin/tasks", "")
	h.ListAllTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAllTasks_WithStatusFilter(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	tasks := []*task.Task{
		{ID: "task_1", UserID: "user-1", Status: task.StatusRunning},
	}

	svc.On("ListAllTasks", mock.Anything).Return(mock.Anything).Return( tasks, nil)

	c, w := newGinContext("GET", "/admin/tasks?status=running", "")
	h.ListAllTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAllTasks_Error(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("ListAllTasks", mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/admin/tasks", "")
	h.ListAllTasks(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestListAllTasks_Empty(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("ListAllTasks", mock.Anything).Return([]*task.Task{}, nil)

	c, w := newGinContext("GET", "/admin/tasks", "")
	h.ListAllTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// ── RetryTask ──

func TestRetryTask_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("RetryTask", mock.Anything).Return(mock.Anything).Return( nil)

	c, w := newGinContext("POST", "/tasks/task_1/retry", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.RetryTask(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "retried") {
		t.Errorf("body should contain retried: %s", w.Body.String())
	}
}

func TestRetryTask_Error(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("RetryTask", mock.Anything).Return(mock.Anything).Return( fmt.Errorf("only failed tasks can be retried"))

	c, w := newGinContext("POST", "/tasks/task_1/retry", "")
	c.Params = gin.Params{{Key: "task_id", Value: "task_1"}}
	h.RetryTask(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

// ── BatchCancelTasks ──

func TestBatchCancelTasks_Success(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("BatchCancelTasks", mock.Anything).Return( nil)

	body := `{"task_ids":["task_1","task_2","task_3"]}`
	c, w := newGinContext("POST", "/tasks/batch-cancel", body)
	h.BatchCancelTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"cancelled":3`) {
		t.Errorf("body should contain cancelled=3: %s", w.Body.String())
	}
}

func TestBatchCancelTasks_SingleTask(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("BatchCancelTasks", mock.Anything).Return( nil)

	body := `{"task_ids":["task_1"]}`
	c, w := newGinContext("POST", "/tasks/batch-cancel", body)
	h.BatchCancelTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"cancelled":1`) {
		t.Errorf("body should contain cancelled=1: %s", w.Body.String())
	}
}

func TestBatchCancelTasks_InvalidJSON(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	c, w := newGinContext("POST", "/tasks/batch-cancel", "bad")
	h.BatchCancelTasks(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBatchCancelTasks_ServiceError(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("BatchCancelTasks", mock.Anything).Return( fmt.Errorf("db error"))

	body := `{"task_ids":["task_1"]}`
	c, w := newGinContext("POST", "/tasks/batch-cancel", body)
	h.BatchCancelTasks(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestBatchCancelTasks_EmptyList(t *testing.T) {
	svc := mocktasksvc.NewTaskService(t)
	h := NewTaskHandler(svc)

	svc.On("BatchCancelTasks", mock.Anything).Return( nil)

	body := `{"task_ids":[]}`
	c, w := newGinContext("POST", "/tasks/batch-cancel", body)
	h.BatchCancelTasks(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
