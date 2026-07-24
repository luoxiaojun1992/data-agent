package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	"github.com/luoxiaojun1992/data-agent/internal/service/config"
)

// ModelConfigHandler handles model configuration HTTP endpoints.
// SPEC-062: supports structured []ModelEntry CRUD (add/delete/set-default)
// plus a paginated LLM-only list for the model selector. Legacy key/value
// upsert is preserved for backward compatibility.
type ModelConfigHandler struct {
	cfgSvc    config.Service
	provider  *modelcfg.Provider
}

// NewModelConfigHandler creates a new ModelConfigHandler.
func NewModelConfigHandler(cfgSvc config.Service, provider *modelcfg.Provider) *ModelConfigHandler {
	return &ModelConfigHandler{cfgSvc: cfgSvc, provider: provider}
}

// RegisterModelConfigRoutes registers model config management routes.
// SPEC-062 adds structured CRUD endpoints alongside the legacy GET/PUT.
func RegisterModelConfigRoutes(api *gin.RouterGroup, h *ModelConfigHandler) {
	api.GET("/models", h.Get)
	api.PUT("/models", h.Put)
	api.GET("/models/list", h.ListLLM)        // LLM-only, paginated (selector source)
	api.POST("/models", h.AddModel)            // add single model (auto-gen ID)
	api.DELETE("/models/:id", h.DeleteModel)   // delete single model
	api.PATCH("/models/:id/default", h.SetDefault) // set as default
}

// Get returns the full model configuration. When the page query param is
// present, returns a paginated structured []ModelEntry response; otherwise
// returns the raw config map (legacy flat keys + structured models).
func (h *ModelConfigHandler) Get(c *gin.Context) {
	if h.provider == nil {
		h.legacyGet(c)
		return
	}
	ctx := c.Request.Context()
	if c.Query("page") != "" {
		h.getPaginated(c, ctx)
		return
	}
	h.getRaw(c, ctx)
}

// getPaginated returns a paginated LLM-only model list.
func (h *ModelConfigHandler) getPaginated(c *gin.Context, ctx context.Context) {
	page, _ := strconv.Atoi(c.Query("page"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	models, total, err := h.provider.ListLLMModels(ctx, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"models": models, "total": total, "page": page, "page_size": pageSize,
	})
}

// getRaw returns the full raw config map (structured + legacy flat keys).
func (h *ModelConfigHandler) getRaw(c *gin.Context, ctx context.Context) {
	raw, err := h.provider.GetRawModelConfig(ctx)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": raw})
}

// legacyGet is the pre-SPEC-062 GET path used when no Provider is wired.
func (h *ModelConfigHandler) legacyGet(c *gin.Context) {
	cfgs, err := h.cfgSvc.GetAll(c.Request.Context(), "models")
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"models": cfgs})
}

// Put upserts a model config value (legacy key/value form). SPEC-062 keeps
// this for backward compatibility; structured list upsert should use the
// Provider's SetModels via POST or the raw config page.
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

// ListLLM returns the LLM-only model list (paginated) for the model selector.
// SPEC-062 §4.1: GET /models/list — only Type==llm models, with pagination.
func (h *ModelConfigHandler) ListLLM(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "model provider not configured"})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	models, total, err := h.provider.ListLLMModels(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"models":    models,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// AddModel adds a single model entry. The backend auto-generates the ID when
// empty. SPEC-062 §4.1: POST /models.
func (h *ModelConfigHandler) AddModel(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "model provider not configured"})
		return
	}
	var entry modelcfg.ModelEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	saved, err := h.provider.AddModel(c.Request.Context(), entry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, saved)
}

// DeleteModel removes a single model by ID. Idempotent (deleting a missing ID
// returns 200). SPEC-062 §4.1: DELETE /models/:id.
func (h *ModelConfigHandler) DeleteModel(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "model provider not configured"})
		return
	}
	id := c.Param("id")
	if err := h.provider.DeleteModel(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除", "id": id})
}

// SetDefault marks the model with :id as the sole default LLM. SPEC-062 §4.1:
// PATCH /models/:id/default.
func (h *ModelConfigHandler) SetDefault(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "model provider not configured"})
		return
	}
	id := c.Param("id")
	if err := h.provider.SetDefaultModel(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已设为默认", "id": id})
}
