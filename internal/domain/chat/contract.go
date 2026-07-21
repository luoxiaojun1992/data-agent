// Package chat defines the domain contracts and entities for the chat
// subsystem. Service implementations depend on these contracts; the
// orchestration layer (internal/logic/agent) depends on the contracts
// rather than on concrete services, eliminating same-layer service
// dependencies.
package chat

import (
	"context"
	"net/http"
	"time"
)

// Message represents a single chat message in a request payload.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ChatRequest is the domain-level chat request DTO. Handlers translate
// gin/HTTP input into this struct; the chat service consumes it without
// any web-framework coupling.
type ChatRequest struct {
	SessionID string    `json:"session_id,omitempty"`
	Model     string    `json:"model,omitempty"`
	Messages  []Message `json:"messages"`
	Message   string    `json:"message,omitempty"` // legacy single-message field from frontend
	Stream    bool      `json:"stream"`
	KBID      string    `json:"kb_id,omitempty"`
}

// ChatResponse is the domain-level non-streaming chat response DTO.
type ChatResponse struct {
	SessionID string       `json:"session_id"`
	Content   string       `json:"content"`
	Usage     map[string]int `json:"usage"`
}

// ChatService is the domain contract for chat processing. The service
// implementation must not depend on gin or any web framework; it takes
// a context plus domain request/user identity and returns a domain
// response. Streaming is handled via Stream which writes SSE events to
// an http.ResponseWriter (a net/http type, not gin).
//
//go:generate mockery --name ChatService --output ./mocks --outpkg mocks
type ChatService interface {
	// Process handles a non-streaming chat request and returns the
	// final assistant content.
	Process(ctx context.Context, req ChatRequest, userID, role string) (*ChatResponse, error)
	// Stream handles a streaming chat request, writing SSE events to w
	// and flushing as events arrive. It is the streaming counterpart of
	// Process.
	Stream(ctx context.Context, req ChatRequest, userID, role string, w http.ResponseWriter) error
}

// SessionService is the domain contract for session lifecycle management.
// The chat.Manager (service layer) implements this contract.
//
//go:generate mockery --name SessionService --output ./mocks --outpkg mocks
type SessionService interface {
	Create(userID, sessionType string) (*Session, error)
	Get(id string) (*Session, error)
	Renew(id string) error
	Cleanup() (int64, error)
	ListByUser(userID string) ([]*Session, error)
	Delete(id string) error
	Restore(id string) error
	ListDeleted(before time.Time, limit int64) ([]*Session, error)
	SetRecoveryHours(hours int) error
}
