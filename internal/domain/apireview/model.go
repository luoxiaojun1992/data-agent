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
	ID         string    `bson:"_id" json:"id"`
	Name       string    `bson:"name" json:"name"`
	FileName   string    `bson:"file_name" json:"file_name"`
	Version    string    `bson:"version" json:"version"` // "3.0"
	Endpoints  int       `bson:"endpoints" json:"endpoints"`
	Domain     string    `bson:"domain" json:"domain"`
	RateLimit  int       `bson:"rate_limit" json:"rate_limit"` // requests/min
	Submitter  string    `bson:"submitter" json:"submitter"`
	Reviewer   string    `bson:"reviewer,omitempty" json:"reviewer,omitempty"`
	RejectReason string  `bson:"reject_reason,omitempty" json:"reject_reason,omitempty"`
	Status     Status    `bson:"status" json:"status"`
	ReviewedAt *time.Time `bson:"reviewed_at,omitempty" json:"reviewed_at,omitempty"`
	CreatedAt  time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt  time.Time `bson:"updated_at" json:"updated_at"`
}

const collName = "api_reviews"
