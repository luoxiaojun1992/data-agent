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
	Role    string `json:"role"`
	Content string `json:"content"`
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
		lastKey := fmt.Sprintf("mock:resp:%x", hash)
		log.Printf("[DEBUG] chat request: model=%s messages=%d last_msg_len=%d last_msg=%q hash=%x key=%s",
			req.Model, len(req.Messages), len(lastContent), lastContent, hash, lastKey)

		// Look up response in Redis — try last message first, then fallback to first user message
		// (needed for ADK ReAct loops where tool results become the last message)
		var candidateKeys []string
		candidateKeys = append(candidateKeys, lastKey)

		if len(req.Messages) > 1 {
			for _, msg := range req.Messages {
				if msg.Role == "user" {
					firstUserHash := sha256.Sum256([]byte(msg.Content))
					firstUserKey := fmt.Sprintf("mock:resp:%x", firstUserHash)
					if firstUserKey != lastKey {
						candidateKeys = append(candidateKeys, firstUserKey)
						log.Printf("[DEBUG] ReAct fallback: first user msg=%q key=%s", msg.Content, firstUserKey)
					}
					break
				}
			}
		}

		ctx := context.Background()
		response := popResponse(ctx, rdb, candidateKeys, defaultReply)

		if req.Stream {
			handleStream(w, response, chunkDelay)
		} else {
			handleNonStream(w, response, req.Model)
		}
	}
}

// popResponse tries each candidate key in order (the first key is the most specific),
// falling through to subsequent keys for ReAct loop support, then returns default.
func popResponse(ctx context.Context, rdb *redis.Client, candidateKeys []string, defaultReply string) string {
	for i, key := range candidateKeys {
		log.Printf("[DEBUG] popResponse: trying key[%d]=%s", i, key)
		if val, err := rdb.LPop(ctx, key).Result(); err == nil && val != "" {
			log.Printf("[DEBUG] popResponse: FOUND match on key[%d], val_len=%d", i, len(val))
			return val
		}
	}

	log.Printf("[DEBUG] popResponse: no match in %d candidate keys, returning default (len=%d)", len(candidateKeys), len(defaultReply))
	return defaultReply
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
