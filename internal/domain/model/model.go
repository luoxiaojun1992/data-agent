package model

import (
	"time"
)

// UserRole defines the three-tier role system.
type UserRole string

const (
	RoleSystemAdmin UserRole = "system_admin"
	RoleAdmin       UserRole = "admin"
	RoleUser        UserRole = "user"
)

// UserStatus defines whether a user is enabled or disabled.
type UserStatus string

const (
	StatusEnabled  UserStatus = "enabled"
	StatusDisabled UserStatus = "disabled"
)

// User represents a system user.
type User struct {
	ID              string     `json:"id"`
	Username        string     `json:"username"`
	PasswordHash    string     `json:"-"`
	Role            UserRole   `json:"role"`
	Status          UserStatus `json:"status"`
	PasswordChanged bool       `json:"password_changed"`
	DisplayName     string     `json:"display_name,omitempty"`
	InvitedBy       string     `json:"invited_by,omitempty"`
	InviteID        string     `json:"-"`
	FeishuAppID     string     `json:"feishu_app_id,omitempty"`
	FeishuAppSecret string     `json:"-"`
	CreatedAt       time.Time  `json:"created_at"`
	UpdatedAt       time.Time  `json:"updated_at"`
}

// InviteStatus defines the lifecycle of an invite token.
type InviteStatus string

const (
	InviteStatusPending  InviteStatus = "pending"
	InviteStatusAccepted InviteStatus = "accepted"
	InviteStatusExpired  InviteStatus = "expired"
	InviteStatusRevoked  InviteStatus = "revoked"
)

// Invite represents an invitation to register in the system.
type Invite struct {
	ID         string       `json:"id"`
	InviteID   string       `json:"invite_id"`
	Email      string       `json:"email,omitempty"`
	Role       string       `json:"role"`
	Status     InviteStatus `json:"status"`
	TokenHash  string       `json:"-"`
	CreatedBy  string       `json:"created_by"`
	CreatedAt  time.Time    `json:"created_at"`
	ExpiresAt  time.Time    `json:"expires_at"`
	AcceptedAt *time.Time   `json:"accepted_at,omitempty"`
	AcceptedBy string       `json:"accepted_by,omitempty"`
}

