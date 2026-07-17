package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	notifsvc "github.com/luoxiaojun1992/data-agent/internal/service/notification"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() { gin.SetMode(gin.TestMode) }

// ── NewNotificationHandler ──

func TestNewNotificationHandler(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)
	if h == nil {
		t.Fatal("NewNotificationHandler returned nil")
	}
	if h.svc != svc {
		t.Error("svc not set correctly")
	}
}

// ── ListNotifications ──

func TestListNotifications_Success(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	notifs := []model.Notification{
		{ID: primitive.NewObjectID(), Title: "Test", Content: "Hello", Type: "info", CreatedAt: time.Now()},
	}

	patches := gomonkey.ApplyMethodReturn(svc, "ListForUser", notifs, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/notifications", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListNotifications_WithLimit(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "ListForUser", []model.Notification{}, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/notifications?limit=10", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListNotifications_DefaultLimit(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "ListForUser", []model.Notification{}, nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/notifications", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListNotifications_Error(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "ListForUser", nil, fmt.Errorf("db error"))
	defer patches.Reset()

	c, w := newGinContext("GET", "/notifications", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── UnreadCount ──

func TestUnreadCount_Success(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "UnreadCount", int64(5), nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/notifications/unread-count", "")
	c.Set("user_id", "user-1")
	h.UnreadCount(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), `"count":5`) {
		t.Errorf("body should contain count=5: %s", w.Body.String())
	}
}

func TestUnreadCount_Zero(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "UnreadCount", int64(0), nil)
	defer patches.Reset()

	c, w := newGinContext("GET", "/notifications/unread-count", "")
	c.Set("user_id", "user-1")
	h.UnreadCount(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), `"count":0`) {
		t.Errorf("body should contain count=0: %s", w.Body.String())
	}
}

func TestUnreadCount_Error(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "UnreadCount", int64(0), fmt.Errorf("db error"))
	defer patches.Reset()

	c, w := newGinContext("GET", "/notifications/unread-count", "")
	c.Set("user_id", "user-1")
	h.UnreadCount(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── MarkRead ──

func TestMarkRead_Success(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "MarkRead", nil)
	defer patches.Reset()

	c, w := newGinContext("POST", "/notifications/notif-1/read", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "id", Value: "notif-1"}}
	h.MarkRead(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "read") {
		t.Errorf("body should contain read: %s", w.Body.String())
	}
}

func TestMarkRead_Error(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "MarkRead", fmt.Errorf("invalid id"))
	defer patches.Reset()

	c, w := newGinContext("POST", "/notifications/bad/read", "")
	c.Set("user_id", "user-1")
	c.Params = gin.Params{{Key: "id", Value: "bad"}}
	h.MarkRead(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── MarkAllRead ──

func TestMarkAllRead_Success(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "MarkAllRead", nil)
	defer patches.Reset()

	c, w := newGinContext("POST", "/notifications/read-all", "")
	c.Set("user_id", "user-1")
	h.MarkAllRead(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "all_read") {
		t.Errorf("body should contain all_read: %s", w.Body.String())
	}
}

func TestMarkAllRead_Error(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "MarkAllRead", fmt.Errorf("db error"))
	defer patches.Reset()

	c, w := newGinContext("POST", "/notifications/read-all", "")
	c.Set("user_id", "user-1")
	h.MarkAllRead(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── SendNotification ──

func TestSendNotification_Success(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	n := &model.Notification{
		ID:        primitive.NewObjectID(),
		Title:     "System Update",
		Content:   "Maintenance tonight",
		Type:      "warning",
		TargetIDs: []string{"user-1", "user-2"},
		CreatedAt: time.Now(),
	}

	patches := gomonkey.ApplyMethodReturn(svc, "Send", n, nil)
	defer patches.Reset()

	body := `{"title":"System Update","content":"Maintenance tonight","type":"warning","target_ids":["user-1","user-2"]}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSendNotification_DefaultType(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	n := &model.Notification{ID: primitive.NewObjectID(), Type: "info"}

	patches := gomonkey.ApplyMethodReturn(svc, "Send", n, nil)
	defer patches.Reset()

	body := `{"title":"Test","content":"Hello world"}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSendNotification_MissingFields(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	body := `{}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSendNotification_MissingTitle(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	body := `{"content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSendNotification_InvalidJSON(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	c, w := newGinContext("POST", "/notifications/send", "bad")
	h.SendNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSendNotification_ServiceError(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "Send", nil, fmt.Errorf("db error"))
	defer patches.Reset()

	body := `{"title":"Test","content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── BroadcastNotification ──

func TestBroadcastNotification_Success(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	n := &model.Notification{
		ID:        primitive.NewObjectID(),
		Title:     "Broadcast",
		Content:   "All hands meeting",
		Type:      "info",
		TargetAll: true,
		CreatedAt: time.Now(),
	}

	patches := gomonkey.ApplyMethodReturn(svc, "Broadcast", n, nil)
	defer patches.Reset()

	body := `{"title":"Broadcast","content":"All hands meeting","type":"info"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBroadcastNotification_DefaultType(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	n := &model.Notification{ID: primitive.NewObjectID(), Type: "info"}

	patches := gomonkey.ApplyMethodReturn(svc, "Broadcast", n, nil)
	defer patches.Reset()

	body := `{"title":"Test","content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBroadcastNotification_MissingFields(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	body := `{}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBroadcastNotification_MissingContent(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	body := `{"title":"Test"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBroadcastNotification_InvalidJSON(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	c, w := newGinContext("POST", "/notifications/broadcast", "bad")
	h.BroadcastNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBroadcastNotification_ServiceError(t *testing.T) {
	svc := &notifsvc.Service{}
	h := NewNotificationHandler(svc)

	patches := gomonkey.ApplyMethodReturn(svc, "Broadcast", nil, fmt.Errorf("db error"))
	defer patches.Reset()

	body := `{"title":"Test","content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
