package handler

import (
	"encoding/base64"
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"

	"github.com/luoxiaojun1992/data-agent/internal/domain/consts"
	"github.com/luoxiaojun1992/data-agent/internal/service/voice"
)

// VoiceHandler exposes the chunked voice-upload endpoints (SPEC-099). It shares
// the chat-view permission with the enhance/redact endpoints — applied at the
// route group in routes.go. The transcription result is returned verbatim.
type VoiceHandler struct {
	svc *voice.Service
}

// NewVoiceHandler creates the voice handler.
func NewVoiceHandler(svc *voice.Service) *VoiceHandler {
	return &VoiceHandler{svc: svc}
}

// RegisterVoiceRoutes registers the start/chunk/finish endpoints on the
// authenticated /api/v1/voice group (PermChatView already applied).
func RegisterVoiceRoutes(rg *gin.RouterGroup, h *VoiceHandler) {
	rg.POST("/start", h.Start)
	rg.POST("/chunk", h.Chunk)
	rg.POST("/finish", h.Finish)
}

// Start begins a recording session and returns a request_id for chunk uploads.
func (h *VoiceHandler) Start(c *gin.Context) {
	var req struct {
		Lang string `json:"lang"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	requestID := h.svc.Start(c.Request.Context(), c.GetString("user_id"), req.Lang)
	c.JSON(http.StatusOK, gin.H{"request_id": requestID})
}

// Chunk buffers one audio chunk (base64 int16 PCM) into the session.
func (h *VoiceHandler) Chunk(c *gin.Context) {
	var req struct {
		RequestID string `json:"request_id"`
		Seq       int64  `json:"seq"`
		Audio     string `json:"audio"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RequestID == "" || req.Audio == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	audio, err := base64.StdEncoding.DecodeString(req.Audio)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	received, err := h.svc.Chunk(c.Request.Context(), req.RequestID, c.GetString("user_id"), req.Seq, audio)
	switch {
	case errors.Is(err, voice.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "voice session not found"})
	case errors.Is(err, voice.ErrSeqMismatch):
		c.JSON(http.StatusConflict, gin.H{"error": "chunk seq out of order"})
	case errors.Is(err, voice.ErrTooLarge):
		c.JSON(http.StatusRequestEntityTooLarge, gin.H{"error": "audio exceeds size limit"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"ok": true, "received": received})
	}
}

// Finish merges the chunks, transcribes them, and returns the raw text.
func (h *VoiceHandler) Finish(c *gin.Context) {
	var req struct {
		RequestID string `json:"request_id"`
	}
	if err := c.ShouldBindJSON(&req); err != nil || req.RequestID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": consts.ErrInvalidReq})
		return
	}
	text, err := h.svc.Finish(c.Request.Context(), req.RequestID, c.GetString("user_id"))
	switch {
	case errors.Is(err, voice.ErrNotFound):
		c.JSON(http.StatusNotFound, gin.H{"error": "voice session not found"})
	case errors.Is(err, voice.ErrEmptyAudio):
		c.JSON(http.StatusBadRequest, gin.H{"error": "no audio received"})
	case errors.Is(err, voice.ErrUnavailable):
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": "voice service unavailable"})
	case err != nil:
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
	default:
		c.JSON(http.StatusOK, gin.H{"text": text})
	}
}
