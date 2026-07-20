package handler

import (
	"fmt"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/mock"
	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	mocknotif "github.com/luoxiaojun1992/data-agent/internal/service/notification/mocks"
	"go.mongodb.org/mongo-driver/bson/primitive"
)

func init() { gin.SetMode(gin.TestMode) }

// ── NewNotificationHandler ──

func TestNewNotificationHandler(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)
	if h == nil {
		t.Fatal("NewNotificationHandler returned nil")
	}
	if h.svc == nil {
		t.Error("svc not set correctly")
	}
}

// ── ListNotifications ──

func TestListNotifications_Success(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	notifs := []model.Notification{
		{ID: primitive.NewObjectID(), Title: "Test", Content: "Hello", Type: "info", CreatedAt: time.Now()},
	}

	svc.On("ListForUser", mock.Anything, mock.Anything).Return(mock.Anything).Return( notifs, nil)

	c, w := newGinContext("GET", "/notifications", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListNotifications_WithLimit(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("ListForUser", mock.Anything, mock.Anything).Return(mock.Anything).Return( []model.Notification{}, nil)

	c, w := newGinContext("GET", "/notifications?limit=10", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListNotifications_DefaultLimit(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("ListForUser", mock.Anything, mock.Anything).Return(mock.Anything).Return( []model.Notification{}, nil)

	c, w := newGinContext("GET", "/notifications", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListNotifications_Error(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("ListForUser", mock.Anything, mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/notifications", "")
	c.Set("user_id", "user-1")
	h.ListNotifications(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── UnreadCount ──

func TestUnreadCount_Success(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("UnreadCount", mock.Anything).Return(mock.Anything).Return( int64(5), nil)

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
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("UnreadCount", mock.Anything).Return(mock.Anything).Return( int64(0), nil)

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
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("UnreadCount", mock.Anything).Return(mock.Anything).Return( int64(0), fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/notifications/unread-count", "")
	c.Set("user_id", "user-1")
	h.UnreadCount(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── MarkRead ──

func TestMarkRead_Success(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("MarkRead", mock.Anything, mock.Anything).Return(mock.Anything).Return( nil)

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
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("MarkRead", mock.Anything, mock.Anything).Return(mock.Anything).Return( fmt.Errorf("invalid id"))

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
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("MarkAllRead", mock.Anything).Return(mock.Anything).Return( nil)

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
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("MarkAllRead", mock.Anything).Return(mock.Anything).Return( fmt.Errorf("db error"))

	c, w := newGinContext("POST", "/notifications/read-all", "")
	c.Set("user_id", "user-1")
	h.MarkAllRead(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── SendNotification ──

func TestSendNotification_Success(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	n := &model.Notification{
		ID:        primitive.NewObjectID(),
		Title:     "System Update",
		Content:   "Maintenance tonight",
		Type:      "warning",
		TargetIDs: []string{"user-1", "user-2"},
		CreatedAt: time.Now(),
	}

	svc.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( n, nil)

	body := `{"title":"System Update","content":"Maintenance tonight","type":"warning","target_ids":["user-1","user-2"]}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSendNotification_DefaultType(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	n := &model.Notification{ID: primitive.NewObjectID(), Type: "info"}

	svc.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( n, nil)

	body := `{"title":"Test","content":"Hello world"}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestSendNotification_MissingFields(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	body := `{}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSendNotification_MissingTitle(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	body := `{"content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSendNotification_InvalidJSON(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	c, w := newGinContext("POST", "/notifications/send", "bad")
	h.SendNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestSendNotification_ServiceError(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("Send", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("db error"))

	body := `{"title":"Test","content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/send", body)
	h.SendNotification(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── BroadcastNotification ──

func TestBroadcastNotification_Success(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	n := &model.Notification{
		ID:        primitive.NewObjectID(),
		Title:     "Broadcast",
		Content:   "All hands meeting",
		Type:      "info",
		TargetAll: true,
		CreatedAt: time.Now(),
	}

	svc.On("Broadcast", mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( n, nil)

	body := `{"title":"Broadcast","content":"All hands meeting","type":"info"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBroadcastNotification_DefaultType(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	n := &model.Notification{ID: primitive.NewObjectID(), Type: "info"}

	svc.On("Broadcast", mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( n, nil)

	body := `{"title":"Test","content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

func TestBroadcastNotification_MissingFields(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	body := `{}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBroadcastNotification_MissingContent(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	body := `{"title":"Test"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBroadcastNotification_InvalidJSON(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	c, w := newGinContext("POST", "/notifications/broadcast", "bad")
	h.BroadcastNotification(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestBroadcastNotification_ServiceError(t *testing.T) {
	svc := mocknotif.NewNotificationService(t)
	h := NewNotificationHandler(svc)

	svc.On("Broadcast", mock.Anything, mock.Anything, mock.Anything).Return(mock.Anything).Return( nil, fmt.Errorf("db error"))

	body := `{"title":"Test","content":"Hello"}`
	c, w := newGinContext("POST", "/notifications/broadcast", body)
	h.BroadcastNotification(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}
