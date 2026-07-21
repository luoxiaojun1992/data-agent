package handler

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func newIMBindGin(method, path, body string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, strings.NewReader(body))
	c.Request.Header.Set("Content-Type", "application/json")
	return c, w
}

func TestIMBindHandler_NilSvc_Get(t *testing.T) {
	h := NewIMBindHandler(nil)
	c, w := newIMBindGin("GET", "/im/bind", "")
	c.Set("user_id", "u1")
	h.Get(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestIMBindHandler_NilSvc_Update(t *testing.T) {
	h := NewIMBindHandler(nil)
	c, w := newIMBindGin("PUT", "/im/bind", `{"k":"v"}`)
	c.Set("user_id", "u1")
	h.Update(c)
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("expected 503, got %d", w.Code)
	}
}

func TestNewIMBindHandler(t *testing.T) {
	h := NewIMBindHandler(nil)
	if h == nil {
		t.Error("handler should not be nil")
	}
}
