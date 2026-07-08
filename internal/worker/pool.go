package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"github.com/redis/go-redis/v9"
)

// Pool manages a pool of goroutine workers that consume from Redis Stream.
type Pool struct {
	mu       sync.Mutex
	queue    *queue.Stream
	redis    *redis.Client
	workers  int
	executor TaskExecutor
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopping bool
}

// TaskExecutor defines the interface for executing tasks.
type TaskExecutor interface {
	Execute(ctx context.Context, t *task.Task) error
}

// NewPool creates a worker pool.
func NewPool(stream *queue.Stream, redisClient *redis.Client, numWorkers int, executor TaskExecutor) *Pool {
	return &Pool{
		queue:    stream,
		redis:    redisClient,
		workers:  numWorkers,
		executor: executor,
	}
}

// Start begins consuming tasks.
func (p *Pool) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)

	for i := 0; i < p.workers; i++ {
		p.wg.Add(1)
		go p.runWorker(ctx, fmt.Sprintf("worker-%d", i))
	}

	// Start heartbeat goroutine
	p.wg.Add(1)
	go p.heartbeat(ctx)
}

// Stop gracefully shuts down the worker pool.
func (p *Pool) Stop() {
	p.mu.Lock()
	p.stopping = true
	p.mu.Unlock()

	if p.cancel != nil {
		p.cancel()
	}

	// Wait for workers to finish current tasks (max 30s)
	done := make(chan struct{})
	go func() {
		p.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		log.Println("Worker pool shutdown timeout — forcing exit")
	}
}

func (p *Pool) runWorker(ctx context.Context, consumerID string) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		msgs, err := p.queue.Dequeue(ctx, consumerID, 5*time.Second)
		if err != nil || len(msgs) == 0 {
			continue
		}

		for _, msg := range msgs {
			data, ok := msg.Values["data"]
			if !ok {
				continue
			}

			var qm task.QueueMessage
			if err := json.Unmarshal([]byte(data.(string)), &qm); err != nil {
				log.Printf("Failed to parse queue message: %v", err)
				continue
			}

			t := &task.Task{
				ID:         qm.TaskID,
				SessionID:  qm.SessionID,
				UserID:     qm.UserID,
				Type:       qm.Type,
				SkillChain: qm.SkillChain,
				Params:     qm.Params,
				Status:     task.StatusRunning,
			}

			start := time.Now()
			err = p.executor.Execute(ctx, t)
			duration := time.Since(start).Milliseconds()

			if err != nil {
				t.RetryCount++
				if t.RetryCount >= t.MaxRetries {
					t.Status = task.StatusFailed
					// Move to DLQ after max retries
					_ = p.queue.MoveToDLQ(ctx, msg.ID, []byte(data.(string)))
				} else {
					t.Status = task.StatusRetrying
				}
			} else {
				t.Status = task.StatusCompleted
			}

			t.DurationMs = duration
			now := time.Now()
			t.CompletedAt = &now

			// Acknowledge processing
			_ = p.queue.Ack(ctx, msg.ID)
		}
	}
}

// heartbeat periodically updates worker health status in Redis.
func (p *Pool) heartbeat(ctx context.Context) {
	defer p.wg.Done()
	workerID := "worker-" + uuid.New().String()[:8]

	ticker := time.NewTicker(5 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			p.redis.Set(ctx, "worker:"+workerID+":heartbeat", "alive", 15*time.Second)
		}
	}
}
