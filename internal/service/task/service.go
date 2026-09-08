package task

import (
	"context"
	"errors"
	"fmt"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"time"
)

// ErrNotFound is returned when a task/run does not exist OR does not belong to
// the caller (SPEC-084 §6.6 IDOR protection — existence is never leaked).
var ErrNotFound = errors.New("not found")

// ErrTaskDisabled is returned by CreateRun when the task's enabled switch is
// off (SPEC-082 §5.3). The task exists but must not create new runs.
var ErrTaskDisabled = errors.New("task is disabled")

// ErrRunTerminal is returned by CancelRun when the run already reached a
// terminal state (completed/failed/cancelled) — completion boundary (SPEC-082
// §5.6). Maps to HTTP 409.
var ErrRunTerminal = errors.New("run already terminated")

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
func (s *Service) CreateTask(userID, taskType string, params map[string]interface{}, modelID, scheduleMode, cronExpr string, scheduledAt *time.Time) (*task.Task, *task.TaskRun, error) {
	ctx := context.Background()
	t := task.NewTask(userID, taskType, params, modelID)
	t.ScheduledEnabled = true
	isScheduled := taskType == task.TaskTypeScheduledExec && scheduleMode != ""
	if isScheduled {
		t.ScheduleMode = scheduleMode
		t.CronExpr = cronExpr
		t.ScheduledAt = scheduledAt
	}
	if err := s.repo.Create(ctx, t); err != nil {
		return nil, nil, fmt.Errorf("insert task def: %w", err)
	}
	// For real-time tasks, create the first run immediately.
	// For scheduled tasks, the scheduler creates runs at the scheduled time.
	if isScheduled {
		return t, nil, nil
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
// atomically bumps run_count + last_run_at on the parent task. Rejects with
// ErrTaskDisabled when the task's enabled switch is off (SPEC-082 §5.3).
func (s *Service) CreateRun(taskID, userID string, isSystemAdmin bool) (*task.TaskRun, error) {
	ctx := context.Background()
	t, err := s.repo.Get(ctx, taskID)
	if err != nil {
		return nil, fmt.Errorf("task %s not found", taskID)
	}
	if !isSystemAdmin && t.UserID != userID {
		return nil, ErrNotFound
	}
	// Enabled switch check: off → no run creation (all task types, SPEC-082).
	if !t.ScheduledEnabled {
		return nil, ErrTaskDisabled
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

func (s *Service) GetTask(id, userID string, isSystemAdmin bool) (*task.Task, error) {
	t, err := s.repo.Get(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("task not found: %w", err)
	}
	if !isSystemAdmin && t.UserID != userID {
		return nil, ErrNotFound
	}
	return t, nil
}

func (s *Service) DeleteTask(id, userID string, isSystemAdmin bool) error {
	t, err := s.repo.Get(context.Background(), id)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}
	if !isSystemAdmin && t.UserID != userID {
		return ErrNotFound
	}
	return s.repo.Delete(context.Background(), id)
}

func (s *Service) SetScheduledEnabled(taskID, userID string, isSystemAdmin bool, enabled bool) error {
	t, err := s.repo.Get(context.Background(), taskID)
	if err != nil {
		return fmt.Errorf("task not found: %w", err)
	}
	if !isSystemAdmin && t.UserID != userID {
		return ErrNotFound
	}
	return s.repo.SetScheduledEnabled(context.Background(), taskID, enabled)
}

func (s *Service) ListTasks(userID string, isSystemAdmin bool, skip, limit int64) ([]*task.Task, int64, error) {
	if isSystemAdmin {
		return s.repo.ListAll(context.Background(), skip, limit)
	}
	return s.repo.List(context.Background(), userID, skip, limit)
}

// ---- Run-level methods used by executors ----

func (s *Service) GetRun(id, userID string, isSystemAdmin bool) (*task.TaskRun, error) {
	run, err := s.runRepo.Get(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("run not found: %w", err)
	}
	if !isSystemAdmin && run.UserID != userID {
		return nil, ErrNotFound
	}
	return run, nil
}

func (s *Service) ListRuns(taskID, userID string, isSystemAdmin bool, status string, skip, limit int64) ([]*task.TaskRun, int64, error) {
	t, err := s.repo.Get(context.Background(), taskID)
	if err != nil {
		return nil, 0, fmt.Errorf("task not found: %w", err)
	}
	if !isSystemAdmin && t.UserID != userID {
		return nil, 0, ErrNotFound
	}
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

func (s *Service) CancelRun(id, userID string, isSystemAdmin bool) error {
	run, err := s.runRepo.Get(context.Background(), id)
	if err != nil {
		return ErrNotFound
	}
	if !isSystemAdmin && run.UserID != userID {
		return ErrNotFound
	}
	switch run.Status {
	case task.StatusCompleted, task.StatusFailed, task.StatusCancelled:
		return ErrRunTerminal
	}
	return s.runRepo.Cancel(context.Background(), id)
}
