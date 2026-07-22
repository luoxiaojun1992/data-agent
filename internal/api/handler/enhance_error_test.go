package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/service/enhance"
)

// TestEnhanceHandler_EmptyPromptAfterJSONParse_ErrorMsg verifies the branch
// where JSON parses successfully but the prompt is an empty string. The
// handler should respond 400 with the ErrInvalidReq message.
func TestEnhanceHandler_EmptyPromptAfterJSONParse_ErrorMsg(t *testing.T) {
	svc := enhance.NewService(nil, nil, nil)
	h := NewEnhanceHandler(svc)
	c, w := newEnhanceGin("POST", "/enhance", `{"prompt":""}`)
	h.Enhance(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), consts.ErrInvalidReq) {
		t.Errorf("expected error message %q, got %s", consts.ErrInvalidReq, w.Body.String())
	}
}

// TestRegisterEnhanceRoute verifies that route registration wires the enhance
// endpoint without panicking and dispatches POST /enhance to the handler.
// This exercises the previously-uncovered RegisterEnhanceRoute function.
func TestRegisterEnhanceRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	svc := enhance.NewService(nil, nil, nil)
	h := NewEnhanceHandler(svc)
	rg := r.Group("/api")
	RegisterEnhanceRoute(rg, h)

	body := `{"prompt":"分析营收"}`
	req := httptest.NewRequest("POST", "/api/enhance", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "分析营收") {
		t.Errorf("expected enhanced echo, got %s", w.Body.String())
	}
}

// TestEnhanceHandler_NilSvc ensures the handler can be constructed with a nil
// service without panicking during construction (defensive constructor check).
func TestEnhanceHandler_NilSvc(t *testing.T) {
	h := NewEnhanceHandler(nil)
	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.svc != nil {
		t.Errorf("svc should be nil, got %v", h.svc)
	}
}
