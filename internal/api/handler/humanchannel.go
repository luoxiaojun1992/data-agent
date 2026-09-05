package handler

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/service/humanchannel"
)

// HumanChannelHandler exposes the human-in-the-loop channel endpoints: an SSE
// stream that the frontend opens to receive confirm/ask events, and a reply
// endpoint that injects the user's answer back into the blocking tool.
//
// RBAC mirrors the chat channel (PermChatView is applied at the route group in
// routes.go) and adds session ownership verification with a system_admin
// exemption — identical to chat's useExistingSession semantics.
type HumanChannelHandler struct {
	hub      *humanchannel.Hub
	sessions domainchat.SessionService
}

// NewHumanChannelHandler creates the handler backed by the channel hub and the
// session service used for ownership verification.
func NewHumanChannelHandler(hub *humanchannel.Hub, sessions domainchat.SessionService) *HumanChannelHandler {
	return &HumanChannelHandler{hub: hub, sessions: sessions}
}

// RegisterHumanChannelRoutes registers the channel SSE and reply endpoints on
// the authenticated /api/v1/chat group (PermChatView already applied).
func RegisterHumanChannelRoutes(rg *gin.RouterGroup, h *HumanChannelHandler) {
	rg.GET("/:session_id/human-channel", h.HandleChannel)
	rg.POST("/:session_id/human-channel/:request_id/reply", h.HandleReply)
}

// verifyOwnership loads the session and enforces ownership (or system_admin
// exemption). Returns false (and writes the HTTP error) when access is denied.
func (h *HumanChannelHandler) verifyOwnership(c *gin.Context, sessionID, userID, role string) bool {
	sess, err := h.sessions.Get(sessionID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "session not found"})
		return false
	}
	if sess.UserID != userID && role != "system_admin" {
		c.JSON(http.StatusForbidden, gin.H{"error": "session does not belong to the current user"})
		return false
	}
	return true
}

// HandleChannel establishes the SSE stream for the session. It subscribes to
// the hub and forwards each confirm/ask event until the client disconnects.
func (h *HumanChannelHandler) HandleChannel(c *gin.Context) {
	sessionID := c.Param("session_id")
	userID := c.GetString("user_id")
	role := c.GetString("role")
	if !h.verifyOwnership(c, sessionID, userID, role) {
		return
	}

	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")
	c.Status(http.StatusOK)

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	subID, eventCh := h.hub.Subscribe(sessionID)
	defer h.hub.Unsubscribe(sessionID, subID)

	// Heartbeat keeps idle connections alive through proxies/nginx and lets
	// the client distinguish a live channel from a silently dropped one.
	heartbeat := time.NewTicker(30 * time.Second)
	defer heartbeat.Stop()

	flusher.Flush()
	for {
		select {
		case <-c.Request.Context().Done():
			return
		case ev := <-eventCh:
			data, err := json.Marshal(ev)
			if err != nil {
				continue
			}
			if _, err := fmt.Fprintf(c.Writer, "data: %s\n\n", data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := fmt.Fprintf(c.Writer, ": ping\n\n"); err != nil {
				return
			}
			flusher.Flush()
		}
	}
}

// HandleReply injects the user's answer into the blocking tool. It verifies
// session ownership and request_id membership (via the hub) before delivery.
func (h *HumanChannelHandler) HandleReply(c *gin.Context) {
	sessionID := c.Param("session_id")
	requestID := c.Param("request_id")
	userID := c.GetString("user_id")
	role := c.GetString("role")
	if !h.verifyOwnership(c, sessionID, userID, role) {
		return
	}

	var reply humanchannel.Reply
	if err := c.ShouldBindJSON(&reply); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	switch err := h.hub.Reply(sessionID, requestID, reply); err {
	case nil:
		c.JSON(http.StatusOK, gin.H{"ok": true})
	case humanchannel.ErrUnknownRequest:
		c.JSON(http.StatusNotFound, gin.H{"error": "request not found or already answered"})
	case humanchannel.ErrNoChannel:
		c.JSON(http.StatusNotFound, gin.H{"error": "human channel not established"})
	default:
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
	}
}
