package scheduler

import (
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

func (a *taskServiceAdapter) CreateTask(sessionID, userID, taskType string, skillChain []string, params map[string]interface{}) (string, error) {
	t, err := a.svc.CreateTask(sessionID, userID, taskType, skillChain, params)
	if err != nil {
		return "", err
	}
	return t.ID, nil
}
