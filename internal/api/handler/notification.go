package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	notifsvc "github.com/luoxiaojun1992/data-agent/internal/service/notification"
)

// NotificationHandler provides HTTP handlers for notifications.
type NotificationHandler struct {
	svc notifsvc.NotificationService
}

// NewNotificationHandler creates a notification handler.
func NewNotificationHandler(svc notifsvc.NotificationService) *NotificationHandler {
	return &NotificationHandler{svc: svc}
}

// ListNotifications returns notifications for the current user.
func (h *NotificationHandler) ListNotifications(c *gin.Context) {
	userID, _ := c.Get("user_id")
	limit, _ := strconv.ParseInt(c.DefaultQuery("limit", "20"), 10, 64)
	notifs, err := h.svc.ListForUser(userID.(string), limit)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, notifs)
}

// UnreadCount returns the unread notification count.
func (h *NotificationHandler) UnreadCount(c *gin.Context) {
	userID, _ := c.Get("user_id")
	count, err := h.svc.UnreadCount(userID.(string))
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"count": count})
}

// MarkRead marks a notification as read.
func (h *NotificationHandler) MarkRead(c *gin.Context) {
	userID, _ := c.Get("user_id")
	if err := h.svc.MarkRead(c.Param("id"), userID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "read"})
}

// MarkAllRead marks all notifications as read.
func (h *NotificationHandler) MarkAllRead(c *gin.Context) {
	userID, _ := c.Get("user_id")
	if err := h.svc.MarkAllRead(userID.(string)); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"status": "all_read"})
}

// SendNotification sends a notification to specific users.
func (h *NotificationHandler) SendNotification(c *gin.Context) {
	var req struct {
		Title     string   `json:"title" binding:"required"`
		Content   string   `json:"content" binding:"required"`
		Type      string   `json:"type"`
		TargetIDs []string `json:"target_ids"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题和内容不能为空"})
		return
	}
	if req.Type == "" {
		req.Type = "info"
	}
	notif, err := h.svc.Send(req.Title, req.Content, req.Type, req.TargetIDs)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, notif)
}

// BroadcastNotification sends a notification to all users.
func (h *NotificationHandler) BroadcastNotification(c *gin.Context) {
	var req struct {
		Title   string `json:"title" binding:"required"`
		Content string `json:"content" binding:"required"`
		Type    string `json:"type"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "标题和内容不能为空"})
		return
	}
	if req.Type == "" {
		req.Type = "info"
	}
	notif, err := h.svc.Broadcast(req.Title, req.Content, req.Type)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusCreated, notif)
}
