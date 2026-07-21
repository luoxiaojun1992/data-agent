package middleware

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// AuditLogger writes audit log entries via repository.AuditRepository,
// keeping the middleware layer free of any mongo-driver dependency.
type AuditLogger struct {
	repo repository.AuditRepository
}

// NewAuditLogger creates a new AuditLogger with the given audit repository.
func NewAuditLogger(repo repository.AuditRepository) *AuditLogger {
	return &AuditLogger{repo: repo}
}

// AuditMiddleware logs all CUD (Create/Update/Delete) operations to the audit
// repository. Logging is fire-and-forget: it uses context.Background() so the
// audit write survives request cancellation after the response is sent.
func (a *AuditLogger) AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip GET/HEAD/OPTIONS — only log mutations
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
		}

		// Capture request body for logging
		var body string
		if c.Request.Body != nil {
			bodyBytes, _ := io.ReadAll(c.Request.Body)
			body = string(bodyBytes)
			// Restore body for downstream handlers
			c.Request.Body = io.NopCloser(bytes.NewBuffer(bodyBytes))
		}

		// Process request
		c.Next()

		// Write audit log asynchronously (fire-and-forget for performance)
		go func() {
			userID, _ := c.Get("user_id")
			username, _ := c.Get("username")

			log := &model.AuditLog{
				Action:     method + " " + c.FullPath(),
				UserID:     toString(userID),
				Resource:   c.Request.URL.Path,
				Details:    truncateString(body, 1000),
				IP:         c.ClientIP(),
				UserAgent:  c.Request.UserAgent(),
				StatusCode: c.Writer.Status(),
				CreatedAt:  time.Now(),
			}
			// username is captured for future enrichment but model.AuditLog
			// does not persist it today; kept here to avoid losing context.
			_ = username

			// Best-effort insert — don't block the response
			_ = a.repo.Create(context.Background(), log)
		}()
	}
}

func toString(v interface{}) string {
	if v == nil {
		return ""
	}
	s, ok := v.(string)
	if !ok {
		return ""
	}
	return s
}

func truncateString(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "..."
}
