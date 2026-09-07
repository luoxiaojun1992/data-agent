package adktools

import (
	"context"
	"testing"

	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	taskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	"github.com/stretchr/testify/mock"
	"google.golang.org/adk/agent"
)

// newToolContextWithUserRole builds a tool context whose state carries
// user_id and role (role used for the system_admin exemption in SPEC-087).
func newToolContextWithUserRole(userID, role string) *fakeToolContext {
	vals := map[string]any{"session_id": "s1"}
	if userID != "" {
		vals["user_id"] = userID
	}
	if role != "" {
		vals["role"] = role
	}
	return &fakeToolContext{
		StrictContextMock: agent.StrictContextMock{Ctx: context.Background()},
		state:             &fakeState{vals: vals},
	}
}

// ---- isCompleted ----

func TestIsCompleted(t *testing.T) {
	cases := []struct {
		status domaintask.Status
		want   bool
	}{
		{domaintask.StatusCompleted, true},
		{domaintask.StatusFailed, false},
		{domaintask.StatusCancelled, false},
		{domaintask.StatusRunning, false},
		{domaintask.StatusPending, false},
		{domaintask.StatusQueued, false},
		{domaintask.StatusRetrying, false},
	}
	for _, c := range cases {
		if got := isCompleted(c.status); got != c.want {
			t.Errorf("isCompleted(%q) = %v, want %v", c.status, got, c.want)
		}
	}
}

// ---- task_create ----

func TestTaskCreate_Success(t *testing.T) {
	svc := taskmocks.NewTaskService(t)
	var gotUserID, gotType, gotModel, gotSchedule, gotCron string
	var gotParams map[string]interface{}
	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotUserID = args.String(0)
			gotType = args.String(1)
			gotParams = args.Get(2).(map[string]interface{})
			gotModel = args.String(3)
			gotSchedule = args.String(4)
			gotCron = args.String(5)
		}).
		Return(&domaintask.Task{ID: "task_1", UserID: "u1", Title: "我的任务"},
			&domaintask.TaskRun{ID: "run_1"}, nil)

	fn := taskCreate(&Deps{TaskDefs: svc})
	res, err := fn(newToolContextWithUserRole("u1", ""), TaskCreateArgs{
		Title:  "我的任务",
		Type:   domaintask.TaskTypeAgentExec,
		Params: map[string]interface{}{"message": "分析数据"},
	})
	if err != nil {
		t.Fatalf("taskCreate: %v", err)
	}
	if res.TaskID != "task_1" || res.RunID != "run_1" {
		t.Errorf("result = %+v, want task_1/run_1", res)
	}
	if gotUserID != "u1" {
		t.Errorf("CreateTask userID = %q, want u1", gotUserID)
	}
	if gotType != domaintask.TaskTypeAgentExec {
		t.Errorf("CreateTask type = %q, want agent_exec", gotType)
	}
	if gotModel != "" || gotSchedule != "" || gotCron != "" {
		t.Errorf("CreateTask model/schedule/cron = %q/%q/%q, want empty", gotModel, gotSchedule, gotCron)
	}
	if gotParams["title"] != "我的任务" {
		t.Errorf("params[title] = %v, want 我的任务", gotParams["title"])
	}
	if gotParams["message"] != "分析数据" {
		t.Errorf("params[message] = %v, want 分析数据", gotParams["message"])
	}
}

func TestTaskCreate_DefaultTypeAndEmptyParams(t *testing.T) {
	svc := taskmocks.NewTaskService(t)
	var gotType string
	var gotParams map[string]interface{}
	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotType = args.String(1)
			gotParams = args.Get(2).(map[string]interface{})
		}).
		Return(&domaintask.Task{ID: "task_2", Title: "t"}, &domaintask.TaskRun{ID: "run_2"}, nil)

	fn := taskCreate(&Deps{TaskDefs: svc})
	res, err := fn(newToolContextWithUserRole("u1", ""), TaskCreateArgs{Title: "t"})
	if err != nil {
		t.Fatalf("taskCreate: %v", err)
	}
	if gotType != domaintask.TaskTypeAgentExec {
		t.Errorf("default type = %q, want agent_exec", gotType)
	}
	if len(gotParams) == 0 || gotParams["title"] != "t" {
		t.Errorf("params = %v, want title=t", gotParams)
	}
	if res.RunID != "run_2" {
		t.Errorf("RunID = %q, want run_2", res.RunID)
	}
}

