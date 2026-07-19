package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"google.golang.org/adk/session"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

// Message represents a chat message in the request payload.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// Service handles real-time chat operations backed by the ADK runtime.
// The HTTP contract (JSON in, SSE out) is unchanged from the legacy engine.
type Service struct {
	rt          *adkruntime.Runtime
	adkSessions session.Service
	sessions    *Manager
	cbReg       *security.CircuitBreakerRegistry
	memoryWrite func(ctx context.Context, sess session.Session) // optional post-run memory hook
}

// NewService creates a new Chat Service backed by the ADK runtime.
func NewService(rt *adkruntime.Runtime, adkSessions session.Service, sessions *Manager, cbReg *security.CircuitBreakerRegistry) *Service {
	return &Service{
		rt:          rt,
		adkSessions: adkSessions,
		sessions:    sessions,
		cbReg:       cbReg,
	}
}

// WithMemoryWrite registers a hook invoked after each completed run with the
// final ADK session, e.g. memory.Service.AddSessionToMemory. Errors are logged
// and never fail the chat response.
func (s *Service) WithMemoryWrite(hook func(ctx context.Context, sess session.Session)) *Service {
	s.memoryWrite = hook
	return s
}

// ChatRequest represents an incoming chat request.
type ChatRequest struct {
	SessionID string    `json:"session_id,omitempty"`
	Model     string    `json:"model,omitempty"`
	Messages  []Message `json:"messages"`
	Message   string    `json:"message,omitempty"` // legacy single-message field from frontend
	Stream    bool      `json:"stream"`
	KBID      string    `json:"kb_id,omitempty"`
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
		req.Messages = []Message{{Role: "user", Content: req.Message}}
	}

	if len(req.Messages) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "messages required"})
		return
	}

	// Validate or create session
	userID, _ := c.Get("user_id")
	userIDStr, _ := userID.(string)
	role, _ := c.Get("role")
	roleStr, _ := role.(string)

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

	// Only the last user message enters the ADK run — history lives in the ADK session.
	lastMsg := lastUserMessage(req.Messages)
	if lastMsg == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user message required"})
		return
	}

	// Ensure the ADK session exists and inject identity into its state so that
	// tools read user_id/role/kb_id from tool.Context.State() instead of LLM params.
	state := map[string]any{
		"user_id":    userIDStr,
		"role":       roleStr,
		"session_id": req.SessionID,
	}
	if req.KBID != "" {
		state["kb_id"] = req.KBID
	}
	if _, err := s.adkSessions.Create(c.Request.Context(), &session.CreateRequest{
		AppName:   s.rt.AppName(),
		UserID:    userIDStr,
		SessionID: req.SessionID,
		State:     state,
	}); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "failed to init agent session"})
		return
	}

	runCfg := adkruntime.RunConfig{
		Streaming:  req.Stream,
		StateDelta: state,
	}

	// Stream mode
	if req.Stream {
		s.handleStream(c, req, userIDStr, lastMsg, runCfg)
		return
	}

	s.handleNonStreamChat(c, req, userIDStr, lastMsg, runCfg)
}

// handleNonStreamChat processes a non-streaming chat request.
func (s *Service) handleNonStreamChat(c *gin.Context, req ChatRequest, userID, message string, runCfg adkruntime.RunConfig) {
	var content string
	cb := s.cbReg.GetOrCreate("chat")
	err := cb.Call(func() error {
		text, err := s.runAndCollect(c.Request.Context(), userID, req.SessionID, message, runCfg)
		if err != nil {
			return err
		}
		content = text
		return nil
	})

	if err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"error": err.Error()})
		return
	}

	s.scheduleMemoryWrite(userID, req.SessionID)
	c.JSON(http.StatusOK, gin.H{
		"session_id": req.SessionID,
		"content":    content,
		"usage":      map[string]int{},
	})
}

// handleStream handles SSE streaming responses.
func (s *Service) handleStream(c *gin.Context, req ChatRequest, userID, message string, runCfg adkruntime.RunConfig) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "streaming not supported"})
		return
	}

	// Send session ID as first event
	sessionData, err := json.Marshal(map[string]string{"session_id": req.SessionID})
	if err != nil {
		fmt.Fprintf(c.Writer, "data: {\"error\":\"marshal failed\"}\n\n")
		flusher.Flush()
		return
	}
	fmt.Fprintf(c.Writer, "data: %s\n\n", string(sessionData))
	flusher.Flush()

	text, runErr := s.runAndCollect(c.Request.Context(), userID, req.SessionID, message, runCfg)
	if runErr != nil {
		log.Printf("[chat] run error: %v", runErr)
		errData, marshalErr := json.Marshal(map[string]string{"error": runErr.Error()})
		if marshalErr != nil {
			fmt.Fprintf(c.Writer, "data: {\"error\":\"internal stream error\"}\n\n")
		} else {
			fmt.Fprintf(c.Writer, "data: %s\n\n", string(errData))
		}
		flusher.Flush()
		fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
		flusher.Flush()
		return
	}

	data, err := json.Marshal(map[string]string{"content": text})
	if err != nil {
		fmt.Fprintf(c.Writer, "data: {\"error\":\"marshal failed\"}\n\n")
	} else {
		fmt.Fprintf(c.Writer, "data: %s\n\n", string(data))
	}
	flusher.Flush()

	fmt.Fprintf(c.Writer, "data: [DONE]\n\n")
	flusher.Flush()

	s.scheduleMemoryWrite(userID, req.SessionID)
}

// runAndCollect executes one ADK turn and returns the final assistant text.
// Intermediate tool call/response events are consumed but not surfaced.
func (s *Service) runAndCollect(ctx context.Context, userID, sessionID, message string, runCfg adkruntime.RunConfig) (string, error) {
	var finalText strings.Builder
	runErr := error(nil)
	for evt, err := range s.rt.Run(ctx, userID, sessionID, message, runCfg) {
		if err != nil {
			runErr = err
			break
		}
		if evt == nil || evt.Content == nil {
			continue
		}
		if !evt.IsFinalResponse() {
			continue
		}
		for _, p := range evt.Content.Parts {
			if p != nil && p.Text != "" {
				finalText.WriteString(p.Text)
			}
		}
		// Got model response. Return immediately; ADK will produce
		// internal agent events after this but the request context
		// cancel will stop them.
		return finalText.String(), nil
	}
	if runErr != nil {
		return "", runErr
	}
	return finalText.String(), nil
}

// scheduleMemoryWrite invokes the memory hook asynchronously after the response.
func (s *Service) scheduleMemoryWrite(userID, sessionID string) {
	if s.memoryWrite == nil {
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := s.adkSessions.Get(ctx, &session.GetRequest{
			AppName:   s.rt.AppName(),
			UserID:    userID,
			SessionID: sessionID,
		})
		if err != nil {
			log.Printf("[chat] memory hook: load session: %v", err)
			return
		}
		s.memoryWrite(ctx, resp.Session)
	}()
}

// lastUserMessage returns the content of the last user message.
func lastUserMessage(messages []Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}
