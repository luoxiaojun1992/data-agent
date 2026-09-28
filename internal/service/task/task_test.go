package task

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"

	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	redismocks "github.com/luoxiaojun1992/data-agent/internal/infra/redis/mocks"
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

	tsk, run, err := s.CreateTask("u1", "agent",
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

	tsk, run, err := s.CreateTask("u1", task.TaskTypeScheduledExec, nil, "",
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

	tsk, run, err := s.CreateTask("u1", task.TaskTypeScheduledExec, nil, "",
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

	tsk, run, err := s.CreateTask("u1", "agent", nil, "", "", "", nil)
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

	_, _, err := s.CreateTask("u1", "agent", nil, "", "", "", nil)
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

	tsk, run, err := s.CreateTask("u1", "agent", nil, "", "", "", nil)
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
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "user1"}, nil)

	tsk, err := s.GetTask("task_1", "user1", false)
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if tsk.ID != "task_1" {
		t.Errorf("ID = %s", tsk.ID)
	}
}

func TestDeleteTask_Success(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "user1"}, nil)
	repo.On("Delete", mock.Anything, "task_1").Return(nil)

	if err := s.DeleteTask("task_1", "user1", false); err != nil {
		t.Fatalf("DeleteTask: %v", err)
	}
	repo.AssertCalled(t, "Delete", mock.Anything, "task_1")
}

func TestSetScheduledEnabled_Success(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "user1"}, nil)
	repo.On("SetScheduledEnabled", mock.Anything, "task_1", false).Return(nil)

	if err := s.SetScheduledEnabled("task_1", "user1", false, false); err != nil {
		t.Fatalf("SetScheduledEnabled: %v", err)
	}
}

func TestListTasks_Success(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("List", mock.Anything, "user1", int64(0), int64(50)).Return(
		[]*task.Task{{ID: "t1"}, {ID: "t2"}}, int64(2), nil,
	)

	tasks, total, err := s.ListTasks("user1", false, 0, 50)
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
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "u1", ScheduledEnabled: true}, nil)
	runRepo.On("Create", mock.Anything, mock.Anything).Return(nil)
	repo.On("UpdateLastRun", mock.Anything, mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(nil)

	run, err := s.CreateRun("task_1", "u1", false)
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

	if _, err := s.CreateRun("missing", "u1", false); err == nil {
		t.Fatal("expected error for missing task")
	}
}

func TestGetRun_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{ID: "run_1", UserID: "u1"}, nil)

	run, err := s.GetRun("run_1", "u1", false)
	if err != nil {
		t.Fatalf("GetRun: %v", err)
	}
	if run.ID != "run_1" {
		t.Errorf("ID = %s", run.ID)
	}
}

func TestListRuns_Success(t *testing.T) {
	s, repo, runRepo, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "user1"}, nil)
	runRepo.On("List", mock.Anything, "task_1", "", int64(0), int64(20)).Return(
		[]*task.TaskRun{{ID: "r1"}, {ID: "r2"}}, int64(2), nil,
	)

	runs, total, err := s.ListRuns("task_1", "user1", false, "", 0, 20)
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
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{ID: "run_1", UserID: "u1", Status: task.StatusRunning}, nil)
	runRepo.On("Cancel", mock.Anything, "run_1").Return(nil)

	if err := s.CancelRun("run_1", "u1", false); err != nil {
		t.Fatalf("CancelRun: %v", err)
	}
	runRepo.AssertCalled(t, "Cancel", mock.Anything, "run_1")
}

func TestCancelRun_TerminalState(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{ID: "run_1", UserID: "u1", Status: task.StatusCompleted}, nil)

	if err := s.CancelRun("run_1", "u1", false); err != ErrRunTerminal {
		t.Fatalf("want ErrRunTerminal, got %v", err)
	}
	runRepo.AssertNotCalled(t, "Cancel", mock.Anything, "run_1")
}

func TestCancelRun_ForbiddenNonOwner(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{ID: "run_1", UserID: "owner", Status: task.StatusRunning}, nil)

	if err := s.CancelRun("run_1", "attacker", false); err != ErrNotFound {
		t.Fatalf("want ErrNotFound (no existence leak), got %v", err)
	}
	runRepo.AssertNotCalled(t, "Cancel", mock.Anything, "run_1")
}

// ── SPEC-084 §6.6 IDOR ownership (归属校验) ──

func TestGetTask_ForbiddenNonOwner(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "owner"}, nil)

	if _, err := s.GetTask("task_1", "attacker", false); err != ErrNotFound {
		t.Fatalf("want ErrNotFound (no existence leak), got %v", err)
	}
}

func TestGetTask_SystemAdminExempt(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "owner"}, nil)

	tsk, err := s.GetTask("task_1", "admin", true)
	if err != nil {
		t.Fatalf("system_admin should be exempt, got %v", err)
	}
	if tsk.ID != "task_1" {
		t.Errorf("ID = %s", tsk.ID)
	}
}

