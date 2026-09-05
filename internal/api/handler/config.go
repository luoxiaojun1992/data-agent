package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/service/config"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// ConfigHandler handles system configuration endpoints.
type ConfigHandler struct {
	cfgSvc config.Service
}

// NewConfigHandler creates a new ConfigHandler.
func NewConfigHandler(cfgSvc config.Service) *ConfigHandler {
	return &ConfigHandler{cfgSvc: cfgSvc}
}

// sysConfigRoutePath is the route path for system configuration endpoints.
const sysConfigRoutePath = "/sysconfig"

// RegisterSysConfigRoutes registers system configuration routes.
// Read requires PermSystemView; write requires PermSystemEdit.
func RegisterSysConfigRoutes(admin *gin.RouterGroup, h *ConfigHandler, rbacSvc *rbacsvc.Service) {
	// /sysconfig/system — the /admin/settings UI hard-codes this path.
	// The legacy /sysconfig page was removed; only this sub-path is used.
	admin.GET(sysConfigRoutePath+"/system", middleware.RequirePermission(rbacSvc, model.PermSystemView), h.Get)
	admin.PUT(sysConfigRoutePath+"/system", middleware.RequirePermission(rbacSvc, model.PermSystemEdit), h.Put)
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
		Key         string `json:"key"`
		Value       string `json:"value"`
		Description string `json:"description"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.cfgSvc.Upsert(c.Request.Context(), req.Key, req.Value, req.Description); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已保存"})
}
