package main

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/redis/go-redis/v9"
	"golang.org/x/net/context"
)

// ChatMessage represents a single message in OpenAI format.
type ChatMessage struct {
	Role         string        `json:"role"`
	Content      string        `json:"content,omitempty"`
	FunctionCall *FunctionCall `json:"function_call,omitempty"`
	ToolCalls    []ToolCall    `json:"tool_calls,omitempty"`
}

// FunctionCall represents the deprecated but still widely used OpenAI function_call format.
type FunctionCall struct {
	Name      string `json:"name"`
	Arguments string `json:"arguments"`
}

// ToolCall represents a tool call in the newer OpenAI tool_calls format.
type ToolCall struct {
	Index    *int          `json:"index,omitempty"`
	ID       string        `json:"id,omitempty"`
	Type     string        `json:"type"`
	Function *FunctionCall `json:"function"`
}

// ChatCompletionRequest mirrors OpenAI /v1/chat/completions request.
type ChatCompletionRequest struct {
	Model    string        `json:"model"`
	Messages []ChatMessage `json:"messages"`
	Stream   bool          `json:"stream"`
}

// ChatCompletionChoice mirrors OpenAI choice format.
type ChatCompletionChoice struct {
	Index        int          `json:"index"`
	Message      *ChatMessage `json:"message,omitempty"`
	Delta        *ChatMessage `json:"delta,omitempty"`
	FinishReason string       `json:"finish_reason"`
}

// ChatCompletionResponse mirrors OpenAI /v1/chat/completions response.
type ChatCompletionResponse struct {
	ID      string                 `json:"id"`
	Object  string                 `json:"object"`
	Created int64                  `json:"created"`
	Model   string                 `json:"model"`
	Choices []ChatCompletionChoice `json:"choices"`
}

// ResponsesPayload is the request body for POST /responses.
type ResponsesPayload struct {
	Key      string `json:"key"`
	Response string `json:"response"`
}

// toolCallResponse is the JSON format used in test seeds for tool call responses.
// When the seeded response matches this format, mockllm returns it as an OpenAI
// function_call instead of plain text content — enabling ADK ReAct tool execution.
type toolCallResponse struct {
	Type  string         `json:"type"`
	Name  string         `json:"name"`
	Input map[string]any `json:"input"`
}

// tryAsFunctionCall checks if the response content is a tool_call JSON block.
// If so, it builds a ChatMessage with tool_calls instead of text content.
func tryAsFunctionCall(content string) *ChatMessage {
	var tc toolCallResponse
	if err := json.Unmarshal([]byte(content), &tc); err != nil || tc.Type != "tool_call" || tc.Name == "" {
		return nil
	}
	argsJSON, err := json.Marshal(tc.Input)
	if err != nil {
		return nil
	}
	callID := fmt.Sprintf("call_%d", time.Now().UnixNano())
	log.Printf("[DEBUG] tool call detected: name=%s args=%s", tc.Name, string(argsJSON))
	return &ChatMessage{
		Role:    "assistant",
		Content: "",
		ToolCalls: []ToolCall{
			{
				ID:   callID,
				Type: "function",
				Function: &FunctionCall{
					Name:      tc.Name,
					Arguments: string(argsJSON),
				},
			},
		},
	}
}

func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

func envOrDefaultInt(key string, defaultVal int) int {
	if v := os.Getenv(key); v != "" {
		n, err := strconv.Atoi(v)
		if err == nil {
			return n
		}
	}
	return defaultVal
}

func main() {
	port := envOrDefault("MOCKLLM_PORT", "8082")
	adminToken := envOrDefault("MOCK_ADMIN_TOKEN", "test-admin-token")

	rdb := redis.NewClient(&redis.Options{
		Addr:     envOrDefault("REDIS_ADDR", "localhost:6379"),
		Password: envOrDefault("REDIS_PASSWORD", ""),
		DB:       envOrDefaultInt("REDIS_DB", 0),
	})

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		log.Fatalf("redis connection failed: %v", err)
	}
	log.Println("redis connected successfully")

	mux := http.NewServeMux()

	// OpenAI-compatible chat completions endpoint
	mux.HandleFunc("/v1/chat/completions", chatHandler(rdb))

	// Management API
	mux.HandleFunc("/responses", responsesHandler(rdb, adminToken))
	mux.HandleFunc("/responses/", responseByKeyHandler(rdb, adminToken))

	// Health check
	mux.HandleFunc("/health", healthHandler)

	addr := ":" + port
	log.Printf("mockllm starting on %s", addr)
	if err := http.ListenAndServe(addr, mux); err != nil {
		log.Fatalf("server error: %v", err)
	}
}

