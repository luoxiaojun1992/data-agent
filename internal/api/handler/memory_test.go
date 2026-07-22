package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func newMemoryGin(method, path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(method, path, nil)
	return c, w
}

func TestMemoryHandler_Search_MissingQuery(t *testing.T) {
	h := NewMemoryHandler(nil, "data-agent")
	c, w := newMemoryGin("GET", "/memory/search")
	h.Search(c)
	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestNewMemoryHandler(t *testing.T) {
	h := NewMemoryHandler(nil, "app")
	if h == nil {
		t.Fatal("handler should not be nil")
	}
	if h.appName != "app" {
		t.Errorf("appName = %q", h.appName)
	}
}
