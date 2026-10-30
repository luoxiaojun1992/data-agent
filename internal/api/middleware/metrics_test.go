package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
)

type fakeCounter struct {
	incrCalls int
	gotMetric metrics.Metric
	gotDelta  int64
}

func (f *fakeCounter) Incr(ctx context.Context, m metrics.Metric, at time.Time, delta int64) error {
	f.incrCalls++
	f.gotMetric = m
	f.gotDelta = delta
	return nil
}

func (f *fakeCounter) Stop() {}

func newMetricsCtx(path string) (*gin.Context, *httptest.ResponseRecorder) {
	gin.SetMode(gin.TestMode)
	w := httptest.NewRecorder()
	c, _ := gin.CreateTestContext(w)
	c.Request = httptest.NewRequest(http.MethodGet, path, nil)
	return c, w
}

// TestMetricsMiddleware_NilCounterIsNoop verifies a nil counter does not panic
// and does not break the request chain (SPEC-072).
func TestMetricsMiddleware_NilCounterIsNoop(t *testing.T) {
	h := MetricsMiddleware(nil)
	c, w := newMetricsCtx("/api/v1/foo")
	h(c)
	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}

// TestMetricsMiddleware_CountsAPIPath verifies the counter is incremented for
// /api/v1/* paths with the correct metric and delta (SPEC-072).
func TestMetricsMiddleware_CountsAPIPath(t *testing.T) {
	fc := &fakeCounter{}
	h := MetricsMiddleware(fc)
	c, _ := newMetricsCtx("/api/v1/sessions")
	h(c)

	if fc.incrCalls != 1 {
		t.Fatalf("incrCalls = %d, want 1", fc.incrCalls)
	}
	if fc.gotMetric != metrics.MetricAPICalls {
		t.Errorf("metric = %v, want %v", fc.gotMetric, metrics.MetricAPICalls)
	}
	if fc.gotDelta != 1 {
		t.Errorf("delta = %d, want 1", fc.gotDelta)
	}
}

// TestMetricsMiddleware_IgnoresNonAPIPath verifies non-/api/v1/ paths are not
// counted (auth/health/404 still pass through untouched).
func TestMetricsMiddleware_IgnoresNonAPIPath(t *testing.T) {
	fc := &fakeCounter{}
	h := MetricsMiddleware(fc)

	for _, p := range []string{"/health", "/login", "/"} {
		c, _ := newMetricsCtx(p)
		h(c)
	}
	if fc.incrCalls != 0 {
		t.Errorf("incrCalls = %d, want 0 for non-/api/v1/ paths", fc.incrCalls)
	}
}
