package handler

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	usersvc "github.com/luoxiaojun1992/data-agent/internal/service/user"
)

// UserHandler handles user management HTTP endpoints.
type UserHandler struct {
	svc usersvc.Service
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(svc usersvc.Service) *UserHandler {
	return &UserHandler{svc: svc}
}

// userIDParam is the per-user route segment. Using a named constant avoids
// duplicating the raw string literal (SonarQube "define a constant" gate).
const userIDParam = "/:id"

// RegisterUserRoutes registers user management routes.
func RegisterUserRoutes(api *gin.RouterGroup, h *UserHandler) {
	ug := api.Group("/users")
	ug.GET("", h.List)
	ug.GET(userIDParam, h.Get)
	ug.POST("", h.Create)
	// PUT /:id is the role-update endpoint used by the frontend (handleEdit).
	// /:id/role is kept as an alias for API clients.
	ug.PUT(userIDParam, h.UpdateRole)
	ug.PUT(userIDParam+"/role", h.UpdateRole)
	ug.PUT(userIDParam+"/status", h.ToggleStatus)
	ug.DELETE(userIDParam, h.Delete)
}

// List returns all users.
func (h *UserHandler) List(c *gin.Context) {
	role := c.DefaultQuery("role", "")
	page, _ := strconv.ParseInt(c.DefaultQuery("page", "1"), 10, 64)
	pageSize, _ := strconv.ParseInt(c.DefaultQuery("page_size", "20"), 10, 64)
	skip := (page - 1) * pageSize

	sortBy := c.DefaultQuery("sort_by", "created_at")
	sortOrder := c.DefaultQuery("sort_order", "desc")

	users, total, err := h.svc.List(c.Request.Context(), role, skip, pageSize, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "total": total})
}

// Get returns a single user.
func (h *UserHandler) Get(c *gin.Context) {
	user, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"id":       user.ID.Hex(),
		"username": user.Username,
		"role":     user.Role,
		"status":   user.Status,
	})
}

// Create creates a new user.
func (h *UserHandler) Create(c *gin.Context) {
	var req struct {
		Username string `json:"username"`
		Password string `json:"password"`
		Role     string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	user, err := h.svc.Create(c.Request.Context(), req.Username, req.Password, req.Role)
	if err != nil {
		if errors.Is(err, usersvc.ErrDuplicate) {
			c.JSON(http.StatusConflict, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{
		"id":       user.ID.Hex(),
		"username": user.Username,
		"role":     user.Role,
		"status":   user.Status,
	})
}

// UpdateRole updates a user's role.
func (h *UserHandler) UpdateRole(c *gin.Context) {
	var req struct {
		Role model.UserRole `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	id := c.Param("id")
	user, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	if user.Role == model.RoleSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "不能修改系统管理员的角色"})
		return
	}
	if req.Role != model.RoleSystemAdmin && req.Role != model.RoleAdmin && req.Role != model.RoleUser {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid role"})
		return
	}
	if err := h.svc.UpdateRole(c.Request.Context(), id, req.Role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功"})
}

// ToggleStatus toggles a user between active and disabled.
func (h *UserHandler) ToggleStatus(c *gin.Context) {
	id := c.Param("id")
	user, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	if user.Role == model.RoleSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "不能停用系统管理员"})
		return
	}
	if err := h.svc.ToggleStatus(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "状态更新成功"})
}

// Delete deletes a user.
func (h *UserHandler) Delete(c *gin.Context) {
	id := c.Param("id")
	user, err := h.svc.Get(c.Request.Context(), id)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	if user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	if user.Role == model.RoleSystemAdmin {
		c.JSON(http.StatusForbidden, gin.H{"error": "不可删除系统管理员"})
		return
	}
	if err := h.svc.Delete(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "用户已删除"})
}
