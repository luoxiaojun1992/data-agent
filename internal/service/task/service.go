package task

import (
	"context"
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Service manages task definitions and runs.
type Service struct {
	repo      repository.TaskRepository
	runRepo   repository.TaskRunRepository
	queueRepo repository.QueueRepository
}

// NewService creates a task service. queueRepo may be nil in test setups.
func NewService(repo repository.TaskRepository, runRepo repository.TaskRunRepository, queueRepo repository.QueueRepository) *Service {
	return &Service{repo: repo, runRepo: runRepo, queueRepo: queueRepo}
}

// SetQueueRepo replaces the queue repository (called after Redis connects).
func (s *Service) SetQueueRepo(qr repository.QueueRepository) {
	s.queueRepo = qr
}

// CreateTask creates a task definition + its first TaskRun, persists both,
// and enqueues the run. Initializes run_count=1 and last_run_at=now.
func (s *Service) CreateTask(userID, taskType string, skillChain []string, params map[string]interface{}, modelID string) (*task.Task, *task.TaskRun, error) {
	ctx := context.Background()
	t := task.NewTask(userID, taskType, skillChain, params, modelID)
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, nil, fmt.Errorf("insert task def: %w", err)
	}
	run := task.NewTaskRun(t)
	run.Status = task.StatusQueued
	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, nil, fmt.Errorf("insert task run: %w", err)
	}
	// Initialize run_count=1 and last_run_at — UpdateLastRun does both atomically.
	if err := s.repo.UpdateLastRun(ctx, t.ID, run.CreatedAt); err != nil {
		_ = err // non-fatal — UI can still re-fetch
	}
	t.RunCount = 1
	t.LastRunAt = &run.CreatedAt

	if s.queueRepo != nil {
		_ = s.queueRepo.Enqueue(ctx, run)
	}
	return t, run, nil
}

// CreateRun creates a new run from the task definition, enqueues it, and
// atomically bumps run_count + last_run_at on the parent task.
func (s *Service) CreateRun(taskID string) (*task.TaskRun, error) {
	ctx := context.Background()
	t, err := s.repo.Get(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	run := task.NewTaskRun(t)
	run.Status = task.StatusQueued
	if err := s.runRepo.Create(ctx, run); err != nil {
		return nil, fmt.Errorf("insert run: %w", err)
	}
	if err := s.repo.UpdateLastRun(ctx, t.ID, run.CreatedAt); err != nil {
		_ = err // non-fatal
	}

	if s.queueRepo != nil {
		_ = s.queueRepo.Enqueue(ctx, run)
	}
	return run, nil
}

func (s *Service) GetTask(id string) (*task.Task, error) {
	return s.repo.Get(context.Background(), id)
}

func (s *Service) CancelTask(id string) error {
	return s.repo.Cancel(context.Background(), id)
}

func (s *Service) ListTasks(userID string, skip, limit int64) ([]*task.Task, int64, error) {
	return s.repo.List(context.Background(), userID, skip, limit)
}

func (s *Service) ListAllTasks(userID string) ([]*task.Task, error) {
	return s.repo.ListAll(context.Background(), userID)
}

func (s *Service) BatchCancelTasks(ids []string) error {
	for _, id := range ids {
		_ = s.repo.Cancel(context.Background(), id)
	}
	return nil
}

// ---- Run-level methods used by executors ----

func (s *Service) GetRun(id string) (*task.TaskRun, error) {
	return s.runRepo.Get(context.Background(), id)
}

func (s *Service) ListRuns(taskID string, status string, skip, limit int64) ([]*task.TaskRun, int64, error) {
	return s.runRepo.List(context.Background(), taskID, status, skip, limit)
}

func (s *Service) UpdateRunStatus(id string, status task.Status) error {
	return s.runRepo.UpdateStatus(context.Background(), id, status)
}

func (s *Service) UpdateRunResult(id string, result map[string]interface{}) error {
	return s.runRepo.UpdateResult(context.Background(), id, result)
}

func (s *Service) UpdateRunError(id string, errMsg string) error {
	return s.runRepo.UpdateError(context.Background(), id, errMsg)
}

func (s *Service) UpdateRunSessionID(id string, sessionID string) error {
	return s.runRepo.UpdateSessionID(context.Background(), id, sessionID)
}

func (s *Service) CancelRun(id string) error {
	return s.runRepo.Cancel(context.Background(), id)
}
