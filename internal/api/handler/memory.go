package handler

import (
	"context"
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"github.com/ieshan/adk-go-memory/adapter"
	"google.golang.org/adk/memory"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/adk/memoryx"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
)

// MemoryHandler exposes the long-term memory search endpoint.
type MemoryHandler struct {
	memSvc      memory.Service
	storage     memoryx.Storage
	appName     string
	userRepo    repository.UserRepository
	sessionRepo repository.SessionRepository
}

// NewMemoryHandler creates a memory search handler.
func NewMemoryHandler(memSvc memory.Service, storage memoryx.Storage, appName string, userRepo repository.UserRepository, sessionRepo repository.SessionRepository) *MemoryHandler {
	return &MemoryHandler{memSvc: memSvc, storage: storage, appName: appName, userRepo: userRepo, sessionRepo: sessionRepo}
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
	// Enrich with user email + session title for display
	items := enrichWithUsers(c.Request.Context(), h.userRepo, h.sessionRepo, obs)
	c.JSON(http.StatusOK, gin.H{"items": items, "total": total, "page": page, "page_size": pageSize, "user_id": targetUser})
}

// enrichWithUsers adds UserEmail to each memory item by looking up the user record.
func enrichWithUsers(ctx context.Context, userRepo repository.UserRepository, sessionRepo repository.SessionRepository, obs []adapter.Observation) []gin.H {
	if userRepo == nil {
		out := make([]gin.H, len(obs))
		for i := range obs {
			out[i] = gin.H{"ID": obs[i].ID, "Content": obs[i].Content, "Level": obs[i].Level, "SessionID": obs[i].SessionID, "UserID": obs[i].UserID, "AppName": obs[i].AppName, "Tags": obs[i].Tags, "TimesDerived": obs[i].TimesDerived, "CreatedAt": obs[i].CreatedAt}
		}
		return out
	}
	emailCache := make(map[string]string)
	titleCache := make(map[string]string)
	out := make([]gin.H, len(obs))
	for i := range obs {
		email := ""
		if obs[i].UserID != "" {
			if cached, ok := emailCache[obs[i].UserID]; ok {
				email = cached
			} else if u, err := userRepo.FindByID(ctx, obs[i].UserID); err == nil && u != nil {
				email = u.Username
				emailCache[obs[i].UserID] = email
			} else {
				email = obs[i].UserID
			}
		}
		sessionTitle := ""
		if obs[i].SessionID != "" && sessionRepo != nil {
			if cached, ok := titleCache[obs[i].SessionID]; ok {
				sessionTitle = cached
			} else if sess, err := sessionRepo.Get(ctx, obs[i].SessionID); err == nil && sess != nil {
				sessionTitle = sess.Title
				titleCache[obs[i].SessionID] = sessionTitle
			}
		}
		out[i] = gin.H{
			"ID": obs[i].ID, "Content": obs[i].Content, "Level": obs[i].Level,
			"SessionID": obs[i].SessionID, "SessionTitle": sessionTitle,
			"UserID": obs[i].UserID, "UserEmail": email,
			"AppName": obs[i].AppName, "Tags": obs[i].Tags, "TimesDerived": obs[i].TimesDerived,
			"CreatedAt": obs[i].CreatedAt,
		}
	}
	return out
}
