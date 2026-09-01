package handler

import (
	"context"
	"iter"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/api/middleware"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	rbacsvc "github.com/luoxiaojun1992/data-agent/internal/service/rbac"
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

func RegisterSessionRoutes(rg *gin.RouterGroup, h *SessionHandler, rbacSvc *rbacsvc.Service) {
	rg.GET("", middleware.RequirePermission(rbacSvc, model.PermChatView), h.List)
	rg.POST("", middleware.RequirePermission(rbacSvc, model.PermChatView), h.Create)
	rg.GET("/deleted", middleware.RequirePermission(rbacSvc, model.PermChatView), h.ListDeleted)
	rg.GET("/:id", middleware.RequirePermission(rbacSvc, model.PermChatView), h.Get)
	rg.GET("/:id/messages", middleware.RequirePermission(rbacSvc, model.PermChatView), h.Messages)
	rg.PUT("/:id", middleware.RequirePermission(rbacSvc, model.PermChatView), h.Renew)
	rg.DELETE("/:id", middleware.RequirePermission(rbacSvc, model.PermChatDelete), h.Delete)
	rg.POST("/:id/restore", middleware.RequirePermission(rbacSvc, model.PermChatView), h.Restore)
}

func (h *SessionHandler) List(c *gin.Context) {
	userID := c.GetString("user_id")
	q := c.Query("q")
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	pageSize, _ := strconv.Atoi(c.DefaultQuery("page_size", "15"))
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 15
	}
	sessions, total, err := h.mgr.ListByUserPaged(userID, q, page, pageSize)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"sessions":  sessions,
		"total":     total,
		"page":      page,
		"page_size": pageSize,
	})
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

// Messages returns the latest 100 ADK events converted to canonical chat form.
// Streaming chunks are already merged by AppendEvent — one event = one complete message.
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

	// Use never-compacted display events when available; fall back to session
	// events for sessions created before this feature existed.
	events := resp.Session.Events()
	if svc, ok := h.adkSessions.(interface {
		DisplayEvents(ctx context.Context, appName, userID, sessionID string, limit int) ([]*session.Event, error)
	}); ok {
		if raw, err := svc.DisplayEvents(c.Request.Context(), h.appName, userID, sessionID, 100); err == nil && len(raw) > 0 {
			events = &eventSlice{items: raw}
		}
	}

	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		if ev.Content == nil {
			continue
		}
		// Compaction summary is internal context; surface only a lightweight
		// notice (no summary content) so the transcript shows a compression
		// happened without exposing the raw summary (SPEC-067 follow-up).
		if chat.IsCompactionEvent(ev) {
			messages = append(messages, domainchat.ChatEvent{
				EventID:   ev.ID,
				Role:      "system",
				Type:      "text",
				Content:   "[compaction] 上下文已自动压缩",
				Timestamp: ev.Timestamp.UTC().Format(time.RFC3339),
			})
			continue
		}
		role := ev.Author
		if role == "" {
			role = "assistant"
		}
		timestamp := ev.Timestamp.UTC().Format(time.RFC3339)
		for _, event := range chat.ChatEventsFromParts(role, ev.ID, timestamp, ev.Content.Parts) {
			messages = append(messages, event)
		}
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// eventSlice adapts a []*session.Event to the session.Events interface.
type eventSlice struct{ items []*session.Event }

func (s *eventSlice) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, e := range s.items {
			if !yield(e) {
				return
			}
		}
	}
}
func (s *eventSlice) Len() int              { return len(s.items) }
func (s *eventSlice) At(i int) *session.Event { return s.items[i] }
