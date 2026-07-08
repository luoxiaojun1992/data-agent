package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
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
				"error":       "insufficient permissions",
				"required":    permission,
				"your_role":   roleStr,
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
			"model:config", "system:config", "user:manage_all",
			"kb:manage_all", "audit:view", "password:change",
			"api:convert", "notify:all",
		}
	case "admin":
		return []string{
			"user:manage", "kb:manage_own", "password:change",
			"api:convert", "notify:group",
		}
	case "user":
		return []string{"kb:manage_own", "password:change"}
	default:
		return nil
	}
}

// hasPermission checks if a permission exists in the list.
func hasPermission(permissions []string, target string) bool {
	for _, p := range permissions {
		if p == target {
			return true
		}
	}
	return false
}
