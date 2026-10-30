package chat

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"

	chatmocks "github.com/luoxiaojun1992/data-agent/internal/domain/chat/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// CreateTaskSession / CreateFeishuSession / ListByUserPaged are the session
// factory/variant methods with real flag propagation and pagination logic that
// the existing Manager tests do not cover.

func TestManager_CreateTaskSession(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	s, err := m.CreateTaskSession("user1", "m1")
	if err != nil {
		t.Fatalf("CreateTaskSession: %v", err)
	}
	if !s.IsTask {
		t.Error("task session should set IsTask=true")
	}
	if s.Type != "task" {
		t.Errorf("Type = %q, want task", s.Type)
	}
	if s.ModelID != "m1" {
		t.Errorf("ModelID = %q, want m1", s.ModelID)
	}

	t.Run("db error", func(t *testing.T) {
		m2, repo2 := newTestManager(t)
		repo2.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))
		if _, err := m2.CreateTaskSession("u1", "m1"); err == nil {
			t.Error("expected db error")
		}
	})
}

func TestManager_CreateFeishuSession(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	s, err := m.CreateFeishuSession("user1", "m1")
	if err != nil {
		t.Fatalf("CreateFeishuSession: %v", err)
	}
	if !s.IsFeishu {
		t.Error("feishu session should set IsFeishu=true")
	}
	if s.Type != "chat" {
		t.Errorf("Type = %q, want chat", s.Type)
	}

	t.Run("db error", func(t *testing.T) {
		m2, repo2 := newTestManager(t)
		repo2.On("Create", mock.Anything, mock.Anything).Return(fmt.Errorf("db down"))
		if _, err := m2.CreateFeishuSession("u1", "m1"); err == nil {
			t.Error("expected db error")
		}
	})
}

func TestManager_ListByUserPaged(t *testing.T) {
	m, repo := newTestManager(t)
	// page=2, pageSize=10 → skip=(2-1)*10=10, limit=10
	repo.On("ListByUserPaged", mock.Anything, "user1", "q", int64(10), int64(10)).Return(
		[]*repository.SessionRecord{{ID: "s1"}}, int64(1), nil,
	)

	sessions, total, err := m.ListByUserPaged("user1", "q", 2, 10)
	if err != nil {
		t.Fatalf("ListByUserPaged: %v", err)
	}
	if len(sessions) != 1 || total != 1 {
		t.Errorf("sessions=%d total=%d, want 1/1", len(sessions), total)
	}
	repo.AssertCalled(t, "ListByUserPaged", mock.Anything, "user1", "q", int64(10), int64(10))
}

func TestManager_ListByUserPaged_Clamps(t *testing.T) {
	m, repo := newTestManager(t)
	// page<1 → 1; pageSize<1 → 20 → skip=0, limit=20
	repo.On("ListByUserPaged", mock.Anything, "user1", "", int64(0), int64(20)).Return(
		[]*repository.SessionRecord{}, int64(0), nil,
	)
	if _, _, err := m.ListByUserPaged("user1", "", 0, 0); err != nil {
		t.Fatalf("ListByUserPaged: %v", err)
	}
	repo.AssertCalled(t, "ListByUserPaged", mock.Anything, "user1", "", int64(0), int64(20))
}

func TestManager_ListByUserPaged_Error(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("ListByUserPaged", mock.Anything, "user1", "", int64(0), int64(20)).Return(
		([]*repository.SessionRecord)(nil), int64(0), fmt.Errorf("db down"),
	)
	if _, _, err := m.ListByUserPaged("user1", "", 0, 0); err == nil {
		t.Error("expected error from repo")
	}
}

func TestManager_ClearHistory_NilStore(t *testing.T) {
	m, _ := newTestManager(t) // historyStore == nil
	if err := m.ClearHistory("s1"); err != nil {
		t.Errorf("ClearHistory with nil store should be a no-op, got %v", err)
	}
}

func TestManager_ClearHistory_WithStore(t *testing.T) {
	m, _ := newTestManager(t)
	store := chatmocks.NewSessionHistoryStore(t)
	m.historyStore = store

	store.On("ClearHistory", mock.Anything, "s1").Return(nil)
	if err := m.ClearHistory("s1"); err != nil {
		t.Errorf("ClearHistory: %v", err)
	}
	store.AssertCalled(t, "ClearHistory", mock.Anything, "s1")

	store.On("ClearHistory", mock.Anything, "s2").Return(fmt.Errorf("history db down"))
	if err := m.ClearHistory("s2"); err == nil {
		t.Error("expected error propagated from history store")
	}
}

func TestManager_Cleanup_ListError(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("ListExpired", mock.Anything, mock.Anything).Return(
		([]*repository.SessionRecord)(nil), fmt.Errorf("db down"),
	)
	deleted, err := m.Cleanup()
	if err == nil {
		t.Error("expected error from ListExpired")
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0", deleted)
	}
}

func TestManager_Cleanup_HardDeleteFailure(t *testing.T) {
	m, repo := newTestManager(t)
	repo.On("ListExpired", mock.Anything, mock.Anything).Return(
		[]*repository.SessionRecord{{ID: "s1"}}, nil,
	)
	repo.On("HardDelete", mock.Anything, "s1").Return(fmt.Errorf("delete failed"))

	// A failing HardDelete is logged and skipped; Cleanup still returns nil.
	deleted, err := m.Cleanup()
	if err != nil {
		t.Fatalf("Cleanup: %v", err)
	}
	if deleted != 0 {
		t.Errorf("deleted = %d, want 0 (delete failed)", deleted)
	}
}

var _ = context.Background
