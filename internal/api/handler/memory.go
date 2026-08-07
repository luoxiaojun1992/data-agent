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
	rg.GET("/memory/list", middleware.RequirePermission(rbacSvc, model.PermMemoryView), h.List)
}

// Search queries the long-term memory store for the given user — DEPRECATED, merged into List.

// List returns paginated memories with data isolation and optional text search:
// user/admin → only own memories; system_admin → all users'.
// Query param 'q' filters by content text (keyword match).
func (h *MemoryHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	role := c.GetString("role")
	targetUser := c.Query("user_id")
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "20"))

	// Data isolation: non-system_admin can only see their own
	if role != "system_admin" {
		targetUser = userID
	}

	obs, total, err := h.storage.List(c.Request.Context(), targetUser, q, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"items": obs, "total": total, "page": page, "page_size": pageSize, "user_id": targetUser})
}
