package task

import (
	"context"
	"errors"
	"reflect"
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
	if s.coll != &coll {
		t.Error("Service.coll should be the Collection returned by db.Collection")
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
	if tk.Type != "agent_exec" {
		t.Errorf("Type: got %q, want %q", tk.Type, "agent_exec")
	}
	if len(tk.SkillChain) != 2 {
		t.Fatalf("SkillChain length: got %d, want 2", len(tk.SkillChain))
	}
	if !reflect.DeepEqual(tk.Params, map[string]interface{}{"key": "val"}) {
		t.Errorf("Params: got %v, want {key: val}", tk.Params)
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
		f := filter.(bson.M)
		if f["_id"] != "task_123" {
			t.Errorf("FindOne filter _id: got %v, want task_123", f["_id"])
		}
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
	if tk.Status != task.StatusRunning {
		t.Errorf("Status: got %q, want %q", tk.Status, task.StatusRunning)
	}
	if tk.SessionID != "session1" {
		t.Errorf("SessionID: got %q, want %q", tk.SessionID, "session1")
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
	patches.ApplyMethodFunc(&coll, "UpdateOne", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		f := filter.(bson.M)
		if f["_id"] != "task_123" {
			t.Errorf("CancelTask filter _id: got %v, want task_123", f["_id"])
		}
		if statusFilter, ok := f["status"]; ok {
			if sf, ok := statusFilter.(bson.M); ok {
				if in, ok := sf["$in"].([]string); ok {
					foundQueued, foundRunning := false, false
					for _, s := range in {
						if s == "queued" {
							foundQueued = true
						}
						if s == "running" {
							foundRunning = true
						}
					}
					if !foundQueued || !foundRunning {
						t.Errorf("CancelTask status filter $in: got %v, want [queued, running]", in)
					}
				}
			}
		}
		u := update.(bson.M)
		setOp := u["$set"].(bson.M)
		if setOp["status"] != task.StatusCancelled {
			t.Errorf("CancelTask update status: got %v, want %q", setOp["status"], task.StatusCancelled)
		}
		return &mongo.UpdateResult{}, nil
	})

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
	patches.ApplyMethodFunc(&coll, "Find", func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
		f := filter.(bson.M)
		if f["user_id"] != "user1" {
			t.Errorf("ListTasks filter user_id: got %v, want user1", f["user_id"])
		}
		return &cur, nil
	})
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
	patches.ApplyMethodFunc(&coll, "UpdateOne", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		f := filter.(bson.M)
		if f["_id"] != "task_123" {
			t.Errorf("UpdateTaskProgress filter _id: got %v, want task_123", f["_id"])
		}
		u := update.(bson.M)
		setOp := u["$set"].(bson.M)
		progress := setOp["progress"].(task.TaskProgress)
		if progress.CurrentStep != 2 {
			t.Errorf("Progress.CurrentStep: got %d, want 2", progress.CurrentStep)
		}
		if progress.TotalSteps != 5 {
			t.Errorf("Progress.TotalSteps: got %d, want 5", progress.TotalSteps)
		}
		if progress.Percent != 40 {
			t.Errorf("Progress.Percent: got %d, want 40", progress.Percent)
		}
		if progress.Message != "Processing step 2" {
			t.Errorf("Progress.Message: got %q", progress.Message)
		}
		return &mongo.UpdateResult{}, nil
	})

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
	patches.ApplyMethodFunc(&coll, "UpdateOne", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		f := filter.(bson.M)
		if f["_id"] != "task_123" {
			t.Errorf("UpdateTaskResult filter _id: got %v, want task_123", f["_id"])
		}
		u := update.(bson.M)
		setOp := u["$set"].(bson.M)
		if setOp["status"] != task.StatusCompleted {
			t.Errorf("UpdateTaskResult status: got %v, want %q", setOp["status"], task.StatusCompleted)
		}
		result := setOp["result"].(map[string]interface{})
		if result["output"] != "done" {
			t.Errorf("UpdateTaskResult result.output: got %v, want done", result["output"])
		}
		if setOp["duration_ms"] != int64(1500) {
			t.Errorf("UpdateTaskResult duration_ms: got %v, want 1500", setOp["duration_ms"])
		}
		if setOp["error"] != "" {
			t.Errorf("UpdateTaskResult error: got %v, want empty", setOp["error"])
		}
		return &mongo.UpdateResult{}, nil
	})

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
	patches.ApplyMethodFunc(&coll, "UpdateOne", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		f := filter.(bson.M)
		if f["_id"] != "task_123" {
			t.Errorf("UpdateStatus filter _id: got %v, want task_123", f["_id"])
		}
		u := update.(bson.M)
		setOp := u["$set"].(bson.M)
		if setOp["status"] != "paused" {
			t.Errorf("UpdateStatus status: got %v, want paused", setOp["status"])
		}
		return &mongo.UpdateResult{}, nil
	})

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
	patches.ApplyMethodFunc(&coll, "Find", func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
		f := filter.(bson.M)
		if f["status"] != string(task.StatusRunning) {
			t.Errorf("ListAllTasks status filter: got %v, want %q", f["status"], task.StatusRunning)
		}
		return &cur, nil
	})
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
	for i, tk := range tasks {
		if tk.Status != task.StatusRunning {
			t.Errorf("task[%d] status: got %q, want %q", i, tk.Status, task.StatusRunning)
		}
	}
}

