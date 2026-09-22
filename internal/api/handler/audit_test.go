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
	auditsvc "github.com/luoxiaojun1992/data-agent/internal/service/audit"
	mockaudit "github.com/luoxiaojun1992/data-agent/internal/service/audit/mocks"
)

func init() { gin.SetMode(gin.TestMode) }

// ── NewAuditHandler ──

func TestNewAuditHandler(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)
	if h == nil {
		t.Fatal("NewAuditHandler returned nil")
	}
	if h.svc == nil {
		t.Error("svc not set correctly")
	}
}

// ── ListAuditLogs ──

func TestListAuditLogs_Success(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	now := time.Now()
	result := &auditsvc.ListResult{
		Logs: []model.AuditLog{
			{
				ID:         "audit-id-001",
				Action:     "POST /api/v1/chat",
				ActionDesc: "Chat 对话",
				UserID:     "user-1",
				Resource:   "chat",
				Details:    "OK",
				IP:         "127.0.0.1",
				StatusCode: 200,
				CreatedAt:  now,
			},
		},
		Total: 1,
	}

	svc.On("List", mock.Anything).Return(result, nil)

	c, w := newGinContext("GET", "/audit/logs", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Chat 对话") {
		t.Errorf("body should contain action_desc: %s", w.Body.String())
	}
}

func TestListAuditLogs_WithFilters(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	result := &auditsvc.ListResult{Logs: []model.AuditLog{}, Total: 0}
	svc.On("List", mock.MatchedBy(func(p auditsvc.ListParams) bool {
		return p.Q == "admin" && p.Path == "sessions" && p.StatusClass == "4xx" &&
			p.Start == "2024-01-01" && p.End == "2024-12-31" && p.Page == 2 && p.PageSize == 50
	})).Return(result, nil)

	c, w := newGinContext("GET", "/audit/logs?q=admin&path=sessions&status_class=4xx&start=2024-01-01&end=2024-12-31&page=2&page_size=50", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAuditLogs_ViewerInjection(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	result := &auditsvc.ListResult{Logs: []model.AuditLog{}, Total: 0}
	svc.On("List", mock.MatchedBy(func(p auditsvc.ListParams) bool {
		return p.ViewerUserID == "u123" && p.IsSystemAdmin == true
	})).Return(result, nil)

	c, w := newGinContext("GET", "/audit/logs", "")
	c.Set("user_id", "u123")
	c.Set("role", "system_admin")
	h.ListAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListAuditLogs_InvalidPage(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	c, w := newGinContext("GET", "/audit/logs?page=abc", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListAuditLogs_InvalidPageSize(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	c, w := newGinContext("GET", "/audit/logs?page_size=500", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListAuditLogs_QTooLong(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	c, w := newGinContext("GET", "/audit/logs?q="+strings.Repeat("a", 101), "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListAuditLogs_ServiceValidationError(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	svc.On("List", mock.Anything).Return((*auditsvc.ListResult)(nil),
		auditsvc.NewValidationError("invalid status_class"))

	c, w := newGinContext("GET", "/audit/logs?status_class=7xx", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestListAuditLogs_ServiceError(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	svc.On("List", mock.Anything).Return((*auditsvc.ListResult)(nil), fmt.Errorf("db error"))

	c, w := newGinContext("GET", "/audit/logs", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

// ── ExportAuditLogs ──

func TestExportAuditLogs_Success(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	now := time.Now()
	logs := []model.AuditLog{
		{
			ID:         "audit-id-002",
			Action:     "POST /api/v1/chat",
			ActionDesc: "Chat 对话",
			UserID:     "user-1",
			Details:    "Login OK",
			IP:         "10.0.0.1",
			StatusCode: 200,
			CreatedAt:  now,
		},
	}

	svc.On("Export", mock.Anything).Return(logs, nil)

	body := `{"format":"csv","limit":100}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "时间,操作人,操作类型") {
		t.Errorf("should contain CSV header: %s", w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "Chat 对话") {
		t.Errorf("should use action_desc in CSV: %s", w.Body.String())
	}
}

func TestExportAuditLogs_InvalidFormat(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	body := `{"format":"json","limit":10}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExportAuditLogs_LimitExceeded(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	body := `{"format":"csv","limit":60000}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExportAuditLogs_InvalidJSON(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	c, w := newGinContext("POST", "/audit/logs/export", "bad")
	h.ExportAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExportAuditLogs_ServiceValidationError(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	svc.On("Export", mock.Anything).Return(([]model.AuditLog)(nil),
		auditsvc.NewValidationError("invalid start date"))

	body := `{"format":"csv","limit":10,"start":"bad"}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestExportAuditLogs_ServiceError(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	svc.On("Export", mock.Anything).Return(([]model.AuditLog)(nil), fmt.Errorf("db error"))

	body := `{"format":"csv","limit":10}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestExportAuditLogs_ContentTypeHeader(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	svc.On("Export", mock.Anything).Return([]model.AuditLog{}, nil)

	body := `{"format":"csv","limit":10,"start":"2024-01-01","end":"2024-12-31"}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	contentType := w.Header().Get("Content-Type")
	if !strings.Contains(contentType, "text/csv") {
		t.Errorf("Content-Type should be text/csv, got %s", contentType)
	}
	disposition := w.Header().Get("Content-Disposition")
	if !strings.Contains(disposition, "attachment") {
		t.Errorf("Content-Disposition should be attachment, got %s", disposition)
	}
}
