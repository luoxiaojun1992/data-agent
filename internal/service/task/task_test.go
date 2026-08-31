package task

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func newTestService(t *testing.T) (*Service, *mockrepo.TaskRepository, *mockrepo.TaskRunRepository, *mockrepo.QueueRepository) {
	t.Helper()
	repo := mockrepo.NewTaskRepository(t)
	runRepo := mockrepo.NewTaskRunRepository(t)
	queue := mockrepo.NewQueueRepository(t)
	return NewService(repo, runRepo, queue), repo, runRepo, queue
}

func TestNewService(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	s := NewService(repo, nil, nil)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
}

// ── CreateTask ──

func TestCreateTask_Success(t *testing.T) {
	s, repo, runRepo, queue := newTestService(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	runRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("UpdateLastRun", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(nil)

	tsk, run, err := s.CreateTask("u1", "agent", []string{"sql", "stats"},
		map[string]interface{}{"query": "SELECT 1"}, "model_1", "", "", nil)
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if tsk == nil || run == nil {
		t.Fatal("task or run should not be nil")
	}
	if run.Status != task.StatusQueued {
		t.Errorf("run status = %s, want queued", run.Status)
	}
	if run.TaskID != tsk.ID {
		t.Errorf("run.TaskID = %s, want %s", run.TaskID, tsk.ID)
	}
	if tsk.ModelID != "model_1" {
		t.Errorf("ModelID: got %s, want model_1", tsk.ModelID)
	}
	if tsk.RunCount != 1 || tsk.LastRunAt == nil {
		t.Errorf("RunCount=%d LastRunAt=%v, want 1 and non-nil", tsk.RunCount, tsk.LastRunAt)
	}
	queue.AssertCalled(t, "Enqueue", mock.Anything, mock.Anything)
}

func TestCreateTask_ScheduledCreatesNoRun(t *testing.T) {
	s, repo, runRepo, _ := newTestService(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	tsk, run, err := s.CreateTask("u1", task.TaskTypeScheduledExec, nil, nil, "",
		task.ScheduleModeRecurring, "0 0 * * *", nil)
	if err != nil {
		t.Fatalf("CreateTask scheduled: %v", err)
	}
	if run != nil {
		t.Error("scheduled task creation must not create a run (scheduler owns runs)")
	}
	if tsk.ScheduleMode != task.ScheduleModeRecurring || tsk.CronExpr != "0 0 * * *" {
		t.Errorf("schedule fields: mode=%s cron=%s", tsk.ScheduleMode, tsk.CronExpr)
	}
	runRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCreateTask_OneTimeScheduledAt(t *testing.T) {
	s, repo, runRepo, _ := newTestService(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	at := time.Now().Add(24 * time.Hour)

	tsk, run, err := s.CreateTask("u1", task.TaskTypeScheduledExec, nil, nil, "",
		task.ScheduleModeOneTime, "", &at)
	if err != nil {
		t.Fatalf("CreateTask one-time: %v", err)
	}
	if run != nil {
		t.Error("one-time scheduled task must not create a run")
	}
	if tsk.ScheduledAt == nil || !tsk.ScheduledAt.Equal(at) {
		t.Errorf("ScheduledAt = %v, want %v", tsk.ScheduledAt, at)
	}
	runRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCreateTask_RepoError(t *testing.T) {
	s, repo, runRepo, _ := newTestService(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("db error"))

	tsk, run, err := s.CreateTask("u1", "agent", nil, nil, "", "", "", nil)
	if err == nil {
		t.Fatal("expected error")
	}
	if tsk != nil || run != nil {
		t.Error("task and run should be nil on repo error")
	}
	runRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
}

func TestCreateTask_RunRepoError(t *testing.T) {
	s, repo, runRepo, _ := newTestService(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	runRepo.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("run insert failed"))

	_, _, err := s.CreateTask("u1", "agent", nil, nil, "", "", "", nil)
	if err == nil {
		t.Fatal("expected run insert error")
	}
}

func TestCreateTask_QueueError_BestEffort(t *testing.T) {
	s, repo, runRepo, queue := newTestService(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	runRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("UpdateLastRun", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(fmt.Errorf("redis down"))

	tsk, run, err := s.CreateTask("u1", "agent", nil, nil, "", "", "", nil)
	if err != nil {
		t.Fatalf("CreateTask should be best-effort on queue error: %v", err)
	}
	if tsk == nil || run == nil {
		t.Fatal("task and run should exist despite queue failure")
	}
}

// ── Task-level methods ──

func TestGetTask_Success(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1"}, nil)

	tsk, err := s.GetTask("task_1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if tsk.ID != "task_1" {
		t.Errorf("ID = %s", tsk.ID)
	}
}

func TestCancelTask_Success(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Cancel", mock.Anything, "task_1").Return(nil)

	if err := s.CancelTask("task_1"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
}

func TestSetScheduledEnabled_Success(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("SetScheduledEnabled", mock.Anything, "task_1", false).Return(nil)

	if err := s.SetScheduledEnabled("task_1", false); err != nil {
		t.Fatalf("SetScheduledEnabled: %v", err)
	}
}

func TestListTasks_Success(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("List", mock.Anything, "user1", int64(0), int64(50)).Return(
		[]*task.Task{{ID: "t1"}, {ID: "t2"}}, int64(2), nil,
	)

	tasks, total, err := s.ListTasks("user1", 0, 50)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 || total != 2 {
		t.Fatalf("got %d tasks (total=%d), want 2", len(tasks), total)
	}
}

// ── Run-level methods (executor contract) ──

func TestCreateRun_Success(t *testing.T) {
	s, repo, runRepo, queue := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "u1"}, nil)
	runRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("UpdateLastRun", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(nil)

	run, err := s.CreateRun("task_1")
	if err != nil {
		t.Fatalf("CreateRun: %v", err)
	}
	if run == nil || run.TaskID != "task_1" || run.Status != task.StatusQueued {
		t.Errorf("run = %+v", run)
	}
	queue.AssertCalled(t, "Enqueue", mock.Anything, mock.Anything)
}

func TestCreateRun_NotFound(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "missing").Return((*task.Task)(nil), fmt.Errorf("not found"))

	if _, err := s.CreateRun("missing"); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestGetRun_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{ID: "run_1"}, nil)

	run, err := s.GetRun("run_1")
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.ID != "run_1" {
		t.Errorf("ID = %s", run.ID)
	}
}

func TestListRuns_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("List", mock.Anything, "task_1", "", int64(0), int64(20)).Return(
		[]*task.TaskRun{{ID: "r1"}, {ID: "r2"}}, int64(2), nil,
	)

	runs, total, err := s.ListRuns("task_1", "", 0, 20)
	if err != nil {
		t.Fatalf("ListRuns: %v", err)
	}
	if len(runs) != 2 || total != 2 {
		t.Fatalf("got %d runs (total=%d), want 2", len(runs), total)
	}
}

func TestUpdateRunStatus_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("UpdateStatus", mock.Anything, "run_1", task.StatusRunning).Return(nil)

	if err := s.UpdateRunStatus("run_1", task.StatusRunning); err != nil {
		t.Fatalf("UpdateRunStatus: %v", err)
	}
}

func TestUpdateRunResult_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("UpdateResult", mock.Anything, "run_1", mock.Anything).Return(nil)

	if err := s.UpdateRunResult("run_1", map[string]interface{}{"answer": 42}); err != nil {
		t.Fatalf("UpdateRunResult: %v", err)
	}
}

func TestUpdateRunError_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("UpdateError", mock.Anything, "run_1", "boom").Return(nil)

	if err := s.UpdateRunError("run_1", "boom"); err != nil {
		t.Fatalf("UpdateRunError: %v", err)
	}
}

func TestUpdateRunError_RepoError(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("UpdateError", mock.Anything, "run_1", mock.Anything).Return(fmt.Errorf("db down"))

	if err := s.UpdateRunError("run_1", "boom"); err == nil {
		t.Error("expected repo error to propagate")
	}
}

func TestUpdateRunSessionID_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("UpdateSessionID", mock.Anything, "run_1", "sess_9").Return(nil)

	if err := s.UpdateRunSessionID("run_1", "sess_9"); err != nil {
		t.Fatalf("UpdateRunSessionID: %v", err)
	}
}

func TestCancelRun_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Cancel", mock.Anything, "run_1").Return(nil)

	if err := s.CancelRun("run_1"); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
}