func TestTaskCreate_ScheduledRecurring_NoRun(t *testing.T) {
	svc := taskmocks.NewTaskService(t)
	var gotType, gotSchedule, gotCron string
	svc.On("CreateTask", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotType = args.String(1)
			gotSchedule = args.String(4)
			gotCron = args.String(5)
		}).
		Return(&domaintask.Task{ID: "task_3", Title: "sched"}, (*domaintask.TaskRun)(nil), nil)

	fn := taskCreate(&Deps{TaskDefs: svc})
	res, err := fn(newToolContextWithUserRole("u1", ""), TaskCreateArgs{
		Title:    "sched",
		Type:     domaintask.TaskTypeScheduledExec,
		CronExpr: "0 1 * * *",
	})
	if err != nil {
		t.Fatalf("taskCreate: %v", err)
	}
	if gotType != domaintask.TaskTypeScheduledExec {
		t.Errorf("type = %q, want scheduled_exec", gotType)
	}
	if gotSchedule != domaintask.ScheduleModeRecurring {
		t.Errorf("scheduleMode = %q, want recurring", gotSchedule)
	}
	if gotCron != "0 1 * * *" {
		t.Errorf("cron = %q, want 0 1 * * *", gotCron)
	}
	if res.RunID != "" {
		t.Errorf("RunID = %q, want empty for scheduled task", res.RunID)
	}
}

func TestTaskCreate_EmptyTitle(t *testing.T) {
	fn := taskCreate(&Deps{TaskDefs: taskmocks.NewTaskService(t)})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskCreateArgs{Title: "   "})
	if err == nil {
		t.Fatal("expected error for empty title")
	}
}

func TestTaskCreate_EmptyUserID(t *testing.T) {
	fn := taskCreate(&Deps{TaskDefs: taskmocks.NewTaskService(t)})
	_, err := fn(newToolContextWithUserRole("", ""), TaskCreateArgs{Title: "t"})
	if err == nil {
		t.Fatal("expected error when session has no user_id")
	}
}

func TestTaskCreate_NilTaskDefs(t *testing.T) {
	fn := taskCreate(&Deps{TaskDefs: nil})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskCreateArgs{Title: "t"})
	if err == nil {
		t.Fatal("expected error when TaskDefs is nil")
	}
}

// ---- task_run_list ----

func TestTaskRunList_Success(t *testing.T) {
	defs := taskmocks.NewTaskService(t)
	defs.On("GetTask", "task_1", "u1", false).
		Return(&domaintask.Task{ID: "task_1", UserID: "u1"}, nil)

	runs := taskmocks.NewTaskRunService(t)
	runs.On("ListRuns", "task_1", "u1", false, "", int64(0), int64(10)).
		Return([]*domaintask.TaskRun{
			{ID: "run_2", Status: domaintask.StatusCompleted},
			{ID: "run_1", Status: domaintask.StatusFailed},
		}, int64(2), nil)

	fn := taskRunList(&Deps{TaskDefs: defs, Tasks: runs})
	res, err := fn(newToolContextWithUserRole("u1", ""), TaskRunListArgs{TaskID: "task_1"})
	if err != nil {
		t.Fatalf("taskRunList: %v", err)
	}
	if res.Count != 2 {
		t.Errorf("Count = %d, want 2", res.Count)
	}
	if len(res.Runs) != 2 {
		t.Fatalf("len(Runs) = %d, want 2", len(res.Runs))
	}
	if res.Runs[0].RunID != "run_2" || !res.Runs[0].Completed {
		t.Errorf("Runs[0] = %+v, want run_2 completed=true", res.Runs[0])
	}
	if res.Runs[1].RunID != "run_1" || res.Runs[1].Completed {
		t.Errorf("Runs[1] = %+v, want run_1 completed=false (failed)", res.Runs[1])
	}
}

