// Package chat defines the domain contracts and entities for the chat
// subsystem. Service implementations depend on these contracts; the
// orchestration layer (internal/logic/agent) depends on the contracts
// rather than on concrete services, eliminating same-layer service
// dependencies.
package chat

import (
	"context"
	"net/http"
)

// ImagePart is a base64-encoded image attachment carried with a chat message.
type ImagePart struct {
	// Data is the raw base64-encoded image bytes (no data-URL prefix).
	Data string `json:"data"`
	// MimeType is the image MIME type, e.g. "image/png".
	MimeType string `json:"mime_type"`
}

// PdfAttachment carries a PDF attachment's filename and its parsed text
// (SPEC-077). Name is used by the frontend to render a 📄 card; Text holds the
// parsed text that the service prepends to the user prompt (never rendered).
type PdfAttachment struct {
	Name string `json:"name"`
	Text string `json:"text"`
}

// MaxChatTextBytes is the merged text limit (user prompt + PDF parsed text,
// UTF-8 bytes) — the single source of truth for chat text size (SPEC-077 §4.3).
const MaxChatTextBytes = 100 * 1024

// Message represents a single chat message in a request payload.
type Message struct {
	Role    string      `json:"role"`
	Content string      `json:"content"`
	Images  []ImagePart `json:"images,omitempty"`
}

// ChatEvent is the canonical wire representation of one persisted or streamed
// conversation event. The SSE stream and GET /sessions/:id/messages both use
// this shape so the live transcript and a reloaded session cannot drift.
type ChatEvent struct {
	EventID   string         `json:"event_id,omitempty"`
	Role      string         `json:"role"`
	Content   string         `json:"content,omitempty"`
	Type      string         `json:"type"` // text | tool_call | tool_result
	Name      string         `json:"name,omitempty"`
	Args      map[string]any `json:"args,omitempty"`
	Result    any            `json:"result,omitempty"`
	Images    []string       `json:"images,omitempty"` // image data URLs, attached to the text event of the same message
	Timestamp string         `json:"timestamp"`
	// Hidden marks internal hint events ([intent]/[plan_hint]) that the
	// frontend must not render (SPEC-080). Present only when true.
	Hidden bool `json:"hidden,omitempty"`
}

// ChatRequest is the domain-level chat request DTO. Handlers translate
// gin/HTTP input into this struct; the chat service consumes it without
// any web-framework coupling.
type ChatRequest struct {
	SessionID string    `json:"session_id,omitempty"`
	Model     string    `json:"model,omitempty"`
	Messages  []Message `json:"messages"`
	Message   string    `json:"message,omitempty"` // legacy single-message field from frontend
	// Images carries image attachments alongside the legacy single Message
	// field. Each entry is a base64-encoded image (see ImagePart).
	Images []ImagePart `json:"images,omitempty"`
	// Pdfs carries PDF attachments (SPEC-077): parsed text prepended to the
	// user prompt, name used only for frontend rendering. Backward compatible —
	// empty means no PDF attachments and behavior is unchanged.
	Pdfs   []PdfAttachment `json:"pdfs,omitempty"`
	Stream bool            `json:"stream"`
	KBID   string          `json:"kb_id,omitempty"`
}

// ChatResponse is the domain-level non-streaming chat response DTO.
type ChatResponse struct {
	SessionID string         `json:"session_id"`
	Content   string         `json:"content"`
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

// SessionHistoryStore is the narrow contract the session service depends on to
// hard-delete and clear the ADK-side chat history (adk_sessions + session_events
// + sub sessions) without coupling to the concrete google.golang.org/adk/session
// type. infra adapts adk/session.Service to this contract (SPEC-090 §5.2.3).
//
//go:generate mockery --name SessionHistoryStore --output ./mocks --outpkg mocks
type SessionHistoryStore interface {
	// Delete hard-deletes a session's ADK record, its raw event stream and any
	// bound sub-agent sessions (cascade). Ownership is validated upstream.
	Delete(ctx context.Context, sessionID string) error
	// ClearHistory wipes a session's chat history (compacted events + raw
	// session_events) while keeping the session document and its state.
	ClearHistory(ctx context.Context, sessionID string) error
}

// SessionService is the domain contract for session lifecycle management.
// The chat.Manager (service layer) implements this contract.
//
//go:generate mockery --name SessionService --output ./mocks --outpkg mocks
type SessionService interface {
	// Create creates a new session bound to the given model ID. modelID is
	// the ModelEntry.ID to bind (empty = resolved to default by the caller
	// before persistence; the DB always stores a concrete ID). Once bound,
	// a session's model cannot be changed.
	Create(userID, sessionType, modelID string) (*Session, error)
	// CreateTaskSession creates a session flagged as an autonomous task run.
	// The flag tells downstream consumers (chat UI, stats) this is not a
	// real-time user conversation.
	CreateTaskSession(userID, modelID string) (*Session, error)
	// CreateFeishuSession creates a session flagged for Feishu IM bot integration.
	CreateFeishuSession(userID, modelID string) (*Session, error)
	Get(id string) (*Session, error)
	Renew(id string) error
	Cleanup() (int64, error)
	ListByUser(userID string) ([]*Session, error)
	// ListByUserPaged returns paginated sessions. q filters by title/id at the
	// DB layer (SPEC-075); empty = no filter.
	ListByUserPaged(userID string, q string, page, pageSize int) ([]*Session, int64, error)
	// Delete archives (soft-deletes) a session: sets deleted_at, keeps
	// workspace + chat history, no TTL auto-delete (SPEC-090).
	Delete(id string) error
	// HardDelete permanently removes a session, its workspace, chat history and
	// sub sessions, but never artifact/memory (SPEC-090).
	HardDelete(id string) error
	// ClearHistory wipes a session's chat history, keeping the session and its
	// workspace (SPEC-090).
	ClearHistory(id string) error
	Restore(id string) error
	// ListDeleted returns the current user's archived sessions (SPEC-090).
	ListDeleted(userID string) ([]*Session, error)
	SetRecoveryHours(hours int) error
	// SetTitle updates the session title (first user prompt snippet).
	SetTitle(id, title string) error
}
