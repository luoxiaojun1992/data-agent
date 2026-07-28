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
	runSvc   task.TaskRunService // SPEC-063: load the full TaskRun from DB
	// kbExecutor handles kb_index runs (optional — nil when kb not configured).
	kbExecutor TaskExecutor
	cancel     context.CancelFunc
	wg         sync.WaitGroup
	stopping   bool
}

// TaskExecutor defines the interface for executing task runs.
type TaskExecutor interface {
	Execute(ctx context.Context, run *task.TaskRun) error
}

// NewPool creates a worker pool. stream is the Redis Stream queue (or a mock in
// tests); runSvc loads the authoritative TaskRun from MongoDB (SPEC-063); the
// executor owns all status/result/error write-back.
func NewPool(stream Queue, redisClient *redis.Client, numWorkers int, executor TaskExecutor, runSvc task.TaskRunService) *Pool {
	return &Pool{
		queue:    stream,
		redis:    redisClient,
		workers:  numWorkers,
		executor: executor,
		runSvc:   runSvc,
	}
}

// SetKBExecutor sets the KB index executor for "kb_index" task type routing.
func (p *Pool) SetKBExecutor(executor TaskExecutor) {
	p.kbExecutor = executor
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

	// SPEC-063: load the full TaskRun from DB instead of rebuilding it in
	// memory from the queue message. The queue message carries only the
	// IDs/params needed to locate the run; the authoritative state lives
	// in MongoDB and may have changed since enqueue.
	run, err := p.runSvc.GetRun(qm.RunID)
	if err != nil || run == nil {
		log.Printf("Failed to load run %s: %v", qm.RunID, err)
		_ = p.queue.Ack(ctx, msg.ID)
		return
	}

	// Route by type: kb_index runs go to the KB executor.
	exec := p.executor
	if run.Type == task.TaskTypeKBIndex && p.kbExecutor != nil {
		exec = p.kbExecutor
	}

	start := time.Now()
	execErr := exec.Execute(ctx, run)

	if execErr != nil {
		log.Printf("[worker] run %s (task %s) failed after %s: %v", run.ID, run.TaskID, time.Since(start), execErr)
		run.RetryCount++
		if run.RetryCount >= run.MaxRetries {
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
