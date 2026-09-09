package handler

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/service/pii"
)

// fakeRedactor is a test double for security.Redactor.
type fakeRedactor struct {
	out string
	err error
}

func (f *fakeRedactor) Redact(_ context.Context, text string) (string, error) {
	if f.err != nil {
		return "", f.err
	}
	return f.out, nil
}

func newRedactGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	req := httptest.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	c, _ := gin.CreateTestContext(w)
	c.Request = req
	return c, w
}

// TestRedactHandler_Success verifies a successful redaction round-trip.
func TestRedactHandler_Success(t *testing.T) {
	h := NewRedactHandler(&fakeRedactor{out: "电话 <PII>"})
	c, w := newRedactGin("POST", "/redact", `{"text":"电话 13800138000"}`)
	h.Redact(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
	if !strings.Contains(w.Body.String(), "PII") {
		t.Errorf("expected redacted text, got %s", w.Body.String())
	}
}

// TestRedactHandler_EmptyText verifies empty text is rejected with 400.
func TestRedactHandler_EmptyText(t *testing.T) {
	h := NewRedactHandler(&fakeRedactor{})
	c, w := newRedactGin("POST", "/redact", `{"text":""}`)
	h.Redact(c)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", w.Code)
	}
}

// TestRedactHandler_ServiceError verifies a Presidio failure surfaces as 500.
func TestRedactHandler_ServiceError(t *testing.T) {
	h := NewRedactHandler(&fakeRedactor{err: errors.New("analyzer down")})
	c, w := newRedactGin("POST", "/redact", `{"text":"电话 13800138000"}`)
	h.Redact(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
	if w.Body.String() == "" {
		t.Errorf("expected non-empty error body")
	}
}

// TestRedactHandler_DisabledSwitch verifies the pii switch-off path returns
// the original text with 200 (redaction skipped, not failed).
func TestRedactHandler_DisabledSwitch(t *testing.T) {
	h := NewRedactHandler(&fakeRedactor{err: pii.ErrDisabled})
	c, w := newRedactGin("POST", "/redact", `{"text":"电话 13800138000"}`)
	h.Redact(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !strings.Contains(w.Body.String(), "13800138000") {
		t.Errorf("expected original text, got %s", w.Body.String())
	}
}

// TestRedactHandler_NilRedactor verifies an unconfigured redactor fails 500.
func TestRedactHandler_NilRedactor(t *testing.T) {
	h := NewRedactHandler(nil)
	c, w := newRedactGin("POST", "/redact", `{"text":"电话 13800138000"}`)
	h.Redact(c)
	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d", w.Code)
	}
}

// TestRegisterRedactRoute verifies route registration wires POST /redact.
func TestRegisterRedactRoute(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	h := NewRedactHandler(&fakeRedactor{out: "x"})
	rg := r.Group("/api/v1/chat")
	RegisterRedactRoute(rg, h)

	req := httptest.NewRequest("POST", "/api/v1/chat/redact", strings.NewReader(`{"text":"hello"}`))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected 200, got %d: %s", w.Code, w.Body.String())
	}
}
