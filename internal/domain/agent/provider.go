package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// OpenAIProvider implements LLMProvider for OpenAI-compatible APIs.
type OpenAIProvider struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// NewOpenAIProvider creates a new OpenAI-compatible provider.
func NewOpenAIProvider(baseURL, apiKey string) *OpenAIProvider {
	return &OpenAIProvider{
		baseURL: baseURL,
		apiKey:  apiKey,
		httpClient: &http.Client{
			Timeout: 120 * time.Second,
		},
	}
}

// Chat sends a chat completion request.
func (p *OpenAIProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	body, err := p.doRequest(ctx, req, false)
	if err != nil {
		return nil, err
	}

	var result struct {
		Choices []struct {
			Message struct {
				Content   string     `json:"content"`
				ToolCalls []ToolCall `json:"tool_calls"`
			} `json:"message"`
		} `json:"choices"`
		Usage Usage `json:"usage"`
	}

	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}

	if len(result.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}

	return &ChatResponse{
		Content:   result.Choices[0].Message.Content,
		ToolCalls: result.Choices[0].Message.ToolCalls,
		Usage:     result.Usage,
	}, nil
}

// ChatStream sends a streaming chat completion request.
func (p *OpenAIProvider) ChatStream(ctx context.Context, req ChatRequest, callback func(chunk string) error) error {
	reqMap := map[string]interface{}{
		"model":       req.Model,
		"messages":    req.Messages,
		"stream":      true,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
	}
	if len(req.Tools) > 0 {
		reqMap["tools"] = req.Tools
	}

	body, err := json.Marshal(reqMap)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("API error %d: %s", resp.StatusCode, string(body))
	}

	return parseSSEStream(resp.Body, callback)
}

func (p *OpenAIProvider) doRequest(ctx context.Context, req ChatRequest, stream bool) ([]byte, error) {
	reqMap := map[string]interface{}{
		"model":       req.Model,
		"messages":    req.Messages,
		"stream":      stream,
		"temperature": req.Temperature,
		"max_tokens":  req.MaxTokens,
	}
	if len(req.Tools) > 0 {
		reqMap["tools"] = req.Tools
	}

	body, err := json.Marshal(reqMap)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, p.baseURL+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Authorization", "Bearer "+p.apiKey)

	resp, err := p.httpClient.Do(httpReq)
	if err != nil {
		return nil, fmt.Errorf("http request: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, string(respBody))
	}

	return respBody, nil
}

// parseSSEStream reads an SSE stream and calls callback for each data chunk.
func parseSSEStream(r io.Reader, callback func(chunk string) error) error {
	buf := make([]byte, 4096)
	var remaining []byte

	for {
		n, err := r.Read(buf)
		if n > 0 {
			remaining = append(remaining, buf[:n]...)
			for {
				idx := bytes.Index(remaining, []byte("\n\n"))
				if idx == -1 {
					break
				}
				line := string(remaining[:idx])
				remaining = remaining[idx+2:]

				if len(line) > 6 && line[:6] == "data: " {
					data := line[6:]
					if data == "[DONE]" {
						return nil
					}
					var chunk struct {
						Choices []struct {
							Delta struct {
								Content string `json:"content"`
							} `json:"delta"`
						} `json:"choices"`
					}
					if err := json.Unmarshal([]byte(data), &chunk); err == nil {
						if len(chunk.Choices) > 0 && chunk.Choices[0].Delta.Content != "" {
							if err := callback(chunk.Choices[0].Delta.Content); err != nil {
								return err
							}
						}
					}
				}
			}
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("read stream: %w", err)
		}
	}
	return nil
}