func TestDeleteTask_ForbiddenNonOwner(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "owner"}, nil)

	if err := s.DeleteTask("task_1", "attacker", false); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	repo.AssertNotCalled(t, "Delete", mock.Anything, "task_1")
}

func TestCreateRun_Disabled(t *testing.T) {
	s, repo, runRepo, queue := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "u1", ScheduledEnabled: false}, nil)

	if _, err := s.CreateRun("task_1", "u1", false); err != ErrTaskDisabled {
		t.Fatalf("want ErrTaskDisabled, got %v", err)
	}
	runRepo.AssertNotCalled(t, "Create", mock.Anything, mock.Anything)
	queue.AssertNotCalled(t, "Enqueue", mock.Anything, mock.Anything)
}

func TestCreateRun_ForbiddenNonOwner(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "owner"}, nil)

	if _, err := s.CreateRun("task_1", "attacker", false); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetRun_ForbiddenNonOwner(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{ID: "run_1", UserID: "owner"}, nil)

	if _, err := s.GetRun("run_1", "attacker", false); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestGetRun_SystemAdminExempt(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{ID: "run_1", UserID: "owner"}, nil)

	run, err := s.GetRun("run_1", "admin", true)
	if err != nil {
		t.Fatalf("system_admin should be exempt, got %v", err)
	}
	if run.ID != "run_1" {
		t.Errorf("ID = %s", run.ID)
	}
}

func TestListRuns_ForbiddenNonOwner(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", UserID: "owner"}, nil)

	if _, _, err := s.ListRuns("task_1", "attacker", false, "", 0, 20); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

func TestListTasks_SystemAdminUsesListAll(t *testing.T) {
	s, repo, _, _ := newTestService(t)
	repo.On("ListAll", mock.Anything, int64(0), int64(50)).Return(
		[]*task.Task{{ID: "t1"}, {ID: "t2"}, {ID: "t3"}}, int64(3), nil,
	)

	tasks, total, err := s.ListTasks("admin", true, 0, 50)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 3 || total != 3 {
		t.Fatalf("got %d tasks (total=%d), want 3", len(tasks), total)
	}
	repo.AssertNotCalled(t, "List", mock.Anything, mock.Anything, mock.Anything, mock.Anything)
}

// ── DeleteRun (SPEC-104 D1/D2) ──

// TestDeleteRun_Success asserts the fixed deletion order: the associated
// session is hard-deleted FIRST, then the run record LAST.
func TestDeleteRun_Success(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)

	var order []string
	sessSvc.On("HardDelete", "sess_1").Return(nil).Run(func(mock.Arguments) {
		order = append(order, "session")
	})
	runRepo.On("Delete", mock.Anything, "run_1").Return(nil).Run(func(mock.Arguments) {
		order = append(order, "run")
	})

	if err := s.DeleteRun("run_1", "u1", false); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
	if len(order) != 2 || order[0] != "session" || order[1] != "run" {
		t.Fatalf("deletion order = %v, want [session run]", order)
	}
}

// TestDeleteRun_NoSession verifies a run without a bound session skips the
// cascade and still deletes the run record.
func TestDeleteRun_NoSession(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "",
	}, nil)
	runRepo.On("Delete", mock.Anything, "run_1").Return(nil)

	if err := s.DeleteRun("run_1", "u1", false); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
	sessSvc.AssertNotCalled(t, "HardDelete", mock.Anything)
	runRepo.AssertCalled(t, "Delete", mock.Anything, "run_1")
}

// TestDeleteRun_SessionDeleteFailure_AbortsRun asserts that when the session
// cascade fails, the run record is NOT deleted (retryable state preserved).
func TestDeleteRun_SessionDeleteFailure_AbortsRun(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)
	sessSvc.On("HardDelete", "sess_1").Return(fmt.Errorf("db down"))

	if err := s.DeleteRun("run_1", "u1", false); err == nil {
		t.Fatal("expected session cascade error to abort the delete")
	}
	runRepo.AssertNotCalled(t, "Delete", mock.Anything, "run_1")
}

// TestDeleteRun_RunDeleteFailure_Retryable asserts the run delete failure is
// surfaced (leaving a retryable "session gone, run remains" state).
func TestDeleteRun_RunDeleteFailure_Retryable(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)
	sessSvc.On("HardDelete", "sess_1").Return(nil)
	runRepo.On("Delete", mock.Anything, "run_1").Return(fmt.Errorf("db down"))

	if err := s.DeleteRun("run_1", "u1", false); err == nil {
		t.Fatal("expected run delete error to be surfaced")
	}
}

// TestDeleteRun_NotFound asserts a missing run maps to ErrNotFound.
func TestDeleteRun_NotFound(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "ghost").Return((*task.TaskRun)(nil), fmt.Errorf("not found"))

	if err := s.DeleteRun("ghost", "u1", false); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
}

// TestDeleteRun_ForbiddenNonOwner asserts IDOR protection (no existence leak).
func TestDeleteRun_ForbiddenNonOwner(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "owner", SessionID: "sess_1",
	}, nil)

	if err := s.DeleteRun("run_1", "attacker", false); err != ErrNotFound {
		t.Fatalf("want ErrNotFound, got %v", err)
	}
	runRepo.AssertNotCalled(t, "Delete", mock.Anything, "run_1")
}

