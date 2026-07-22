package agent

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	domainchatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	domaintaskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
)

func newTestOrchestrator(t *testing.T) (*Orchestrator, *domainchatmocks.SessionService, *domaintaskmocks.TaskService) {
	t.Helper()
	sessions := domainchatmocks.NewSessionService(t)
	tasks := domaintaskmocks.NewTaskService(t)
	return NewOrchestrator(sessions, tasks), sessions, tasks
}

func TestCreateAgentTask_Success(t *testing.T) {
	orch, sessions, tasks := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	tk := &domaintask.Task{ID: "task_1", SessionID: "s1", UserID: "u1", Status: domaintask.StatusPending, CreatedAt: time.Now()}
	tasks.On("CreateTask", "s1", "u1", "agent", []string{"stats_engine"}, mock.Anything).Return(tk, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{
		Title:      "t",
		SkillChain: []string{"stats_engine"},
		Params:     map[string]interface{}{"k": 1},
	})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.TaskID != "task_1" || resp.SessionID != "s1" || resp.Status != "pending" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if resp.Note != "" {
		t.Errorf("success path should not carry a note: %q", resp.Note)
	}
}

func TestCreateAgentTask_NoTaskService(t *testing.T) {
	sessions := domainchatmocks.NewSessionService(t)
	orch := NewOrchestrator(sessions, nil) // no task service
	sessions.On("Create", "u1", "agent").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err != nil {
		t.Fatalf("expected fallback success, got %v", err)
	}
	if resp.TaskID != "task_memory_fallback" {
		t.Errorf("fallback task id = %v", resp.TaskID)
	}
	if resp.Note == "" {
		t.Errorf("fallback should carry a note")
	}
	if resp.Status != "queued" {
		t.Errorf("fallback status = %v", resp.Status)
	}
}

func TestCreateAgentTask_SessionError(t *testing.T) {
	orch, sessions, _ := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent").Return((*domainchat.Session)(nil), errSessionDown)

	_, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err == nil {
		t.Error("expected session error")
	}
}

func TestCreateAgentTask_TaskError(t *testing.T) {
	orch, sessions, tasks := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	tasks.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return((*domaintask.Task)(nil), errQueueDown)

	_, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err == nil {
		t.Error("expected task creation error")
	}
}

func TestCreateAgentTask_NilSkillChainNormalized(t *testing.T) {
	orch, sessions, tasks := newTestOrchestrator(t)
	sessions.On("Create", "u1", "agent").Return(&domainchat.Session{ID: "s1", UserID: "u1"}, nil)
	tk := &domaintask.Task{ID: "task_2", SessionID: "s1", UserID: "u1", Status: domaintask.StatusQueued, CreatedAt: time.Now()}
	// Nil skill chain should be normalized to empty slice, not nil.
	tasks.On("CreateTask", "s1", "u1", "agent", []string{}, mock.Anything).Return(tk, nil)

	resp, err := orch.CreateAgentTask(context.Background(), "u1", CreateAgentTaskRequest{Title: "t"})
	if err != nil {
		t.Fatalf("expected success, got %v", err)
	}
	if resp.TaskID != "task_2" {
		t.Errorf("task id = %v", resp.TaskID)
	}
}

// sentinel errors for test clarity.
var (
	errSessionDown = errString("session store unavailable")
	errQueueDown   = errString("queue unavailable")
)

type errString string

func (e errString) Error() string { return string(e) }
