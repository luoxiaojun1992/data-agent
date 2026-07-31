package chat

import "time"

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
	// IsFeishu marks sessions created for Feishu (Lark) IM bot integrations.
	// These sessions are bound to a specific FeishuConfig and receive messages
	// from the Feishu websocket connector.
	IsFeishu      bool       `json:"is_feishu,omitempty"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	RecoveryUntil *time.Time `json:"recovery_until,omitempty"`
}
