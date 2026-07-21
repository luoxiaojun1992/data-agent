// Package task defines the domain contracts and entities for the async
// task subsystem. The TaskService contract lives here so the orchestration
// layer (internal/logic/agent) can depend on the contract without importing
// the service implementation.
package task

// TaskService is the domain contract for task management. The service/task
// package implements this contract; service/task re-exports it as a type
// alias for backward compatibility with existing handler/test imports.
//
//go:generate mockery --name TaskService --output ./mocks --outpkg mocks
type TaskService interface {
	CreateTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}) (*Task, error)
	GetTask(id string) (*Task, error)
	CancelTask(id string) error
	ListTasks(userID string, status string, skip, limit int64) ([]*Task, int64, error)
	UpdateTaskProgress(id string, p *TaskProgress) error
	UpdateTaskResult(id string, result map[string]interface{}) error
	UpdateStatus(id string, status Status) error
	ListAllTasks(userID string) ([]*Task, error)
	RetryTask(id string) (*Task, error)
	BatchCancelTasks(ids []string) error
}
