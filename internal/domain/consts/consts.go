// Package consts defines shared string constants for paths, error messages,
// and other frequently used literals to satisfy Sonar "duplicate literal" rules.
package consts

// ═══════════════════════════════════════════════
// Route path constants
// ═══════════════════════════════════════════════
const (
	PathRegister = "/register"
	PathUserByID = "/users/:id"
	PathUsers    = "/users"
)

// ═══════════════════════════════════════════════
// Error message constants
// ═══════════════════════════════════════════════
const (
	ErrUserNotFound  = "user not found"
	ErrInvalidReq    = "invalid request"
	ErrDBUnavailable = "database not available"
)

// ═══════════════════════════════════════════════
// Namespace constants
// ═══════════════════════════════════════════════
const (
	DataAgentNS = "data-agent"
)
