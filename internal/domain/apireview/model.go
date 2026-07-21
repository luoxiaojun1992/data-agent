package apireview

import "time"

// Status represents the review status.
type Status string

const (
	StatusPending  Status = "pending"
	StatusApproved Status = "approved"
	StatusRejected Status = "rejected"
)

// APIReview represents an API-to-MCP conversion review entry.
type APIReview struct {
	ID           string     `json:"id"`
	Name         string     `json:"name"`
	FileName     string     `json:"file_name"`
	Version      string     `json:"version"` // "3.0"
	Endpoints    int        `json:"endpoints"`
	Domain       string     `json:"domain"`
	RateLimit    int        `json:"rate_limit"` // requests/min
	Submitter    string     `json:"submitter"`
	Reviewer     string     `json:"reviewer,omitempty"`
	RejectReason string     `json:"reject_reason,omitempty"`
	Status       Status     `json:"status"`
	ReviewedAt   *time.Time `json:"reviewed_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
}
