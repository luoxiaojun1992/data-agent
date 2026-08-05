package model

import "time"

// RBACRole defines a role in the RBAC hierarchy. Level is immutable after creation.
// Max depth: L0(root) → L1 → L2 (max 3 levels).
type RBACRole struct {
	ID              string    `bson:"_id" json:"id"`
	Name            string    `bson:"name" json:"name"`
	DisplayName     string    `bson:"display_name" json:"display_name"`
	Description     string    `bson:"description,omitempty" json:"description,omitempty"`
	ParentID        string    `bson:"parent_id" json:"parent_id"` // "" = root
	Level           int       `bson:"level" json:"level"`         // 0, 1, or 2
	Type            string    `bson:"type" json:"type"`           // "builtin" / "custom"
	ChildCount      int       `bson:"child_count" json:"child_count"`
	PermissionCount int       `bson:"permission_count" json:"permission_count"`
	CreatedAt       time.Time `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time `bson:"updated_at" json:"updated_at"`
}

const (
	RBACRoleTypeBuiltin = "builtin"
	RBACRoleTypeCustom  = "custom"
	MaxRoleLevel        = 2
)

// RBACPermission defines a single permission key.
type RBACPermission struct {
	ID          string    `bson:"_id" json:"id"`
	Key         string    `bson:"key" json:"key"` // "dashboard:view"
	Name        string    `bson:"name" json:"name"`
	Description string    `bson:"description,omitempty" json:"description,omitempty"`
	Module      string    `bson:"module" json:"module"`
	Type        string    `bson:"type" json:"type"` // "builtin" / "custom"
	CreatedAt   time.Time `bson:"created_at" json:"created_at"`
}

const (
	RBACPermTypeBuiltin = "builtin"
	RBACPermTypeCustom  = "custom"
)

// RBACRolePermission links a role to a permission.
type RBACRolePermission struct {
	ID           string    `bson:"_id" json:"id"`
	RoleID       string    `bson:"role_id" json:"role_id"`
	PermissionID string    `bson:"permission_id" json:"permission_id"`
	CreatedAt    time.Time `bson:"created_at" json:"created_at"`
}

// UserRBACRole links a user to an RBAC role.
type UserRBACRole struct {
	ID        string    `bson:"_id" json:"id"`
	UserID    string    `bson:"user_id" json:"user_id"`
	RoleID    string    `bson:"role_id" json:"role_id"`
	CreatedAt time.Time `bson:"created_at" json:"created_at"`
}

// Built-in permission key constants.
const (
	PermDashboardView  = "dashboard:view"
	PermChatSend       = "chat:send"
	PermChatView       = "chat:view"
	PermChatDelete     = "chat:delete"
	PermAgentView      = "agent:view"
	PermAgentCreate    = "agent:create"
	PermAgentDelete    = "agent:delete"
	PermKBView         = "kb:view"
	PermKBUpload       = "kb:upload"
	PermKBDelete       = "kb:delete"
	PermHermesView     = "hermes:view"
	PermArtifactView   = "artifact:view"
	PermArtifactDelete = "artifact:delete"
	PermIMView         = "im:view"
	PermUserView       = "user:view"
	PermUserCreate     = "user:create"
	PermUserEdit       = "user:edit"
	PermUserDelete     = "user:delete"
	PermModelView      = "model:view"
	PermModelEdit      = "model:edit"
	PermTaskView       = "task:view"
	PermTaskCreate     = "task:create"
	PermTaskEdit       = "task:edit"
	PermTaskDelete     = "task:delete"
	PermSystemView     = "system:view"
	PermSystemEdit     = "system:edit"
	PermAuditView      = "audit:view"
	PermInviteView     = "invite:view"
	PermInviteCreate   = "invite:create"
	PermAPIReviewView  = "apireview:view"
	PermSkillsView     = "skills:view"
	PermSkillsEdit     = "skills:edit"
	PermStatsView      = "stats:view"
	PermMemorySearch   = "memory:search"
	PermRBACManage     = "rbac:manage"
)
