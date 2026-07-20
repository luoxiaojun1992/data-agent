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

// RegisterUserRoutes registers user management routes.
func RegisterUserRoutes(api *gin.RouterGroup, h *UserHandler) {
	api.GET("/users", h.List)
	api.GET("/users/:id", h.Get)
	api.POST("/users", h.Create)
	api.PUT("/users/:id/role", h.UpdateRole)
	api.PUT("/users/:id/status", h.ToggleStatus)
	api.DELETE("/users/:id", h.Delete)
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
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"user": user})
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
	c.JSON(http.StatusCreated, gin.H{"user": user})
}

// UpdateRole updates a user's role.
func (h *UserHandler) UpdateRole(c *gin.Context) {
	var req struct {
		Role string `json:"role"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	if err := h.svc.UpdateRole(c.Request.Context(), c.Param("id"), model.UserRole(req.Role)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功"})
}

// ToggleStatus toggles a user between active and disabled.
func (h *UserHandler) ToggleStatus(c *gin.Context) {
	if err := h.svc.ToggleStatus(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "状态更新成功"})
}

// Delete deletes a user.
func (h *UserHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "用户已删除"})
}
