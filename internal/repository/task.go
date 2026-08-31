package repository

import (
	"context"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

//go:generate mockery --name TaskRepository --output ./mocks --outpkg mocks

// TaskRepository defines the data access contract for task definitions.
type TaskRepository interface {
	Create(ctx context.Context, t *task.Task) error
	Get(ctx context.Context, id string) (*task.Task, error)
	UpdateLastRun(ctx context.Context, id string, runAt time.Time) error
	Cancel(ctx context.Context, id string) error
	List(ctx context.Context, userID string, skip, limit int64) ([]*task.Task, int64, error)
	// ListScheduled returns scheduled tasks (cron_expr not empty OR scheduled_at <= now).
	ListScheduled(ctx context.Context, skip, limit int64, now time.Time) ([]*task.Task, int64, error)
	// MarkScheduledDone sets scheduled_done=true for a one-shot task.
	MarkScheduledDone(ctx context.Context, id string) error
	// SetScheduledEnabled toggles the scheduled_enabled flag on a task.
	SetScheduledEnabled(ctx context.Context, id string, enabled bool) error
}

//go:generate mockery --name TaskRunRepository --output ./mocks --outpkg mocks

// TaskRunRepository defines the data access contract for task execution runs.
type TaskRunRepository interface {
	Create(ctx context.Context, r *task.TaskRun) error
	Get(ctx context.Context, id string) (*task.TaskRun, error)
	List(ctx context.Context, taskID string, status string, skip, limit int64) ([]*task.TaskRun, int64, error)
	UpdateStatus(ctx context.Context, id string, status task.Status) error
	UpdateResult(ctx context.Context, id string, result map[string]interface{}) error
	UpdateError(ctx context.Context, id string, errMsg string) error
	UpdateSessionID(ctx context.Context, id, sessionID string) error
	Cancel(ctx context.Context, id string) error
}

//go:generate mockery --name QueueRepository --output ./mocks --outpkg mocks

// QueueRepository defines the data access contract for task run queues.
type QueueRepository interface {
	Enqueue(ctx context.Context, r *task.TaskRun) error
	Dequeue(ctx context.Context, timeout time.Duration) (*task.TaskRun, error)
	// EnqueueRaw pushes a raw job payload (e.g. KB indexing) without creating task/task_run records.
	EnqueueRaw(ctx context.Context, jobType string, payload interface{}) error
}
