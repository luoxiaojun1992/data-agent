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

	conv := &contentConverter{role: role}
	for _, p := range c.Parts {
		if p != nil {
			conv.addPart(p)
		}
	}
	conv.flush()
	return conv.out
}

// contentConverter accumulates text and tool calls into OpenAI messages.
type contentConverter struct {
	role    string
	out     []chatMessage
	textBuf strings.Builder
	calls   []toolCall
	callIdx int
}

func (cv *contentConverter) addPart(p *genai.Part) {
	switch {
	case p.FunctionResponse != nil:
		cv.addFunctionResponse(p.FunctionResponse)
	case p.FunctionCall != nil:
		cv.addFunctionCall(p.FunctionCall)
	default:
		cv.textBuf.WriteString(p.Text)
	}
}

func (cv *contentConverter) addFunctionCall(fc *genai.FunctionCall) {
	args, err := json.Marshal(fc.Args)
	if err != nil {
		args = []byte("{}")
	}
	tc := toolCall{ID: functionCallID(fc.ID, fc.Name, cv.callIdx), Type: "function"}
	tc.Function.Name = fc.Name
	tc.Function.Arguments = string(args)
	cv.calls = append(cv.calls, tc)
	cv.callIdx++
}

func (cv *contentConverter) addFunctionResponse(fr *genai.FunctionResponse) {
	// Flush pending assistant content before emitting the tool result.
	cv.flush()
	respJSON, err := json.Marshal(fr.Response)
	if err != nil {
		respJSON = []byte(`{"error":"marshal failed"}`)
	}
	cv.out = append(cv.out, chatMessage{
		Role:       "tool",
		Content:    string(respJSON),
		ToolCallID: functionCallID(fr.ID, fr.Name, cv.callIdx),
	})
	cv.callIdx++
}

func (cv *contentConverter) flush() {
	if cv.textBuf.Len() == 0 && len(cv.calls) == 0 {
		return
	}
	msg := chatMessage{Role: cv.role, Content: cv.textBuf.String()}
	if len(cv.calls) > 0 {
		msg.ToolCalls = cv.calls
	}
	cv.out = append(cv.out, msg)
	cv.textBuf.Reset()
	cv.calls = nil
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
	acc := &sseAccumulator{callBufs: map[int]*toolCall{}}
	if err := consumeSSE(r, acc.processChunk); err != nil {
		return nil, err
	}
	return acc.result(), nil
}

// sseAccumulator aggregates streamed text and tool call deltas.
type sseAccumulator struct {
	textBuf  strings.Builder
	callBufs map[int]*toolCall
	finished bool
}

// streamChunk mirrors the OpenAI streaming chunk payload.
type streamChunk struct {
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

func (a *sseAccumulator) processChunk(data string) {
	if a.finished {
		return
	}
	var chunk streamChunk
	if err := json.Unmarshal([]byte(data), &chunk); err != nil {
		return
	}
	for _, choice := range chunk.Choices {
		a.textBuf.WriteString(choice.Delta.Content)
		for _, dtc := range choice.Delta.ToolCalls {
			a.appendToolCallDelta(dtc.Index, dtc.ID, dtc.Function.Name, dtc.Function.Arguments)
		}
		if choice.FinishReason != "" {
			a.finished = true
		}
	}
}

func (a *sseAccumulator) appendToolCallDelta(index int, id, name, args string) {
	buf, ok := a.callBufs[index]
	if !ok {
		buf = &toolCall{Type: "function"}
		a.callBufs[index] = buf
	}
	if id != "" {
		buf.ID = id
	}
	if name != "" {
		buf.Function.Name = name
	}
	buf.Function.Arguments += args
}

func (a *sseAccumulator) result() *model.LLMResponse {
	var calls []toolCall
	for i := 0; i < len(a.callBufs); i++ {
		if tc, ok := a.callBufs[i]; ok {
			calls = append(calls, *tc)
		}
	}
	return buildLLMResponse(a.textBuf.String(), calls, false)
}

// consumeSSE reads an SSE stream line by line, invoking fn for each data payload.
func consumeSSE(r io.Reader, fn func(data string)) error {
	buf := make([]byte, 4096)
	var remaining []byte
	for {
		n, err := r.Read(buf)
		if n > 0 {
			remaining = append(remaining, buf[:n]...)
			var done bool
			remaining, done = drainSSELines(remaining, fn)
			if done {
				return nil
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

// drainSSELines processes all complete lines in the buffer, returning the
// unconsumed remainder and whether [DONE] was seen.
func drainSSELines(remaining []byte, fn func(data string)) ([]byte, bool) {
	for {
		idx := bytes.Index(remaining, []byte("\n"))
		if idx == -1 {
			return remaining, false
		}
		line := strings.TrimRight(string(remaining[:idx]), "\r")
		remaining = remaining[idx+1:]
		if done := dispatchSSELine(line, fn); done {
			return remaining, true
		}
	}
}

// dispatchSSELine handles a single SSE line, returning true for [DONE].
func dispatchSSELine(line string, fn func(data string)) bool {
	if !strings.HasPrefix(line, "data: ") {
		return false
	}
	data := line[6:]
	if data == "[DONE]" {
		return true
	}
	fn(data)
	return false
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
