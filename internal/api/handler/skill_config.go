package handler

import (
	"net/http"

	skillsvc "github.com/luoxiaojun1992/data-agent/internal/service/skill"
	"github.com/gin-gonic/gin"
)

// SkillConfigHandler serves the admin skill configuration API.
type SkillConfigHandler struct {
	svc *skillsvc.ConfigService
}

// NewSkillConfigHandler creates a new handler.
func NewSkillConfigHandler(svc *skillsvc.ConfigService) *SkillConfigHandler {
	return &SkillConfigHandler{svc: svc}
}

// RegisterSkillConfigRoutes registers admin skill config routes.
// Only routes registered; auth middleware is applied by the caller.
func RegisterSkillConfigRoutes(rg *gin.RouterGroup, h *SkillConfigHandler) {
	rg.GET("/skills", h.List)
	rg.GET("/skills/:name", h.Get)
	rg.PUT("/skills/:name", h.Upsert)
}

// List returns all skill configs (predefined defaults merged with saved overrides).
// GET /admin/skills
func (h *SkillConfigHandler) List(c *gin.Context) {
	page, pageSize := parsePage(c)
	configs, total, err := h.svc.List(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"skills": configs, "total": total, "page": page, "page_size": pageSize})
}

// Get returns a single skill config by name.
// GET /admin/skills/:name
func (h *SkillConfigHandler) Get(c *gin.Context) {
	name := c.Param("name")
	cfg, err := h.svc.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, cfg)
}

// Upsert validates and saves a skill config.
// PUT /admin/skills/:name
func (h *SkillConfigHandler) Upsert(c *gin.Context) {
	name := c.Param("name")
	var req struct {
		Enabled    bool   `json:"enabled"`
		ConfigJSON string `json:"config_json"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	cfg, err := h.svc.Get(c.Request.Context(), name)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}
	cfg.Enabled = req.Enabled
	cfg.ConfigJSON = req.ConfigJSON
	if err := h.svc.Upsert(c.Request.Context(), *cfg); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已保存"})
}
