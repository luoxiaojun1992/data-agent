package task

import (
	"context"
	"fmt"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Service handles task lifecycle operations.
type Service struct {
	repo repository.TaskRepository
	// queue is kept as concrete *queue.Stream for now — will be refactored in Phase 5.
}

// NewService creates a task service.
func NewService(repo repository.TaskRepository) *Service {
	return &Service{repo: repo}
}

// CreateTask creates a new task, persists it.
func (s *Service) CreateTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}) (*task.Task, error) {
	t := task.NewTask(sessionID, userID, taskType, skillChain, params)
	t.Status = task.StatusQueued
	if err := s.repo.Create(context.Background(), t); err != nil {
		return nil, fmt.Errorf("insert task: %w", err)
	}
	return t, nil
}

// GetTask retrieves a task by ID.
func (s *Service) GetTask(id string) (*task.Task, error) {
	return s.repo.Get(context.Background(), id)
}

// CancelTask cancels a running/queued task.
func (s *Service) CancelTask(id string) error {
	t, err := s.repo.Get(context.Background(), id)
	if err != nil {
		return fmt.Errorf("task %s not found", id)
	}
	switch t.Status {
	case task.StatusCancelled, task.StatusCompleted, task.StatusFailed:
		return fmt.Errorf("task %s is %s, cannot cancel", id, t.Status)
	}
	return s.repo.Cancel(context.Background(), id)
}

// ListTasks lists tasks for a user with optional status filter.
func (s *Service) ListTasks(userID string, status string, skip, limit int64) ([]*task.Task, int64, error) {
	return s.repo.List(context.Background(), userID, status, skip, limit)
}

// UpdateTaskProgress updates task progress.
func (s *Service) UpdateTaskProgress(id string, p *task.TaskProgress) error {
	return s.repo.UpdateProgress(context.Background(), id, p)
}

// UpdateTaskResult marks a task as completed with a result.
func (s *Service) UpdateTaskResult(id string, result map[string]interface{}) error {
	return s.repo.UpdateResult(context.Background(), id, result)
}

// UpdateStatus updates the task status field only.
func (s *Service) UpdateStatus(id string, status task.Status) error {
	t, err := s.repo.Get(context.Background(), id)
	if err != nil {
		return err
	}
	t.Status = status
	t.UpdatedAt = time.Now()
	return s.repo.Retry(context.Background(), id, t)
}

// ListAllTasks returns all tasks for a user.
func (s *Service) ListAllTasks(userID string) ([]*task.Task, error) {
	return s.repo.ListAll(context.Background(), userID)
}

// RetryTask resets a failed task for retry.
func (s *Service) RetryTask(id string) (*task.Task, error) {
	t, err := s.repo.Get(context.Background(), id)
	if err != nil {
		return nil, fmt.Errorf("task %s not found", id)
	}
	if t.Status != task.StatusFailed {
		return nil, fmt.Errorf("only failed tasks can be retried")
	}
	t.Status = task.StatusQueued
	t.RetryCount++
	t.UpdatedAt = time.Now()
	if err := s.repo.Retry(context.Background(), id, t); err != nil {
		return nil, err
	}
	return t, nil
}

// BatchCancelTasks cancels multiple tasks.
func (s *Service) BatchCancelTasks(ids []string) error {
	for _, id := range ids {
		if err := s.CancelTask(id); err != nil {
			_ = err // best-effort
		}
	}
	return nil
}
