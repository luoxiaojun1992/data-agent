package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

type ModelConfigHandler struct {
	repo repository.SysConfigRepository
}

func NewModelConfigHandler(repo repository.SysConfigRepository) *ModelConfigHandler {
	return &ModelConfigHandler{repo: repo}
}

func RegisterModelConfigRoutes(api *gin.RouterGroup, h *ModelConfigHandler) {
	api.GET("/models", h.Get)
	api.PUT("/models", h.Put)
}

func (h *ModelConfigHandler) Get(c *gin.Context) {
	cfgs, err := h.repo.GetAll(c.Request.Context(), "models")
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
	if err := h.repo.Upsert(c.Request.Context(), "models", req.Key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已保存"})
}
