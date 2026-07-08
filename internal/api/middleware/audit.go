package middleware

import (
	"bytes"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"go.mongodb.org/mongo-driver/bson/primitive"
	"go.mongodb.org/mongo-driver/mongo"
)

// AuditLogger writes audit log entries to MongoDB.
type AuditLogger struct {
	coll *mongo.Collection
}

// NewAuditLogger creates a new AuditLogger with the given MongoDB collection.
func NewAuditLogger(coll *mongo.Collection) *AuditLogger {
	return &AuditLogger{coll: coll}
}

// AuditLogEntry represents an audit log entry to be written.
type AuditLogEntry struct {
	ID         primitive.ObjectID `bson:"_id,omitempty"`
	Action     string             `bson:"action"`
	UserID     string             `bson:"user_id"`
	Username   string             `bson:"username"`
	Resource   string             `bson:"resource"`
	Details    string             `bson:"details"`
	IP         string             `bson:"ip"`
	UserAgent  string             `bson:"user_agent"`
	StatusCode int                `bson:"status_code"`
	CreatedAt  time.Time          `bson:"created_at"`
}

// AuditMiddleware logs all CUD (Create/Update/Delete) operations to MongoDB.
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

			entry := AuditLogEntry{
				ID:         primitive.NewObjectID(),
				Action:     method + " " + c.FullPath(),
				UserID:     toString(userID),
				Username:   toString(username),
				Resource:   c.Request.URL.Path,
				Details:    truncateString(body, 1000),
				IP:         c.ClientIP(),
				UserAgent:  c.Request.UserAgent(),
				StatusCode: c.Writer.Status(),
				CreatedAt:  time.Now(),
			}

			// Best-effort insert — don't block the response
			_, _ = a.coll.InsertOne(c.Request.Context(), entry)
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
