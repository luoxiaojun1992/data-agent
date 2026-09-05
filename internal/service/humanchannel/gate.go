package humanchannel

import (
	"context"
	"time"
)

// DefaultTimeout is the maximum time a Confirm/Ask blocks waiting for a user
// reply before defaulting to "deny" / "timeout". SPEC-089 decision point D1.
const DefaultTimeout = 5 * time.Minute

// Gate is the blocking human-in-the-loop interface consumed by function tools
// (file_delete / dir_delete / ask_user) and backed by a Hub.
type Gate interface {
	// Confirm blocks until the user confirms or denies the pending action.
	// A missing channel/subscriber, cancellation, or timeout all resolve to
	// false (never perform the action).
	Confirm(ctx context.Context, sessionID, hint string) (bool, error)
	// Ask blocks until the user answers the question (an option or free text).
	Ask(ctx context.Context, sessionID, question string, options []string) (string, error)
}

type gate struct {
	hub     *Hub
	timeout time.Duration
}

// NewGate builds a Gate backed by hub with the given reply timeout. A zero or
// negative timeout falls back to DefaultTimeout (5 minutes).
func NewGate(hub *Hub, timeout time.Duration) Gate {
	if timeout <= 0 {
		timeout = DefaultTimeout
	}
	return &gate{hub: hub, timeout: timeout}
}

func (g *gate) Confirm(ctx context.Context, sessionID, hint string) (bool, error) {
	_, replyCh, err := g.hub.Request(sessionID, Event{Type: TypeConfirm, Hint: hint})
	if err != nil {
		return false, err
	}
	reply, err := g.wait(ctx, replyCh)
	if err != nil {
		return false, err
	}
	return reply.Confirmed, nil
}

func (g *gate) Ask(ctx context.Context, sessionID, question string, options []string) (string, error) {
	_, replyCh, err := g.hub.Request(sessionID, Event{Type: TypeAsk, Question: question, Options: options})
	if err != nil {
		return "", err
	}
	reply, err := g.wait(ctx, replyCh)
	if err != nil {
		return "", err
	}
	return reply.Answer, nil
}

// wait blocks for a reply, or aborts on context cancellation / timeout. The
// context is the tool's agent.ToolContext, which is cancelled when the chat
// SSE stream closes — so a client disconnect immediately unblocks the tool
// with ErrCancelled rather than leaking a goroutine.
func (g *gate) wait(ctx context.Context, replyCh <-chan Reply) (Reply, error) {
	select {
	case reply := <-replyCh:
		return reply, nil
	case <-ctx.Done():
		return Reply{}, ErrCancelled
	case <-time.After(g.timeout):
		return Reply{}, ErrTimeout
	}
}
