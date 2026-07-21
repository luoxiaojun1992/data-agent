package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/service/enhance"
)

// EnhanceHandler exposes the prompt-enhancement endpoint.
type EnhanceHandler struct {
	svc *enhance.Service
}

// NewEnhanceHandler creates an enhance handler.
func NewEnhanceHandler(svc *enhance.Service) *EnhanceHandler {
	return &EnhanceHandler{svc: svc}
}

// RegisterEnhanceRoute registers POST /enhance on the given chat router group.
func RegisterEnhanceRoute(rg *gin.RouterGroup, h *EnhanceHandler) {
	rg.POST("/enhance", h.Enhance)
}

// Enhance optimizes a user prompt and returns the enhanced version.
func (h *EnhanceHandler) Enhance(c *gin.Context) {
	var req struct {
		Prompt string `json:"prompt"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Prompt == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	enhanced := h.svc.Enhance(c.Request.Context(), req.Prompt)
	c.JSON(http.StatusOK, gin.H{"enhanced": enhanced})
}
