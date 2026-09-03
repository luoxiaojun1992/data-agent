package handler

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
)

func TestHealthCheck(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/health", nil)
	HealthCheck(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v", resp["status"])
	}
	if resp["time"] == nil {
		t.Errorf("time field missing")
	}
	// time should be parseable RFC3339.
	if _, err := time.Parse(time.RFC3339, resp["time"].(string)); err != nil {
		t.Errorf("invalid time format: %v", err)
	}
}

func TestDBUnavailable(t *testing.T) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/", nil)
	DBUnavailable(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["error"] == nil {
		t.Errorf("error field missing")
	}
}

func TestHealthHandler_Check_WithService(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := monitor.NewHealthService("v1.5.0",
		monitor.Probe{Name: "mongodb", Check: func(_ context.Context) error { return nil }},
	)
	h := NewHealthHandler(svc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/health", nil)

	h.Check(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	deps, ok := resp["dependencies"].(map[string]any)
	if !ok {
		t.Fatalf("dependencies missing or wrong type: %T", resp["dependencies"])
	}
	if deps["mongodb"] == nil {
		t.Errorf("mongodb dependency missing from response")
	}
	if resp["latency_ms"] == nil {
		t.Errorf("latency_ms missing from response")
	}
}

func TestHealthHandler_Check_NilService_FallsBack(t *testing.T) {
	gin.SetMode(gin.TestMode)
	h := NewHealthHandler(nil)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/health", nil)

	h.Check(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok (fallback)", resp["status"])
	}
}

func TestHealthHandler_Check_DegradedStill200(t *testing.T) {
	gin.SetMode(gin.TestMode)
	svc := monitor.NewHealthService("v1",
		monitor.Probe{Name: "redis", Check: func(_ context.Context) error { return errors.New("down") }},
	)
	h := NewHealthHandler(svc)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest("GET", "/api/v1/health", nil)

	h.Check(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 (degraded carried in status, not HTTP code), got %d", w.Code)
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "degraded" {
		t.Errorf("status = %v, want degraded", resp["status"])
	}
}
