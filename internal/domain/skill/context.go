package skill

import "context"

// Skill defines the interface that all skills must implement.
type Skill interface {
	Name() string
	Description() string
	Parameters() []Parameter
	Execute(ctx SkillContext, params map[string]any) (any, error)
	Permissions() []string
	RateLimit() *RateLimitConfig
}

// Parameter defines a skill parameter schema.
type Parameter struct {
	Name        string `json:"name"`
	Type        string `json:"type"`
	Description string `json:"description"`
	Required    bool   `json:"required"`
}

// RateLimitConfig specifies rate limiting for a skill.
type RateLimitConfig struct {
	MaxRequests int `json:"max_requests"`
	WindowSec   int `json:"window_sec"`
}

// SkillContext carries the execution context automatically injected by the engine.
// Skill implementations MUST NOT overwrite SessionID, UserID, or TaskID.
type SkillContext struct {
	context.Context
	SessionID string
	UserID    string
	TaskID    string
	TraceID   string
	Role      string
}

// NewSkillContext creates a SkillContext from the base context and metadata.
func NewSkillContext(ctx context.Context, sessionID, userID, taskID, traceID, role string) SkillContext {
	return SkillContext{
		Context:   ctx,
		SessionID: sessionID,
		UserID:    userID,
		TaskID:    taskID,
		TraceID:   traceID,
		Role:      role,
	}
}
