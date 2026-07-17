package model

import (
	"time"

	"go.mongodb.org/mongo-driver/bson/primitive"
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
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Username        string             `bson:"username" json:"username"`
	PasswordHash    string             `bson:"password_hash" json:"-"`
	Role            UserRole           `bson:"role" json:"role"`
	Status          UserStatus         `bson:"status" json:"status"`
	PasswordChanged bool               `bson:"password_changed" json:"password_changed"`
	DisplayName     string             `bson:"display_name,omitempty" json:"display_name,omitempty"`
	InvitedBy       string             `bson:"invited_by,omitempty"   json:"invited_by,omitempty"`
	InviteID        string             `bson:"invite_id,omitempty"    json:"-"`
	FeishuAppID     string             `bson:"feishu_app_id,omitempty" json:"feishu_app_id,omitempty"`
	FeishuAppSecret string             `bson:"feishu_app_secret,omitempty" json:"-"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
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
	ID         primitive.ObjectID `bson:"_id,omitempty"    json:"id"`
	InviteID   string             `bson:"invite_id"        json:"invite_id"`
	Email      string             `bson:"email,omitempty"  json:"email,omitempty"`
	Role       string             `bson:"role"             json:"role"`
	Status     InviteStatus       `bson:"status"           json:"status"`
	TokenHash  string             `bson:"token_hash"       json:"-"`
	CreatedBy  string             `bson:"created_by"       json:"created_by"`
	CreatedAt  time.Time          `bson:"created_at"       json:"created_at"`
	ExpiresAt  time.Time          `bson:"expires_at"       json:"expires_at"`
	AcceptedAt *time.Time         `bson:"accepted_at,omitempty" json:"accepted_at,omitempty"`
	AcceptedBy string             `bson:"accepted_by,omitempty" json:"accepted_by,omitempty"`
}

// Role defines permissions for a role.
type Role struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        string             `bson:"name" json:"name"`
	DisplayName string             `bson:"display_name" json:"display_name"`
	Description string             `bson:"description" json:"description"`
	Permissions []string           `bson:"permissions" json:"permissions"`
	Type        string             `bson:"type" json:"type"` // "fixed" or "custom"
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt   time.Time          `bson:"updated_at" json:"updated_at"`
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
	ID         primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Action     string             `bson:"action" json:"action"`
	UserID     string             `bson:"user_id" json:"user_id"`
	Resource   string             `bson:"resource" json:"resource"`
	Details    string             `bson:"details" json:"details"`
	IP         string             `bson:"ip" json:"ip"`
	UserAgent  string             `bson:"user_agent" json:"user_agent"`
	StatusCode int                `bson:"status_code" json:"status_code"`
	CreatedAt  time.Time          `bson:"created_at" json:"created_at"`
}

// Notification represents a system notification.
type Notification struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Title     string             `bson:"title" json:"title"`
	Content   string             `bson:"content" json:"content"`
	Type      string             `bson:"type" json:"type"` // "info", "warning", "error"
	TargetAll bool               `bson:"target_all" json:"target_all"`
	TargetIDs []string           `bson:"target_ids" json:"target_ids,omitempty"`
	ReadBy    []string           `bson:"read_by" json:"read_by,omitempty"`
	CreatedAt time.Time          `bson:"created_at" json:"created_at"`
}

// SystemConfig represents a system-wide configuration entry.
type SystemConfig struct {
	ID        primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Namespace string             `bson:"namespace" json:"namespace"`
	Key       string             `bson:"key" json:"key"`
	Value     string             `bson:"value" json:"value"`
	UpdatedAt time.Time          `bson:"updated_at" json:"updated_at"`
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
func FixedRoles() []Role {
	now := time.Now()
	return []Role{
		{
			ID:          primitive.NewObjectID(),
			Name:        "system_admin",
			DisplayName: "系统管理员",
			Description: "全部系统权限",
			Permissions: GetAllPermissionKeys(),
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          primitive.NewObjectID(),
			Name:        "data_analyst",
			DisplayName: "数据分析师",
			Description: "发起批量分析、创建定时任务、使用全部 MCP 工具",
			Permissions: []string{PermUserManage, PermModelConfig, PermKBManageOwn, "task:manage", PermAuditLogView, PermPasswordChange, PermAPIConvert},
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          primitive.NewObjectID(),
			Name:        "kb_admin",
			DisplayName: "知识管理员",
			Description: "管理知识库文档、审核 API→MCP 转换",
			Permissions: []string{PermKBManageAll, PermAuditLogView, PermAPIConvert, PermPasswordChange},
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          primitive.NewObjectID(),
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
