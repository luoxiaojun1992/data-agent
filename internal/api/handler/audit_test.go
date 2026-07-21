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
				Action:     "user.login",
				UserID:     "user-1",
				Resource:   "auth",
				Details:    "Login successful",
				IP:         "127.0.0.1",
				StatusCode: 200,
				CreatedAt:  now,
			},
		},
		Total: 1,
	}

	svc.On("List", mock.Anything).Return( result, nil)

	c, w := newGinContext("GET", "/audit/logs", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "user.login") {
		t.Errorf("body should contain user.login: %s", w.Body.String())
	}
}

func TestListAuditLogs_WithFilters(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	result := &auditsvc.ListResult{
		Logs:  []model.AuditLog{},
		Total: 0,
	}

	svc.On("List", mock.Anything).Return( result, nil)

	c, w := newGinContext("GET", "/audit/logs?action=user.login&user_id=user-1&start=2024-01-01&end=2024-12-31&skip=10&limit=50", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestListAuditLogs_DefaultPagination(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	result := &auditsvc.ListResult{
		Logs:  []model.AuditLog{},
		Total: 0,
	}

	svc.On("List", mock.Anything).Return( result, nil)

	// No pagination params — defaults to skip=0, limit=20
	c, w := newGinContext("GET", "/audit/logs", "")
	h.ListAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

func TestListAuditLogs_ServiceError(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	svc.On("List", mock.Anything).Return( (*auditsvc.ListResult)(nil), fmt.Errorf("db error"))

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
	result := &auditsvc.ListResult{
		Logs: []model.AuditLog{
			{
				ID:         "audit-id-002",
				Action:     "user.login",
				UserID:     "user-1",
				Details:    "Login OK",
				IP:         "10.0.0.1",
				StatusCode: 200,
				CreatedAt:  now,
			},
		},
		Total: 1,
	}

	svc.On("List", mock.Anything).Return( result, nil)

	body := `{"action":"user.login","limit":100}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	// Verify CSV headers present
	if !strings.Contains(w.Body.String(), "时间,操作人,操作类型") {
		t.Errorf("should contain CSV header: %s", w.Body.String())
	}
}

func TestExportAuditLogs_DefaultLimit(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	result := &auditsvc.ListResult{
		Logs:  []model.AuditLog{},
		Total: 0,
	}

	svc.On("List", mock.Anything).Return( result, nil)

	// No limit specified — defaults to 5000
	body := `{}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}

func TestExportAuditLogs_LimitExceeded(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	body := `{"limit":60000}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "50,000") {
		t.Errorf("should mention 50,000 limit: %s", w.Body.String())
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

func TestExportAuditLogs_ServiceError(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	svc.On("List", mock.Anything).Return( (*auditsvc.ListResult)(nil), fmt.Errorf("db error"))

	body := `{"limit":10}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
}

func TestExportAuditLogs_ContentTypeHeader(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	result := &auditsvc.ListResult{
		Logs:  []model.AuditLog{},
		Total: 0,
	}

	svc.On("List", mock.Anything).Return( result, nil)

	body := `{"start":"2024-01-01","end":"2024-12-31"}`
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

func TestExportAuditLogs_MultipleLogs(t *testing.T) {
	svc := mockaudit.NewAuditService(t)
	h := NewAuditHandler(svc)

	now := time.Now()
	result := &auditsvc.ListResult{
		Logs: []model.AuditLog{
			{ID: "audit-id-003", Action: "a", UserID: "u1", Details: "d1", IP: "1.1.1.1", StatusCode: 200, CreatedAt: now},
			{ID: "audit-id-004", Action: "b", UserID: "u2", Details: "d2", IP: "2.2.2.2", StatusCode: 404, CreatedAt: now},
			{ID: "audit-id-005", Action: "c", UserID: "u3", Details: "d3", IP: "3.3.3.3", StatusCode: 500, CreatedAt: now},
		},
		Total: 3,
	}

	svc.On("List", mock.Anything).Return( result, nil)

	body := `{"limit":100}`
	c, w := newGinContext("POST", "/audit/logs/export", body)
	h.ExportAuditLogs(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Should have header + 3 data rows
	lines := strings.Split(strings.TrimSpace(w.Body.String()), "\n")
	if len(lines) < 4 {
		t.Errorf("expected at least 4 lines (header + 3 rows), got %d", len(lines))
	}
}
