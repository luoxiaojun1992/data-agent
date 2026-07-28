package task

import (
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
)

// TaskService is the domain contract for task definition management,
// re-exported here as a type alias for backward compatibility.
//
//go:generate mockery --name TaskService --output ./mocks --outpkg mocks
type TaskService = domaintask.TaskService

// TaskRunService is the domain contract for run-level execution state,
// re-exported here as a type alias.
//
//go:generate mockery --name TaskRunService --output ./mocks --outpkg mocks
type TaskRunService = domaintask.TaskRunService

var _ TaskService = (*Service)(nil)
var _ TaskRunService = (*Service)(nil)
