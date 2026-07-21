package task

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestNewService(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	s := NewService(repo, nil)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
}

func TestCreateTask_Success(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	tsk, err := NewService(repo, nil).CreateTask("s1", "u1", "analysis", []string{"sql", "stats"}, map[string]interface{}{"query": "SELECT 1"})
	if err != nil {
		t.Fatalf("CreateTask: %v", err)
	}
	if tsk == nil || tsk.Status != task.StatusQueued {
		t.Errorf("unexpected task: status=%s", tsk.Status)
	}
}

func TestGetTask_Success(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", Status: task.StatusRunning}, nil)

	tsk, err := NewService(repo, nil).GetTask("task_1")
	if err != nil {
		t.Fatalf("GetTask: %v", err)
	}
	if tsk.Status != task.StatusRunning {
		t.Errorf("Status: got %s, want running", tsk.Status)
	}
}

func TestCancellationToken_Success(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", Status: task.StatusQueued}, nil)
	repo.On("Cancel", mock.Anything, "task_1").Return(nil)

	if err := NewService(repo, nil).CancelTask("task_1"); err != nil {
		t.Fatalf("CancelTask: %v", err)
	}
}

func TestListTasks_Success(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("List", mock.Anything, "user1", "running", int64(0), int64(50)).Return(
		[]*task.Task{{ID: "t1"}, {ID: "t2"}}, int64(2), nil,
	)

	tasks, total, err := NewService(repo, nil).ListTasks("user1", "running", 0, 50)
	if err != nil {
		t.Fatalf("ListTasks: %v", err)
	}
	if len(tasks) != 2 || total != 2 {
		t.Fatalf("got %d tasks (total=%d), want 2", len(tasks), total)
	}
}

func TestRetryTask_Success(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Get", mock.Anything, "task_1").Return(&task.Task{ID: "task_1", Status: task.StatusFailed, RetryCount: 0}, nil)
	repo.On("Retry", mock.Anything, "task_1", mock.Anything).Return(nil)

	tsk, err := NewService(repo, nil).RetryTask("task_1")
	if err != nil {
		t.Fatalf("RetryTask: %v", err)
	}
	if tsk == nil {
		t.Fatal("expected retried task")
	}
}

func TestTask_CreateError(t *testing.T) {
	repo := mockrepo.NewTaskRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("db error"))

	_, err := NewService(repo, nil).CreateTask("s1", "u1", "analysis", nil, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}
