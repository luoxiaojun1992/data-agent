package handler

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/chat"
	"google.golang.org/adk/session"
)

type SessionHandler struct {
	mgr         chat.SessionService
	adkSessions session.Service
	appName     string
}

func NewSessionHandler(mgr chat.SessionService, adkSessions session.Service) *SessionHandler {
	return &SessionHandler{mgr: mgr, adkSessions: adkSessions, appName: "data-agent"}
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

// Messages returns the ADK session events as a flat list of {role, content}
// messages suitable for re-hydrating the frontend chat UI. Events without
// text content (tool calls, intermediate) are filtered out; compaction
// summaries are skipped so the user only sees the original conversation.
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

	type msg struct {
		Role      string         `json:"role"`
		Content   string         `json:"content"`
		Type      string         `json:"type,omitempty"`
		Name      string         `json:"name,omitempty"`
		Args      map[string]any `json:"args,omitempty"`
		Result    map[string]any `json:"result,omitempty"`
		Timestamp string         `json:"timestamp"`
	}
	events := resp.Session.Events()
	// Merge consecutive text parts from the same author into a single message.
	// The ieshan OpenAI client emits text as Partial=true chunks; the ADK
	// runner's final event has finish_reason but empty content. Without
	// merging, each streaming chunk would show up as a separate bubble.
	var messages []msg
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		if ev == nil || ev.LLMResponse.Content == nil {
			continue
		}
		if ev.LLMResponse.CustomMetadata != nil {
			if _, ok := ev.LLMResponse.CustomMetadata["compaction"]; ok {
				continue
			}
		}
		role := ev.Author
		if role == "" {
			role = "assistant"
		}
		ts := ev.Timestamp.UTC().Format(time.RFC3339)
		for _, p := range ev.LLMResponse.Content.Parts {
			if p == nil {
				continue
			}
			switch {
			case p.Text != "":
				// Append or merge with the previous text message from the same author.
				if n := len(messages); n > 0 && messages[n-1].Role == role && messages[n-1].Type == "text" {
					messages[n-1].Content += p.Text
				} else {
					messages = append(messages, msg{Role: role, Content: p.Text, Type: "text", Timestamp: ts})
				}
			case p.FunctionCall != nil:
				messages = append(messages, msg{
					Role: role, Type: "tool_call",
					Name: p.FunctionCall.Name, Args: p.FunctionCall.Args,
					Timestamp: ts,
				})
			case p.FunctionResponse != nil:
				messages = append(messages, msg{
					Role: role, Type: "tool_result",
					Name: p.FunctionResponse.Name, Result: asMap(p.FunctionResponse.Response),
					Timestamp: ts,
				})
			}
		}
	}
	c.JSON(http.StatusOK, gin.H{"messages": messages})
}

// asMap converts a generic JSON-decoded value into a map. Used for tool
// response payloads which may be any shape after JSON round-tripping.
func asMap(v any) map[string]any {
	if v == nil {
		return nil
	}
	if m, ok := v.(map[string]any); ok {
		return m
	}
	// Re-marshal/unmarshal to normalize any nested map types.
	b, err := json.Marshal(v)
	if err != nil {
		return nil
	}
	var m map[string]any
	_ = json.Unmarshal(b, &m)
	return m
}
