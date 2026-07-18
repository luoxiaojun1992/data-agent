// Package adkmodel provides OpenAI-compatible model.LLM implementations
// for the ADK engine. It replaces the legacy hand-written Router/Provider
// with an ADK-native model that supports streaming, tool calls, and a
// fallback chain across multiple backends.
package adkmodel

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"strings"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// Backend describes one OpenAI-compatible endpoint.
type Backend struct {
	// Model is the model name sent in the request payload.
	Model string
	// BaseURL is the endpoint root, e.g. "http://mockllm:8082" ("/v1/chat/completions" is appended).
	BaseURL string
	// APIKey is sent as Bearer token. May be empty for local mock services.
	APIKey string
	// MaxTokens caps generation when the request does not specify one.
	MaxTokens int
	// Temperature is applied when the request does not specify one.
	Temperature float64
}

// OpenAIModel implements model.LLM against an OpenAI-compatible /v1/chat/completions API.
type OpenAIModel struct {
	backend Backend
	client  *http.Client
}

// NewOpenAIModel creates an OpenAI-compatible model.LLM.
func NewOpenAIModel(b Backend) *OpenAIModel {
	if b.MaxTokens <= 0 {
		b.MaxTokens = 8192
	}
	return &OpenAIModel{
		backend: b,
		client:  &http.Client{Timeout: 120 * time.Second},
	}
}

// Name returns the model name.
func (m *OpenAIModel) Name() string { return m.backend.Model }

// GenerateContent implements model.LLM. When stream is true the SSE stream is
// consumed internally and a single aggregated response is yielded, so callers
// always observe exactly one complete LLMResponse (or an error).
func (m *OpenAIModel) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		httpReq, err := m.buildRequest(ctx, req, stream)
		if err != nil {
			yield(nil, err)
			return
		}

		resp, err := m.client.Do(httpReq)
		if err != nil {
			yield(nil, fmt.Errorf("openai model %q: http request: %w", m.backend.Model, err))
			return
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			yield(nil, fmt.Errorf("openai model %q: API error %d: %s", m.backend.Model, resp.StatusCode, string(body)))
			return
		}

		var llmResp *model.LLMResponse
		if stream {
			llmResp, err = aggregateSSE(resp.Body)
		} else {
			llmResp, err = parseJSONResponse(resp.Body)
		}
		if err != nil {
			yield(nil, fmt.Errorf("openai model %q: %w", m.backend.Model, err))
			return
		}
		yield(llmResp, nil)
	}
}

// ---- request building ----

type chatMessage struct {
	Role       string     `json:"role"`
	Content    string     `json:"content,omitempty"`
	ToolCalls  []toolCall `json:"tool_calls,omitempty"`
	ToolCallID string     `json:"tool_call_id,omitempty"`
}

type toolCall struct {
	ID       string `json:"id"`
	Type     string `json:"type"`
	Function struct {
		Name      string `json:"name"`
		Arguments string `json:"arguments"`
	} `json:"function"`
}

type toolDef struct {
	Type     string `json:"type"`
	Function struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Parameters  json.RawMessage `json:"parameters,omitempty"`
	} `json:"function"`
}

func (m *OpenAIModel) buildRequest(ctx context.Context, req *model.LLMRequest, stream bool) (*http.Request, error) {
	messages := contentsToMessages(req)
	payload := map[string]any{
		"model":    m.backend.Model,
		"messages": messages,
		"stream":   stream,
	}

	maxTokens := m.backend.MaxTokens
	temperature := m.backend.Temperature
	if req.Config != nil {
		if req.Config.MaxOutputTokens > 0 {
			maxTokens = int(req.Config.MaxOutputTokens)
		}
		if req.Config.Temperature != nil {
			temperature = float64(*req.Config.Temperature)
		}
		if tools := convertTools(req.Config.Tools); len(tools) > 0 {
			payload["tools"] = tools
		}
	}
	if maxTokens > 0 {
		payload["max_tokens"] = maxTokens
	}
	payload["temperature"] = temperature

	body, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost,
		strings.TrimSuffix(m.backend.BaseURL, "/")+"/v1/chat/completions", bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	httpReq.Header.Set("Content-Type", "application/json")
	if m.backend.APIKey != "" {
		httpReq.Header.Set("Authorization", "Bearer "+m.backend.APIKey)
	}
	return httpReq, nil
}

