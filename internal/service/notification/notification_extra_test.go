package notification

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestSend_CreateError(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("db down"))
	if _, err := NewService(repo).Send("t", "c", "info", []string{"u1"}); err == nil {
		t.Fatal("expected error when Create fails")
	}
}

func TestBroadcast_CreateError(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(errors.New("db down"))
	if _, err := NewService(repo).Broadcast("t", "c", "warning"); err == nil {
		t.Fatal("expected error when Create fails")
	}
}

func TestListForUser_Error(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("ListForUser", mock.Anything, "user1", int64(0), int64(50)).Return(
		([]*model.Notification)(nil), int64(0), errors.New("db down"),
	)
	if _, err := NewService(repo).ListForUser("user1", 50); err == nil {
		t.Fatal("expected error when ListForUser fails")
	}
}

func TestListForUser_SkipsNilElements(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("ListForUser", mock.Anything, "user1", int64(0), int64(50)).Return(
		[]*model.Notification{nil, {Title: "N1"}, nil}, int64(3), nil,
	)
	ns, err := NewService(repo).ListForUser("user1", 50)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(ns) != 3 {
		t.Fatalf("got %d elements, want 3 (nils preserved as zero values)", len(ns))
	}
	if ns[1].Title != "N1" {
		t.Errorf("ns[1].Title = %q, want N1", ns[1].Title)
	}
}
