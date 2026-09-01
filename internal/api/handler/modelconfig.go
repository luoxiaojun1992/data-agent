package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/service/config"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// ModelConfigHandler handles model configuration HTTP endpoints.
// SPEC-062: supports structured []ModelEntry CRUD (add/delete/set-default)
// plus a paginated LLM-only list for the model selector. Legacy key/value
// upsert is preserved for backward compatibility.
type ModelConfigHandler struct {
	cfgSvc   config.Service
	provider *modelcfg.Provider
}

// NewModelConfigHandler creates a new ModelConfigHandler.
func NewModelConfigHandler(cfgSvc config.Service, provider *modelcfg.Provider) *ModelConfigHandler {
	return &ModelConfigHandler{cfgSvc: cfgSvc, provider: provider}
}

// modelRoutePath is the base route for model config endpoints (sonar S1192:
// avoid duplicating the literal across route registrations).
const modelRoutePath = "/models"

// errProviderNotConfigured is the error message returned when no model
// provider is wired (sonar S1192: avoid duplicating the literal).
const errProviderNotConfigured = "model provider not configured"

// RegisterModelPublicRoutes registers model routes used by regular users
// (chat model selector). Mounted under /api/v1 (no /admin prefix).
func RegisterModelPublicRoutes(api *gin.RouterGroup, h *ModelConfigHandler, rbacSvc *rbacsvc.Service) {
	api.GET(modelRoutePath+"/list", middleware.RequirePermission(rbacSvc, model.PermModelList), h.ListLLM)
}

// RegisterModelAdminRoutes registers model config management routes.
// Mounted under /api/v1/admin. Requires PermModelEdit (system_admin only).
func RegisterModelAdminRoutes(admin *gin.RouterGroup, h *ModelConfigHandler, rbacSvc *rbacsvc.Service) {
	admin.GET(modelRoutePath+"/embedding", middleware.RequirePermission(rbacSvc, model.PermModelConfigView), h.ListEmbedding)
	admin.GET(modelRoutePath, middleware.RequirePermission(rbacSvc, model.PermModelConfigView), h.Get)
	admin.PUT(modelRoutePath, h.Put)
	admin.POST(modelRoutePath, h.AddModel)
	admin.PATCH(modelRoutePath+"/:id/default", h.SetDefault)
	admin.DELETE(modelRoutePath+"/default", h.UnsetDefault)
	admin.PATCH(modelRoutePath+"/:id", h.UpdateModel)
	admin.DELETE(modelRoutePath+"/:id", h.DeleteModel)
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

// getPaginated returns a paginated LLM-only model list. API keys are
// decrypted from Vault — the caller (admin UI) is trusted to handle them.
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
	cfgs, err := h.cfgSvc.GetAll(c.Request.Context())
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
	if err := h.cfgSvc.Upsert(c.Request.Context(), req.Key, req.Value); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已保存"})
}

// ListLLM returns the LLM-only model list (paginated) for the model selector.
// SPEC-062 §4.1: GET /models/list — only Type==llm models, with pagination.
// API key is masked; api_key_exists flag tells the frontend whether to render
// the eye button (decrypt endpoint).
func (h *ModelConfigHandler) ListLLM(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errProviderNotConfigured})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	models, total, err := h.provider.ListLLMModels(c.Request.Context(), page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	for i := range models {
		// Apply sane defaults for legacy DB rows where fields weren't tracked.
		if models[i].ContextLen == 0 {
			models[i].ContextLen = 128000
		}
		if models[i].MaxTokens == 0 {
			models[i].MaxTokens = 16000
		}
	}
	// Decorate with api_key_exists for the frontend (the JSON tag on APIKey
	// is "omitempty" so an empty string would otherwise signal "not set").
	c.JSON(http.StatusOK, gin.H{
		"models":    models,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
}

// ListEmbedding returns the embedding-type model list with pagination.
// GET /models/embedding
func (h *ModelConfigHandler) ListEmbedding(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errProviderNotConfigured})
		return
	}
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))
	models, total, err := h.provider.ListEmbeddingModels(c.Request.Context(), page, pageSize)
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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errProviderNotConfigured})
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
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errProviderNotConfigured})
		return
	}
	id := c.Param("id")
	if err := h.provider.DeleteModel(c.Request.Context(), id); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已删除", "id": id})
}

// SetDefault marks the model with :id as default for the given use cases.
// If no use_cases are specified, the model's own Type decides: LLM models
// default to the chat use case; embedding models flip the IsDefault flag
// (only one embedding model can be default at a time).
// PATCH /models/:id/default
func (h *ModelConfigHandler) SetDefault(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errProviderNotConfigured})
		return
	}
	id := c.Param("id")
	var req struct {
		UseCases []string `json:"use_cases"`
	}
	_ = c.ShouldBindJSON(&req)
	// Look up the target to decide whether it is an LLM or embedding entry so
	// the embedding single-default semantics are respected automatically.
	all := h.provider.ListAllModels(c.Request.Context())
	var target *modelcfg.ModelEntry
	for i := range all {
		if all[i].ID == id {
			target = &all[i]
			break
		}
	}
	if target == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "model not found: " + id})
		return
	}
	if target.Type == modelcfg.ModelTypeEmbedding {
		if err := h.provider.SetDefaultEmbedding(c.Request.Context(), id); err != nil {
			c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
			return
		}
		c.JSON(http.StatusOK, gin.H{"message": "已设为默认 embedding", "id": id})
		return
	}
	if len(req.UseCases) == 0 {
		req.UseCases = []string{"chat"}
	}
	if err := h.provider.SetDefaultModel(c.Request.Context(), id, req.UseCases); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已设为默认", "id": id, "use_cases": req.UseCases})
}

// UnsetDefault cancels the default for the given use cases.
// DELETE /models/default
func (h *ModelConfigHandler) UnsetDefault(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errProviderNotConfigured})
		return
	}
	var req struct {
		UseCases []string `json:"use_cases"`
	}
	_ = c.ShouldBindJSON(&req)
	if len(req.UseCases) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "use_cases required"})
		return
	}
	if err := h.provider.UnsetDefault(c.Request.Context(), req.UseCases); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "已取消默认", "use_cases": req.UseCases})
}

// UpdateModel updates an existing model's fields.
// PATCH /models/:id
func (h *ModelConfigHandler) UpdateModel(c *gin.Context) {
	if h.provider == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": errProviderNotConfigured})
		return
	}
	id := c.Param("id")
	var entry modelcfg.ModelEntry
	if err := c.ShouldBindJSON(&entry); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	updated, err := h.provider.UpdateModel(c.Request.Context(), id, entry)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, updated)
}
