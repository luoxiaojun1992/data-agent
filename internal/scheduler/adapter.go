package scheduler

import (
	task_svc "github.com/luoxiaojun1992/data-agent/internal/service/task"
)

// taskServiceAdapter adapts task_svc.Service to the scheduler.TaskCreator interface.
type taskServiceAdapter struct {
	svc *task_svc.Service
}

// NewTaskCreatorFromService creates a TaskCreator backed by a task_svc.Service.
// The adapter calls svc.CreateRun(taskID) — each schedule trigger creates a
// new TaskRun from the persistent Task definition.
func NewTaskCreatorFromService(svc *task_svc.Service) TaskCreator {
	return &taskServiceAdapter{svc: svc}
}

func (a *taskServiceAdapter) CreateRun(taskID string) (string, error) {
	run, err := a.svc.CreateRun(taskID)
	if err != nil {
		return "", err
	}
	return run.ID, nil
}