// healthHandler returns 200 OK for health checks.
func healthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// chatHandler handles OpenAI-compatible chat completions.
func chatHandler(rdb *redis.Client) http.HandlerFunc {
	chunkDelay := envOrDefaultInt("MOCK_CHUNK_DELAY_MS", 5)
	defaultReply := envOrDefault("MOCK_DEFAULT_REPLY", "Mock LLM: no response configured")

	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}

		var req ChatCompletionRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, fmt.Sprintf("invalid request: %v", err), http.StatusBadRequest)
			return
		}

		if len(req.Messages) == 0 {
			http.Error(w, "messages array is empty", http.StatusBadRequest)
			return
		}

		// Generate lookup key from last message content (full SHA256 hash)
		lastContent := req.Messages[len(req.Messages)-1].Content
		hash := sha256.Sum256([]byte(lastContent))
		lookupKey := fmt.Sprintf("mock:resp:%x", hash)
		log.Printf("[DEBUG] chat request: model=%s messages=%d last_msg_len=%d last_msg=%q hash=%x key=%s",
			req.Model, len(req.Messages), len(lastContent), lastContent, hash, lookupKey)

		// Look up response in Redis
		ctx := context.Background()
		response := popResponse(ctx, rdb, lookupKey, defaultReply)

		// If the response is a tool_call JSON, return as OpenAI function_call
		if fc := tryAsFunctionCall(response); fc != nil {
			if req.Stream {
				handleStreamFunctionCall(w, fc, req.Model)
			} else {
				handleFunctionCall(w, fc, req.Model)
			}
			return
		}

		if req.Stream {
			handleStream(w, response, chunkDelay)
		} else {
			handleNonStream(w, response, req.Model)
		}
	}
}

// popResponse tries exact match first, then returns default.
func popResponse(ctx context.Context, rdb *redis.Client, exactKey, defaultReply string) string {
	log.Printf("[DEBUG] popResponse: looking up key=%s", exactKey)
	// Exact match
	if val, err := rdb.LPop(ctx, exactKey).Result(); err == nil && val != "" {
		log.Printf("[DEBUG] popResponse: FOUND exact match, val_len=%d", len(val))
		return val
	}

	log.Printf("[DEBUG] popResponse: no exact match, returning default (len=%d)", len(defaultReply))
	return defaultReply
}

