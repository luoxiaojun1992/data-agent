package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	"github.com/luoxiaojun1992/data-agent/internal/service/config"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// ConfigHandler handles system configuration and password management endpoints.
type ConfigHandler struct {
	cfgSvc   config.Service
	userRepo repository.UserRepository
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(cfgSvc config.Service, userRepo repository.UserRepository) *ConfigHandler {
	return &ConfigHandler{cfgSvc: cfgSvc, userRepo: userRepo}
}

// sysConfigRoutePath is the route path for system configuration endpoints.
const sysConfigRoutePath = "/sysconfig"

// RegisterSysConfigRoutes registers system configuration routes.
// Sysconfig write operations require PermSystemEdit.
func RegisterSysConfigRoutes(admin *gin.RouterGroup, h *ConfigHandler, rbacSvc *rbacsvc.Service) {
	admin.GET(sysConfigRoutePath, middleware.RequirePermission(rbacSvc, model.PermSystemEdit), h.Get)
	admin.PUT(sysConfigRoutePath, middleware.RequirePermission(rbacSvc, model.PermSystemEdit), h.Put)
	admin.DELETE(sysConfigRoutePath, middleware.RequirePermission(rbacSvc, model.PermSystemEdit), h.Delete)
	// /sysconfig/system — the new /admin/settings UI hard-codes this path
	// (added to fix a long-standing 404 orphan). Same handler, same payload.
	// Kept in addition to /sysconfig so legacy callers (admin/sysconfig page)
	// continue to work unchanged.
	admin.GET(sysConfigRoutePath+"/system", middleware.RequirePermission(rbacSvc, model.PermSystemEdit), h.Get)
	admin.PUT(sysConfigRoutePath+"/system", middleware.RequirePermission(rbacSvc, model.PermSystemEdit), h.Put)
	admin.POST("/change-password", h.ChangePassword)
}

func (h *ConfigHandler) Get(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	cfgs, total, err := h.cfgSvc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configs": cfgs, "total": total, "page": page, "page_size": pageSize})
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
	if err := h.cfgSvc.Upsert(c.Request.Context(), req.Key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已保存"})
}

// Delete removes a config entry by key. Idempotent — returns 200
// even if the entry does not exist.
func (h *ConfigHandler) Delete(c *gin.Context) {
	var req struct {
		Key string `json:"key"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.cfgSvc.Delete(c.Request.Context(), req.Key); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除"})
}

func validatePasswordComplexity(pw string) bool {
	hasUpper, hasLower, hasDigit := false, false, false
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
