package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
)

type SessionHandler struct {
	mgr chat.SessionService
}

func NewSessionHandler(mgr chat.SessionService) *SessionHandler {
	return &SessionHandler{mgr: mgr}
}

func RegisterSessionRoutes(rg *gin.RouterGroup, h *SessionHandler) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	// /deleted must be registered before /:id so the static path wins (UI-181).
	rg.GET("/deleted", h.ListDeleted)
	rg.GET("/:id", h.Get)
	rg.PUT("/:id", h.Renew)
	rg.DELETE("/:id", h.Delete)
	rg.POST("/:id/restore", h.Restore)
}

func (h *SessionHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	sessions, _ := h.mgr.ListByUser(userID)
	c.JSON(http.StatusOK, gin.H{"sessions": sessions})
}

func (h *SessionHandler) Create(c *gin.Context) {
	userID := c.GetString("user_id")
	sType := c.DefaultQuery("type", "chat")
	s, err := h.mgr.Create(userID, sType)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Restore main's HTTP contract: top-level session_id + expires_at.
	// The frontend (chat/page.tsx createSession) reads data.session_id directly.
	c.JSON(http.StatusCreated, gin.H{
		"session_id": s.ID,
		"expires_at": s.ExpiresAt,
	})
}

func (h *SessionHandler) Get(c *gin.Context) {
	s, err := h.mgr.Get(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"session": s})
}

func (h *SessionHandler) Renew(c *gin.Context) {
	if err := h.mgr.Renew(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "renewed"})
}

func (h *SessionHandler) Delete(c *gin.Context) {
	if err := h.mgr.Delete(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "deleted"})
}

func (h *SessionHandler) Restore(c *gin.Context) {
	if err := h.mgr.Restore(c.Param("id")); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"message": "restored"})
}

// ListDeleted returns soft-deleted sessions for the current user (recovery
// window). The frontend chat page calls GET /sessions/deleted to render the
// session-recovery-banner (UI-181).
func (h *SessionHandler) ListDeleted(c *gin.Context) {
	userID := c.GetString("user_id")
	sessions, err := h.mgr.ListDeleted(time.Now(), 100)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	var userSessions []*chat.Session
	for _, s := range sessions {
		if s.UserID == userID {
			userSessions = append(userSessions, s)
		}
	}
	c.JSON(http.StatusOK, gin.H{"sessions": userSessions})
}
