package adkmemory

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// OpenAIEmbeddingConfig configures an OpenAI-compatible /v1/embeddings endpoint.
// In the test environment this points at Ollama (nomic-embed-text).
type OpenAIEmbeddingConfig struct {
	BaseURL string // e.g. "http://ollama:11434/v1"
	Model   string // e.g. "nomic-embed-text"
	APIKey  string // optional
}

// NewOpenAIEmbedding returns an EmbeddingFunc backed by an OpenAI-compatible
// embeddings API.
func NewOpenAIEmbedding(cfg OpenAIEmbeddingConfig) EmbeddingFunc {
	client := &http.Client{Timeout: 30 * time.Second}
	endpoint := strings.TrimSuffix(cfg.BaseURL, "/") + "/embeddings"

	return func(ctx context.Context, text string) ([]float32, error) {
		payload := map[string]any{
			"model": cfg.Model,
			"input": text,
		}
		body, err := json.Marshal(payload)
		if err != nil {
			return nil, fmt.Errorf("marshal embedding request: %w", err)
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(body))
		if err != nil {
			return nil, fmt.Errorf("create embedding request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		if cfg.APIKey != "" {
			req.Header.Set("Authorization", "Bearer "+cfg.APIKey)
		}

		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("embedding request: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			respBody, _ := io.ReadAll(resp.Body)
			return nil, fmt.Errorf("embedding API error %d: %s", resp.StatusCode, string(respBody))
		}

		var parsed struct {
			Data []struct {
				Embedding []float32 `json:"embedding"`
			} `json:"data"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&parsed); err != nil {
			return nil, fmt.Errorf("parse embedding response: %w", err)
		}
		if len(parsed.Data) == 0 || len(parsed.Data[0].Embedding) == 0 {
			return nil, fmt.Errorf("empty embedding response")
		}
		return parsed.Data[0].Embedding, nil
	}
}
