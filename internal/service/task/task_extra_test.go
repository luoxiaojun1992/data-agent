package task

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestCancelTask_NotFound(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "missing").Return((*task.Task)(nil), fmt.Errorf("not found"))
	err := NewService(repo, nil).CancelTask("missing")
	if err == nil {
		t.Error("expected error for missing task")
	}
}

func TestCancelTask_AlreadyCompleted(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "t1").Return(&task.Task{ID: "t1", Status: task.StatusCompleted}, nil)
	err := NewService(repo, nil).CancelTask("t1")
	if err == nil {
		t.Error("expected error for completed task")
	}
}

func TestCancelTask_AlreadyCancelled(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "t1").Return(&task.Task{ID: "t1", Status: task.StatusCancelled}, nil)
	err := NewService(repo, nil).CancelTask("t1")
	if err == nil {
		t.Error("expected error for already-cancelled task")
	}
}

func TestUpdateTaskProgress(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("UpdateProgress", mock.Anything, "t1", mock.Anything).Return(nil)
	err := NewService(repo, nil).UpdateTaskProgress("t1", &task.TaskProgress{CurrentStep: 1, TotalSteps: 3})
	if err != nil {
		t.Fatalf("UpdateTaskProgress: %v", err)
	}
}

func TestUpdateTaskResult(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("UpdateResult", mock.Anything, "t1", mock.Anything).Return(nil)
	err := NewService(repo, nil).UpdateTaskResult("t1", map[string]interface{}{"answer": 42})
	if err != nil {
		t.Fatalf("UpdateTaskResult: %v", err)
	}
}

func TestUpdateStatus_Success(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "t1").Return(&task.Task{ID: "t1", Status: task.StatusRunning}, nil)
	repo.On("Retry", mock.Anything, "t1", mock.Anything).Return(nil)
	err := NewService(repo, nil).UpdateStatus("t1", task.Status("paused"))
	if err != nil {
		t.Fatalf("UpdateStatus: %v", err)
	}
}

func TestUpdateStatus_NotFound(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "missing").Return((*task.Task)(nil), fmt.Errorf("not found"))
	err := NewService(repo, nil).UpdateStatus("missing", task.Status("paused"))
	if err == nil {
		t.Error("expected error for missing task")
	}
}

func TestListAllTasks(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("ListAll", mock.Anything, "u1").Return([]*task.Task{{ID: "t1"}, {ID: "t2"}}, nil)
	tasks, err := NewService(repo, nil).ListAllTasks("u1")
	if err != nil {
		t.Fatalf("ListAllTasks: %v", err)
	}
	if len(tasks) != 2 {
		t.Errorf("got %d tasks, want 2", len(tasks))
	}
}

func TestRetryTask_NotFound(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "missing").Return((*task.Task)(nil), fmt.Errorf("not found"))
	_, err := NewService(repo, nil).RetryTask("missing")
	if err == nil {
		t.Error("expected error for missing task")
	}
}

func TestRetryTask_NotFailed(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "t1").Return(&task.Task{ID: "t1", Status: task.StatusRunning}, nil)
	_, err := NewService(repo, nil).RetryTask("t1")
	if err == nil {
		t.Error("expected error for non-failed task")
	}
}

func TestBatchCancelTasks(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "t1").Return(&task.Task{ID: "t1", Status: task.StatusQueued}, nil)
	repo.On("Get", mock.Anything, "t2").Return(&task.Task{ID: "t2", Status: task.StatusQueued}, nil)
	repo.On("Cancel", mock.Anything, "t1").Return(nil)
	repo.On("Cancel", mock.Anything, "t2").Return(nil)
	err := NewService(repo, nil).BatchCancelTasks([]string{"t1", "t2"})
	if err != nil {
		t.Fatalf("BatchCancelTasks: %v", err)
	}
}

func TestBatchCancelTasks_PartialFailure(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "t1").Return((*task.Task)(nil), fmt.Errorf("not found"))
	repo.On("Get", mock.Anything, "t2").Return(&task.Task{ID: "t2", Status: task.StatusQueued}, nil)
	repo.On("Cancel", mock.Anything, "t2").Return(nil)
	// best-effort: returns nil even if one fails
	err := NewService(repo, nil).BatchCancelTasks([]string{"t1", "t2"})
	if err != nil {
		t.Fatalf("BatchCancelTasks should be best-effort: %v", err)
	}
}

func TestCreateTask_WithQueue(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	queue := mockrepo.NewQueueRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(nil)
	tsk, err := NewService(repo, queue).CreateTask("s1", "u1", "agent", []string{"sql"}, nil, "model_x")
	if err != nil {
		t.Fatalf("CreateTask with queue: %v", err)
	}
	if tsk.Status != task.StatusQueued {
		t.Errorf("status = %s", tsk.Status)
	}
}

func TestCreateTask_QueueError_BestEffort(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	queue := mockrepo.NewQueueRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)
	queue.On("Enqueue", mock.Anything, mock.Anything).Return(fmt.Errorf("redis down"))
	// best-effort: returns task even if queue fails
	tsk, err := NewService(repo, queue).CreateTask("s1", "u1", "agent", nil, nil, "")
	if err != nil {
		t.Fatalf("CreateTask should be best-effort on queue error: %v", err)
	}
	if tsk == nil {
		t.Error("task should not be nil")
	}
}

func TestRetryTask_Success_Requeued(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "t1").Return(&task.Task{ID: "t1", Status: task.StatusFailed, RetryCount: 0}, nil)
	repo.On("Retry", mock.Anything, "t1", mock.Anything).Return(nil)
	tsk, err := NewService(repo, nil).RetryTask("t1")
	if err != nil {
		t.Fatalf("RetryTask: %v", err)
	}
	if tsk.Status != task.StatusQueued || tsk.RetryCount != 1 {
		t.Errorf("task = %+v", tsk)
	}
}
