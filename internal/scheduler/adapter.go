package scheduler

import (
	"context"
	"time"

	task_model "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
)

// taskServiceAdapter adapts task_svc.Service to the scheduler.TaskCreator interface.
type taskServiceAdapter struct {
	svc *task_svc.Service
}

// NewTaskCreatorFromService creates a TaskCreator backed by a task_svc.Service.
func NewTaskCreatorFromService(svc *task_svc.Service) TaskCreator {
	return &taskServiceAdapter{svc: svc}
}

func (a *taskServiceAdapter) CreateRun(taskID string) (string, error) {
	// SPEC-084: scheduler is a system process — exempt from ownership checks.
	run, err := a.svc.CreateRun(taskID, "", true)
	if err != nil {
		return "", err
	}
	return run.ID, nil
}

// taskRepoAdapter adapts repository.TaskRepository to ScheduleProvider.
type taskRepoAdapter struct {
	repo repository.TaskRepository
}

// NewScheduleProviderFromRepo creates a ScheduleProvider from a TaskRepository.
func NewScheduleProviderFromRepo(repo repository.TaskRepository) ScheduleProvider {
	return &taskRepoAdapter{repo: repo}
}

func (a *taskRepoAdapter) ListScheduled(ctx context.Context, skip, limit int64, now time.Time) ([]TaskDef, int64, error) {
	tasks, total, err := a.repo.ListScheduled(ctx, skip, limit, now)
	if err != nil {
		return nil, 0, err
	}
	defs := make([]TaskDef, len(tasks))
	for i, t := range tasks {
		defs[i] = TaskDef{
			ID:           t.ID,
			UserID:       t.UserID,
			Title:        t.Title,
			ScheduleMode: t.ScheduleMode,
			CronExpr:     t.CronExpr,
			ScheduledAt:  t.ScheduledAt,
			SkillChain: t.SkillChain,
			Params:     t.Params,
			ModelID:    t.ModelID,
		}
	}
	return defs, total, nil
}

func (a *taskRepoAdapter) MarkScheduledDone(ctx context.Context, id string) error {
	return a.repo.MarkScheduledDone(ctx, id)
}

var _ = task_model.Task{} // ensure import
