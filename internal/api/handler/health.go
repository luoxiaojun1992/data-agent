package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/service/monitor"
)

// HealthCheck is the unauthenticated health-check endpoint. It serves as the
// fallback (and historical /health contract) when no enhanced HealthService is
// wired, so the route never 404s on the database-unavailable startup path.
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// HealthHandler serves the SPEC-079 enhanced health-check payload. It delegates
// to monitor.HealthService when wired, falling back to the legacy HealthCheck
// when svc is nil (e.g. database-unavailable startup, or nil-handler tests).
type HealthHandler struct {
	svc *monitor.HealthService
}

// NewHealthHandler builds a HealthHandler around an optional HealthService.
func NewHealthHandler(svc *monitor.HealthService) *HealthHandler {
	return &HealthHandler{svc: svc}
}

// Check returns the full dependency health payload (HTTP 200 always — degraded
// is carried in the "status" field, not the HTTP code, so the frontend fetch
// keeps the dependency detail on every path).
func (h *HealthHandler) Check(c *gin.Context) {
	if h == nil || h.svc == nil {
		HealthCheck(c)
		return
	}
	c.JSON(http.StatusOK, h.svc.Check(c.Request.Context()))
}

// DBUnavailable responds with a 503 indicating the database is not ready.
func DBUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
}
