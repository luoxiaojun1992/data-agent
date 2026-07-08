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

// User represents a system user.
type User struct {
	ID              primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Username        string             `bson:"username" json:"username"`
	PasswordHash    string             `bson:"password_hash" json:"-"`
	Role            UserRole           `bson:"role" json:"role"`
	PasswordChanged bool               `bson:"password_changed" json:"password_changed"`
	CreatedAt       time.Time          `bson:"created_at" json:"created_at"`
	UpdatedAt       time.Time          `bson:"updated_at" json:"updated_at"`
}

// Role defines permissions for a role.
type Role struct {
	ID          primitive.ObjectID `bson:"_id,omitempty" json:"id"`
	Name        UserRole           `bson:"name" json:"name"`
	Permissions []string           `bson:"permissions" json:"permissions"`
	CreatedAt   time.Time          `bson:"created_at" json:"created_at"`
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
