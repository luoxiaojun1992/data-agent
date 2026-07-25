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
	"github.com/redis/go-redis/v9"
)

// Queue is the subset of queue.Stream the Pool consumes. *queue.Stream
// satisfies it; tests inject a mock to drive Ack/MoveToDLQ/Dequeue without
// Redis (SPEC-063 pool tests).
type Queue interface {
	Dequeue(ctx context.Context, consumerID string, block time.Duration) ([]redis.XMessage, error)
	Ack(ctx context.Context, messageID string) error
	MoveToDLQ(ctx context.Context, msgID string, data []byte) error
}

// Pool manages a pool of goroutine workers that consume from Redis Stream.
type Pool struct {
	mu       sync.Mutex
	queue    Queue
	redis    *redis.Client
	workers  int
	executor TaskExecutor
	taskSvc  task.TaskService // SPEC-063: load the full task from DB
	cancel   context.CancelFunc
	wg       sync.WaitGroup
	stopping bool
}

// TaskExecutor defines the interface for executing tasks.
type TaskExecutor interface {
	Execute(ctx context.Context, t *task.Task) error
}

// NewPool creates a worker pool. stream is the Redis Stream queue (or a mock in
// tests); taskSvc loads the authoritative task from MongoDB (SPEC-063); the
// executor owns all status/result/error write-back.
func NewPool(stream Queue, redisClient *redis.Client, numWorkers int, executor TaskExecutor, taskSvc task.TaskService) *Pool {
	return &Pool{
		queue:    stream,
		redis:    redisClient,
		workers:  numWorkers,
		executor: executor,
		taskSvc:  taskSvc,
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
			p.processWorkerMessage(ctx, msg)
		}
	}
}

func (p *Pool) processWorkerMessage(ctx context.Context, msg redis.XMessage) {
	data, ok := msg.Values["data"]
	if !ok {
		return
	}

	var qm task.QueueMessage
	if err := json.Unmarshal([]byte(data.(string)), &qm); err != nil {
		log.Printf("Failed to parse queue message: %v", err)
		return
	}

	// SPEC-063: load the full task from DB instead of rebuilding it in memory
	// from the queue message. The queue message carries only the IDs/params
	// needed to locate the task; the authoritative state (status, result,
	// model_id, retry counts) lives in MongoDB and may have changed since
	// enqueue. Loading fresh also fixes the defect where mid-flight task
	// edits were ignored.
	t, err := p.taskSvc.GetTask(qm.TaskID)
	if err != nil || t == nil {
		log.Printf("Failed to load task %s: %v", qm.TaskID, err)
		// Drop unrecoverable messages so the stream doesn't stall.
		_ = p.queue.Ack(ctx, msg.ID)
		return
	}

	start := time.Now()
	execErr := p.executor.Execute(ctx, t) // executor owns all DB write-back (status/result/error)

	// Retry / DLQ policy. The executor has already persisted the failure
	// status + error; this block only decides whether to dead-letter the
	// stream message after exhausting retries. The in-memory retry counter is
	// per-delivery (not persisted) — matching the pre-existing behavior.
	if execErr != nil {
		log.Printf("[worker] task %s failed after %s: %v", t.ID, time.Since(start), execErr)
		t.RetryCount++
		if t.RetryCount >= t.MaxRetries {
			_ = p.queue.MoveToDLQ(ctx, msg.ID, []byte(data.(string)))
		}
	}

	_ = p.queue.Ack(ctx, msg.ID)
}

// heartbeat periodically updates worker health status in Redis.
func (p *Pool) heartbeat(ctx context.Context) {
	defer p.wg.Done()
	workerID := "worker-" + uuid.New().String()

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
