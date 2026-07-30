package chat

import (
	"os"
	"path/filepath"
	"time"
)

// Session represents a user session for Chat or Agent operations.
// It is a pure domain entity (no SDK coupling); persistence mapping
// lives in the infra/repository layer.
type Session struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Type          string     `json:"type"`
	Title         string     `json:"title"`    // first user prompt snippet (auto-populated on first message)
	ModelID       string     `json:"model_id"` // bound model ID (ModelEntry.ID); empty = use default at create time
	Status        string     `json:"status"`
	// IsTask marks sessions created by the async/scheduled task executor.
	// They share ADK chat-session infrastructure but represent autonomous
	// analysis runs rather than real-time user conversations.
	IsTask        bool       `json:"is_task,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	RecoveryUntil *time.Time `json:"recovery_until,omitempty"`
}

// WorkspaceDir returns the isolated temp directory for this session.
// All agent-generated files (pptx, csv, images, etc.) live here.
func (s *Session) WorkspaceDir() string {
	return filepath.Join(os.TempDir(), "data-agent-sessions", s.ID)
}

// EnsureWorkspace creates the session workspace directory if it doesn't exist.
func (s *Session) EnsureWorkspace() error {
	return os.MkdirAll(s.WorkspaceDir(), 0700)
}

// RemoveWorkspace deletes the entire session workspace (best-effort, never errors).
func (s *Session) RemoveWorkspace() {
	_ = os.RemoveAll(s.WorkspaceDir())
}
