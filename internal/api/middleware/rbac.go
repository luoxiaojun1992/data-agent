package middleware

import (
	"net/http"

	"github.com/gin-gonic/gin"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// RequirePermission checks if the authenticated user has the required RBAC permission.
// For "rbac:manage", only system_admin passes. All other permissions go through the
// RBAC service (user → roles → recursive permissions lookup).
func RequirePermission(svc *rbacsvc.Service, permission string) gin.HandlerFunc {
	return func(c *gin.Context) {
		userID := c.GetString("user_id")
		role, _ := c.Get("role")
		roleStr, _ := role.(string)

		// RBAC management itself is gated by user role attribute.
		if permission == "rbac:manage" {
			if roleStr == "system_admin" {
				c.Next()
				return
			}
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "insufficient role for RBAC management",
				"required": "system_admin",
			})
			return
		}

		// All other permissions: query RBAC service.
		if userID == "" {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{"error": "user not authenticated"})
			return
		}
		if svc == nil {
			// RBAC service not wired (misconfiguration): fail closed instead of
			// panicking on a nil receiver.
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "permission service unavailable"})
			return
		}

		has, err := svc.HasPermission(c.Request.Context(), userID, permission)
		if err != nil {
			c.AbortWithStatusJSON(http.StatusInternalServerError, gin.H{"error": "permission check failed"})
			return
		}
		if !has {
			c.AbortWithStatusJSON(http.StatusForbidden, gin.H{
				"error":    "insufficient permissions",
				"required": permission,
			})
			return
		}

		c.Next()
	}
}
