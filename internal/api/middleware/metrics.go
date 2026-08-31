package middleware

import (
	"strings"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
)

// MetricsMiddleware increments the api_calls counter for every data-agent
// backend HTTP request under /api/v1/* (SPEC-072). The increment is a buffered
// O(1) write — no per-request DB IO. Applied globally so auth/404 responses
// are counted too. A nil counter is a no-op.
func MetricsMiddleware(counter metrics.Counter) gin.HandlerFunc {
	return func(c *gin.Context) {
		if counter != nil && strings.HasPrefix(c.Request.URL.Path, "/api/v1/") {
			_ = counter.Incr(c.Request.Context(), metrics.MetricAPICalls, time.Now(), 1)
		}
		c.Next()
	}
}
