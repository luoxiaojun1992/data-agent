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
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
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
			PermAPIConvert, PermNotifyGroup,
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

const (
	// MongoDB collections
	CollUsers         = "users"
	CollRoles         = "roles"
	CollAuditLogs     = "audit_logs"
	CollNotifications = "notifications"
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
			Permissions: []string{"user:manage", "model:config", "kb:manage_own", "task:manage", "audit:view", "password:change", "api:convert"},
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          primitive.NewObjectID(),
			Name:        "kb_admin",
			DisplayName: "知识管理员",
			Description: "管理知识库文档、审核 API→MCP 转换",
			Permissions: []string{"kb:manage_all", "audit:view", "api:convert", "password:change"},
			Type:        "fixed",
			CreatedAt:   now,
			UpdatedAt:   now,
		},
		{
			ID:          primitive.NewObjectID(),
			Name:        "auditor",
			DisplayName: "审计员",
			Description: "只读查看所有审计日志",
			Permissions: []string{"audit:view", "password:change"},
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
