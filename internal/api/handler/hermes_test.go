package handler

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestRegisterHermesProxy_EmptyURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	// No-op when URL empty: no route registered.
	RegisterHermesProxy(router, "")
	w := httptest.NewRecorder()
	req := httptest.NewRequest("GET", "/api/v1/hermes/foo", nil)
	router.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404 (route not registered), got %d", w.Code)
	}
}

func TestRegisterHermesProxy_WithURL(t *testing.T) {
	gin.SetMode(gin.TestMode)
	router := gin.New()
	RegisterHermesProxy(router, "http://localhost:9999")
	routes := router.Routes()
	found := false
	for _, r := range routes {
		if r.Path == "/api/v1/hermes/*path" {
			found = true
			break
		}
	}
	if !found {
		t.Error("hermes proxy route should be registered when URL provided")
	}
}

func TestNewHermesHandler(t *testing.T) {
	// hermesProxyHandler returns a gin.HandlerFunc; verify it's non-nil.
	h := hermesProxyHandler("http://localhost:9999")
	if h == nil {
		t.Error("hermesProxyHandler should return non-nil")
	}
}