// handleFunctionCall writes a response with tool_calls format,
// enabling ADK to detect and execute tools via ReAct loop.
func handleFunctionCall(w http.ResponseWriter, msg *ChatMessage, model string) {
	resp := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChoice{
			{
				Index:   0,
				Message: msg,
				FinishReason: "tool_calls",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
	log.Printf("[DEBUG] function_call response: name=%s", msg.FunctionCall.Name)
}

// handleStreamFunctionCall sends tool_calls in SSE delta format.
// ADK uses streaming mode — tool_calls must appear as delta.tool_calls chunks.
func handleStreamFunctionCall(w http.ResponseWriter, msg *ChatMessage, model string) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	chatID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()

	// Map message ToolCalls to SSE delta format
	type sseDelta struct {
		Role      string    `json:"role,omitempty"`
		ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	}

	type sseChunk struct {
		ID      string `json:"id"`
		Object  string `json:"object"`
		Created int64  `json:"created"`
		Model   string `json:"model"`
		Choices []struct {
			Index        int      `json:"index"`
			Delta        sseDelta `json:"delta"`
			FinishReason string   `json:"finish_reason"`
		} `json:"choices"`
	}

	// Send full tool_calls delta in one chunk with finish_reason
	// Map message ToolCalls to SSE delta format (MUST include index)
	callID := msg.ToolCalls[0].ID
	callType := msg.ToolCalls[0].Type
	callName := msg.ToolCalls[0].Function.Name
	callArgs := msg.ToolCalls[0].Function.Arguments

	// Step 1: send role + tool call header with empty arguments (index required)
	idx0 := 0
	chunk1 := sseChunk{
		ID:      chatID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []struct {
			Index        int      `json:"index"`
			Delta        sseDelta `json:"delta"`
			FinishReason string   `json:"finish_reason"`
		}{
			{
				Index: 0,
				Delta: sseDelta{
					Role: msg.Role,
					ToolCalls: []ToolCall{
						{
							Index:    &idx0,
							ID:       callID,
							Type:     callType,
							Function: &FunctionCall{Name: callName, Arguments: ""},
						},
					},
				},
			},
		},
	}
	data1, _ := json.Marshal(chunk1)
	fmt.Fprintf(w, "data: %s\n\n", data1)
	flusher.Flush()

	// Step 2: send arguments chunk with finish_reason
	chunk2 := sseChunk{
		ID:      chatID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   model,
		Choices: []struct {
			Index        int      `json:"index"`
			Delta        sseDelta `json:"delta"`
			FinishReason string   `json:"finish_reason"`
		}{
			{
				Index: 0,
				Delta: sseDelta{
					ToolCalls: []ToolCall{
						{
							Index:    &idx0,
							Function: &FunctionCall{Arguments: callArgs},
						},
					},
				},
				FinishReason: "tool_calls",
			},
		},
	}
	data2, _ := json.Marshal(chunk2)
	fmt.Fprintf(w, "data: %s\n\n", data2)
	flusher.Flush()

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
	log.Printf("[DEBUG] stream function_call: name=%s", msg.ToolCalls[0].Function.Name)
}

// handleNonStream writes a single JSON response.
func handleNonStream(w http.ResponseWriter, content string, model string) {
	resp := ChatCompletionResponse{
		ID:      fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano()),
		Object:  "chat.completion",
		Created: time.Now().Unix(),
		Model:   model,
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Message: &ChatMessage{
					Role:    "assistant",
					Content: content,
				},
				FinishReason: "stop",
			},
		},
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(resp)
}

// handleStream writes SSE streaming response with configurable chunk delay.
func handleStream(w http.ResponseWriter, content string, chunkDelayMs int) {
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")

	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	chatID := fmt.Sprintf("chatcmpl-%d", time.Now().UnixNano())
	created := time.Now().Unix()
	chunks := splitIntoChunks(content, 10) // 10 chars per chunk

	for chunkIdx, chunk := range chunks {
		delta := ChatCompletionResponse{
			ID:      chatID,
			Object:  "chat.completion.chunk",
			Created: created,
			Model:   "mock-gpt-4o",
			Choices: []ChatCompletionChoice{
				{
					Index: chunkIdx,
					Delta: &ChatMessage{
						Role:    "assistant",
						Content: chunk,
					},
					FinishReason: "",
				},
			},
		}

		data, _ := json.Marshal(delta)
		fmt.Fprintf(w, "data: %s\n\n", data)
		flusher.Flush()

		if chunkDelayMs > 0 {
			time.Sleep(time.Duration(chunkDelayMs) * time.Millisecond)
		}
	}

	// Send final chunk with finish_reason
	finalDelta := ChatCompletionResponse{
		ID:      chatID,
		Object:  "chat.completion.chunk",
		Created: created,
		Model:   "mock-gpt-4o",
		Choices: []ChatCompletionChoice{
			{
				Index: 0,
				Delta: &ChatMessage{
					Content: "",
				},
				FinishReason: "stop",
			},
		},
	}

	data, _ := json.Marshal(finalDelta)
	fmt.Fprintf(w, "data: %s\n\n", data)
	flusher.Flush()

	fmt.Fprintf(w, "data: [DONE]\n\n")
	flusher.Flush()
}

