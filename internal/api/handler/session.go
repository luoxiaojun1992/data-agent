package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	"google.golang.org/adk/session"
)

type SessionHandler struct {
	mgr         chat.SessionService
	adkSessions session.Service
	appName     string
}

func NewSessionHandler(mgr chat.SessionService, adkSessions ...session.Service) *SessionHandler {
	var service session.Service
	if len(adkSessions) > 0 {
		service = adkSessions[0]
	}
	return &SessionHandler{mgr: mgr, adkSessions: service, appName: "data-agent"}
}

func RegisterSessionRoutes(rg *gin.RouterGroup, h *SessionHandler) {
	rg.GET("", h.List)
	rg.POST("", h.Create)
	// /deleted must be registered before /:id so the static path wins (UI-181).
	rg.GET("/deleted", h.ListDeleted)
	rg.GET("/:id", h.Get)
	rg.GET("/:id/messages", h.Messages)
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
	var body struct {
		ModelID string `json:"model_id"`
	}
	_ = c.ShouldBindJSON(&body) // optional body; ignore parse errors (empty model_id)
	s, err := h.mgr.Create(userID, sType, body.ModelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	// Restore main's HTTP contract: top-level session_id + expires_at.
	// The frontend (chat/page.tsx createSession) reads data.session_id directly.
	c.JSON(http.StatusCreated, gin.H{
		"session_id": s.ID,
		"model_id":   s.ModelID,
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

// Messages returns the latest 50 ADK events converted to canonical chat form.
// Compaction summaries are skipped; everything else (user, agent text, tool calls,
// tool results) is kept exactly as stored in MongoDB. Consecutive text parts from
// the same role are merged into a single bubble (streaming tokens).
func (h *SessionHandler) Messages(c *gin.Context) {
	if h.adkSessions == nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "adk session service not configured"})
		return
	}
	userID := c.GetString("user_id")
	sessionID := c.Param("id")

	resp, err := h.adkSessions.Get(c.Request.Context(), &session.GetRequest{
		AppName:   h.appName,
		UserID:    userID,
		SessionID: sessionID,
	})
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return
	}

	var messages []domainchat.ChatEvent
	events := resp.Session.Events()
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		if chat.IsCompactionEvent(ev) || ev.Content == nil {
			continue
		}
		role := ev.Author
		if role == "" {
			role = "assistant"
		}
		timestamp := ev.Timestamp.UTC().Format(time.RFC3339)
		for _, event := range chat.ChatEventsFromParts(role, ev.ID, timestamp, ev.Content.Parts) {
			// Match the live renderer: consecutive text parts by the same
			// author are one transcript bubble, including streaming chunks.
			if event.Type == "text" && len(messages) > 0 {
				last := &messages[len(messages)-1]
				if last.Type == "text" && last.Role == event.Role {
					last.Content += event.Content
					continue
				}
			}
			messages = append(messages, event)
		}
	}
	// Keep only the latest 50.
	if len(messages) > 50 {
		messages = messages[len(messages)-50:]
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}
