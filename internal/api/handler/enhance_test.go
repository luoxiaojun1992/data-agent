package handler

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/service/enhance"
)

func newEnhanceGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestEnhanceHandler_Success(t *testing.T) {
	svc := enhance.NewService(nil, nil, nil) // fallback returns original prompt
	h := NewEnhanceHandler(svc)
	c, w := newEnhanceGin("POST", "/enhance", `{"prompt":"分析营收"}`)
	h.Enhance(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	var resp map[string]any
	_ = json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["enhanced"] != "分析营收" {
		t.Errorf("enhanced = %v", resp["enhanced"])
	}
}

func TestEnhanceHandler_EmptyPrompt(t *testing.T) {
	svc := enhance.NewService(nil, nil, nil)
	h := NewEnhanceHandler(svc)
	c, w := newEnhanceGin("POST", "/enhance", `{"prompt":""}`)
	h.Enhance(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestEnhanceHandler_InvalidBody(t *testing.T) {
	svc := enhance.NewService(nil, nil, nil)
	h := NewEnhanceHandler(svc)
	c, w := newEnhanceGin("POST", "/enhance", "not-json")
	h.Enhance(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNewEnhanceHandler(t *testing.T) {
	h := NewEnhanceHandler(nil)
	if h == nil {
		t.Error("handler should not be nil")
	}
}
