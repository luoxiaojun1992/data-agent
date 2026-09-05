package humanchannel

import (
	"testing"
)

func TestHubRequestNoChannel(t *testing.T) {
	h := NewHub()
	_, _, err := h.Request("s1", Event{Type: TypeConfirm, Hint: "x"})
	if err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel, got %v", err)
	}
}

func TestHubRequestNoSubscriber(t *testing.T) {
	h := NewHub()
	subID, _ := h.Subscribe("s1")
	// First request registers a pending entry, so the channel is NOT released
	// when the subscriber leaves below.
	_, _, err := h.Request("s1", Event{Type: TypeConfirm, Hint: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h.Unsubscribe("s1", subID)

	// Channel still exists (pending), but no subscriber → ErrNoSubscriber.
	if _, _, err := h.Request("s1", Event{Type: TypeConfirm, Hint: "y"}); err != ErrNoSubscriber {
		t.Fatalf("expected ErrNoSubscriber, got %v", err)
	}
}

func TestHubSubscribeUnsubscribeReleasesChannel(t *testing.T) {
	h := NewHub()
	subID, evCh := h.Subscribe("s1")
	if subID == "" {
		t.Fatal("expected non-empty subscriber id")
	}
	if evCh == nil {
		t.Fatal("expected event channel")
	}

	h.mu.RLock()
	_, exists := h.channels["s1"]
	h.mu.RUnlock()
	if !exists {
		t.Fatal("expected channel to exist after subscribe")
	}

	h.Unsubscribe("s1", subID)

	h.mu.RLock()
	_, exists = h.channels["s1"]
	h.mu.RUnlock()
	if exists {
		t.Fatal("expected channel to be released after last subscriber leaves")
	}
}

func TestHubRequestFansOutToSubscriber(t *testing.T) {
	h := NewHub()
	subID, evCh := h.Subscribe("s1")

	requestID, replyCh, err := h.Request("s1", Event{Type: TypeConfirm, Hint: "delete?"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if requestID == "" {
		t.Fatal("expected non-empty request id")
	}
	if replyCh == nil {
		t.Fatal("expected reply channel")
	}

	ev := <-evCh
	if ev.Type != TypeConfirm {
		t.Fatalf("expected confirm event, got %q", ev.Type)
	}
	if ev.Hint != "delete?" {
		t.Fatalf("expected hint %q, got %q", "delete?", ev.Hint)
	}
	if ev.RequestID != requestID {
		t.Fatalf("expected request id %q, got %q", requestID, ev.RequestID)
	}

	h.Unsubscribe("s1", subID)
}

func TestHubReplyDelivers(t *testing.T) {
	h := NewHub()
	subID, _ := h.Subscribe("s1")
	requestID, replyCh, err := h.Request("s1", Event{Type: TypeConfirm, Hint: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if err := h.Reply("s1", requestID, Reply{Confirmed: true}); err != nil {
		t.Fatalf("unexpected reply error: %v", err)
	}
	reply := <-replyCh
	if !reply.Confirmed {
		t.Fatal("expected confirmed=true reply")
	}

	h.Unsubscribe("s1", subID)
}

func TestHubReplyUnknownRequest(t *testing.T) {
	h := NewHub()
	h.Subscribe("s1")
	if err := h.Reply("s1", "req_999", Reply{Confirmed: true}); err != ErrUnknownRequest {
		t.Fatalf("expected ErrUnknownRequest, got %v", err)
	}
}

func TestHubReplyAlreadyAnswered(t *testing.T) {
	h := NewHub()
	subID, _ := h.Subscribe("s1")
	requestID, _, err := h.Request("s1", Event{Type: TypeConfirm, Hint: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := h.Reply("s1", requestID, Reply{Confirmed: true}); err != nil {
		t.Fatalf("unexpected reply error: %v", err)
	}
	// Second reply for the same request id must be rejected (already answered).
	if err := h.Reply("s1", requestID, Reply{Confirmed: false}); err != ErrUnknownRequest {
		t.Fatalf("expected ErrUnknownRequest for double reply, got %v", err)
	}
	h.Unsubscribe("s1", subID)
}

func TestHubReplyNoChannel(t *testing.T) {
	h := NewHub()
	if err := h.Reply("s1", "req_1", Reply{Confirmed: true}); err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel, got %v", err)
	}
}

func TestHubUnsubscribeUnknownSession(t *testing.T) {
	h := NewHub()
	// Unsubscribing a session with no channel must be a safe no-op.
	h.Unsubscribe("missing", "sub_1")
}

func TestHubUnsubscribeKeepsChannelWhenOtherSubs(t *testing.T) {
	h := NewHub()
	sub1, _ := h.Subscribe("s1")
	sub2, _ := h.Subscribe("s1")
	h.Unsubscribe("s1", sub1) // one subscriber remains

	h.mu.RLock()
	ch, exists := h.channels["s1"]
	h.mu.RUnlock()
	if !exists {
		t.Fatal("expected channel to remain with a live subscriber")
	}
	if ch == nil {
		t.Fatal("expected non-nil channel")
	}

	h.Unsubscribe("s1", sub2)
	h.mu.RLock()
	_, exists = h.channels["s1"]
	h.mu.RUnlock()
	if exists {
		t.Fatal("expected channel released after all subscribers leave")
	}
}

func TestHubReplyCrossSessionIsolated(t *testing.T) {
	h := NewHub()
	subID, _ := h.Subscribe("s1")
	requestID, _, err := h.Request("s1", Event{Type: TypeConfirm, Hint: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The pending request belongs to s1; answering via s2 must not reach it.
	if err := h.Reply("s2", requestID, Reply{Confirmed: true}); err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel for cross-session reply, got %v", err)
	}
	h.Unsubscribe("s1", subID)
}
