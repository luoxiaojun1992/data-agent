package chat

import (
	"context"
	"fmt"
	"net/http/httptest"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	redismocks "github.com/luoxiaojun1992/data-agent/internal/infra/redis/mocks"
)

// ── Manager.Delete archive lock (SPEC-104 D4) ──

// TestManager_Delete_SessionBusy asserts that archiving a session whose chat
// turn holds the lock returns ErrSessionBusy and never touches the repo.
func TestManager_Delete_SessionBusy(t *testing.T) {
	m, repo := newTestManager(t)
	lock := redismocks.NewLocker(t)
	m.WithLocker(lock)

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(false, nil)

	err := m.Delete("s1")
	require.ErrorIs(t, err, domainchat.ErrSessionBusy)
	repo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
	lock.AssertNotCalled(t, "Release", mock.Anything, mock.Anything, mock.Anything)
}

// TestManager_Delete_LockAcquiredAndReleased asserts the happy path acquires
// the archive lock, deletes, and releases on exit.
func TestManager_Delete_LockAcquiredAndReleased(t *testing.T) {
	m, repo := newTestManager(t)
	lock := redismocks.NewLocker(t)
	m.WithLocker(lock)

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(true, nil)
	lock.On("Release", mock.Anything, "lock:session:s1", mock.Anything).Return(nil)
	repo.On("Delete", mock.Anything, "s1").Return(nil)

	require.NoError(t, m.Delete("s1"))
	lock.AssertCalled(t, "Release", mock.Anything, "lock:session:s1", mock.Anything)
}

// TestManager_Delete_LockAcquireErrorDegrades asserts a Redis infra error
// degrades gracefully (archive proceeds unlocked).
func TestManager_Delete_LockAcquireErrorDegrades(t *testing.T) {
	m, repo := newTestManager(t)
	lock := redismocks.NewLocker(t)
	m.WithLocker(lock)

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(false, fmt.Errorf("redis down"))
	repo.On("Delete", mock.Anything, "s1").Return(nil)

	require.NoError(t, m.Delete("s1"))
	repo.AssertCalled(t, "Delete", mock.Anything, "s1")
}

// TestManager_Delete_ReleaseError asserts a release error is non-fatal (the
// archive already succeeded).
func TestManager_Delete_ReleaseError(t *testing.T) {
	m, repo := newTestManager(t)
	lock := redismocks.NewLocker(t)
	m.WithLocker(lock)

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(true, nil)
	lock.On("Release", mock.Anything, "lock:session:s1", mock.Anything).Return(fmt.Errorf("release fail"))
	repo.On("Delete", mock.Anything, "s1").Return(nil)

	require.NoError(t, m.Delete("s1"))
	lock.AssertCalled(t, "Release", mock.Anything, "lock:session:s1", mock.Anything)
}

// ── Service.acquireSessionLock (SPEC-104 D4 use side) ──

// TestAcquireSessionLock_NilLock asserts a nil locker degrades to a no-op
// release func with no error.
func TestAcquireSessionLock_NilLock(t *testing.T) {
	s := &Service{} // lock nil
	unlock, err := s.acquireSessionLock(context.Background(), "s1")
	require.NoError(t, err)
	require.NotNil(t, unlock)
	require.NotPanics(t, unlock)
}

// TestAcquireSessionLock_Conflict asserts a held lock yields ErrSessionBusy.
func TestAcquireSessionLock_Conflict(t *testing.T) {
	lock := redismocks.NewLocker(t)
	s := &Service{lock: lock}

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(false, nil)

	_, err := s.acquireSessionLock(context.Background(), "s1")
	require.ErrorIs(t, err, domainchat.ErrSessionBusy)
}

// TestAcquireSessionLock_InfraErrorDegrades asserts a Redis infra error
// degrades gracefully (turn proceeds unlocked).
func TestAcquireSessionLock_InfraErrorDegrades(t *testing.T) {
	lock := redismocks.NewLocker(t)
	s := &Service{lock: lock}

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(false, fmt.Errorf("redis down"))

	unlock, err := s.acquireSessionLock(context.Background(), "s1")
	require.NoError(t, err)
	require.NotNil(t, unlock)
}

// TestAcquireSessionLock_SuccessAndRelease asserts the success path returns a
// release func that frees the lock with a background context.
func TestAcquireSessionLock_SuccessAndRelease(t *testing.T) {
	lock := redismocks.NewLocker(t)
	s := &Service{lock: lock}

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(true, nil)
	lock.On("Release", mock.Anything, "lock:session:s1", mock.Anything).Return(nil)

	unlock, err := s.acquireSessionLock(context.Background(), "s1")
	require.NoError(t, err)
	require.NotNil(t, unlock)
	require.NotPanics(t, unlock)
	lock.AssertCalled(t, "Release", mock.Anything, "lock:session:s1", mock.Anything)
}

// TestAcquireSessionLock_ReleaseError asserts a release error is non-fatal.
func TestAcquireSessionLock_ReleaseError(t *testing.T) {
	lock := redismocks.NewLocker(t)
	s := &Service{lock: lock}

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(true, nil)
	lock.On("Release", mock.Anything, "lock:session:s1", mock.Anything).Return(fmt.Errorf("release fail"))

	unlock, err := s.acquireSessionLock(context.Background(), "s1")
	require.NoError(t, err)
	require.NotPanics(t, unlock)
}

// ── Process/Stream integration with a held lock ──

// TestProcess_SessionBusy asserts Process short-circuits with ErrSessionBusy
// BEFORE invoking the LLM when the archive lock is already held.
func TestProcess_SessionBusy(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "answer"})
	lock := redismocks.NewLocker(t)
	svc.WithLocker(lock)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(false, nil)

	_, err := svc.Process(context.Background(), domainchat.ChatRequest{Message: "hi"}, "u1", "admin")
	require.ErrorIs(t, err, domainchat.ErrSessionBusy)
}

// TestStream_SessionBusy asserts Stream returns ErrSessionBusy as a clean
// error (before SSE headers are written) when the archive lock is held.
func TestStream_SessionBusy(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "answer"})
	lock := redismocks.NewLocker(t)
	svc.WithLocker(lock)

	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patchSessionCreate(patches, svc, &domainchat.Session{ID: "s1", UserID: "u1"}, nil)

	lock.On("Acquire", mock.Anything, "lock:session:s1", mock.Anything, mock.Anything).Return(false, nil)

	w := httptest.NewRecorder()
	err := svc.Stream(context.Background(), domainchat.ChatRequest{Message: "hi"}, "u1", "admin", w)
	require.ErrorIs(t, err, domainchat.ErrSessionBusy)
	require.Equal(t, "", w.Header().Get("Content-Type"), "no SSE header should be written on a 409")
}
