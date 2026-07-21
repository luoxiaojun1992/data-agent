package notification

import (
	"testing"

	"github.com/stretchr/testify/mock"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
)

func TestNewService(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	s := NewService(repo)
	if s == nil {
		t.Fatal("NewService should not return nil")
	}
}

func TestSend_Success(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	n, err := NewService(repo).Send("Hello", "Body", "info", []string{"user1"})
	if err != nil {
		t.Fatalf("Send: %v", err)
	}
	if n == nil || n.Title != "Hello" {
		t.Errorf("unexpected notification: %+v", n)
	}
}

func TestBroadcast_Success(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("Create", mock.Anything, mock.Anything).Return(nil)

	n, err := NewService(repo).Broadcast("Alert", "Important", "warning")
	if err != nil {
		t.Fatalf("Broadcast: %v", err)
	}
	if n == nil {
		t.Fatal("expected notification")
	}
}

func TestListForUser_Success(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("ListForUser", mock.Anything, "user1", int64(0), int64(50)).Return(
		[]*model.Notification{{Title: "N1"}, {Title: "N2"}}, int64(2), nil,
	)

	ns, err := NewService(repo).ListForUser("user1", 50)
	if err != nil {
		t.Fatalf("ListForUser: %v", err)
	}
	if len(ns) != 2 {
		t.Fatalf("got %d, want 2", len(ns))
	}
}

func TestUnreadCount_Success(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("CountUnread", mock.Anything, "user1").Return(int64(5), nil)

	n, err := NewService(repo).UnreadCount("user1")
	if err != nil {
		t.Fatalf("UnreadCount: %v", err)
	}
	if n != 5 {
		t.Errorf("got %d, want 5", n)
	}
}

func TestMarkRead_Success(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("MarkRead", mock.Anything, "n1", "user1").Return(nil)

	if err := NewService(repo).MarkRead("n1", "user1"); err != nil {
		t.Fatalf("MarkRead: %v", err)
	}
}

func TestMarkAllRead_Success(t *testing.T) {
	repo := mockrepo.NewNotificationRepository(t)
	repo.On("MarkAllRead", mock.Anything, "user1").Return(nil)

	if err := NewService(repo).MarkAllRead("user1"); err != nil {
		t.Fatalf("MarkAllRead: %v", err)
	}
}
