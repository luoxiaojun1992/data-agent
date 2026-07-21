package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"google.golang.org/adk/memory"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
)

// MemoryHandler exposes the long-term memory search endpoint.
type MemoryHandler struct {
	memSvc  memory.Service
	appName string
}

// NewMemoryHandler creates a memory search handler.
func NewMemoryHandler(memSvc memory.Service, appName string) *MemoryHandler {
	return &MemoryHandler{memSvc: memSvc, appName: appName}
}

// RegisterMemoryRoute registers GET /memory/search on the given authenticated
// router group (requires PermUserManage).
func RegisterMemoryRoute(rg *gin.RouterGroup, h *MemoryHandler) {
	rg.GET("/memory/search", middleware.RequirePermission(model.PermUserManage), h.Search)
}

// Search queries the long-term memory store for the given user.
func (h *MemoryHandler) Search(c *gin.Context) {
	query := c.Query("q")
	if query == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "query parameter 'q' required"})
		return
	}
	userID := c.Query("user_id")
	if userID == "" {
		userID = c.GetString("user_id")
	}

	results, err := h.memSvc.SearchMemory(c.Request.Context(), &memory.SearchRequest{
		Query:   query,
		UserID:  userID,
		AppName: h.appName,
	})
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var texts []string
	for _, m := range results.Memories {
		if m.Content != nil {
			for _, p := range m.Content.Parts {
				if p != nil {
					texts = append(texts, p.Text)
				}
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"results": texts, "count": len(texts)})
}
