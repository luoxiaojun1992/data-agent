package middleware

import (
	"context"
	"net/http"
	"net/url"
	"strings"
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
//
// Security: the request body is intentionally NOT captured — a large body (e.g.
// an uploaded file) could exhaust memory and bloat the DB, and bodies may
// contain credentials. Only the URL path (Resource) and sanitized query string
// (Details) are recorded. No request headers are read, so the Authorization
// token never reaches the audit log.
func (a *AuditLogger) AuditMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		// Skip GET/HEAD/OPTIONS — only log mutations
		method := c.Request.Method
		if method == http.MethodGet || method == http.MethodHead || method == http.MethodOptions {
			c.Next()
			return
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
				Details:    truncateString(sanitizeQuery(c.Request.URL.RawQuery), 1000),
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

// sensitiveQueryKeys lists query-string keys whose values carry credentials and
// must be redacted before writing to the audit log.
var sensitiveQueryKeys = map[string]struct{}{
	"token":         {},
	"access_token":  {},
	"access-token":  {},
	"accesstoken":   {},
	"auth_token":    {},
	"auth-token":    {},
	"authtoken":     {},
	"authorization": {},
	"api_key":       {},
	"api-key":       {},
	"apikey":        {},
	"secret":        {},
	"password":      {},
	"passwd":        {},
	"pwd":           {},
}

// sanitizeQuery redacts the values of sensitive query-string keys (token /
// credential class) to "***" and leaves other parameters intact, preserving the
// original parameter order. It is used in place of recording the request body,
// so no password, token, or file content ends up in the audit log. Sensitive
// keys are matched case-insensitively and after URL-decoding the key name.
func sanitizeQuery(rawQuery string) string {
	if rawQuery == "" {
		return ""
	}
	parts := strings.Split(rawQuery, "&")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if part == "" {
			continue
		}
		kv := strings.SplitN(part, "=", 2)
		key := kv[0]
		if decoded, err := url.QueryUnescape(key); err == nil {
			key = decoded
		}
		if _, sensitive := sensitiveQueryKeys[strings.ToLower(key)]; sensitive {
			// Keep the original (possibly encoded) key, redact the value.
			out = append(out, kv[0]+"=***")
			continue
		}
		out = append(out, part)
	}
	return strings.Join(out, "&")
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