func TestListAllTasks_AllFilter(t *testing.T) {
	var coll mongo.Collection
	var cur mongo.Cursor

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodFunc(&coll, "Find", func(ctx context.Context, filter interface{}, opts ...*options.FindOptions) (*mongo.Cursor, error) {
		f := filter.(bson.M)
		if _, ok := f["status"]; ok {
			t.Error("ListAllTasks with 'all' filter should not have status filter")
		}
		return &cur, nil
	})
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
	patches.ApplyMethodFunc(&coll, "ReplaceOne", func(ctx context.Context, filter interface{}, replacement interface{}, opts ...*options.ReplaceOptions) (*mongo.UpdateResult, error) {
		f := filter.(bson.M)
		if f["_id"] != "task_123" {
			t.Errorf("ReplaceOne filter _id: got %v, want task_123", f["_id"])
		}
		tk := replacement.(*task.Task)
		if tk.Status != task.StatusQueued {
			t.Errorf("RetryTask status: got %q, want %q", tk.Status, task.StatusQueued)
		}
		if tk.RetryCount != 1 {
			t.Errorf("RetryTask retry_count: got %d, want 1", tk.RetryCount)
		}
		return &mongo.UpdateResult{}, nil
	})
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
	patches.ApplyMethodFunc(&coll, "UpdateMany", func(ctx context.Context, filter interface{}, update interface{}, opts ...*options.UpdateOptions) (*mongo.UpdateResult, error) {
		f := filter.(bson.M)
		if ids, ok := f["_id"].(bson.M)["$in"].([]string); ok {
			if len(ids) != 2 || ids[0] != "task_1" || ids[1] != "task_2" {
				t.Errorf("BatchCancelTasks ids: got %v, want [task_1, task_2]", ids)
			}
		}
		if statusFilter, ok := f["status"]; ok {
			if sf, ok := statusFilter.(bson.M); ok {
				if in, ok := sf["$in"].([]string); ok {
					foundQueued, foundRunning := false, false
					for _, s := range in {
						if s == "queued" {
							foundQueued = true
						}
						if s == "running" {
							foundRunning = true
						}
					}
					if !foundQueued || !foundRunning {
						t.Errorf("BatchCancelTasks status filter missing queued/running")
					}
				}
			}
		}
		u := update.(bson.M)
		setOp := u["$set"].(bson.M)
		if setOp["status"] != task.StatusCancelled {
			t.Errorf("BatchCancelTasks update status: got %v, want %q", setOp["status"], task.StatusCancelled)
		}
		return &mongo.UpdateResult{}, nil
	})

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
