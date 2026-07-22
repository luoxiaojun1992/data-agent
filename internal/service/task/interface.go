package task

import (
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

// TaskService is the domain contract for task management, re-exported here
// as a type alias for backward compatibility with existing handler/test
// imports. The canonical definition lives in internal/domain/task.
//
//go:generate mockery --name TaskService --output ./mocks --outpkg mocks
type TaskService = domaintask.TaskService

var _ TaskService = (*Service)(nil)
