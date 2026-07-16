package chat

import (
	"testing"
)

func TestNewService(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	if s == nil {
		t.Fatal("NewService() should not return nil")
	}
}

func TestNewService_InitializesContextManager(t *testing.T) {
	s := NewService(nil, nil, nil, nil)
	if s.context == nil {
		t.Error("context should be initialized by NewService")
	}
}

func TestChatRequest_Defaults(t *testing.T) {
	req := ChatRequest{
		Message: "Hello",
		Stream:  true,
	}
	if req.Message != "Hello" {
		t.Errorf("Message = %q, want %q", req.Message, "Hello")
	}
	if !req.Stream {
		t.Error("Stream should be true")
	}
}

func TestChatRequest_EmptyDefaults(t *testing.T) {
	req := ChatRequest{}
	if req.SessionID != "" {
		t.Error("SessionID should be empty by default")
	}
	if req.Stream {
		t.Error("Stream should be false by default")
	}
}

func TestChatRequest_WithSessionID(t *testing.T) {
	req := ChatRequest{
		SessionID: "sess-123",
		Model:     "gpt-4",
	}
	if req.SessionID != "sess-123" {
		t.Errorf("SessionID = %q, want %q", req.SessionID, "sess-123")
	}
	if req.Model != "gpt-4" {
		t.Errorf("Model = %q, want %q", req.Model, "gpt-4")
	}
}
