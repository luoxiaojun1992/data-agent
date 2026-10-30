package agent

import (
	"errors"
	"strings"
	"testing"

	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	domaintaskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
)

// wasRunCancelled has defensive branches (GetRun error / nil result) plus the
// cancelled/not-cancelled decisions; truncateForError has a length clamp branch.
// Both are small helpers not directly exercised by the Execute flow tests.

func newExecWithRuns(runs domaintask.TaskRunService) *AgentExecutor {
	return NewAgentExecutor(nil, nil, runs, nil, nil, nil)
}

func TestWasRunCancelled_GetRunError(t *testing.T) {
	runs := domaintaskmocks.NewTaskRunService(t)
	runs.On("GetRun", "r1", "", true).Return((*domaintask.TaskRun)(nil), errors.New("db down")).Once()
	if newExecWithRuns(runs).wasRunCancelled("r1") {
		t.Error("expected false when GetRun errors")
	}
}

func TestWasRunCancelled_NilRun(t *testing.T) {
	runs := domaintaskmocks.NewTaskRunService(t)
	runs.On("GetRun", "r2", "", true).Return((*domaintask.TaskRun)(nil), nil).Once()
	if newExecWithRuns(runs).wasRunCancelled("r2") {
		t.Error("expected false when GetRun returns nil")
	}
}

func TestWasRunCancelled_Cancelled(t *testing.T) {
	runs := domaintaskmocks.NewTaskRunService(t)
	runs.On("GetRun", "r3", "", true).Return(&domaintask.TaskRun{ID: "r3", Status: domaintask.StatusCancelled}, nil).Once()
	if !newExecWithRuns(runs).wasRunCancelled("r3") {
		t.Error("expected true for a cancelled run")
	}
}

func TestWasRunCancelled_NotCancelled(t *testing.T) {
	runs := domaintaskmocks.NewTaskRunService(t)
	runs.On("GetRun", "r4", "", true).Return(&domaintask.TaskRun{ID: "r4", Status: domaintask.StatusRunning}, nil).Once()
	if newExecWithRuns(runs).wasRunCancelled("r4") {
		t.Error("expected false for a running run")
	}
}

func TestTruncateForError(t *testing.T) {
	if got := truncateForError("short message"); got != "short message" {
		t.Errorf("truncateForError(short) = %q, want unchanged", got)
	}
	long := strings.Repeat("x", 600)
	got := truncateForError(long)
	want := strings.Repeat("x", 500) + "..."
	if got != want {
		t.Errorf("truncateForError(long) length = %d, want %d", len(got), len(want))
	}
	if len(got) != 503 {
		t.Errorf("truncateForError(long) length = %d, want 503", len(got))
	}
}
