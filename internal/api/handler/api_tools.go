package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/service/apicollection"
)

type APIToolsHandler struct {
	svc *apicollection.Service
}

func NewAPIToolsHandler(svc *apicollection.Service) *APIToolsHandler {
	return &APIToolsHandler{svc: svc}
}

// Search handles external_api_search — fuzzy search approved collections by description.
func (h *APIToolsHandler) Search(c *gin.Context) {
	query := c.Query("q")
	result, err := h.svc.SearchApproved(c.Request.Context(), query, 10)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	type item struct {
		ID          string `json:"id"`
		Name        string `json:"name"`
		Description string `json:"description"`
	}
	items := make([]item, 0, len(result))
	for _, r := range result {
		items = append(items, item{ID: r.ID, Name: r.Name, Description: r.Description})
	}
	c.JSON(http.StatusOK, gin.H{"collections": items, "total": len(items)})
}

// Summary handles external_api_summary — list APIs in a collection.
func (h *APIToolsHandler) Summary(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "10"))
	result, err := h.svc.GetAPISummary(c.Request.Context(), c.Query("collection_id"), page, pageSize)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// Method handles external_api_method — get API method details.
func (h *APIToolsHandler) Method(c *gin.Context) {
	result, err := h.svc.GetAPIMethod(c.Request.Context(),
		c.Query("collection_id"), c.Query("path"), c.Query("method"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}

// Call handles external_api_call — invoke an external API.
func (h *APIToolsHandler) Call(c *gin.Context) {
	var req struct {
		CollectionID string            `json:"collection_id"`
		Path         string            `json:"path"`
		Method       string            `json:"method"`
		Params       map[string]string `json:"params"`
		Body         interface{}       `json:"body"`
		Headers      map[string]string `json:"headers"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	if req.Params == nil {
		req.Params = map[string]string{}
	}
	if req.Headers == nil {
		req.Headers = map[string]string{}
	}
	result, err := h.svc.CallAPI(c.Request.Context(), req.CollectionID, req.Path, req.Method, req.Params, req.Body, req.Headers)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, result)
}
