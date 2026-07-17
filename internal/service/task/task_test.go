package task

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/luoxiaojun1992/data-agent/internal/domain/task"
	"github.com/luoxiaojun1992/data-agent/internal/queue"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestCollTasks(t *testing.T) {
	if collTasks != "agent_tasks" {
		t.Errorf("collTasks = %q, want %q", collTasks, "agent_tasks")
	}
}

func TestNewService(t *testing.T) {
	db := &mongo.Database{}
	var coll mongo.Collection
	patches := gomonkey.ApplyMethodReturn(db, "Collection", &coll)
	defer patches.Reset()

	s := NewService(db, nil)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
}

func TestCreateTask_Success(t *testing.T) {
	var coll mongo.Collection
	var stream queue.Stream

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)
	patches.ApplyMethodReturn(&stream, "Enqueue", nil)

	svc := &Service{coll: &coll, stream: &stream}
	tk, err := svc.CreateTask("session1", "user1", "agent_exec", []string{"skill1", "skill2"}, map[string]interface{}{"key": "val"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk == nil {
		t.Fatal("expected non-nil Task")
	}
	if tk.Status != task.StatusQueued {
		t.Errorf("Status: got %q, want %q", tk.Status, task.StatusQueued)
	}
	if tk.SessionID != "session1" {
		t.Errorf("SessionID: got %q, want %q", tk.SessionID, "session1")
	}
	if tk.UserID != "user1" {
		t.Errorf("UserID: got %q, want %q", tk.UserID, "user1")
	}
}

func TestCreateTask_InsertError(t *testing.T) {
	var coll mongo.Collection
	var stream queue.Stream

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", (*mongo.InsertOneResult)(nil), errors.New("insert failed"))

	svc := &Service{coll: &coll, stream: &stream}
	_, err := svc.CreateTask("session1", "user1", "agent_exec", []string{"skill1"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCreateTask_EnqueueError(t *testing.T) {
	var coll mongo.Collection
	var stream queue.Stream

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "InsertOne", &mongo.InsertOneResult{}, nil)
	patches.ApplyMethodReturn(&stream, "Enqueue", errors.New("enqueue failed"))

	svc := &Service{coll: &coll, stream: &stream}
	_, err := svc.CreateTask("session1", "user1", "agent_exec", []string{"skill1"}, nil)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTask_Success(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		tk := v.(*task.Task)
		tk.ID = "task_123"
		tk.Status = task.StatusRunning
		tk.SessionID = "session1"
		tk.UserID = "user1"
		return nil
	})

	svc := &Service{coll: &coll}
	tk, err := svc.GetTask("task_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tk.ID != "task_123" {
		t.Errorf("ID: got %q, want %q", tk.ID, "task_123")
	}
}

func TestGetTask_NotFound(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", mongo.ErrNoDocuments)

	svc := &Service{coll: &coll}
	_, err := svc.GetTask("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestGetTask_OtherError(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", errors.New("db error"))

	svc := &Service{coll: &coll}
	_, err := svc.GetTask("task_123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestCancelTask_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", &mongo.UpdateResult{}, nil)

	svc := &Service{coll: &coll}
	err := svc.CancelTask("task_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestCancelTask_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: &coll}
	err := svc.CancelTask("task_123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListTasks_Success(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	tasks, err := svc.ListTasks("user1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 0 {
		t.Errorf("expected empty tasks, got %d", len(tasks))
	}
}

func TestListTasks_FindError(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("find failed"))

	svc := &Service{coll: &coll}
	_, err := svc.ListTasks("user1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListTasks_CursorAllError(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodReturn(&cur, "All", errors.New("cursor all failed"))

	svc := &Service{coll: &coll}
	_, err := svc.ListTasks("user1")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateTaskProgress_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", &mongo.UpdateResult{}, nil)

	svc := &Service{coll: &coll}
	err := svc.UpdateTaskProgress("task_123", task.TaskProgress{
		CurrentStep: 2,
		TotalSteps:  5,
		Message:     "Processing step 2",
		Percent:     40,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateTaskProgress_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: &coll}
	err := svc.UpdateTaskProgress("task_123", task.TaskProgress{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateTaskResult_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", &mongo.UpdateResult{}, nil)

	svc := &Service{coll: &coll}
	err := svc.UpdateTaskResult("task_123", task.StatusCompleted, map[string]interface{}{"output": "done"}, "", 1500)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateTaskResult_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: &coll}
	err := svc.UpdateTaskResult("task_123", task.StatusFailed, nil, "something went wrong", 0)
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestUpdateStatus_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", &mongo.UpdateResult{}, nil)

	svc := &Service{coll: &coll}
	err := svc.UpdateStatus("task_123", "paused")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestUpdateStatus_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateOne", (*mongo.UpdateResult)(nil), errors.New("update failed"))

	svc := &Service{coll: &coll}
	err := svc.UpdateStatus("task_123", "paused")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAllTasks_Success(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	tasks, err := svc.ListAllTasks("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tasks == nil {
		t.Error("expected non-nil tasks slice")
	}
}

func TestListAllTasks_WithStatusFilter(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		slice := results.(*[]task.Task)
		*slice = []task.Task{
			{ID: "task_1", Status: task.StatusRunning},
			{ID: "task_2", Status: task.StatusRunning},
		}
		return nil
	})

	svc := &Service{coll: &coll}
	tasks, err := svc.ListAllTasks(string(task.StatusRunning))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(tasks) != 2 {
		t.Fatalf("expected 2 tasks, got %d", len(tasks))
	}
}

func TestListAllTasks_AllFilter(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodFunc(&cur, "All", func(ctx context.Context, results interface{}) error {
		return nil
	})

	svc := &Service{coll: &coll}
	tasks, err := svc.ListAllTasks("all")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if tasks == nil {
		t.Error("expected non-nil tasks slice")
	}
}

func TestListAllTasks_FindError(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", (*mongo.Cursor)(nil), errors.New("find failed"))

	svc := &Service{coll: &coll}
	_, err := svc.ListAllTasks("")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRetryTask_Success(t *testing.T) {
	var coll mongo.Collection
	var stream queue.Stream

	// GetTask -> FindOne + Decode
	var sr mongo.SingleResult
	// RetryTask's s.coll.ReplaceOne
	// RetryTask's s.stream.Enqueue

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		tk := v.(*task.Task)
		tk.ID = "task_123"
		tk.Status = task.StatusFailed
		tk.RetryCount = 0
		return nil
	})
	patches.ApplyMethodReturn(&coll, "ReplaceOne", &mongo.UpdateResult{}, nil)
	patches.ApplyMethodReturn(&stream, "Enqueue", nil)

	svc := &Service{coll: &coll, stream: &stream}
	err := svc.RetryTask("task_123")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestRetryTask_NotFailed(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		tk := v.(*task.Task)
		tk.ID = "task_123"
		tk.Status = task.StatusRunning
		return nil
	})

	svc := &Service{coll: &coll}
	err := svc.RetryTask("task_123")
	if err == nil {
		t.Fatal("expected error for non-failed task")
	}
}

func TestRetryTask_GetTaskError(t *testing.T) {
	var coll mongo.Collection
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodReturn(&sr, "Decode", mongo.ErrNoDocuments)

	svc := &Service{coll: &coll}
	err := svc.RetryTask("nonexistent")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestRetryTask_ReplaceOneError(t *testing.T) {
	var coll mongo.Collection
	var stream queue.Stream
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		tk := v.(*task.Task)
		tk.ID = "task_123"
		tk.Status = task.StatusFailed
		return nil
	})
	patches.ApplyMethodReturn(&coll, "ReplaceOne", (*mongo.UpdateResult)(nil), errors.New("replace failed"))

	svc := &Service{coll: &coll, stream: &stream}
	err := svc.RetryTask("task_123")
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestBatchCancelTasks_Success(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateMany", &mongo.UpdateResult{}, nil)

	svc := &Service{coll: &coll}
	err := svc.BatchCancelTasks([]string{"task_1", "task_2"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestBatchCancelTasks_Error(t *testing.T) {
	var coll mongo.Collection

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "UpdateMany", (*mongo.UpdateResult)(nil), errors.New("batch cancel failed"))

	svc := &Service{coll: &coll}
	err := svc.BatchCancelTasks([]string{"task_1"})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestListAllTasks_CursorAllError(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(&coll, "Find", &cur, nil)
	patches.ApplyMethodFunc(&cur, "Close", func(ctx context.Context) error { return nil })
	patches.ApplyMethodReturn(&cur, "All", errors.New("cursor all failed"))

	svc := &Service{coll: &coll}
	_, err := svc.ListAllTasks("")
	if err == nil {
		t.Fatal("expected cursor.All error")
	}
}

func TestRetryTask_EnqueueError(t *testing.T) {
	var coll mongo.Collection
	var stream queue.Stream
	var sr mongo.SingleResult

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "FindOne", func(ctx context.Context, filter interface{}, opts ...*options.FindOneOptions) *mongo.SingleResult {
		return &sr
	})
	patches.ApplyMethodFunc(&sr, "Decode", func(v interface{}) error {
		tk := v.(*task.Task)
		tk.ID = "task_123"
		tk.Status = task.StatusFailed
		tk.RetryCount = 0
		return nil
	})
	patches.ApplyMethodReturn(&coll, "ReplaceOne", &mongo.UpdateResult{}, nil)
	patches.ApplyMethodReturn(&stream, "Enqueue", errors.New("enqueue failed"))

	svc := &Service{coll: &coll, stream: &stream}
	err := svc.RetryTask("task_123")
	if err == nil {
		t.Fatal("expected enqueue error")
	}
}

// Ensure bson is used
var _ = bson.M{}
