package chat

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
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

// Chat image attachment limits: at most 5 images per message; each image at
// most 2 MiB decoded; at most 5 MiB decoded in total per message. The caps
// keep the MongoDB session document (events + raw_events both store the
// persisted event) well below the 16 MiB BSON document limit.
const (
	maxChatImages      = 5
	maxChatImageBytes  = 2 * 1024 * 1024
	maxTotalImageBytes = 5 * 1024 * 1024
)

// allowedChatImageMimes is the whitelist of accepted image MIME types.
var allowedChatImageMimes = map[string]bool{
	"image/png":  true,
	"image/jpeg": true,
	"image/webp": true,
	"image/gif":  true,
}

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
// into state, and returns the resolved session ID, the built user content
// (text plus optional image parts), the last user text (for titles), run
// config, and the model-bound Runtime. Shared by Process and Stream.
//
// SPEC-062: On new sessions, the model is resolved from req.Model (empty →
// default) and bound permanently. On existing sessions, req.Model is IGNORED
// and the session's bound ModelID is used (model cannot be changed).
func (s *Service) prepareRun(ctx context.Context, req domainchat.ChatRequest, userID, role string) (rt *adkruntime.Runtime, sessionID string, content *genai.Content, lastText string, runCfg adkruntime.RunConfig, err error) {
	messages := normalizeMessages(req)
	if len(messages) == 0 {
		err = domainchat.ErrMessagesRequired
		return
	}

	lastText, images := lastUserMessage(messages)
	if strings.TrimSpace(lastText) == "" && len(images) == 0 {
		err = domainchat.ErrUserMessageRequired
		return
	}

	content, err = buildUserContent(lastText, images)
	if err != nil {
		return
	}

	sessionID, modelID, rErr := s.resolveSession(ctx, req, userID)
	if rErr != nil {
		err = rErr
		return
	}

	rt, err = s.registry.GetOrCreate(ctx, modelID)
	if err != nil {
		err = domainchat.ErrADKSessionInitFailed
		return
	}

	// Auto-set session title from the first user message. Failures are
	// non-fatal — the session continues to work, just without a nice title.
	title := strings.TrimSpace(lastText)
	if title == "" {
		title = fmt.Sprintf("[图片] %d 张", len(images))
	}
	if titleErr := s.sessions.SetTitle(sessionID, truncateTitle(title, 30)); titleErr != nil {
		log.Printf("[chat] set title: %v (session=%s)", titleErr, sessionID)
	}

	state := buildState(userID, role, sessionID, req.KBID)
	if _, cerr := s.adkSessions.Create(ctx, &session.CreateRequest{
		AppName:   rt.AppName(),
		UserID:    userID,
		SessionID: sessionID,
		State:     state,
	}); cerr != nil {
		err = domainchat.ErrADKSessionInitFailed
		return
	}

	runCfg = adkruntime.RunConfig{Streaming: req.Stream, StateDelta: state}
	return
}

// normalizeMessages converts a legacy single-message request to the messages
// array form. Top-level Images (sent alongside the legacy Message field) are
// folded into the synthesized user message.
func normalizeMessages(req domainchat.ChatRequest) []domainchat.Message {
	if len(req.Messages) > 0 {
		return req.Messages
	}
	if req.Message != "" || len(req.Images) > 0 {
		return []domainchat.Message{{Role: "user", Content: req.Message, Images: req.Images}}
	}
	return nil
}

// buildUserContent validates the image attachments and assembles the genai
// user content: one text part (when non-empty) followed by one InlineData part
// per image.
func buildUserContent(text string, images []domainchat.ImagePart) (*genai.Content, error) {
	decoded, err := validateImages(images)
	if err != nil {
		return nil, err
	}
	parts := make([]*genai.Part, 0, len(decoded)+1)
	if strings.TrimSpace(text) != "" {
		parts = append(parts, genai.NewPartFromText(text))
	}
	for i, img := range decoded {
		parts = append(parts, genai.NewPartFromBytes(img, images[i].MimeType))
	}
	return &genai.Content{Role: "user", Parts: parts}, nil
}

// validateImages enforces the attachment limits and decodes each image's
// base64 payload, returning the decoded byte slices in order.
func validateImages(images []domainchat.ImagePart) ([][]byte, error) {
	if len(images) > maxChatImages {
		return nil, domainchat.ErrTooManyImages
	}
	decoded := make([][]byte, 0, len(images))
	var total int
	for _, img := range images {
		if !allowedChatImageMimes[strings.ToLower(strings.TrimSpace(img.MimeType))] {
			return nil, domainchat.ErrInvalidImage
		}
		data, derr := base64.StdEncoding.DecodeString(img.Data)
		if derr != nil {
			return nil, domainchat.ErrInvalidImage
		}
		if len(data) > maxChatImageBytes {
			return nil, domainchat.ErrImageTooLarge
		}
		total += len(data)
		if total > maxTotalImageBytes {
			return nil, domainchat.ErrImageTooLarge
		}
		decoded = append(decoded, data)
	}
	return decoded, nil
}

