package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/config"
)

// ModelConfigHandler handles model configuration HTTP endpoints.
type ModelConfigHandler struct {
	cfgSvc config.Service
}

// NewModelConfigHandler creates a new ModelConfigHandler.
func NewModelConfigHandler(cfgSvc config.Service) *ModelConfigHandler {
	return &ModelConfigHandler{cfgSvc: cfgSvc}
}

// RegisterModelConfigRoutes registers model config management routes.
func RegisterModelConfigRoutes(api *gin.RouterGroup, h *ModelConfigHandler) {
	api.GET("/models", h.Get)
	api.PUT("/models", h.Put)
}

func (h *ModelConfigHandler) Get(c *gin.Context) {
	cfgs, err := h.cfgSvc.GetAll(c.Request.Context(), "models")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": cfgs})
}

func (h *ModelConfigHandler) Put(c *gin.Context) {
	var req struct {
		Key   string `json:"key"`
		Value string `json:"value"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if err := h.cfgSvc.Upsert(c.Request.Context(), "models", req.Key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已保存"})
}
