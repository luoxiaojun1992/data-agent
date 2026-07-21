package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"github.com/luoxiaojun1992/data-agent/internal/service/config"
	"github.com/luoxiaojun1992/data-agent/internal/service/role"
)

// ConfigHandler handles system configuration, password, and role management endpoints.
type ConfigHandler struct {
	cfgSvc   config.Service
	roleSvc  role.Service
	userRepo repository.UserRepository
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(cfgSvc config.Service, roleSvc role.Service, userRepo repository.UserRepository) *ConfigHandler {
	return &ConfigHandler{cfgSvc: cfgSvc, roleSvc: roleSvc, userRepo: userRepo}
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

func validatePasswordComplexity(pw string) bool {
	hasUpper := false
	hasLower := false
	hasDigit := false
	for _, c := range pw {
		switch {
		case c >= 'A' && c <= 'Z':
			hasUpper = true
		case c >= 'a' && c <= 'z':
			hasLower = true
		case c >= '0' && c <= '9':
			hasDigit = true
		}
	}
	return len(pw) >= 8 && hasUpper && hasLower && hasDigit
}

func (h *ConfigHandler) ChangePassword(c *gin.Context) {
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	var req struct {
		OldPassword string `json:"old_password" binding:"required"`
		NewPassword string `json:"new_password" binding:"required"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧密码和新密码不能为空"})
		return
	}
	if !validatePasswordComplexity(req.NewPassword) {
		c.JSON(http.StatusBadRequest, gin.H{"error": "密码至少 8 位，需包含大小写字母和数字"})
		return
	}

	user, err := h.userRepo.FindByID(c.Request.Context(), userIDStr)
	if err != nil || user == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "用户不存在"})
		return
	}

	if middleware.CheckPassword(user.PasswordHash, req.OldPassword) != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "旧密码不正确"})
		return
	}

	newHash, err := middleware.HashPassword(req.NewPassword)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码加密失败"})
		return
	}

	if err := h.userRepo.UpdatePassword(c.Request.Context(), userIDStr, newHash); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "密码更新失败"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "密码修改成功"})
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
