// Package task defines the domain contracts and entities for the async
// task subsystem. The TaskService contract lives here so the orchestration
// layer (internal/logic/agent) can depend on the contract without importing
// the service implementation.
package task

import "time"

// TaskService is the domain contract for task definition management.
// Task definitions are persistent metadata; execution state lives on TaskRun.
//
// SPEC-084 §6.6: by-ID operations take a userID + isSystemAdmin pair to enforce
// ownership (IDOR protection). Non-system_admin callers may only operate on
// their own tasks; system_admin is exempt. Internal system callers (scheduler,
// worker) pass isSystemAdmin=true.
//
//go:generate mockery --name TaskService --output ./mocks --outpkg mocks
type TaskService interface {
	// CreateTask creates a new task definition and a first TaskRun,
	// enqueuing the run. Returns (taskDef, taskRun, error).
	CreateTask(userID, taskType string, skillChain []string, params map[string]interface{}, modelID, scheduleMode, cronExpr string, scheduledAt *time.Time) (*Task, *TaskRun, error)
	GetTask(id, userID string, isSystemAdmin bool) (*Task, error)
	CancelTask(id, userID string, isSystemAdmin bool) error
	ListTasks(userID string, isSystemAdmin bool, skip, limit int64) ([]*Task, int64, error)
	// CreateRun creates a new TaskRun from the task definition and enqueues it.
	CreateRun(taskID, userID string, isSystemAdmin bool) (*TaskRun, error)
	// SetScheduledEnabled toggles the on/off flag for scheduled tasks.
	SetScheduledEnabled(taskID, userID string, isSystemAdmin bool, enabled bool) error
}

// TaskRunService is the domain contract for run-level execution state.
// Executors and workers use this contract to read/write run status.
//
//go:generate mockery --name TaskRunService --output ./mocks --outpkg mocks
type TaskRunService interface {
	GetRun(id, userID string, isSystemAdmin bool) (*TaskRun, error)
	ListRuns(taskID, userID string, isSystemAdmin bool, status string, skip, limit int64) ([]*TaskRun, int64, error)
	UpdateRunStatus(id string, status Status) error
	UpdateRunResult(id string, result map[string]interface{}) error
	UpdateRunError(id string, errMsg string) error
	UpdateRunSessionID(id string, sessionID string) error
	CancelRun(id string) error
}