// contentsToMessages converts ADK genai contents into OpenAI chat messages.
func contentsToMessages(req *model.LLMRequest) []chatMessage {
	var messages []chatMessage

	if req.Config != nil && req.Config.SystemInstruction != nil {
		if text := joinTextParts(req.Config.SystemInstruction); text != "" {
			messages = append(messages, chatMessage{Role: "system", Content: text})
		}
	}

	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		messages = append(messages, contentToMessages(c)...)
	}
	return messages
}

// contentToMessages converts a single genai.Content into one or more OpenAI messages.
// FunctionResponse parts become separate "tool" role messages.
func contentToMessages(c *genai.Content) []chatMessage {
	role := c.Role
	if role == "model" {
		role = "assistant"
	}

	var out []chatMessage
	var textBuf strings.Builder
	var calls []toolCall
	callIdx := 0

	flush := func() {
		if textBuf.Len() == 0 && len(calls) == 0 {
			return
		}
		msg := chatMessage{Role: role, Content: textBuf.String()}
		if len(calls) > 0 {
			msg.ToolCalls = calls
		}
		out = append(out, msg)
		textBuf.Reset()
		calls = nil
	}

	for _, p := range c.Parts {
		if p == nil {
			continue
		}
		switch {
		case p.FunctionResponse != nil:
			// Flush pending assistant content before emitting tool result.
			flush()
			respJSON, err := json.Marshal(p.FunctionResponse.Response)
			if err != nil {
				respJSON = []byte(`{"error":"marshal failed"}`)
			}
			out = append(out, chatMessage{
				Role:       "tool",
				Content:    string(respJSON),
				ToolCallID: functionCallID(p.FunctionResponse.ID, p.FunctionResponse.Name, callIdx),
			})
			callIdx++
		case p.FunctionCall != nil:
			args, err := json.Marshal(p.FunctionCall.Args)
			if err != nil {
				args = []byte("{}")
			}
			tc := toolCall{ID: functionCallID(p.FunctionCall.ID, p.FunctionCall.Name, callIdx), Type: "function"}
			tc.Function.Name = p.FunctionCall.Name
			tc.Function.Arguments = string(args)
			calls = append(calls, tc)
			callIdx++
		default:
			textBuf.WriteString(p.Text)
		}
	}
	flush()

	return out
}

// functionCallID returns a stable tool call id.
func functionCallID(id, name string, idx int) string {
	if id != "" {
		return id
	}
	if name != "" {
		return fmt.Sprintf("call_%s_%d", name, idx)
	}
	return fmt.Sprintf("call_%d", idx)
}

func joinTextParts(c *genai.Content) string {
	var sb strings.Builder
	for _, p := range c.Parts {
		if p != nil {
			sb.WriteString(p.Text)
		}
	}
	return sb.String()
}

// convertTools converts genai tool declarations into OpenAI tool definitions.
func convertTools(tools []*genai.Tool) []toolDef {
	var out []toolDef
	for _, t := range tools {
		if t == nil {
			continue
		}
		for _, fd := range t.FunctionDeclarations {
			if fd == nil {
				continue
			}
			td := toolDef{Type: "function"}
			td.Function.Name = fd.Name
			td.Function.Description = fd.Description
			td.Function.Parameters = functionSchemaJSON(fd)
			out = append(out, td)
		}
	}
	return out
}

// functionSchemaJSON renders the function parameter schema as raw JSON.
func functionSchemaJSON(fd *genai.FunctionDeclaration) json.RawMessage {
	var raw []byte
	var err error
	switch {
	case fd.ParametersJsonSchema != nil:
		raw, err = json.Marshal(fd.ParametersJsonSchema)
	case fd.Parameters != nil:
		raw, err = json.Marshal(fd.Parameters)
	default:
		return nil
	}
	if err != nil || string(raw) == "null" || len(raw) == 0 {
		return nil
	}
	return raw
}

// ---- response parsing ----

