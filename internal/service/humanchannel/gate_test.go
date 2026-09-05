package humanchannel

import (
	"context"
	"testing"
	"time"
)

func confirmWithReply(t *testing.T, confirmed bool) (bool, error) {
	h := NewHub()
	g := NewGate(h, 0)
	subID, evCh := h.Subscribe("s1")
	defer h.Unsubscribe("s1", subID)

	type result struct {
		ok  bool
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		ok, err := g.Confirm(context.Background(), "s1", "delete file?")
		resCh <- result{ok, err}
	}()

	ev := <-evCh
	if ev.Type != TypeConfirm {
		t.Fatalf("expected confirm event, got %q", ev.Type)
	}
	if ev.Hint != "delete file?" {
		t.Fatalf("expected hint, got %q", ev.Hint)
	}
	if err := h.Reply("s1", ev.RequestID, Reply{Confirmed: confirmed}); err != nil {
		t.Fatalf("reply error: %v", err)
	}

	select {
	case res := <-resCh:
		return res.ok, res.err
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Confirm")
		return false, nil
	}
}

func TestConfirmApproved2(t *testing.T) {
	ok, err := confirmWithReply(t, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Fatal("expected approved")
	}
}

func TestConfirmDenied(t *testing.T) {
	ok, err := confirmWithReply(t, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Fatal("expected denied")
	}
}

func TestConfirmNoChannel(t *testing.T) {
	g := NewGate(NewHub(), 0)
	ok, err := g.Confirm(context.Background(), "missing", "x")
	if ok {
		t.Fatal("expected false (deny) with no channel")
	}
	if err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel, got %v", err)
	}
}

func TestConfirmNoSubscriber(t *testing.T) {
	h := NewHub()
	g := NewGate(h, 0)
	subID, _ := h.Subscribe("s1")
	_, _, _ = h.Request("s1", Event{Type: TypeConfirm})
	h.Unsubscribe("s1", subID) // channel retained (pending), no subscriber

	ok, err := g.Confirm(context.Background(), "s1", "x")
	if ok {
		t.Fatal("expected false (deny) with no subscriber")
	}
	if err != ErrNoSubscriber {
		t.Fatalf("expected ErrNoSubscriber, got %v", err)
	}
}

func TestConfirmCancelled(t *testing.T) {
	h := NewHub()
	g := NewGate(h, 0)
	subID, evCh := h.Subscribe("s1")
	defer h.Unsubscribe("s1", subID)

	ctx, cancel := context.WithCancel(context.Background())

	errCh := make(chan error, 1)
	go func() {
		_, err := g.Confirm(ctx, "s1", "x")
		errCh <- err
	}()

	<-evCh // consume the pushed event
	cancel()

	select {
	case err := <-errCh:
		if err != ErrCancelled {
			t.Fatalf("expected ErrCancelled, got %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for cancellation")
	}
}

func TestConfirmTimeout(t *testing.T) {
	h := NewHub()
	g := NewGate(h, 50*time.Millisecond)
	subID, _ := h.Subscribe("s1")
	defer h.Unsubscribe("s1", subID)

	start := time.Now()
	ok, err := g.Confirm(context.Background(), "s1", "x")
	if ok {
		t.Fatal("expected false (deny) on timeout")
	}
	if err != ErrTimeout {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
	if elapsed := time.Since(start); elapsed > 2*time.Second {
		t.Fatalf("timeout too slow: %v", elapsed)
	}
}

func askWithReply(t *testing.T, answer string) (string, error) {
	h := NewHub()
	g := NewGate(h, 0)
	subID, evCh := h.Subscribe("s1")
	defer h.Unsubscribe("s1", subID)

	type result struct {
		ans string
		err error
	}
	resCh := make(chan result, 1)
	go func() {
		ans, err := g.Ask(context.Background(), "s1", "维度?", []string{"A", "B"})
		resCh <- result{ans, err}
	}()

	ev := <-evCh
	if ev.Type != TypeAsk {
		t.Fatalf("expected ask event, got %q", ev.Type)
	}
	if ev.Question != "维度?" {
		t.Fatalf("expected question, got %q", ev.Question)
	}
	if len(ev.Options) != 2 {
		t.Fatalf("expected 2 options, got %d", len(ev.Options))
	}
	if err := h.Reply("s1", ev.RequestID, Reply{Answer: answer}); err != nil {
		t.Fatalf("reply error: %v", err)
	}

	select {
	case res := <-resCh:
		return res.ans, res.err
	case <-time.After(2 * time.Second):
		t.Fatal("timeout waiting for Ask")
		return "", nil
	}
}

func TestAskAnswered(t *testing.T) {
	ans, err := askWithReply(t, "按地区")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ans != "按地区" {
		t.Fatalf("expected answer %q, got %q", "按地区", ans)
	}
}

func TestAskNoChannel(t *testing.T) {
	g := NewGate(NewHub(), 0)
	_, err := g.Ask(context.Background(), "missing", "q", nil)
	if err != ErrNoChannel {
		t.Fatalf("expected ErrNoChannel, got %v", err)
	}
}

func TestAskTimeout(t *testing.T) {
	h := NewHub()
	g := NewGate(h, 50*time.Millisecond)
	subID, _ := h.Subscribe("s1")
	defer h.Unsubscribe("s1", subID)

	_, err := g.Ask(context.Background(), "s1", "q", nil)
	if err != ErrTimeout {
		t.Fatalf("expected ErrTimeout, got %v", err)
	}
}
