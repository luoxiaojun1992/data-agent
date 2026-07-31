package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/feishu"
)

type FeishuConfigHandler struct {
	svc *feishu.ConfigService
}

func NewFeishuConfigHandler(svc *feishu.ConfigService) *FeishuConfigHandler {
	return &FeishuConfigHandler{svc: svc}
}

// POST /api/v1/im/feishu/configs
func (h *FeishuConfigHandler) Create(c *gin.Context) {
	var body struct {
		Name      string `json:"name" binding:"required"`
		AppID     string `json:"app_id" binding:"required"`
		AppSecret string `json:"app_secret" binding:"required"`
		ModelID   string `json:"model_id,omitempty"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "name, app_id, app_secret required"})
		return
	}
	userID := c.GetString("user_id")
	cfg, err := h.svc.Create(c.Request.Context(), userID, body.Name, body.AppID, body.AppSecret, body.ModelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, cfg)
}

// GET /api/v1/im/feishu/configs
func (h *FeishuConfigHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	list, total, err := h.svc.ListByUser(c.Request.Context(), userID, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"configs": list, "total": total, "page": page, "page_size": pageSize})
}

// GET /api/v1/im/feishu/configs/:id
func (h *FeishuConfigHandler) Get(c *gin.Context) {
	cfg, err := h.svc.Get(c.Request.Context(), c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	// Mask AppSecret
	cfg.AppSecret = "****"
	c.JSON(http.StatusOK, cfg)
}

// PUT /api/v1/im/feishu/configs/:id
func (h *FeishuConfigHandler) Update(c *gin.Context) {
	var body struct {
		Enabled *bool `json:"enabled" binding:"required"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled required"})
		return
	}
	if err := h.svc.UpdateEnabled(c.Request.Context(), c.Param("id"), *body.Enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "updated"})
}

// DELETE /api/v1/im/feishu/configs/:id
func (h *FeishuConfigHandler) Delete(c *gin.Context) {
	if err := h.svc.Delete(c.Request.Context(), c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "deleted"})
}