func TestTaskRunList_EmptyTaskID(t *testing.T) {
	fn := taskRunList(&Deps{TaskDefs: taskmocks.NewTaskService(t), Tasks: taskmocks.NewTaskRunService(t)})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskRunListArgs{TaskID: "  "})
	if err == nil {
		t.Fatal("expected error for empty task_id")
	}
}

func TestTaskRunList_OwnershipDenied(t *testing.T) {
	defs := taskmocks.NewTaskService(t)
	// GetTask returns a task owned by another user → tool must refuse.
	defs.On("GetTask", "task_x", "u1", false).
		Return(&domaintask.Task{ID: "task_x", UserID: "other_user"}, nil)

	fn := taskRunList(&Deps{TaskDefs: defs, Tasks: taskmocks.NewTaskRunService(t)})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskRunListArgs{TaskID: "task_x"})
	if err == nil {
		t.Fatal("expected ownership error for another user's task")
	}
}

func TestTaskRunList_SystemAdminExempt(t *testing.T) {
	defs := taskmocks.NewTaskService(t)
	// system_admin can read any task; GetTask called with isSystemAdmin=true.
	defs.On("GetTask", "task_x", "u1", true).
		Return(&domaintask.Task{ID: "task_x", UserID: "other_user"}, nil)
	runs := taskmocks.NewTaskRunService(t)
	runs.On("ListRuns", "task_x", "u1", true, "", int64(0), int64(10)).
		Return([]*domaintask.TaskRun{{ID: "run_1", Status: domaintask.StatusCompleted}}, int64(1), nil)

	fn := taskRunList(&Deps{TaskDefs: defs, Tasks: runs})
	res, err := fn(newToolContextWithUserRole("u1", "system_admin"), TaskRunListArgs{TaskID: "task_x"})
	if err != nil {
		t.Fatalf("system_admin should be exempt: %v", err)
	}
	if res.Count != 1 {
		t.Errorf("Count = %d, want 1", res.Count)
	}
}

func TestTaskRunList_TopNClamped(t *testing.T) {
	defs := taskmocks.NewTaskService(t)
	defs.On("GetTask", "task_1", "u1", false).
		Return(&domaintask.Task{ID: "task_1", UserID: "u1"}, nil)
	runs := taskmocks.NewTaskRunService(t)
	runs.On("ListRuns", "task_1", "u1", false, "", int64(0), int64(50)).
		Return([]*domaintask.TaskRun{}, int64(0), nil)

	fn := taskRunList(&Deps{TaskDefs: defs, Tasks: runs})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskRunListArgs{TaskID: "task_1", TopN: 100})
	if err != nil {
		t.Fatalf("taskRunList: %v", err)
	}
}

func TestTaskRunList_EmptyUserID(t *testing.T) {
	fn := taskRunList(&Deps{TaskDefs: taskmocks.NewTaskService(t), Tasks: taskmocks.NewTaskRunService(t)})
	_, err := fn(newToolContextWithUserRole("", ""), TaskRunListArgs{TaskID: "task_1"})
	if err == nil {
		t.Fatal("expected error when session has no user_id")
	}
}

func TestTaskRunList_NilDeps(t *testing.T) {
	fn := taskRunList(&Deps{})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskRunListArgs{TaskID: "task_1"})
	if err == nil {
		t.Fatal("expected error when Tasks/TaskDefs is nil")
	}
}

// ---- task_run_detail ----