// splitIntoChunks splits a string into chunks of given size.
func splitIntoChunks(s string, size int) []string {
	if size <= 0 {
		return []string{s}
	}

	runes := []rune(s)
	var chunks []string
	for i := 0; i < len(runes); i += size {
		end := i + size
		if end > len(runes) {
			end = len(runes)
		}
		chunks = append(chunks, string(runes[i:end]))
	}
	return chunks
}

// responsesHandler handles POST/GET/DELETE on /responses (list operations).
func responsesHandler(rdb *redis.Client, adminToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r, adminToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodPost:
			// Inject a response
			var payload ResponsesPayload
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusBadRequest)
				return
			}
			if payload.Key == "" {
				http.Error(w, `{"error":"key is required"}`, http.StatusBadRequest)
				return
			}
			if payload.Response == "" {
				http.Error(w, `{"error":"response is required"}`, http.StatusBadRequest)
				return
			}

			ctx := context.Background()
			// Hash key to match lookup format (SHA256 full hex)
			keyHash := sha256.Sum256([]byte(payload.Key))
			redisKey := "mock:resp:" + fmt.Sprintf("%x", keyHash)
			log.Printf("[DEBUG] responses POST: raw_key=%q raw_key_len=%d hash=%x redis_key=%s response_len=%d",
				payload.Key, len(payload.Key), keyHash, redisKey, len(payload.Response))
			if err := rdb.LPush(ctx, redisKey, payload.Response).Err(); err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
				return
			}

			log.Printf("response injected: key=%s", payload.Key)
			w.WriteHeader(http.StatusCreated)
			json.NewEncoder(w).Encode(map[string]string{
				"key":      payload.Key,
				"status":   "injected",
				"redisKey": redisKey,
			})

		case http.MethodGet:
			// List all response keys
			ctx := context.Background()
			keys := listMockKeys(ctx, rdb)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"keys":  keys,
				"count": len(keys),
			})

		case http.MethodDelete:
			// Clear all mock responses
			ctx := context.Background()
			keys := listMockKeys(ctx, rdb)
			for _, key := range keys {
				rdb.Del(ctx, key)
			}
			log.Printf("[DEBUG] responses DELETE: clearing %d keys: %v", len(keys), keys)
			log.Printf("all responses cleared (%d keys)", len(keys))
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status":  "cleared",
				"removed": len(keys),
			})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// responseByKeyHandler handles GET/DELETE on /responses/:key.
func responseByKeyHandler(rdb *redis.Client, adminToken string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !checkAuth(r, adminToken) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}

		// Extract key from path: /responses/<key>
		key := strings.TrimPrefix(r.URL.Path, "/responses/")
		if key == "" {
			http.Error(w, "key is required", http.StatusBadRequest)
			return
		}

		redisKey := "mock:resp:" + key
		w.Header().Set("Content-Type", "application/json")

		switch r.Method {
		case http.MethodGet:
			// Peek at the first response (without removing)
			ctx := context.Background()
			vals, err := rdb.LRange(ctx, redisKey, 0, -1).Result()
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"key":       key,
				"redisKey":  redisKey,
				"responses": vals,
				"count":     len(vals),
			})

		case http.MethodDelete:
			// Delete to clear specific key
			ctx := context.Background()
			n, err := rdb.Del(ctx, redisKey).Result()
			if err != nil {
				http.Error(w, fmt.Sprintf(`{"error":"%v"}`, err), http.StatusInternalServerError)
				return
			}
			log.Printf("response deleted: key=%s (removed=%d)", key, n)
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"key":    key,
				"status": "deleted",
				"count":  n,
			})

		default:
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		}
	}
}

// checkAuth validates Bearer token.
func checkAuth(r *http.Request, expectedToken string) bool {
	token := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	return token == expectedToken
}

// listMockKeys scans Redis for all "mock:resp:*" keys.
func listMockKeys(ctx context.Context, rdb *redis.Client) []string {
	var keys []string
	iter := rdb.Scan(ctx, 0, "mock:resp:*", 100).Iterator()
	for iter.Next(ctx) {
		keys = append(keys, iter.Val())
	}
	return keys
}