// resolveSession validates or creates the session and returns (sessionID,
// modelID, error). On new sessions the model is resolved from req.Model
// (empty → default) and bound permanently. On existing sessions req.Model is
// ignored — the bound model is used (immutable binding).
func (s *Service) resolveSession(ctx context.Context, req domainchat.ChatRequest, userID string) (string, string, error) {
	if req.SessionID == "" {
		return s.createNewSession(ctx, req, userID)
	}
	return s.useExistingSession(req, userID)
}

// createNewSession creates a session with a model resolved from req.Model or
// the provider default, then returns the session ID and bound model ID.
func (s *Service) createNewSession(ctx context.Context, req domainchat.ChatRequest, userID string) (string, string, error) {
	modelID := req.Model
	if modelID == "" && s.provider != nil {
		if dm, dErr := s.provider.DefaultModel(ctx); dErr == nil && dm != nil {
			modelID = dm.ID
		}
	}
	sess, cErr := s.sessions.Create(userID, "chat", modelID)
	if cErr != nil {
		return "", "", domainchat.ErrSessionCreateFailed
	}
	return sess.ID, sess.ModelID, nil
}

// useExistingSession loads an existing session, verifies ownership, renews it,
// and returns its ID and bound model ID. The model cannot be changed.
func (s *Service) useExistingSession(req domainchat.ChatRequest, userID string) (string, string, error) {
	sess, gErr := s.sessions.Get(req.SessionID)
	if gErr != nil || sess.UserID != userID {
		return "", "", domainchat.ErrUnauthorizedSession
	}
	_ = s.sessions.Renew(req.SessionID)
	return req.SessionID, sess.ModelID, nil
}

// buildState constructs the ADK session state map with identity injection.
func buildState(userID, role, sessionID, kbID string) map[string]any {
	state := map[string]any{
		"user_id":    userID,
		"role":       role,
		"session_id": sessionID,
	}
	if kbID != "" {
		state["kb_id"] = kbID
	}
	return state
}

// Process handles a non-streaming chat request and returns the final
// assistant content. Implements domain/chat.ChatService.
func (s *Service) Process(ctx context.Context, req domainchat.ChatRequest, userID, role string) (*domainchat.ChatResponse, error) {
	rt, sessionID, content, _, runCfg, err := s.prepareRun(ctx, req, userID, role)
	if err != nil {
		return nil, err
	}

	var assistantText string
	cb := s.cbReg.GetOrCreate("chat")
	if cErr := cb.Call(func() error {
		text, rErr := s.runAndCollect(ctx, rt, userID, sessionID, content, runCfg)
		if rErr != nil {
			return rErr
		}
		assistantText = text
		return nil
	}); cErr != nil {
		return nil, cErr
	}

	s.scheduleMemoryWrite(userID, sessionID)
	return &domainchat.ChatResponse{
		SessionID: sessionID,
		Content:   assistantText,
		Usage:     map[string]int{},
	}, nil
}

