package queue

import (
	"context"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// streamAdapter adapts *Stream to repository.QueueRepository.
type streamAdapter struct {
	stream *Stream
}

// QueueRepository wraps a Stream as a repository.QueueRepository.
func QueueRepository(stream *Stream) repository.QueueRepository {
	return &streamAdapter{stream: stream}
}

func (a *streamAdapter) Enqueue(ctx context.Context, r *task.TaskRun) error {
	return a.stream.Enqueue(ctx, r)
}

func (a *streamAdapter) Dequeue(ctx context.Context, timeout time.Duration) (*task.TaskRun, error) {
	return nil, nil
}

func (a *streamAdapter) EnqueueRaw(ctx context.Context, jobType string, payload interface{}) error {
	return a.stream.EnqueueRaw(ctx, jobType, payload)
}
