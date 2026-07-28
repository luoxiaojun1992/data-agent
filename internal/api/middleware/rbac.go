package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

// RequirePermission checks if the authenticated user has the required permission.
func RequirePermission(permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		role, exists := c.Get("role")
		if !exists {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "role not found in context"})
			return
		}

		roleStr, ok := role.(string)
		if !ok {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "invalid role type"})
			return
		}

		// Check if the role has the required permission
		perms := getRolePermissions(roleStr)
		if !hasPermission(perms, permission) {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":     "insufficient permissions",
				"required":  permission,
				"your_role": roleStr,
			})
			return
		}

		c.Next()
	}
}

// getRolePermissions returns the permissions for a given role.
// Maps to the RBAC matrix defined in the spec.
func getRolePermissions(role string) []string {
	switch role {
	case "system_admin":
		return []string{
			model.PermModelConfig, model.PermSystemConfig, model.PermUserManageAll,
			model.PermKBManageAll, model.PermAuditLogView, model.PermPasswordChange,
			model.PermAPIConvert, model.PermNotifyAll,
		}
	case "admin":
		return []string{
			model.PermUserManage, model.PermKBManageOwn, model.PermPasswordChange,
			model.PermAPIConvert, model.PermNotifyGroup,
		}
	case "user":
		return []string{model.PermKBManageOwn, model.PermPasswordChange}
	default:
		return nil
	}
}

// hasPermission checks if a permission exists in the list.
// A `_all` suffix grants the corresponding scoped permission too:
// e.g. `user:manage_all` also satisfies a `user:manage` requirement.
func hasPermission(permissions []string, target string) bool {
	for _, p := range permissions {
		if p == target {
			return true
		}
		// _all suffix implies the base permission.
		if strings.HasSuffix(p, "_all") {
			base := strings.TrimSuffix(p, "_all")
			if base == target {
				return true
			}
		}
	}
	return false
}
