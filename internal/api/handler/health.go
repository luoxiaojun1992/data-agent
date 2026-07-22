package handler

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
)

// HealthCheck is the unauthenticated health-check endpoint.
func HealthCheck(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{
		"status": "ok",
		"time":   time.Now().UTC().Format(time.RFC3339),
	})
}

// DBUnavailable responds with a 503 indicating the database is not ready.
func DBUnavailable(c *gin.Context) {
	c.JSON(http.StatusServiceUnavailable, gin.H{"error": consts.ErrDBUnavailable})
}
