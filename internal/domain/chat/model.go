package chat

import "time"

// Session represents a user session for Chat or Agent operations.
// It is a pure domain entity (no SDK coupling); persistence mapping
// lives in the infra/repository layer.
type Session struct {
	ID            string     `json:"id"`
	UserID        string     `json:"user_id"`
	Type          string     `json:"type"`
	ModelID       string     `json:"model_id"` // bound model ID (ModelEntry.ID); empty = use default at create time
	Status        string     `json:"status"`
	CreatedAt     time.Time  `json:"created_at"`
	UpdatedAt     time.Time  `json:"updated_at"`
	ExpiresAt     time.Time  `json:"expires_at"`
	DeletedAt     *time.Time `json:"deleted_at,omitempty"`
	RecoveryUntil *time.Time `json:"recovery_until,omitempty"`
}
