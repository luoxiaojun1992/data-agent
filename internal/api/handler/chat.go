package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
)

// ChatHandler exposes the chat completion endpoint. It translates gin/HTTP
// input into domain requests and writes domain responses back as JSON or
// SSE. The chat service itself contains no gin dependency.
type ChatHandler struct {
	svc domainchat.ChatService
}

// NewChatHandler creates a chat handler backed by the domain ChatService.
func NewChatHandler(svc domainchat.ChatService) *ChatHandler {
	return &ChatHandler{svc: svc}
}

// RegisterChatRoutes registers the chat completion route on the given
// authenticated router group.
func RegisterChatRoutes(rg *gin.RouterGroup, h *ChatHandler) {
	rg.POST("", h.HandleChat)
}

// HandleChat handles a chat completion request with optional SSE streaming.
func (h *ChatHandler) HandleChat(c *gin.Context) {
	var req domainchat.ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	userID := c.GetString("user_id")
	role := c.GetString("role")

	ctx := c.Request.Context()

	if req.Stream {
		if err := h.svc.Stream(ctx, req, userID, role, c.Writer); err != nil {
			// Stream only returns an error before any SSE bytes are written
			// (prepareRun failures); map to the appropriate HTTP status.
			c.JSON(chatErrorStatus(err), gin.H{"error": err.Error()})
		}
		return
	}

	resp, err := h.svc.Process(ctx, req, userID, role)
	if err != nil {
		c.JSON(chatErrorStatus(err), gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, resp)
}

// chatErrorStatus maps a domain chat error to an HTTP status code, preserving
// the transport semantics of the legacy gin-coupled implementation.
func chatErrorStatus(err error) int {
	switch {
	case errors.Is(err, domainchat.ErrMessagesRequired),
		errors.Is(err, domainchat.ErrUserMessageRequired):
		return http.StatusBadRequest
	case errors.Is(err, domainchat.ErrUnauthorizedSession):
		return http.StatusUnauthorized
	case errors.Is(err, domainchat.ErrSessionCreateFailed),
		errors.Is(err, domainchat.ErrADKSessionInitFailed):
		return http.StatusInternalServerError
	default:
		// Model / circuit-breaker errors are treated as service unavailable.
		return http.StatusServiceUnavailable
	}
}
