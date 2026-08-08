package worker

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/panjf2000/ants/v2"

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

// Pool manages the task execution pipeline with a single dequeue goroutine
// and an ants goroutine pool for synchronous handler dispatch. The consumer
// only dequeues from Redis Stream when the goroutine pool has free capacity,
// providing natural backpressure.
type Pool struct {
	mu       sync.Mutex
	queue    Queue
	workers  int    // goroutine pool size (configurable via WORKER_POOL_SIZE)
	executor TaskExecutor
	runSvc   task.TaskRunService // SPEC-063: load the full TaskRun from DB
	// kbExecutor handles kb_index runs (optional — nil when kb not configured).
	kbExecutor TaskExecutor
	pool       *ants.Pool       // goroutine pool
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
		workers:  numWorkers,
		executor: executor,
		runSvc:   runSvc,
	}
}

// SetKBExecutor sets the KB index executor for "kb_index" task type routing.
func (p *Pool) SetKBExecutor(executor TaskExecutor) {
	p.kbExecutor = executor
}

// Start begins consuming tasks. Creates an ants goroutine pool with the
// configured size, then launches a single consumer goroutine that dequeues
// only when the pool has free workers.
func (p *Pool) Start(ctx context.Context) {
	ctx, p.cancel = context.WithCancel(ctx)

	var err error
	p.pool, err = ants.NewPool(p.workers, ants.WithPreAlloc(false))
	if err != nil {
		log.Printf("[worker] failed to create goroutine pool: %v — falling back to direct execution", err)
		// Fallback: still start a consumer, but execute directly (no pool).
	}

	// Single consumer goroutine — dequeues only when pool has capacity.
	p.wg.Add(1)
	go p.consumer(ctx, fmt.Sprintf("consumer-%s", uuid.New().String()[:8]))

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

	if p.pool != nil {
		p.pool.Release()
	}
}

// consumer is the single goroutine that dequeues from Redis Stream.
// It blocks (pauses) when the ants pool is full, providing backpressure
// so messages stay in Redis until capacity is available.
func (p *Pool) consumer(ctx context.Context, consumerID string) {
	defer p.wg.Done()

	for {
		select {
		case <-ctx.Done():
			return
		default:
		}

		// Backpressure: only dequeue when pool has free capacity.
		if p.pool != nil && p.pool.Free() == 0 {
			time.Sleep(200 * time.Millisecond)
			continue
		}

		msgs, err := p.queue.Dequeue(ctx, consumerID, 1*time.Second)
		if err != nil || len(msgs) == 0 {
			continue
		}

		for _, msg := range msgs {
			// Capture msg in closure so it's not overwritten by the loop.
			m := msg
			if p.pool != nil {
				_ = p.pool.Submit(func() {
					p.processWorkerMessage(context.Background(), m)
				})
			} else {
				// Fallback: execute directly when pool creation failed.
				go func() { p.processWorkerMessage(context.Background(), m) }()
			}
		}
	}
}

func (p *Pool) processWorkerMessage(ctx context.Context, msg redis.XMessage) {
	data, ok := msg.Values["data"]
	if !ok {
		return
	}

	rawBytes := []byte(data.(string))

	// Try raw kb_index job first (no task/task_run records).
	var rawJob struct {
		Type    string                 `json:"type"`
		Payload map[string]interface{} `json:"payload"`
	}
	if err := json.Unmarshal(rawBytes, &rawJob); err == nil && rawJob.Type == "kb_index" && p.kbExecutor != nil {
		// Build a minimal TaskRun for the KB executor (it only needs Params).
		run := &task.TaskRun{
			ID:   fmt.Sprintf("raw_kb_%s", msg.ID),
			Type: task.TaskTypeKBIndex,
			Params: rawJob.Payload,
		}
		start := time.Now()
		if err := p.kbExecutor.Execute(context.Background(), run); err != nil {
			log.Printf("[worker] kb_index raw job %s failed after %s: %v", msg.ID, time.Since(start), err)
		}
		_ = p.queue.Ack(context.Background(), msg.ID)
		return
	}

	var qm task.QueueMessage
	if err := json.Unmarshal(rawBytes, &qm); err != nil {
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
		_ = p.queue.Ack(context.Background(), msg.ID)
		return
	}

	// Route by type: kb_index runs go to the KB executor (synchronous blocking).
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
			_ = p.queue.MoveToDLQ(context.Background(), msg.ID, []byte(data.(string)))
		}
	}

	_ = p.queue.Ack(context.Background(), msg.ID)
}

// heartbeat writes periodic liveness timestamps.
func (p *Pool) heartbeat(ctx context.Context) {
	defer p.wg.Done()
	t := time.NewTicker(10 * time.Second)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
		// Simple liveness: log worker status
		if p.pool != nil {
			log.Printf("[worker] pool status: running=%d free=%d", p.pool.Running(), p.pool.Free())
		}
	}
}
