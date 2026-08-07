package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"google.golang.org/adk/memory"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/adk/memoryx"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// MemoryHandler exposes the long-term memory search endpoint.
type MemoryHandler struct {
	memSvc  memory.Service
	storage memoryx.Storage
	appName string
}

// NewMemoryHandler creates a memory search handler.
func NewMemoryHandler(memSvc memory.Service, storage memoryx.Storage, appName string) *MemoryHandler {
	return &MemoryHandler{memSvc: memSvc, storage: storage, appName: appName}
}

// RegisterMemoryRoute registers GET /memory/search on the given authenticated
// router group (requires PermUserManage).
func RegisterMemoryRoute(rg *gin.RouterGroup, h *MemoryHandler, rbacSvc *rbacsvc.Service) {
	rg.GET("/memory/search", middleware.RequirePermission(rbacSvc, model.PermMemorySearch), h.Search)
}

// Search queries the long-term memory store for the given user.

// List returns paginated memories with data isolation:
// user/admin → only own memories; system_admin → all users'.
func (h *MemoryHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	role := c.GetString("role")
	targetUser := c.Query("user_id")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Data isolation: non-system_admin can only see their own
	if role != "system_admin" {
		targetUser = userID
	}

	obs, total, err := h.storage.List(c.Request.Context(), targetUser, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": obs, "total": total, "page": page, "page_size": pageSize, "user_id": targetUser})
}

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
