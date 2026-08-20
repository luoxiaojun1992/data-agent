package task

import (
	"strings"
	"testing"
)

func TestNewTask(t *testing.T) {
	userID := "user-1"
	taskType := "agent_exec"
	modelID := "model_abc"

	t.Run("basic creation", func(t *testing.T) {
		params := map[string]interface{}{"title": "分析营收"}
		tsk := NewTask(userID, taskType, []string{"skill_a", "skill_b"}, params, modelID)

		if !strings.HasPrefix(tsk.ID, "task_") {
			t.Errorf("task ID should start with 'task_': got %s", tsk.ID)
		}
		if tsk.UserID != userID {
			t.Errorf("UserID: got %s, want %s", tsk.UserID, userID)
		}
		if tsk.Type != taskType {
			t.Errorf("Type: got %s, want %s", tsk.Type, taskType)
		}
		if tsk.ModelID != modelID {
			t.Errorf("ModelID: got %s, want %s", tsk.ModelID, modelID)
		}
		if tsk.Title != "分析营收" {
			t.Errorf("Title: got %s, want 分析营收", tsk.Title)
		}
	})

	t.Run("title empty without params", func(t *testing.T) {
		tsk := NewTask(userID, taskType, nil, nil, "")
		if tsk.Title != "" {
			t.Errorf("Title should be empty, got %q", tsk.Title)
		}
	})

	t.Run("skill chain stored", func(t *testing.T) {
		chain := []string{"a", "b", "c"}
		tsk := NewTask(userID, taskType, chain, nil, "")
		if len(tsk.SkillChain) != 3 {
			t.Errorf("SkillChain len: got %d, want 3", len(tsk.SkillChain))
		}
	})

	t.Run("params are stored", func(t *testing.T) {
		params := map[string]interface{}{"key": "value", "num": 42}
		tsk := NewTask(userID, taskType, nil, params, "")
		if tsk.Params["key"] != "value" {
			t.Errorf("Params[key]: got %v, want 'value'", tsk.Params["key"])
		}
	})

	t.Run("unique IDs per call", func(t *testing.T) {
		t1 := NewTask(userID, taskType, nil, nil, "")
		t2 := NewTask(userID, taskType, nil, nil, "")
		if t1.ID == t2.ID {
			t.Error("two NewTask calls should produce different IDs")
		}
	})

	t.Run("created equals updated", func(t *testing.T) {
		tsk := NewTask(userID, taskType, nil, nil, "")
		if !tsk.CreatedAt.Equal(tsk.UpdatedAt) {
			t.Error("CreatedAt should equal UpdatedAt for new task")
		}
	})
}

func TestNewTaskRun(t *testing.T) {
	parent := NewTask("user-1", "agent_exec", []string{"a", "b"}, map[string]interface{}{"q": 1}, "model_1")

	t.Run("inherits task identity", func(t *testing.T) {
		run := NewTaskRun(parent)
		if !strings.HasPrefix(run.ID, "run_") {
			t.Errorf("run ID should start with 'run_': got %s", run.ID)
		}
		if run.TaskID != parent.ID {
			t.Errorf("TaskID: got %s, want %s", run.TaskID, parent.ID)
		}
		if run.UserID != parent.UserID {
			t.Errorf("UserID: got %s, want %s", run.UserID, parent.UserID)
		}
		if run.ModelID != parent.ModelID {
			t.Errorf("ModelID: got %s, want %s", run.ModelID, parent.ModelID)
		}
		if run.Params["q"] != 1 {
			t.Errorf("Params not inherited: %v", run.Params)
		}
	})

	t.Run("status pending", func(t *testing.T) {
		run := NewTaskRun(parent)
		if run.Status != StatusPending {
			t.Errorf("Status: got %s, want %s", run.Status, StatusPending)
		}
	})

	t.Run("progress total steps from skill chain", func(t *testing.T) {
		run := NewTaskRun(parent)
		if run.Progress.TotalSteps != 2 {
			t.Errorf("TotalSteps: got %d, want 2", run.Progress.TotalSteps)
		}
	})

	t.Run("empty skill chain defaults to 1 step", func(t *testing.T) {
		p := NewTask("user-1", "agent_exec", nil, nil, "")
		run := NewTaskRun(p)
		if run.Progress.TotalSteps != 1 {
			t.Errorf("TotalSteps with nil chain: got %d, want 1", run.Progress.TotalSteps)
		}
	})

	t.Run("max retries defaults to 3", func(t *testing.T) {
		run := NewTaskRun(parent)
		if run.MaxRetries != 3 {
			t.Errorf("MaxRetries: got %d, want 3", run.MaxRetries)
		}
	})

	t.Run("unique IDs per call", func(t *testing.T) {
		r1 := NewTaskRun(parent)
		r2 := NewTaskRun(parent)
		if r1.ID == r2.ID {
			t.Error("two NewTaskRun calls should produce different IDs")
		}
	})
}
