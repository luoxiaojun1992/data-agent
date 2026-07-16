package worker

import (
	"context"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	goredis "github.com/redis/go-redis/v9"
)

type mockExecutor struct{}

func (m *mockExecutor) Execute(ctx context.Context, t *task.Task) error { return nil }

func TestNewPool(t *testing.T) {
	stream := &queue.Stream{}
	client := &goredis.Client{}
	exec := &mockExecutor{}

	p := NewPool(stream, client, 4, exec)
	if p == nil {
		t.Error("NewPool should not return nil")
	}
	if p.workers != 4 {
		t.Errorf("workers: got %d, want 4", p.workers)
	}
}

func TestPool_StartStop(t *testing.T) {
	stream := &queue.Stream{}
	client := &goredis.Client{}
	exec := &mockExecutor{}

	p := NewPool(stream, client, 2, exec)
	if p == nil {
		t.Fatal("NewPool returned nil")
	}

	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		defer func() { recover() }()
		p.Start(ctx)
	}()
	cancel()
	p.Stop()
}
