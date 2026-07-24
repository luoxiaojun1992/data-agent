package worker

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domaintask "github.com/luoxiaojun1992/data-agent/internal/domain/task"
	domaintaskmocks "github.com/luoxiaojun1992/data-agent/internal/domain/task/mocks"
	"github.com/redis/go-redis/v9"
)

// ── Hand-rolled mocks (small interfaces, no mockery config needed) ──

type mockQueue struct {
	ackIDs []string
	dlqIDs []string
}

func (m *mockQueue) Dequeue(_ context.Context, _ string, _ time.Duration) ([]redis.XMessage, error) {
	return nil, nil
}
func (m *mockQueue) Ack(_ context.Context, messageID string) error {
	m.ackIDs = append(m.ackIDs, messageID)
	return nil
}
func (m *mockQueue) MoveToDLQ(_ context.Context, msgID string, _ []byte) error {
	m.dlqIDs = append(m.dlqIDs, msgID)
	return nil
}

type mockExecutor struct {
	err   error
	calls int
	last  *domaintask.Task
}

func (m *mockExecutor) Execute(_ context.Context, t *domaintask.Task) error {
	m.calls++
	m.last = t
	return m.err
}

// ── Harness ──

// newTestPool wires a Pool with mocked queue + executor + task service. The
// GetTask expectation matches any ID; tests assert on the specific ID called.
func newTestPool(t *testing.T, execErr error, task *domaintask.Task, taskErr error) (*Pool, *mockQueue, *mockExecutor, *domaintaskmocks.TaskService) {
	t.Helper()
	q := &mockQueue{}
	exec := &mockExecutor{err: execErr}
	tasks := domaintaskmocks.NewTaskService(t)
	if taskErr != nil {
		tasks.On("GetTask", mock.Anything).Return((*domaintask.Task)(nil), taskErr)
	} else {
		tasks.On("GetTask", mock.Anything).Return(task, nil)
	}
	pool := NewPool(q, nil, 1, exec, tasks)
	return pool, q, exec, tasks
}

// queueMsg builds a Redis XMessage whose "data" field is the JSON-encoded
// QueueMessage for the given task ID.
func queueMsg(t *testing.T, taskID string) redis.XMessage {
	t.Helper()
	data, err := json.Marshal(domaintask.QueueMessage{TaskID: taskID, UserID: "u1", Type: "agent"})
	require.NoError(t, err)
	return redis.XMessage{ID: "msg-1", Values: map[string]interface{}{"data": string(data)}}
}

// ── Tests ──

func TestProcessWorkerMessage_Success_LoadsFromDBAndExecutes(t *testing.T) {
	tk := &domaintask.Task{ID: "task_1", UserID: "u1", Status: domaintask.StatusQueued, MaxRetries: 3}
	pool, q, exec, tasks := newTestPool(t, nil, tk, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "task_1"))

	// SPEC-063: task loaded from DB (GetTask called), executor invoked with the
	// DB-loaded task, message acknowledged.
	tasks.AssertCalled(t, "GetTask", "task_1")
	require.Equal(t, 1, exec.calls)
	assert.Equal(t, "task_1", exec.last.ID)
	assert.Equal(t, []string{"msg-1"}, q.ackIDs)
	assert.Empty(t, q.dlqIDs, "no DLQ on success")
}

func TestProcessWorkerMessage_MissingDataKey_NoAck(t *testing.T) {
	// No GetTask expectation: the malformed message must short-circuit before
	// any DB load. (If a bug called GetTask, the default nil return would still
	// trigger an Ack and fail the assertion below.)
	q := &mockQueue{}
	exec := &mockExecutor{}
	tasks := domaintaskmocks.NewTaskService(t)
	pool := NewPool(q, nil, 1, exec, tasks)

	pool.processWorkerMessage(context.Background(), redis.XMessage{ID: "msg-1", Values: map[string]interface{}{}})

	assert.Empty(t, q.ackIDs, "malformed message should be dropped silently")
	assert.Equal(t, 0, exec.calls)
}

func TestProcessWorkerMessage_InvalidJSON_NoAck(t *testing.T) {
	q := &mockQueue{}
	exec := &mockExecutor{}
	tasks := domaintaskmocks.NewTaskService(t)
	pool := NewPool(q, nil, 1, exec, tasks)

	msg := redis.XMessage{ID: "msg-1", Values: map[string]interface{}{"data": "not-json"}}
	pool.processWorkerMessage(context.Background(), msg)

	assert.Empty(t, q.ackIDs)
	assert.Equal(t, 0, exec.calls)
}

func TestProcessWorkerMessage_GetTaskFails_AcksToDrop(t *testing.T) {
	// When the task can't be loaded (deleted/expired), the message is acked so
	// the stream doesn't stall on a poison message.
	pool, q, exec, _ := newTestPool(t, nil, nil, errors.New("not found"))

	pool.processWorkerMessage(context.Background(), queueMsg(t, "missing"))

	assert.Equal(t, []string{"msg-1"}, q.ackIDs, "unrecoverable message should be acked")
	assert.Equal(t, 0, exec.calls, "executor must not run when task load fails")
}

func TestProcessWorkerMessage_GetTaskNil_AcksToDrop(t *testing.T) {
	pool, q, exec, _ := newTestPool(t, nil, nil, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "nil-task"))

	assert.Equal(t, []string{"msg-1"}, q.ackIDs)
	assert.Equal(t, 0, exec.calls)
}

func TestProcessWorkerMessage_ExecuteFails_RetriesBelowMax_AcksNoDLQ(t *testing.T) {
	tk := &domaintask.Task{ID: "task_1", UserID: "u1", RetryCount: 0, MaxRetries: 3}
	pool, q, exec, _ := newTestPool(t, errors.New("boom"), tk, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "task_1"))

	require.Equal(t, 1, exec.calls)
	assert.Equal(t, []string{"msg-1"}, q.ackIDs)
	assert.Empty(t, q.dlqIDs, "below max retries should not DLQ")
}

func TestProcessWorkerMessage_ExecuteFails_AtMaxRetries_MovesToDLQ(t *testing.T) {
	// RetryCount starts at max-1; the pool increments to max → DLQ.
	tk := &domaintask.Task{ID: "task_1", UserID: "u1", RetryCount: 2, MaxRetries: 3}
	pool, q, exec, _ := newTestPool(t, errors.New("persistent failure"), tk, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "task_1"))

	require.Equal(t, 1, exec.calls)
	assert.Equal(t, []string{"msg-1"}, q.dlqIDs, "exhausted retries should DLQ")
	assert.Equal(t, []string{"msg-1"}, q.ackIDs, "DLQ'd message is still acked")
}

func TestNewPool_WiresDependencies(t *testing.T) {
	q := &mockQueue{}
	exec := &mockExecutor{}
	tasks := domaintaskmocks.NewTaskService(t)
	pool := NewPool(q, nil, 4, exec, tasks)

	assert.Equal(t, 4, pool.workers)
	assert.Same(t, q, pool.queue)
	assert.Same(t, exec, pool.executor)
	assert.Same(t, tasks, pool.taskSvc)
}