// Stream handles a streaming chat request, writing SSE events to w.
// Implements domain/chat.ChatService. The writer must implement
// http.Flusher (gin and httptest.ResponseRecorder both do).
func (s *Service) Stream(ctx context.Context, req domainchat.ChatRequest, userID, role string, w http.ResponseWriter) error {
	rt, sessionID, content, _, runCfg, err := s.prepareRun(ctx, req, userID, role)
	if err != nil {
		return err
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no")

	flusher, ok := w.(http.Flusher)
	if !ok {
		return fmt.Errorf("streaming not supported")
	}

	// Send session ID as first event.
	sessionData, _ := json.Marshal(map[string]string{"session_id": sessionID})
	fmt.Fprintf(w, "data: %s\n\n", sessionData)
	flusher.Flush()

	for evt, rErr := range rt.RunContent(ctx, userID, sessionID, content, runCfg) {
		if rErr != nil {
			if isSessionPersistenceError(rErr) {
				log.Printf("[chat] session persistence failed (response already delivered, ignoring): %v (session=%s)", rErr, sessionID)
				fmt.Fprintf(w, "data: [DONE]\n\n")
				flusher.Flush()
				return nil
			}
			log.Printf("[chat] run error: %v (session=%s)", rErr, sessionID)
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
		forwardSSEEvent(w, flusher, evt)
	}

	log.Printf("[chat] stream completed normally (session=%s)", sessionID)
	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()

	s.scheduleMemoryWrite(userID, sessionID)
	return nil
}

// forwardSSEEvent writes the same canonical event shape used by the session
// history endpoint. Keeping conversion in one place prevents live SSE and
// reloaded history from using different field names or dropping tool payloads.
func forwardSSEEvent(w http.ResponseWriter, flusher http.Flusher, ev *session.Event) {
	for _, chatEvent := range ChatEventsFromADKEvent(ev) {
		data, err := json.Marshal(chatEvent)
		if err != nil {
			log.Printf("[chat] marshal stream event: %v", err)
			continue
		}
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()
	}
}

// ChatEventsFromADKEvent converts one persisted/streamed ADK event, including
// canonical author/timestamp metadata. Synthetic compaction summaries are
// filtered here so SSE and history apply the exact same rule.
func ChatEventsFromADKEvent(ev *session.Event) []domainchat.ChatEvent {
	if ev == nil || ev.Content == nil || IsCompactionEvent(ev) {
		return nil
	}
	role := ev.Author
	if role == "" {
		role = "assistant"
	}
	return ChatEventsFromParts(
		role,
		ev.ID,
		ev.Timestamp.UTC().Format(time.RFC3339),
		ev.Content.Parts,
	)
}

// ChatEventsFromParts converts ADK content parts to the canonical chat event
// representation shared by streaming and history responses. Inline image
// parts are converted to data URLs and grouped onto the text event of the
// same message (or a standalone image event for image-only messages) so a
// reloaded transcript renders the user message with its attachments intact.
func ChatEventsFromParts(role, eventID, timestamp string, parts []*genai.Part) []domainchat.ChatEvent {
	events := make([]domainchat.ChatEvent, 0, len(parts))
	var imageDataURLs []string
	var textIdx = -1
	for _, p := range parts {
		if p == nil {
			continue
		}
		switch {
		case p.FunctionCall != nil:
			events = append(events, domainchat.ChatEvent{
				EventID: eventID, Role: role, Type: "tool_call",
				Name: p.FunctionCall.Name, Args: p.FunctionCall.Args,
				Timestamp: timestamp,
			})
		case p.FunctionResponse != nil:
			events = append(events, domainchat.ChatEvent{
				EventID: eventID, Role: role, Type: "tool_result",
				Name: p.FunctionResponse.Name, Result: p.FunctionResponse.Response,
				Timestamp: timestamp,
			})
		case p.InlineData != nil && strings.HasPrefix(p.InlineData.MIMEType, "image/"):
			b64 := base64.StdEncoding.EncodeToString(p.InlineData.Data)
			imageDataURLs = append(imageDataURLs, fmt.Sprintf("data:%s;base64,%s", p.InlineData.MIMEType, b64))
		case p.Text != "":
			textIdx = len(events)
			events = append(events, domainchat.ChatEvent{
				EventID: eventID, Role: role, Type: "text",
				Content: p.Text, Timestamp: timestamp,
			})
		}
	}
	if len(imageDataURLs) == 0 {
		return events
	}
	if textIdx >= 0 {
		events[textIdx].Images = imageDataURLs
		return events
	}
	events = append(events, domainchat.ChatEvent{
		EventID: eventID, Role: role, Type: "text", Content: "",
		Images: imageDataURLs, Timestamp: timestamp,
	})
	return events
}

// IsCompactionEvent identifies synthetic summaries that are context for the
// model but are not original user/assistant/tool transcript entries.
func IsCompactionEvent(ev *session.Event) bool {
	if ev == nil {
		return false
	}
	if ev.Author == "compaction" {
		return true
	}
	if ev.LLMResponse.CustomMetadata != nil {
		if _, ok := ev.LLMResponse.CustomMetadata["compaction"]; ok {
			return true
		}
	}
	return false
}

// runAndCollect executes one ADK turn and returns the final assistant text.
// Intermediate tool call/response events are consumed but not surfaced.
// Delegates to Runtime.RunAndCollectContent (shared with the async executor,
// SPEC-063) so real-time and async paths use identical collection semantics.
func (s *Service) runAndCollect(ctx context.Context, rt *adkruntime.Runtime, userID, sessionID string, content *genai.Content, runCfg adkruntime.RunConfig) (string, error) {
	return rt.RunAndCollectContent(ctx, userID, sessionID, content, runCfg)
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

// lastUserMessage returns the content and image attachments of the last user
// message. A message qualifies when its text is non-empty OR it carries
// images (image-only messages are valid).
func lastUserMessage(messages []domainchat.Message) (string, []domainchat.ImagePart) {
	for i := len(messages) - 1; i >= 0; i-- {
		if messages[i].Role != "user" {
			continue
		}
		if strings.TrimSpace(messages[i].Content) != "" || len(messages[i].Images) > 0 {
			return messages[i].Content, messages[i].Images
		}
	}
	return "", nil
}

// isSessionPersistenceError reports whether err is a session-append failure
// (AppendEvent returned context canceled) that occurs AFTER the LLM stream
// has already delivered the assistant response. In that case the response
// text has reached the user; the only thing that failed is the post-stream
// MongoDB write, and surfacing it to the frontend would replace the visible
// answer with a "network error".
func isSessionPersistenceError(err error) bool {
	if err == nil {
		return false
	}
	s := err.Error()
	return strings.Contains(s, "failed to add event to session") ||
		strings.Contains(s, "append event: context canceled") ||
		errors.Is(err, context.Canceled) && strings.Contains(s, "session")
}

// truncateTitle returns the first maxRunes of s, trimming trailing whitespace
// so the session list shows a clean snippet.
func truncateTitle(s string, maxRunes int) string {
	s = strings.TrimSpace(s)
	runes := []rune(s)
	if len(runes) <= maxRunes {
		return s
	}
	return string(runes[:maxRunes])
}
