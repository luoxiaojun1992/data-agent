package repository

import (
	"context"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

//go:generate mockery --name TaskRepository --output ./mocks --outpkg mocks

// TaskRepository defines the data access contract for agent tasks.
type TaskRepository interface {
	Create(ctx context.Context, t *task.Task) error
	Get(ctx context.Context, id string) (*task.Task, error)
	Cancel(ctx context.Context, id string) error
	List(ctx context.Context, userID string, status string, skip, limit int64) ([]*task.Task, int64, error)
	ListAll(ctx context.Context, userID string) ([]*task.Task, error)
	UpdateProgress(ctx context.Context, id string, p *task.TaskProgress) error
	UpdateResult(ctx context.Context, id string, result map[string]interface{}) error
	Retry(ctx context.Context, id string, t *task.Task) error
	CountByStatus(ctx context.Context, userID string, status string) (int64, error)
}

//go:generate mockery --name QueueRepository --output ./mocks --outpkg mocks

// QueueRepository defines the data access contract for task queues.
type QueueRepository interface {
	Enqueue(ctx context.Context, t *task.Task) error
	Dequeue(ctx context.Context, timeout time.Duration) (*task.Task, error)
}
