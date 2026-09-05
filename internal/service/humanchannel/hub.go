// Package humanchannel implements an in-memory human-in-the-loop channel that
// lets a function tool pause and ask the user (confirm an action / answer a
// question) via a session-scoped SSE stream, then resume once the user replies.
//
// The channel is a process-memory construct keyed by session ID — it is never
// persisted. Its lifecycle is bound to the chat session: the frontend opens it
// with a GET SSE stream, and when that stream closes (chat closed / page
// unloaded / network drop) the subscriber is removed and the channel is
// released once it has no pending requests. A tool blocking on Confirm/Ask
// observes the same cancellation through its context and returns immediately.
package humanchannel

import (
	"errors"
	"fmt"
	"sync"
)

// Event is a backend → frontend message pushed on the channel SSE stream.
type Event struct {
	Type      string   `json:"type"`                 // TypeConfirm | TypeAsk
	RequestID string   `json:"request_id"`           // correlates the eventual reply
	Hint      string   `json:"hint,omitempty"`       // confirm: what is about to happen
	Question  string   `json:"question,omitempty"`   // ask: the question text
	Options   []string `json:"options,omitempty"`    // ask: candidate answers (optional)
}

// Reply is a frontend → backend message injected via the reply API.
type Reply struct {
	Confirmed bool   `json:"confirmed,omitempty"` // confirm reply
	Answer    string `json:"answer,omitempty"`    // ask reply (option or free text)
}

// Event types.
const (
	TypeConfirm = "confirm"
	TypeAsk     = "ask"
)

// Sentinel errors. Callers (file_delete/ask_user) translate ErrNoChannel /
// ErrNoSubscriber into a "deny" decision — there is no human to ask.
var (
	// ErrNoChannel means no channel exists for the session (frontend never
	// opened the human-channel SSE, e.g. autonomous task mode).
	ErrNoChannel = errors.New("human channel not established for session")
	// ErrNoSubscriber means the channel exists but has no active subscriber.
	ErrNoSubscriber = errors.New("no active subscriber on human channel")
	// ErrUnknownRequest means the reply referenced a request_id that is not
	// pending for the session (stale, already answered, or cross-session).
	ErrUnknownRequest = errors.New("unknown request id")
	// ErrCancelled means the tool context was cancelled while waiting (chat
	// SSE closed) — the interaction aborted with no reply.
	ErrCancelled = errors.New("human interaction cancelled")
	// ErrTimeout means the user did not reply within the configured timeout.
	ErrTimeout = errors.New("human interaction timed out")
)

// Channel represents the human-interaction channel for a single session.
type Channel struct {
	sessionID string

	mu      sync.Mutex
	subs    map[string]chan Event   // subscriber ID → buffered event chan
	pending map[string]chan Reply   // request ID → reply chan
	nextSeq int64                   // monotonic id generator (subs + requests)
}

func newChannel(sessionID string) *Channel {
	return &Channel{
		sessionID: sessionID,
		subs:      make(map[string]chan Event),
		pending:   make(map[string]chan Reply),
	}
}

// addSub registers a subscriber and returns its ID and buffered event channel.
func (c *Channel) addSub() (string, chan Event) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.nextSeq++
	subID := fmt.Sprintf("sub_%d", c.nextSeq)
	ch := make(chan Event, 16)
	c.subs[subID] = ch
	return subID, ch
}

// removeSub unregisters a subscriber and reports whether any remain.
func (c *Channel) removeSub(subID string) (hasSubs bool) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.subs, subID)
	return len(c.subs) > 0
}

// request registers a pending request, assigns its request_id, and fans the
// event out to every subscriber. It returns the request id and the reply
// channel the blocking caller will select on. With no subscriber it returns
// ErrNoSubscriber (the caller should treat this as a deny / error).
func (c *Channel) request(ev Event) (string, <-chan Reply, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.subs) == 0 {
		return "", nil, ErrNoSubscriber
	}
	c.nextSeq++
	requestID := fmt.Sprintf("req_%d", c.nextSeq)
	ev.RequestID = requestID
	replyCh := make(chan Reply, 1)
	c.pending[requestID] = replyCh
	for _, ch := range c.subs {
		// Non-blocking send: a slow/stalled subscriber must not deadlock the
		// tool goroutine; it simply misses the event (its connection is dead).
		select {
		case ch <- ev:
		default:
		}
	}
	return requestID, replyCh, nil
}

// reply delivers a user reply to the pending request, if still pending.
func (c *Channel) reply(requestID string, reply Reply) error {
	c.mu.Lock()
	ch, ok := c.pending[requestID]
	if !ok {
		c.mu.Unlock()
		return ErrUnknownRequest
	}
	delete(c.pending, requestID)
	c.mu.Unlock()
	select {
	case ch <- reply:
	default:
	}
	return nil
}

// idle reports whether the channel has no subscribers and no pending requests
// (and is therefore eligible for garbage collection).
func (c *Channel) idle() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.subs) == 0 && len(c.pending) == 0
}

// Hub maintains the sessionID → Channel registry. It is safe for concurrent
// use and is the single coordination point shared by the handler and tools.
type Hub struct {
	mu       sync.RWMutex
	channels map[string]*Channel
}

// NewHub creates an empty channel hub.
func NewHub() *Hub {
	return &Hub{channels: make(map[string]*Channel)}
}

func (h *Hub) getOrCreate(sessionID string) *Channel {
	h.mu.Lock()
	defer h.mu.Unlock()
	ch, ok := h.channels[sessionID]
	if !ok {
		ch = newChannel(sessionID)
		h.channels[sessionID] = ch
	}
	return ch
}

// Subscribe registers a new subscriber on the session's channel and returns
// its ID plus the event channel to consume.
func (h *Hub) Subscribe(sessionID string) (string, <-chan Event) {
	return h.getOrCreate(sessionID).addSub()
}

// Unsubscribe removes a subscriber. When the channel ends up idle (no
// subscribers and no pending requests) it is released from the registry.
func (h *Hub) Unsubscribe(sessionID, subID string) {
	h.mu.RLock()
	ch, ok := h.channels[sessionID]
	h.mu.RUnlock()
	if !ok {
		return
	}
	if ch.removeSub(subID) {
		return
	}
	if !ch.idle() {
		return
	}
	h.mu.Lock()
	// Double-check under the write lock to avoid racing a concurrent Subscribe.
	if cur, ok := h.channels[sessionID]; ok && cur.idle() {
		delete(h.channels, sessionID)
	}
	h.mu.Unlock()
}

// Request registers a pending request on the session's channel and fans the
// event out to subscribers. It returns the request id and the reply channel.
func (h *Hub) Request(sessionID string, ev Event) (string, <-chan Reply, error) {
	h.mu.RLock()
	ch, ok := h.channels[sessionID]
	h.mu.RUnlock()
	if !ok {
		return "", nil, ErrNoChannel
	}
	return ch.request(ev)
}

// Reply delivers a user reply to the session's pending request.
func (h *Hub) Reply(sessionID, requestID string, reply Reply) error {
	h.mu.RLock()
	ch, ok := h.channels[sessionID]
	h.mu.RUnlock()
	if !ok {
		return ErrNoChannel
	}
	return ch.reply(requestID, reply)
}
