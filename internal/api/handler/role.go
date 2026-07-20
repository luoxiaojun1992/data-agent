package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

type RoleHandler struct {
	repo repository.RoleRepository
}

func NewRoleHandler(repo repository.RoleRepository) *RoleHandler {
	return &RoleHandler{repo: repo}
}

func RegisterRoleRoutes(api *gin.RouterGroup, h *RoleHandler) {
	api.GET("/roles", h.List)
	api.GET("/roles/permissions", h.ListPermissions)
	api.POST("/roles", h.Create)
	api.PUT("/roles/:id", h.Update)
	api.DELETE("/roles/:id", h.Delete)
}

func (h *RoleHandler) List(c *gin.Context) {
	roles, err := h.repo.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

func (h *RoleHandler) ListPermissions(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"permissions": model.GetAllPermissions()})
}

func (h *RoleHandler) Create(c *gin.Context) {
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	role := &model.Role{Name: req.Name, Permissions: req.Permissions}
	if err := h.repo.Create(c.Request.Context(), role); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "创建角色失败"})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"role": role})
}

func (h *RoleHandler) Update(c *gin.Context) {
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	if err := h.repo.Update(c.Request.Context(), c.Param("id"), req.Permissions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "更新角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色更新成功"})
}

func (h *RoleHandler) Delete(c *gin.Context) {
	if err := h.repo.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "删除角色失败"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "角色已删除"})
}
