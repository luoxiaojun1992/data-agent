package chat

import (
	"encoding/json"
	"fmt"
	"log"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/luoxiaojun1992/data-agent/internal/domain/agent"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

// Service handles real-time chat operations with SSE streaming.
type Service struct {
	engine   *agent.Engine
	sessions *Manager
	context  *ContextManager
	auditor  *security.Auditor
	cbReg    *security.CircuitBreakerRegistry
}

// NewService creates a new Chat Service.
func NewService(engine *agent.Engine, sessions *Manager, auditor *security.Auditor, cbReg *security.CircuitBreakerRegistry) *Service {
	return &Service{
		engine:   engine,
		sessions: sessions,
		context:  NewContextManager(128000, 0.5), // Default 128K context, 50% threshold
		auditor:  auditor,
		cbReg:    cbReg,
	}
}

// ChatRequest represents an incoming chat request.
type ChatRequest struct {
	SessionID string          `json:"session_id,omitempty"`
	Model     string          `json:"model,omitempty"`
	Messages  []agent.Message `json:"messages"`
	Message   string          `json:"message,omitempty"` // legacy single-message field from frontend
	Stream    bool            `json:"stream"`
}

// HandleChat handles a chat completion request with optional SSE streaming.
func (s *Service) HandleChat(c *gin.Context) {
	var req ChatRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}

	// Convert legacy single message to messages array
	if len(req.Messages) == 0 && req.Message != "" {
		req.Messages = []agent.Message{{Role: "user", Content: req.Message}}
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages required"})
		return
	}

	// Validate or create session
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)

	if req.SessionID == "" {
		sess, err := s.sessions.Create(userIDStr, "chat")
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to create session"})
			return
		}
		req.SessionID = sess.ID
	} else {
		sess, err := s.sessions.Get(req.SessionID)
		if err != nil || sess.UserID != userIDStr {
			c.JSON(http.StatusUnauthorized, gin.H{"error": "invalid or unauthorized session"})
			return
		}
		_ = s.sessions.Renew(req.SessionID)
	}

	// Context window management
	if s.context.ShouldCompress(agent.EstimateTotalTokens(req.Messages)) {
		req.Messages = s.context.TruncateMessages(req.Messages, 64000)
	}

	// Stream mode
	if req.Stream {
		log.Printf("[DEBUG chat] HandleChat: routing to handleStream, stream=%v", req.Stream)
		s.handleStream(c, req)
		return
	}

	log.Printf("[DEBUG chat] HandleChat: routing to non-stream, messages=%d", len(req.Messages))
	// Non-stream mode
	agentReq := agent.ChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
	}

	cb := s.cbReg.GetOrCreate("chat")
	err := cb.Call(func() error {
		resp, err := s.engine.Run(c.Request.Context(), agentReq)
		if err != nil {
			return err
		}
		c.JSON(http.StatusOK, gin.H{
			"session_id": req.SessionID,
			"content":    resp.Content,
			"usage":      resp.Usage,
		})
		return nil
	})

	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
	}
}

// handleStream handles SSE streaming responses.
func (s *Service) handleStream(c *gin.Context, req ChatRequest) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	// Send session ID as first event
	sessionData, _ := json.Marshal(map[string]string{"session_id": req.SessionID})
	fmt.Fprintf(c.Writer, "data: %s\n\n", string(sessionData))
	flusher.Flush()

	agentReq := agent.ChatRequest{
		Model:    req.Model,
		Messages: req.Messages,
	}

	log.Printf("[DEBUG chat] handleStream: model=%q messages=%d stream=%v first_msg=%q",
		req.Model, len(req.Messages), req.Stream,
		func() string { if len(req.Messages) > 0 { return req.Messages[0].Content }; return "" }())

	err := s.engine.RunStream(c.Request.Context(), agentReq, func(chunk string) error {
		data, _ := json.Marshal(map[string]string{"content": chunk})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
		flusher.Flush()
		return nil
	})

	if err != nil {
		log.Printf("[DEBUG chat] RunStream error: %v", err)
		errData, _ := json.Marshal(map[string]string{"error": err.Error()})
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(errData))
		flusher.Flush()
	}

	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()
}
