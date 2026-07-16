package task

import (
	"strings"
	"testing"
)

func TestNewTask(t *testing.T) {
	sessionID := "sess-1"
	userID := "user-1"
	taskType := "agent_exec"

	t.Run("basic creation", func(t *testing.T) {
		task := NewTask(sessionID, userID, taskType, []string{"skill_a", "skill_b"}, nil)

		if !strings.HasPrefix(task.ID, "task_") {
			t.Errorf("task ID should start with 'task_': got %s", task.ID)
		}
		if task.SessionID != sessionID {
			t.Errorf("SessionID: got %s, want %s", task.SessionID, sessionID)
		}
		if task.UserID != userID {
			t.Errorf("UserID: got %s, want %s", task.UserID, userID)
		}
		if task.Type != taskType {
			t.Errorf("Type: got %s, want %s", task.Type, taskType)
		}
		if task.Status != StatusPending {
			t.Errorf("Status: got %s, want %s", task.Status, StatusPending)
		}
	})

	t.Run("skill chain sets total steps", func(t *testing.T) {
		task := NewTask(sessionID, userID, taskType, []string{"a", "b", "c"}, nil)
		if task.Progress.TotalSteps != 3 {
			t.Errorf("TotalSteps: got %d, want 3", task.Progress.TotalSteps)
		}
	})

	t.Run("empty skill chain defaults to 1 step", func(t *testing.T) {
		task := NewTask(sessionID, userID, taskType, nil, nil)
		if task.Progress.TotalSteps != 1 {
			t.Errorf("TotalSteps with nil chain: got %d, want 1", task.Progress.TotalSteps)
		}

		task2 := NewTask(sessionID, userID, taskType, []string{}, nil)
		if task2.Progress.TotalSteps != 1 {
			t.Errorf("TotalSteps with empty chain: got %d, want 1", task2.Progress.TotalSteps)
		}
	})

	t.Run("progress initialized correctly", func(t *testing.T) {
		task := NewTask(sessionID, userID, taskType, []string{"a"}, nil)
		if task.Progress.CurrentStep != 0 {
			t.Errorf("CurrentStep: got %d, want 0", task.Progress.CurrentStep)
		}
		if task.Progress.Percent != 0 {
			t.Errorf("Percent: got %d, want 0", task.Progress.Percent)
		}
		if task.Progress.Message != "Task created" {
			t.Errorf("Message: got %s, want 'Task created'", task.Progress.Message)
		}
	})

	t.Run("max retries defaults to 3", func(t *testing.T) {
		task := NewTask(sessionID, userID, taskType, nil, nil)
		if task.MaxRetries != 3 {
			t.Errorf("MaxRetries: got %d, want 3", task.MaxRetries)
		}
	})

	t.Run("created equals updated", func(t *testing.T) {
		task := NewTask(sessionID, userID, taskType, nil, nil)
		if !task.CreatedAt.Equal(task.UpdatedAt) {
			t.Error("CreatedAt should equal UpdatedAt for new task")
		}
	})

	t.Run("params are stored", func(t *testing.T) {
		params := map[string]interface{}{"key": "value", "num": 42}
		task := NewTask(sessionID, userID, taskType, nil, params)
		if task.Params["key"] != "value" {
			t.Errorf("Params[key]: got %v, want 'value'", task.Params["key"])
		}
	})

	t.Run("unique IDs per call", func(t *testing.T) {
		t1 := NewTask(sessionID, userID, taskType, nil, nil)
		t2 := NewTask(sessionID, userID, taskType, nil, nil)
		if t1.ID == t2.ID {
			t.Error("two NewTask calls should produce different IDs")
		}
	})
}
