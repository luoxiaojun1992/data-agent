package chat

import (
	"testing"
	"time"
)

func TestNewContextManager(t *testing.T) {
	cm := NewContextManager(128000, 0.5)
	if cm == nil {
		t.Error("NewContextManager should not return nil")
	}
}

func TestSessionDefaults(t *testing.T) {
	s := Session{
		ID:     "sess-1",
		UserID: "user-1",
		Type:   "chat",
		Status: "active",
	}
	if s.ID != "sess-1" {
		t.Errorf("ID: got %s", s.ID)
	}
	if s.Status != "active" {
		t.Errorf("Status: got %s", s.Status)
	}
	if s.Type != "chat" {
		t.Errorf("Type: got %s", s.Type)
	}
}

func TestSession_DeletedAt(t *testing.T) {
	now := time.Now()
	s := Session{
		ID:        "sess-1",
		DeletedAt: &now,
	}
	if s.DeletedAt == nil {
		t.Error("DeletedAt should not be nil")
	}
}