func TestTaskRunDetail_Success(t *testing.T) {
	runs := taskmocks.NewTaskRunService(t)
	runs.On("GetRun", "run_1", "u1", false).
		Return(&domaintask.TaskRun{
			ID:     "run_1",
			TaskID: "task_1",
			UserID: "u1",
			Status: domaintask.StatusCompleted,
			Result: map[string]interface{}{"content": "结果内容"},
			Error:  "",
		}, nil)

	fn := taskRunDetail(&Deps{Tasks: runs})
	res, err := fn(newToolContextWithUserRole("u1", ""), TaskRunDetailArgs{RunID: "run_1"})
	if err != nil {
		t.Fatalf("taskRunDetail: %v", err)
	}
	if res.RunID != "run_1" || res.TaskID != "task_1" {
		t.Errorf("result ids = %q/%q, want run_1/task_1", res.RunID, res.TaskID)
	}
	if res.Status != "completed" || !res.Completed {
		t.Errorf("status/completed = %q/%v, want completed/true", res.Status, res.Completed)
	}
	if res.Result["content"] != "结果内容" {
		t.Errorf("result.content = %v, want 结果内容", res.Result["content"])
	}
}

func TestTaskRunDetail_FailedRun(t *testing.T) {
	runs := taskmocks.NewTaskRunService(t)
	runs.On("GetRun", "run_2", "u1", false).
		Return(&domaintask.TaskRun{
			ID:     "run_2",
			TaskID: "task_1",
			UserID: "u1",
			Status: domaintask.StatusFailed,
			Error:  "boom",
		}, nil)

	fn := taskRunDetail(&Deps{Tasks: runs})
	res, err := fn(newToolContextWithUserRole("u1", ""), TaskRunDetailArgs{RunID: "run_2"})
	if err != nil {
		t.Fatalf("taskRunDetail: %v", err)
	}
	if res.Completed {
		t.Error("failed run must have completed=false")
	}
	if res.Error != "boom" {
		t.Errorf("Error = %q, want boom", res.Error)
	}
}

func TestTaskRunDetail_EmptyRunID(t *testing.T) {
	fn := taskRunDetail(&Deps{Tasks: taskmocks.NewTaskRunService(t)})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskRunDetailArgs{RunID: "  "})
	if err == nil {
		t.Fatal("expected error for empty run_id")
	}
}

func TestTaskRunDetail_OwnershipDenied(t *testing.T) {
	runs := taskmocks.NewTaskRunService(t)
	runs.On("GetRun", "run_x", "u1", false).
		Return(&domaintask.TaskRun{ID: "run_x", UserID: "other_user", Status: domaintask.StatusCompleted}, nil)

	fn := taskRunDetail(&Deps{Tasks: runs})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskRunDetailArgs{RunID: "run_x"})
	if err == nil {
		t.Fatal("expected ownership error for another user's run")
	}
}

func TestTaskRunDetail_SystemAdminExempt(t *testing.T) {
	runs := taskmocks.NewTaskRunService(t)
	runs.On("GetRun", "run_x", "u1", true).
		Return(&domaintask.TaskRun{ID: "run_x", UserID: "other_user", Status: domaintask.StatusCompleted}, nil)

	fn := taskRunDetail(&Deps{Tasks: runs})
	res, err := fn(newToolContextWithUserRole("u1", "system_admin"), TaskRunDetailArgs{RunID: "run_x"})
	if err != nil {
		t.Fatalf("system_admin should be exempt: %v", err)
	}
	if !res.Completed {
		t.Error("completed should be true")
	}
}

func TestTaskRunDetail_EmptyUserID(t *testing.T) {
	fn := taskRunDetail(&Deps{Tasks: taskmocks.NewTaskRunService(t)})
	_, err := fn(newToolContextWithUserRole("", ""), TaskRunDetailArgs{RunID: "run_1"})
	if err == nil {
		t.Fatal("expected error when session has no user_id")
	}
}

func TestTaskRunDetail_NilTasks(t *testing.T) {
	fn := taskRunDetail(&Deps{Tasks: nil})
	_, err := fn(newToolContextWithUserRole("u1", ""), TaskRunDetailArgs{RunID: "run_1"})
	if err == nil {
		t.Fatal("expected error when Tasks is nil")
	}
}
