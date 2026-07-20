package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/config"
	"github.com/luoxiaojun1992/data-agent/internal/service/role"
)

// ConfigHandler handles system configuration and role management endpoints.
type ConfigHandler struct {
	cfgSvc  config.Service
	roleSvc role.Service
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(cfgSvc config.Service, roleSvc role.Service) *ConfigHandler {
	return &ConfigHandler{cfgSvc: cfgSvc, roleSvc: roleSvc}
}

// RegisterSysConfigRoutes registers system configuration routes.
// Role routes are registered separately via RegisterRoleRoutes.
func RegisterSysConfigRoutes(admin *gin.RouterGroup, h *ConfigHandler) {
	admin.GET("/sysconfig/:namespace", h.Get)
	admin.PUT("/sysconfig/:namespace", h.Put)
	admin.POST("/change-password", h.ChangePassword)
}

func (h *ConfigHandler) Get(c *gin.Context) {
	cfgs, err := h.cfgSvc.GetAll(c.Request.Context(), c.Param("namespace"))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configs": cfgs})
}

func (h *ConfigHandler) Put(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.cfgSvc.Upsert(c.Request.Context(), c.Param("namespace"), req.Key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已保存"})
}

func (h *ConfigHandler) ChangePassword(c *gin.Context) {
	c.JSON(http.StatusNotImplemented, gin.H{"message": "use auth service"})
}

func (h *ConfigHandler) ListRoles(c *gin.Context) {
	roles, err := h.roleSvc.List(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"roles": roles})
}

func (h *ConfigHandler) CreateRole(c *gin.Context) {
	var req struct {
		Name        string   `json:"name"`
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	r, err := h.roleSvc.Create(c.Request.Context(), req.Name, req.Permissions)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, gin.H{"role": r})
}

func (h *ConfigHandler) UpdateRole(c *gin.Context) {
	var req struct {
		Permissions []string `json:"permissions"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.roleSvc.Update(c.Request.Context(), c.Param("id"), req.Permissions); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已更新"})
}

func (h *ConfigHandler) DeleteRole(c *gin.Context) {
	if err := h.roleSvc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}
