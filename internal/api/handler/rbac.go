package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

type RBACHandler struct {
	svc *rbac.Service
}

func NewRBACHandler(svc *rbac.Service) *RBACHandler {
	return &RBACHandler{svc: svc}
}

// ── Roles ────────────────────────────────────────────────────────────

func (h *RBACHandler) ListRoles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	roles, total, err := h.svc.ListRoles(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles, "total": total, "page": page, "page_size": pageSize})
}

func (h *RBACHandler) GetRole(c *gin.Context) {
	role, err := h.svc.GetRole(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"role": role})
}

type createRoleReq struct {
	Name        string `json:"name" binding:"required"`
	DisplayName string `json:"display_name" binding:"required"`
	Description string `json:"description"`
	ParentID    string `json:"parent_id"`
}

func (h *RBACHandler) CreateRole(c *gin.Context) {
	var req createRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role, err := h.svc.CreateRole(c.Request.Context(), req.Name, req.DisplayName, req.Description, req.ParentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"role": role})
}

type updateRoleReq struct {
	DisplayName string `json:"display_name"`
	Description string `json:"description"`
	ParentID    string `json:"parent_id"`
}

func (h *RBACHandler) UpdateRole(c *gin.Context) {
	var req updateRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	role, err := h.svc.UpdateRole(c.Request.Context(), c.Param("id"), req.DisplayName, req.Description, req.ParentID)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"role": role})
}

func (h *RBACHandler) DeleteRole(c *gin.Context) {
	if err := h.svc.DeleteRole(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

func (h *RBACHandler) AvailableParents(c *gin.Context) {
	role, err := h.svc.GetRole(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "role not found"})
		return
	}
	parents, err := h.svc.AvailableParents(c.Request.Context(), role.Level)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"parents": parents})
}

// ── Permissions ──────────────────────────────────────────────────────

func (h *RBACHandler) ListPermissions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	perms, total, err := h.svc.ListPermissions(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"permissions": perms, "total": total, "page": page, "page_size": pageSize})
}

func (h *RBACHandler) GetPermission(c *gin.Context) {
	perm, err := h.svc.GetPermission(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "permission not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"permission": perm})
}

func (h *RBACHandler) DeletePermission(c *gin.Context) {
	if err := h.svc.DeletePermission(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusForbidden, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}

// ── Role-Permission Association ──────────────────────────────────────

func (h *RBACHandler) ListRolePermissions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	perms, total, err := h.svc.ListRolePermissions(c.Request.Context(), c.Param("roleId"), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"permissions": perms, "total": total, "page": page, "page_size": pageSize})
}

type addRolePermReq struct {
	PermissionID string `json:"permission_id" binding:"required"`
}

func (h *RBACHandler) AddRolePermission(c *gin.Context) {
	var req addRolePermReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.AddRolePermission(c.Request.Context(), c.Param("roleId"), req.PermissionID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "added"})
}

func (h *RBACHandler) RemoveRolePermission(c *gin.Context) {
	if err := h.svc.RemoveRolePermission(c.Request.Context(), c.Param("roleId"), c.Param("permId")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}

func (h *RBACHandler) EffectivePermissions(c *gin.Context) {
	keys, err := h.svc.GetEffectivePermissions(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if keys == nil {
		keys = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"permissions": keys})
}

// ── My Permissions ───────────────────────────────────────────────────

func (h *RBACHandler) MyPermissions(c *gin.Context) {
	userID := c.GetString("user_id")
	keys, err := h.svc.GetUserPermissionKeys(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if keys == nil {
		keys = []string{}
	}
	c.JSON(http.StatusOK, gin.H{"permissions": keys})
}

// ── User-Role Association ────────────────────────────────────────────

func (h *RBACHandler) ListUserRoles(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	roles, total, err := h.svc.ListUserRoles(c.Request.Context(), c.Param("userId"), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles, "total": total, "page": page, "page_size": pageSize})
}

type addUserRoleReq struct {
	RoleID string `json:"role_id" binding:"required"`
}

func (h *RBACHandler) AddUserRole(c *gin.Context) {
	var req addUserRoleReq
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.svc.AddUserRole(c.Request.Context(), c.Param("userId"), req.RoleID); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"status": "added"})
}

func (h *RBACHandler) RemoveUserRole(c *gin.Context) {
	if err := h.svc.RemoveUserRole(c.Request.Context(), c.Param("userId"), c.Param("roleId")); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "removed"})
}