// TestDeleteRun_SystemAdminExempt asserts system_admin bypasses ownership.
func TestDeleteRun_SystemAdminExempt(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "owner", SessionID: "sess_1",
	}, nil)
	sessSvc.On("HardDelete", "sess_1").Return(nil)
	runRepo.On("Delete", mock.Anything, "run_1").Return(nil)

	if err := s.DeleteRun("run_1", "admin", true); err != nil {
		t.Fatalf("system_admin should be exempt, got %v", err)
	}
}

// TestDeleteRun_RunBusy asserts a held run lock maps to ErrRunBusy and neither
// the session nor the run is deleted.
func TestDeleteRun_RunBusy(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)
	lock := redismocks.NewLocker(t)
	s.SetLocker(lock)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)
	lock.On("Acquire", mock.Anything, "lock:run:run_1", mock.Anything, mock.Anything).Return(false, nil)

	if err := s.DeleteRun("run_1", "u1", false); err != ErrRunBusy {
		t.Fatalf("want ErrRunBusy, got %v", err)
	}
	sessSvc.AssertNotCalled(t, "HardDelete", mock.Anything)
	runRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	lock.AssertNotCalled(t, "Release", mock.Anything, mock.Anything, mock.Anything)
}

// TestDeleteRun_SessionServiceNil_StillDeletesRun asserts a missing session
// manager degrades gracefully: the run record is still deleted.
func TestDeleteRun_SessionServiceNil_StillDeletesRun(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	// sessionService is left nil.

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)
	runRepo.On("Delete", mock.Anything, "run_1").Return(nil)

	if err := s.DeleteRun("run_1", "u1", false); err != nil {
		t.Fatalf("DeleteRun should not abort when session service is unset: %v", err)
	}
	runRepo.AssertCalled(t, "Delete", mock.Anything, "run_1")
}

// TestDeleteRun_LockAcquireErrorDegrades asserts a Redis infra error degrades
// gracefully (proceeds without the lock), mirroring the executor's degrade.
func TestDeleteRun_LockAcquireErrorDegrades(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)
	lock := redismocks.NewLocker(t)
	s.SetLocker(lock)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)
	lock.On("Acquire", mock.Anything, "lock:run:run_1", mock.Anything, mock.Anything).Return(false, fmt.Errorf("redis down"))
	sessSvc.On("HardDelete", "sess_1").Return(nil)
	runRepo.On("Delete", mock.Anything, "run_1").Return(nil)

	if err := s.DeleteRun("run_1", "u1", false); err != nil {
		t.Fatalf("DeleteRun should degrade on lock infra error: %v", err)
	}
	runRepo.AssertCalled(t, "Delete", mock.Anything, "run_1")
}

// TestDeleteRun_ReleaseAfterSuccess asserts the lock is released via defer on
// the success path.
func TestDeleteRun_ReleaseAfterSuccess(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)
	lock := redismocks.NewLocker(t)
	s.SetLocker(lock)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)
	lock.On("Acquire", mock.Anything, "lock:run:run_1", mock.Anything, mock.Anything).Return(true, nil)
	lock.On("Release", mock.Anything, "lock:run:run_1", mock.Anything).Return(nil)
	sessSvc.On("HardDelete", "sess_1").Return(nil)
	runRepo.On("Delete", mock.Anything, "run_1").Return(nil)

	if err := s.DeleteRun("run_1", "u1", false); err != nil {
		t.Fatalf("DeleteRun: %v", err)
	}
	lock.AssertCalled(t, "Release", mock.Anything, "lock:run:run_1", mock.Anything)
}

// TestDeleteRun_ReleaseError asserts a release error is logged (non-fatal) and
// the delete still succeeds.
func TestDeleteRun_ReleaseError(t *testing.T) {
	s, _, runRepo, _ := newTestService(t)
	sessSvc := chatmocks.NewSessionService(t)
	s.SetSessionService(sessSvc)
	lock := redismocks.NewLocker(t)
	s.SetLocker(lock)

	runRepo.On("Get", mock.Anything, "run_1").Return(&task.TaskRun{
		ID: "run_1", UserID: "u1", SessionID: "sess_1",
	}, nil)
	lock.On("Acquire", mock.Anything, "lock:run:run_1", mock.Anything, mock.Anything).Return(true, nil)
	lock.On("Release", mock.Anything, "lock:run:run_1", mock.Anything).Return(fmt.Errorf("redis down on release"))
	sessSvc.On("HardDelete", "sess_1").Return(nil)
	runRepo.On("Delete", mock.Anything, "run_1").Return(nil)

	if err := s.DeleteRun("run_1", "u1", false); err != nil {
		t.Fatalf("release error must be non-fatal: %v", err)
	}
	lock.AssertCalled(t, "Release", mock.Anything, "lock:run:run_1", mock.Anything)
}
