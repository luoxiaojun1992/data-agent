package handler

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
)

// hermesProxyHandler rewrites the request path to the captured "*path" param
// and forwards to the Hermes upstream. This asserts the path rewrite and the
// upstream round-trip (the proxy is a stdlib httputil single-host proxy).
//
// The request is given a cancellable context (with a non-nil Done channel) so
// that httputil.ReverseProxy takes its context-cancellation path instead of
// probing the CloseNotifier interface — which httptest.ResponseRecorder does
// not implement (and gin's responseWriter would then assert on).

func TestHermesProxyHandler_ForwardsWithRewrittenPath(t *testing.T) {
	var gotPath string
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.WriteHeader(http.StatusOK)
	}))
	defer target.Close()

	h := hermesProxyHandler(target.URL)
	c, w := newGinContext("GET", "/api/v1/hermes/anything", "")
	c.Params = gin.Params{{Key: "path", Value: "/rewritten/path"}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	c.Request = c.Request.WithContext(ctx)
	h(c)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from upstream, got %d", w.Code)
	}
	if gotPath != "/rewritten/path" {
		t.Errorf("upstream received path %q, want /rewritten/path", gotPath)
	}
}

func TestRegisterHermesProxy_EmptyURLNoop(t *testing.T) {
	gin.SetMode(gin.TestMode)
	r := gin.New()
	RegisterHermesProxy(r, "")
	// Empty URL is a no-op: no /api/v1/hermes route should be registered.
	if len(r.Routes()) != 0 {
		t.Errorf("expected no routes registered for empty HERMES_URL, got %d", len(r.Routes()))
	}
}
