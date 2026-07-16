package task

import (
	"testing"
)

func TestCollTasksConstant(t *testing.T) {
	if collTasks != "agent_tasks" {
		t.Errorf("collTasks = %q, want %q", collTasks, "agent_tasks")
	}
}
