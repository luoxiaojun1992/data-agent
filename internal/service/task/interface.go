package task

import (
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

//go:generate mockery --name TaskService --output ./mocks --outpkg mocks

// TaskService defines the task management service contract.
type TaskService interface {
	CreateTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}) (*task.Task, error)
	GetTask(id string) (*task.Task, error)
	CancelTask(id string) error
	ListTasks(userID string, status string, skip, limit int64) ([]*task.Task, int64, error)
	UpdateTaskProgress(id string, p *task.TaskProgress) error
	UpdateTaskResult(id string, result map[string]interface{}) error
	UpdateStatus(id string, status task.Status) error
	ListAllTasks(userID string) ([]*task.Task, error)
	RetryTask(id string) (*task.Task, error)
	BatchCancelTasks(ids []string) error
}

var _ TaskService = (*Service)(nil)
