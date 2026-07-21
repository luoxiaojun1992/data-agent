package handler

import (
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/im"
)

// IMBindHandler exposes the per-user IM binding endpoints.
type IMBindHandler struct {
	svc *im.BindService
}

// NewIMBindHandler creates an IM bind handler. svc may be nil when the
// database is unavailable; the handlers respond with 503 in that case.
func NewIMBindHandler(svc *im.BindService) *IMBindHandler {
	return &IMBindHandler{svc: svc}
}

// RegisterIMBindRoutes registers GET/PUT /im/bind on the given authenticated
// router group.
func RegisterIMBindRoutes(rg *gin.RouterGroup, h *IMBindHandler) {
	rg.GET("", h.Get)
	rg.PUT("", h.Update)
}

// Get returns the caller's IM binding (always wrapped in a "binds" array to
// preserve the existing API contract).
func (h *IMBindHandler) Get(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
		return
	}
	userID := c.GetString("user_id")
	bind, err := h.svc.Get(c.Request.Context(), userID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	binds := []interface{}{}
	if bind != nil {
		binds = append(binds, bind)
	}
	c.JSON(http.StatusOK, gin.H{"binds": binds})
}

// Update upserts the caller's IM binding from an arbitrary JSON body.
func (h *IMBindHandler) Update(c *gin.Context) {
	if h.svc == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "数据库不可用"})
		return
	}
	userID := c.GetString("user_id")
	var body map[string]interface{}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "参数解析失败"})
		return
	}
	if err := h.svc.Upsert(c.Request.Context(), userID, body); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
