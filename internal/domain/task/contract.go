// Package task defines the domain contracts and entities for the async
// task subsystem. The TaskService contract lives here so the orchestration
// layer (internal/logic/agent) can depend on the contract without importing
// the service implementation.
package task

import "time"

// TaskService is the domain contract for task definition management.
// Task definitions are persistent metadata; execution state lives on TaskRun.
//
//go:generate mockery --name TaskService --output ./mocks --outpkg mocks
type TaskService interface {
	// CreateTask creates a new task definition and a first TaskRun,
	// enqueuing the run. Returns (taskDef, taskRun, error).
	CreateTask(userID, taskType string, skillChain []string, params map[string]interface{}, modelID, scheduleMode, cronExpr string, scheduledAt *time.Time) (*Task, *TaskRun, error)
	GetTask(id string) (*Task, error)
	CancelTask(id string) error
	ListTasks(userID string, skip, limit int64) ([]*Task, int64, error)
	// CreateRun creates a new TaskRun from the task definition and enqueues it.
	CreateRun(taskID string) (*TaskRun, error)
	// SetScheduledEnabled toggles the on/off flag for scheduled tasks.
	SetScheduledEnabled(taskID string, enabled bool) error
}

// TaskRunService is the domain contract for run-level execution state.
// Executors and workers use this contract to read/write run status.
//
//go:generate mockery --name TaskRunService --output ./mocks --outpkg mocks
type TaskRunService interface {
	GetRun(id string) (*TaskRun, error)
	ListRuns(taskID string, status string, skip, limit int64) ([]*TaskRun, int64, error)
	UpdateRunStatus(id string, status Status) error
	UpdateRunResult(id string, result map[string]interface{}) error
	UpdateRunError(id string, errMsg string) error
	UpdateRunSessionID(id string, sessionID string) error
	CancelRun(id string) error
}