type chatCompletionResponse struct {
	Choices []struct {
		Message struct {
			Content   string     `json:"content"`
			ToolCalls []toolCall `json:"tool_calls"`
		} `json:"message"`
	} `json:"choices"`
	Usage *struct {
		PromptTokens     int `json:"prompt_tokens"`
		CompletionTokens int `json:"completion_tokens"`
		TotalTokens      int `json:"total_tokens"`
	} `json:"usage"`
}

// parseJSONResponse parses a non-streaming chat completion response.
func parseJSONResponse(r io.Reader) (*model.LLMResponse, error) {
	body, err := io.ReadAll(r)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}
	var parsed chatCompletionResponse
	if err := json.Unmarshal(body, &parsed); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	if len(parsed.Choices) == 0 {
		return nil, fmt.Errorf("no choices in response")
	}
	return buildLLMResponse(parsed.Choices[0].Message.Content, parsed.Choices[0].Message.ToolCalls, parsed.Usage != nil), nil
}

// aggregateSSE consumes an OpenAI SSE stream and returns one aggregated response.
func aggregateSSE(r io.Reader) (*model.LLMResponse, error) {
	var textBuf strings.Builder
	callBufs := map[int]*toolCall{}
	finish := false

	err := consumeSSE(r, func(data string) {
		if finish {
			return
		}
		var chunk struct {
			Choices []struct {
				Delta struct {
					Content   string `json:"content"`
					ToolCalls []struct {
						Index    int    `json:"index"`
						ID       string `json:"id"`
						Type     string `json:"type"`
						Function struct {
							Name      string `json:"name"`
							Arguments string `json:"arguments"`
						} `json:"function"`
					} `json:"tool_calls"`
				} `json:"delta"`
				FinishReason string `json:"finish_reason"`
			} `json:"choices"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err != nil {
			return
		}
		for _, choice := range chunk.Choices {
			textBuf.WriteString(choice.Delta.Content)
			for _, dtc := range choice.Delta.ToolCalls {
				buf, ok := callBufs[dtc.Index]
				if !ok {
					buf = &toolCall{ID: dtc.ID, Type: "function"}
					callBufs[dtc.Index] = buf
				}
				if dtc.ID != "" {
					buf.ID = dtc.ID
				}
				if dtc.Function.Name != "" {
					buf.Function.Name = dtc.Function.Name
				}
				buf.Function.Arguments += dtc.Function.Arguments
			}
			if choice.FinishReason != "" {
				finish = true
			}
		}
	})
	if err != nil {
		return nil, err
	}

	var calls []toolCall
	for i := 0; i < len(callBufs); i++ {
		if tc, ok := callBufs[i]; ok {
			calls = append(calls, *tc)
		}
	}
	return buildLLMResponse(textBuf.String(), calls, false), nil
}

// consumeSSE reads an SSE stream line by line, invoking fn for each data payload.
func consumeSSE(r io.Reader, fn func(data string)) error {
	buf := make([]byte, 4096)
	var remaining []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			remaining = append(remaining, buf[:n]...)
			for {
				idx := bytes.Index(remaining, []byte("\n"))
				if idx == -1 {
					break
				}
				line := strings.TrimRight(string(remaining[:idx]), "\r")
				remaining = remaining[idx+1:]
				if !strings.HasPrefix(line, "data: ") {
					continue
				}
				data := line[6:]
				if data == "[DONE]" {
					return nil
				}
				fn(data)
			}
		}
		if err == io.EOF {
			return nil
		}
		if err != nil {
			return fmt.Errorf("read stream: %w", err)
		}
	}
}

// buildLLMResponse assembles an ADK LLMResponse from content and tool calls.
func buildLLMResponse(content string, calls []toolCall, _ bool) *model.LLMResponse {
	var parts []*genai.Part
	if content != "" {
		parts = append(parts, &genai.Part{Text: content})
	}
	for _, tc := range calls {
		var args map[string]any
		if err := json.Unmarshal([]byte(tc.Function.Arguments), &args); err != nil {
			args = map[string]any{"_raw": tc.Function.Arguments}
		}
		parts = append(parts, &genai.Part{FunctionCall: &genai.FunctionCall{
			ID:   tc.ID,
			Name: tc.Function.Name,
			Args: args,
		}})
	}
	return &model.LLMResponse{
		Content: &genai.Content{Role: "model", Parts: parts},
	}
}
