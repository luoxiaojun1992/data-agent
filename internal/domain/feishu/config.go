package feishu

import "time"

// Config represents a Feishu (Lark) bot integration configuration.
type Config struct {
	ID        string    `json:"id"`
	UserID    string    `json:"user_id"`
	Name      string    `json:"name"`
	AppID     string    `json:"app_id"`
	AppSecret string    `json:"app_secret"`
	ModelID   string    `json:"model_id"`   // optional, empty = use default
	SessionID string    `json:"session_id"` // associated Feishu session
	Enabled   bool      `json:"enabled"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
