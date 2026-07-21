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

func (a *streamAdapter) Enqueue(ctx context.Context, t *task.Task) error {
	return a.stream.Enqueue(ctx, t)
}

func (a *streamAdapter) Dequeue(ctx context.Context, timeout time.Duration) (*task.Task, error) {
	// Not used by TaskService; Dequeue semantics differ between QueueRepository and Stream.
	return nil, nil
}