// Role defines permissions for a role.
type Role struct {
	ID          string    `json:"id"`
	Name        string    `json:"name"`
	DisplayName string    `json:"display_name"`
	Description string    `json:"description"`
	Permissions []string  `json:"permissions"`
	Type        string    `json:"type"` // "fixed" or "custom"
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

// PermissionInfo defines display metadata for a permission.
type PermissionInfo struct {
	Key         string `json:"key"`
	Name        string `json:"name"`
	Description string `json:"description"`
}

// Permission constants.
const (
	PermModelConfig    = "model:config"
	PermSystemConfig   = "system:config"
	PermUserManageAll  = "user:manage_all"
	PermUserManage     = "user:manage"
	PermKBManageAll    = "kb:manage_all"
	PermKBManageOwn    = "kb:manage_own"
	PermAuditLogView   = "audit:view"
	PermPasswordChange = "password:change"
	PermAPIConvert     = "api:convert"
	PermNotifyAll      = "notify:all"
	PermNotifyGroup    = "notify:group"
)

// GetDefaultPermissions returns the default permissions for a role.
func GetDefaultPermissions(role UserRole) []string {
	switch role {
	case RoleSystemAdmin:
		return []string{
			PermModelConfig, PermSystemConfig, PermUserManageAll,
			PermKBManageAll, PermAuditLogView, PermPasswordChange,
			PermAPIConvert, PermNotifyAll,
		}
	case RoleAdmin:
		return []string{
			PermUserManage, PermKBManageOwn, PermPasswordChange,
			PermAPIConvert, PermNotifyGroup, PermAuditLogView,
		}
	case RoleUser:
		return []string{
			PermKBManageOwn, PermPasswordChange,
		}
	default:
		return nil
	}
}

// AuditLog represents an audit log entry.
type AuditLog struct {
	ID         string    `json:"id"`
	Action     string    `json:"action"`
	UserID     string    `json:"user_id"`
	Resource   string    `json:"resource"`
	Details    string    `json:"details"`
	IP         string    `json:"ip"`
	UserAgent  string    `json:"user_agent"`
	StatusCode int       `json:"status_code"`
	CreatedAt  time.Time `json:"created_at"`
}

// Notification represents a system notification.
type Notification struct {
	ID        string    `json:"id"`
	Title     string    `json:"title"`
	Content   string    `json:"content"`
	Type      string    `json:"type"` // "info", "warning", "error"
	TargetAll bool      `json:"target_all"`
	TargetIDs []string  `json:"target_ids,omitempty"`
	ReadBy    []string  `json:"read_by,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}

// SystemConfig represents a system-wide configuration entry.
type SystemConfig struct {
	ID        string    `json:"id"`
	Namespace string    `json:"namespace"`
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	UpdatedAt time.Time `json:"updated_at"`
}

const (
	// MongoDB collections
	CollUsers         = "users"
	CollRoles         = "roles"
	CollInvites       = "invites"
	CollAuditLogs     = "audit_logs"
	CollNotifications = "notifications"
	CollSystemConfigs = "system_configs"
)

// GetAllPermissions returns metadata for all defined permissions.
func GetAllPermissions() []PermissionInfo {
	return []PermissionInfo{
		{Key: "user:manage_all", Name: "用户管理（全部）", Description: "管理所有用户，含 system_admin"},
		{Key: "user:manage", Name: "用户管理", Description: "管理普通用户和 admin"},
		{Key: "model:config", Name: "模型配置", Description: "管理 LLM 模型配置与参数"},
		{Key: "system:config", Name: "系统配置", Description: "全局系统参数配置"},
		{Key: "kb:manage_all", Name: "知识库管理（全部）", Description: "管理所有知识库文档"},
		{Key: "kb:manage_own", Name: "知识库管理（个人）", Description: "管理个人知识库文档"},
		{Key: "task:manage", Name: "任务管理", Description: "创建、查看、终止分析任务"},
		{Key: "audit:view", Name: "审计日志查看", Description: "查看系统操作审计记录"},
		{Key: "password:change", Name: "密码修改", Description: "修改个人登录密码"},
		{Key: "api:convert", Name: "API 转换", Description: "将外部 API 转换为 MCP 工具"},
		{Key: "notify:all", Name: "通知管理（全部）", Description: "发送全站通知"},
		{Key: "notify:group", Name: "通知管理（分组）", Description: "发送分组通知"},
	}
}

// FixedRoles returns the predefined fixed role definitions.
// IDs are deterministic strings so repeated upserts are idempotent
// (no duplicate fixed roles on every restart).
func FixedRoles() []Role {
	now := time.Now()
	return []Role{
		{
			ID:          "role_system_admin",
			Name:        "system_admin",
			DisplayName: "系统管理员",
			Description: "全部系统权限",
			Permissions: GetAllPermissionKeys(),
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "role_data_analyst",
			Name:        "data_analyst",
			DisplayName: "数据分析师",
			Description: "发起批量分析、创建定时任务、使用全部 MCP 工具",
			Permissions: []string{PermUserManage, PermModelConfig, PermKBManageOwn, "task:manage", PermAuditLogView, PermPasswordChange, PermAPIConvert},
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "role_kb_admin",
			Name:        "kb_admin",
			DisplayName: "知识管理员",
			Description: "管理知识库文档、审核 API→MCP 转换",
			Permissions: []string{PermKBManageAll, PermAuditLogView, PermAPIConvert, PermPasswordChange},
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          "role_auditor",
			Name:        "auditor",
			DisplayName: "审计员",
			Description: "只读查看所有审计日志",
			Permissions: []string{PermAuditLogView, PermPasswordChange},
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
	}
}

// GetAllPermissionKeys returns all permission key strings.
func GetAllPermissionKeys() []string {
	all := GetAllPermissions()
	keys := make([]string, len(all))
	for i, p := range all {
		keys[i] = p.Key
	}
	return keys
}
