package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"google.golang.org/adk/session"
	genai "google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

// Service handles real-time chat operations backed by the ADK runtime.
// It implements domain/chat.ChatService and contains no gin dependency;
// HTTP/SSE translation is the handler's responsibility.
//
// SPEC-062: Service resolves the Runtime per session via the Registry (keyed
// by session.ModelID) instead of a single shared Runtime. The model is bound
// at session creation and cannot be changed afterwards.
type Service struct {
	registry    *adkruntime.Registry
	provider    *modelcfg.Provider
	adkSessions session.Service
	sessions    *Manager
	cbReg       *security.CircuitBreakerRegistry
	memoryWrite func(ctx context.Context, sess session.Session) // optional post-run memory hook
}

// ensure Service satisfies the domain ChatService contract.
var _ domainchat.ChatService = (*Service)(nil)

// NewService creates a new Chat Service backed by the ADK runtime registry.
func NewService(registry *adkruntime.Registry, provider *modelcfg.Provider, adkSessions session.Service, sessions *Manager, cbReg *security.CircuitBreakerRegistry) *Service {
	return &Service{
		registry:    registry,
		provider:    provider,
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

// prepareRun validates the request, resolves/creates the session (binding the
// model ID on creation), ensures the ADK session exists with identity injected
// into state, and returns the resolved session ID, last user message, run
// config, and the model-bound Runtime. Shared by Process and Stream.
//
// SPEC-062: On new sessions, the model is resolved from req.Model (empty →
// default) and bound permanently. On existing sessions, req.Model is IGNORED
// and the session's bound ModelID is used (model cannot be changed).
func (s *Service) prepareRun(ctx context.Context, req domainchat.ChatRequest, userID, role string) (rt *adkruntime.Runtime, sessionID, lastMsg string, runCfg adkruntime.RunConfig, err error) {
	// Convert legacy single message to messages array.
	messages := req.Messages
	if len(messages) == 0 && req.Message != "" {
		messages = []domainchat.Message{{Role: "user", Content: req.Message}}
	}
	if len(messages) == 0 {
		err = domainchat.ErrMessagesRequired
		return
	}

	var modelID string
	// Validate or create session.
	if req.SessionID == "" {
		// New session: resolve modelId from request or default, then bind.
		modelID = req.Model
		if modelID == "" && s.provider != nil {
			if dm, dErr := s.provider.DefaultModel(ctx); dErr == nil && dm != nil {
				modelID = dm.ID
			}
		}
		sess, cErr := s.sessions.Create(userID, "chat", modelID)
		if cErr != nil {
			err = domainchat.ErrSessionCreateFailed
			return
		}
		sessionID = sess.ID
		modelID = sess.ModelID
	} else {
		// Existing session: use the bound model (ignore req.Model — cannot change).
		sess, gErr := s.sessions.Get(req.SessionID)
		if gErr != nil || sess.UserID != userID {
			err = domainchat.ErrUnauthorizedSession
			return
		}
		sessionID = req.SessionID
		modelID = sess.ModelID
		_ = s.sessions.Renew(sessionID)
	}

	// Resolve the Runtime for this session's bound model.
	rt, rErr := s.registry.GetOrCreate(ctx, modelID)
	if rErr != nil {
		err = domainchat.ErrADKSessionInitFailed
		return
	}

	lastMsg = lastUserMessage(messages)
	if lastMsg == "" {
		err = domainchat.ErrUserMessageRequired
		return
	}

	// Inject identity into ADK session state so tools read user_id/role/kb_id
	// from tool.Context.State() instead of LLM params.
	state := map[string]any{
		"user_id":    userID,
		"role":       role,
		"session_id": sessionID,
	}
	if req.KBID != "" {
		state["kb_id"] = req.KBID
	}
	if _, cerr := s.adkSessions.Create(ctx, &session.CreateRequest{
		AppName:   rt.AppName(),
		UserID:    userID,
		SessionID: sessionID,
		State:     state,
	}); cerr != nil {
		err = domainchat.ErrADKSessionInitFailed
		return
	}

	runCfg = adkruntime.RunConfig{
		Streaming:  req.Stream,
		StateDelta: state,
	}
	return
}

// Process handles a non-streaming chat request and returns the final
// assistant content. Implements domain/chat.ChatService.
func (s *Service) Process(ctx context.Context, req domainchat.ChatRequest, userID, role string) (*domainchat.ChatResponse, error) {
	rt, sessionID, lastMsg, runCfg, err := s.prepareRun(ctx, req, userID, role)
	if err != nil {
		return nil, err
	}

	var content string
	cb := s.cbReg.GetOrCreate("chat")
	if cErr := cb.Call(func() error {
		text, rErr := s.runAndCollect(ctx, rt, userID, sessionID, lastMsg, runCfg)
		if rErr != nil {
			return rErr
		}
		content = text
		return nil
	}); cErr != nil {
		return nil, cErr
	}

	s.scheduleMemoryWrite(userID, sessionID)
	return &domainchat.ChatResponse{
		SessionID: sessionID,
		Content:   content,
		Usage:     map[string]int{},
	}, nil
}

// Stream handles a streaming chat request, writing SSE events to w.
// Implements domain/chat.ChatService. The writer must implement
// http.Flusher (gin and httptest.ResponseRecorder both do).
func (s *Service) Stream(ctx context.Context, req domainchat.ChatRequest, userID, role string, w http.ResponseWriter) error {
	rt, sessionID, lastMsg, runCfg, err := s.prepareRun(ctx, req, userID, role)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	// Send session ID as first event.
	sessionData, _ := json.Marshal(map[string]string{"session_id": sessionID})
	fmt.Fprintf(w, "data: %s\n\n", sessionData)
	flusher.Flush()

	for evt, rErr := range rt.Run(ctx, userID, sessionID, lastMsg, runCfg) {
		if rErr != nil {
			log.Printf("[chat] run error: %v", rErr)
			errData, _ := json.Marshal(map[string]string{"error": rErr.Error()})
			fmt.Fprintf(w, "data: %s\n\n", errData)
			flusher.Flush()
			fmt.Fprintf(w, "data: [DONE]\n\n")
			flusher.Flush()
			return nil
		}
		if evt == nil || evt.Content == nil {
			continue
		}
		forwardSSEParts(w, flusher, evt.Content.Parts)
	}

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	s.scheduleMemoryWrite(userID, sessionID)
	return nil
}

// forwardSSEParts writes each ADK event part to the SSE stream as-is.
// Extracted from Stream to reduce cognitive complexity.
func forwardSSEParts(w http.ResponseWriter, flusher http.Flusher, parts []*genai.Part) {
	for _, p := range parts {
		if p == nil {
			continue
		}
		switch {
		case p.FunctionCall != nil:
			argsJSON, _ := json.Marshal(p.FunctionCall.Args)
			data, _ := json.Marshal(map[string]any{
				"type": "tool_call",
				"name": p.FunctionCall.Name,
				"args": json.RawMessage(argsJSON),
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case p.FunctionResponse != nil:
			respJSON, _ := json.Marshal(p.FunctionResponse.Response)
			data, _ := json.Marshal(map[string]any{
				"type":     "tool_result",
				"name":     p.FunctionResponse.Name,
				"response": json.RawMessage(respJSON),
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		case p.Text != "":
			data, _ := json.Marshal(map[string]string{
				"type":    "text",
				"content": p.Text,
			})
			fmt.Fprintf(w, "data: %s\n\n", data)
			flusher.Flush()
		}
	}
}

// runAndCollect executes one ADK turn and returns the final assistant text.
// Intermediate tool call/response events are consumed but not surfaced.
func (s *Service) runAndCollect(ctx context.Context, rt *adkruntime.Runtime, userID, sessionID, message string, runCfg adkruntime.RunConfig) (string, error) {
	var finalText strings.Builder
	runErr := error(nil)
	for evt, err := range rt.Run(ctx, userID, sessionID, message, runCfg) {
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
	}
	if runErr != nil {
		return "", runErr
	}
	return finalText.String(), nil
}

// scheduleMemoryWrite invokes the memory hook asynchronously after the response.
// Uses the registry's shared app name (all Runtimes share it).
func (s *Service) scheduleMemoryWrite(userID, sessionID string) {
	if s.memoryWrite == nil {
		return
	}
	appName := s.registry.AppName()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		resp, err := s.adkSessions.Get(ctx, &session.GetRequest{
			AppName:   appName,
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
func lastUserMessage(messages []domainchat.Message) string {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role == "user" && strings.TrimSpace(messages[i].Content) != "" {
			return messages[i].Content
		}
	}
	return ""
}
