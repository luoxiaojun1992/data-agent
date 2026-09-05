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
	last  *domaintask.TaskRun
}

func (m *mockExecutor) Execute(_ context.Context, run *domaintask.TaskRun) error {
	m.calls++
	m.last = run
	return m.err
}

// ── Harness ──

// newTestPool wires a Pool with mocked queue + executor + run service. The
// GetRun expectation matches any ID; tests assert on the specific ID called.
func newTestPool(t *testing.T, execErr error, run *domaintask.TaskRun, runErr error) (*Pool, *mockQueue, *mockExecutor, *domaintaskmocks.TaskRunService) {
	t.Helper()
	q := &mockQueue{}
	exec := &mockExecutor{err: execErr}
	runs := domaintaskmocks.NewTaskRunService(t)
	if runErr != nil {
		runs.On("GetRun", mock.Anything, mock.Anything, mock.Anything).Return((*domaintask.TaskRun)(nil), runErr)
	} else {
		runs.On("GetRun", mock.Anything, mock.Anything, mock.Anything).Return(run, nil)
	}
	pool := NewPool(q, nil, 1, exec, runs)
	return pool, q, exec, runs
}

// queueMsg builds a Redis XMessage whose "data" field is the JSON-encoded
// QueueMessage envelope ({type, payload}) carrying an AgentTaskPayload.
func queueMsg(t *testing.T, runID string) redis.XMessage {
	t.Helper()
	payload, err := json.Marshal(domaintask.AgentTaskPayload{RunID: runID, TaskID: "task_1", UserID: "u1"})
	require.NoError(t, err)
	data, err := json.Marshal(domaintask.QueueMessage{Type: "agent_task", Payload: payload})
	require.NoError(t, err)
	return redis.XMessage{ID: "msg-1", Values: map[string]interface{}{"data": string(data)}}
}

// ── Tests ──

func TestProcessWorkerMessage_Success_LoadsFromDBAndExecutes(t *testing.T) {
	run := &domaintask.TaskRun{ID: "run_1", TaskID: "task_1", UserID: "u1", Status: domaintask.StatusPending, MaxRetries: 3}
	pool, q, exec, runs := newTestPool(t, nil, run, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "run_1"))

	// SPEC-063: run loaded from DB (GetRun called), executor invoked with the
	// DB-loaded run, message acknowledged.
	runs.AssertCalled(t, "GetRun", "run_1", "", true)
	require.Equal(t, 1, exec.calls)
	assert.Equal(t, "run_1", exec.last.ID)
	assert.Equal(t, []string{"msg-1"}, q.ackIDs)
	assert.Empty(t, q.dlqIDs, "no DLQ on success")
}

func TestProcessWorkerMessage_MissingDataKey_NoAck(t *testing.T) {
	// No GetRun expectation: the malformed message must short-circuit before
	// any DB load. (If a bug called GetRun, the default nil return would still
	// trigger an Ack and fail the assertion below.)
	q := &mockQueue{}
	exec := &mockExecutor{}
	runs := domaintaskmocks.NewTaskRunService(t)
	pool := NewPool(q, nil, 1, exec, runs)

	pool.processWorkerMessage(context.Background(), redis.XMessage{ID: "msg-1", Values: map[string]interface{}{}})

	assert.Empty(t, q.ackIDs, "malformed message should be dropped silently")
	assert.Equal(t, 0, exec.calls)
}

func TestProcessWorkerMessage_InvalidJSON_NoAck(t *testing.T) {
	q := &mockQueue{}
	exec := &mockExecutor{}
	runs := domaintaskmocks.NewTaskRunService(t)
	pool := NewPool(q, nil, 1, exec, runs)

	msg := redis.XMessage{ID: "msg-1", Values: map[string]interface{}{"data": "not-json"}}
	pool.processWorkerMessage(context.Background(), msg)

	assert.Empty(t, q.ackIDs)
	assert.Equal(t, 0, exec.calls)
}

func TestProcessWorkerMessage_GetRunFails_AcksToDrop(t *testing.T) {
	// When the run can't be loaded (deleted/expired), the message is acked so
	// the stream doesn't stall on a poison message.
	pool, q, exec, _ := newTestPool(t, nil, nil, errors.New("not found"))

	pool.processWorkerMessage(context.Background(), queueMsg(t, "missing"))

	assert.Equal(t, []string{"msg-1"}, q.ackIDs, "unrecoverable message should be acked")
	assert.Equal(t, 0, exec.calls, "executor must not run when run load fails")
}

func TestProcessWorkerMessage_GetRunNil_AcksToDrop(t *testing.T) {
	pool, q, exec, _ := newTestPool(t, nil, nil, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "nil-run"))

	assert.Equal(t, []string{"msg-1"}, q.ackIDs)
	assert.Equal(t, 0, exec.calls)
}

func TestProcessWorkerMessage_ExecuteFails_RetriesBelowMax_AcksNoDLQ(t *testing.T) {
	run := &domaintask.TaskRun{ID: "run_1", TaskID: "task_1", UserID: "u1", RetryCount: 0, MaxRetries: 3}
	pool, q, exec, _ := newTestPool(t, errors.New("boom"), run, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "run_1"))

	require.Equal(t, 1, exec.calls)
	assert.Equal(t, []string{"msg-1"}, q.ackIDs)
	assert.Empty(t, q.dlqIDs, "below max retries should not DLQ")
}

func TestProcessWorkerMessage_ExecuteFails_AtMaxRetries_MovesToDLQ(t *testing.T) {
	// RetryCount starts at max-1; the pool increments to max → DLQ.
	run := &domaintask.TaskRun{ID: "run_1", TaskID: "task_1", UserID: "u1", RetryCount: 2, MaxRetries: 3}
	pool, q, exec, _ := newTestPool(t, errors.New("persistent failure"), run, nil)

	pool.processWorkerMessage(context.Background(), queueMsg(t, "run_1"))

	require.Equal(t, 1, exec.calls)
	assert.Equal(t, []string{"msg-1"}, q.dlqIDs, "exhausted retries should DLQ")
	assert.Equal(t, []string{"msg-1"}, q.ackIDs, "DLQ'd message is still acked")
}

func TestNewPool_WiresDependencies(t *testing.T) {
	q := &mockQueue{}
	exec := &mockExecutor{}
	runs := domaintaskmocks.NewTaskRunService(t)
	pool := NewPool(q, nil, 4, exec, runs)

	assert.Equal(t, 4, pool.workers)
	assert.Same(t, q, pool.queue)
	assert.Same(t, exec, pool.executor)
	assert.Same(t, runs, pool.runSvc)
}
