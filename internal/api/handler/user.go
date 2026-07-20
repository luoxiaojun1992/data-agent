package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// UserHandler handles user management HTTP endpoints.
type UserHandler struct {
	repo repository.UserRepository
}

// NewUserHandler creates a new UserHandler.
func NewUserHandler(repo repository.UserRepository) *UserHandler {
	return &UserHandler{repo: repo}
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

	users, total, err := h.repo.ListSorted(c.Request.Context(), role, skip, pageSize, sortBy, sortOrder)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"users": users, "total": total})
}

// Get returns a single user.
func (h *UserHandler) Get(c *gin.Context) {
	user, err := h.repo.FindByID(c.Request.Context(), c.Param("id"))
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
	existing, _ := h.repo.FindByUsername(c.Request.Context(), req.Username)
	if existing != nil {
		c.JSON(http.StatusConflict, gin.H{"error": "用户名已存在"})
		return
	}
	user := &model.User{
		Username: req.Username,
		Role:     model.UserRole(req.Role),
		Status:   model.StatusEnabled,
	}
	if err := h.repo.Create(c.Request.Context(), user); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建用户失败"})
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
	if err := h.repo.UpdateRole(c.Request.Context(), c.Param("id"), model.UserRole(req.Role)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功"})
}

// ToggleStatus toggles a user between active and disabled.
func (h *UserHandler) ToggleStatus(c *gin.Context) {
	user, err := h.repo.FindByID(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}
	newStatus := model.StatusEnabled
	if user.Status == model.StatusEnabled {
		newStatus = model.StatusDisabled
	}
	if err := h.repo.UpdateStatus(c.Request.Context(), c.Param("id"), newStatus); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新状态失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "状态更新成功"})
}

// Delete deletes a user.
func (h *UserHandler) Delete(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除用户失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "用户已删除"})
}
