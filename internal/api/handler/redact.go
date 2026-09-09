package handler

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	"github.com/luoxiaojun1992/data-agent/internal/service/pii"
)

// RedactHandler exposes the chat-input PII redaction endpoint backed by the
// Presidio services (SPEC-093). It shares the chat route group with the
// enhance endpoint, so the permission is identical (chat:view).
type RedactHandler struct {
	redactor security.Redactor
}

// NewRedactHandler creates a redact handler. redactor may be nil (Presidio
// unconfigured) — requests then fail with 500 so the frontend surfaces the
// error instead of silently passing PII through.
func NewRedactHandler(redactor security.Redactor) *RedactHandler {
	return &RedactHandler{redactor: redactor}
}

// RegisterRedactRoute registers POST /redact on the chat router group.
func RegisterRedactRoute(rg *gin.RouterGroup, h *RedactHandler) {
	rg.POST("/redact", h.Redact)
}

// Redact redacts PII from the given text via Presidio.
//
// Responses:
//   - 400: empty text (invalid request)
//   - 200: {redacted}: success, or the pii_redaction_enabled switch is off
//     (redaction skipped by admin config — the original text is returned)
//   - 500: Presidio unavailable or call failure (frontend surfaces the error)
func (h *RedactHandler) Redact(c *gin.Context) {
	var req struct {
		Text string `json:"text"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.Text == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	if h.redactor == nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "redaction service not configured"})
		return
	}
	redacted, err := h.redactor.Redact(c.Request.Context(), req.Text)
	if err != nil {
		// The admin turned PII redaction off: redaction is skipped, not failed.
		if errors.Is(err, pii.ErrDisabled) {
			c.JSON(http.StatusOK, gin.H{"redacted": req.Text})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	c.JSON(http.StatusOK, gin.H{"redacted": redacted})
}
