package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/role"
)

// RoleHandler handles role management HTTP endpoints.
type RoleHandler struct {
	svc role.Service
}

// NewRoleHandler creates a new RoleHandler.
func NewRoleHandler(svc role.Service) *RoleHandler {
	return &RoleHandler{svc: svc}
}

// RegisterRoleRoutes registers role management routes.
func RegisterRoleRoutes(api *gin.RouterGroup, h *RoleHandler) {
	api.GET("/roles", h.List)
	// main registers permissions at /api/v1/permissions (not /roles/permissions).
	// The frontend permissions tab (UI-089) calls this exact path.
	api.GET("/permissions", h.ListPermissions)
	api.POST("/roles", h.Create)
	api.PUT("/roles/:id", h.Update)
	api.DELETE("/roles/:id", h.Delete)
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.svc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Restore main's contract: include total count.
	c.JSON(http.StatusOK, gin.H{"roles": roles, "total": len(roles)})
}

func (h *RoleHandler) ListPermissions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"permissions": h.svc.ListPermissions()})
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req struct {
		Name        string   `json:"name"`
		DisplayName string   `json:"display_name"`
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	role, err := h.svc.Create(c.Request.Context(), req.Name, req.DisplayName, req.Permissions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Restore main's HTTP contract: top-level fields so the frontend can read
	// newRole.id directly (role.spec.ts UI-090/091 rely on this).
	c.JSON(http.StatusCreated, gin.H{
		"id":           role.ID,
		"name":         role.Name,
		"display_name": role.DisplayName,
		"permissions":  role.Permissions,
		"type":         role.Type,
	})
}

func (h *RoleHandler) Update(c *gin.Context) {
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	if err := h.svc.Update(c.Request.Context(), c.Param("id"), req.Permissions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功"})
}

func (h *RoleHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色已删除"})
}
