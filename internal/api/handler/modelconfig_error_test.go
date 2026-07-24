package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/mock"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	configmocks "github.com/luoxiaojun1992/data-agent/internal/service/config/mocks"
)

// TestModelConfigHandler_Put_ServiceError verifies Put returns 500 when the
// underlying config service Upsert call fails.
func TestModelConfigHandler_Put_ServiceError(t *testing.T) {
	svc := configmocks.NewService(t)
	svc.On("Upsert", mock.Anything, "models", "key1", "val1").Return(errStr("db down"))
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("PUT", "/models", `{"key":"key1","value":"val1"}`)
	h.Put(c)
	if w.Code != http.StatusInternalServerError {
		t.Errorf("expected 500, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestModelConfigHandler_Get_ServiceErrorReturningEmptyList ensures the
// success-path branch with an empty (but non-nil) slice still serializes
// correctly. This exercises the Get happy path with no rows.
func TestModelConfigHandler_Get_ServiceErrorReturningEmptyList(t *testing.T) {
	svc := configmocks.NewService(t)
	svc.On("GetAll", mock.Anything, "models").Return([]model.SystemConfig{}, nil)
	h := NewModelConfigHandler(svc, nil)
	c, w := newModelCfgGin("GET", "/models", "")
	h.Get(c)
	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d", w.Code)
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty body")
	}
}

// TestRegisterModelConfigRoutes verifies that RegisterModelConfigRoutes wires
// GET/PUT /models on the given router group. This exercises the previously
// uncovered RegisterModelConfigRoutes function.
func TestRegisterModelConfigRoutes(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := configmocks.NewService(t)
	svc.On("GetAll", mock.Anything, "models").Return([]model.SystemConfig{{Key: "k", Value: "v"}}, nil)
	svc.On("Upsert", mock.Anything, "models", "k", "v").Return(nil)
	h := NewModelConfigHandler(svc, nil)
	api := r.Group("/api/v1")
	RegisterModelConfigRoutes(api, h)

	// GET /api/v1/models → 200
	req := httptest.NewRequest("GET", "/api/v1/models", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Get expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "models") {
		t.Errorf("expected models field in body, got %s", w.Body.String())
	}

	// PUT /api/v1/models → 200
	req = httptest.NewRequest("PUT", "/api/v1/models", strings.NewReader(`{"key":"k","value":"v"}`))
	req.Header.Set("Content-Type", "application/json")
	w = httptest.NewRecorder()
	r.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Errorf("Put expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "已保存") {
		t.Errorf("expected saved message in body, got %s", w.Body.String())
	}
}
