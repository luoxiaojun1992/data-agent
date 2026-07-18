package adkmodel

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"iter"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// newTestServer starts an httptest server with the given handler and returns
// it with a model pointing at it.
func newTestServer(t *testing.T, h http.HandlerFunc) (*httptest.Server, *OpenAIModel) {
	t.Helper()
	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	m := NewOpenAIModel(Backend{Model: "test-model", BaseURL: srv.URL, APIKey: "k"})
	return srv, m
}

func textRequest(text string) *model.LLMRequest {
	return &model.LLMRequest{
		Model:    "test-model",
		Contents: []*genai.Content{genai.NewContentFromText(text, "user")},
	}
}

// collect drains a GenerateContent iterator.
func collect(t *testing.T, seq func(func(*model.LLMResponse, error) bool)) ([]*model.LLMResponse, error) {
	t.Helper()
	var resps []*model.LLMResponse
	var outErr error
	for resp, err := range seq {
		if err != nil {
			outErr = err
			break
		}
		resps = append(resps, resp)
	}
	return resps, outErr
}

func TestName(t *testing.T) {
	m := NewOpenAIModel(Backend{Model: "m1"})
	if m.Name() != "m1" {
		t.Errorf("Name() = %q, want m1", m.Name())
	}
}

func TestNewOpenAIModel_Defaults(t *testing.T) {
	m := NewOpenAIModel(Backend{Model: "m"})
	if m.backend.MaxTokens != 8192 {
		t.Errorf("default MaxTokens = %d, want 8192", m.backend.MaxTokens)
	}
	if m.client == nil {
		t.Error("http client should be initialized")
	}
}

func TestGenerateContent_NonStream_Success(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" {
			t.Errorf("unexpected path %s", r.URL.Path)
		}
		if r.Header.Get("Authorization") != "Bearer k" {
			t.Errorf("missing auth header")
		}
		body, _ := io.ReadAll(r.Body)
		var payload map[string]any
		_ = json.Unmarshal(body, &payload)
		if payload["model"] != "test-model" {
			t.Errorf("payload model = %v", payload["model"])
		}
		if payload["stream"] != false {
			t.Errorf("stream should be false")
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"hello world"}}],"usage":{"prompt_tokens":1,"completion_tokens":2,"total_tokens":3}}`)
	})

	resps, err := collect(t, m.GenerateContent(context.Background(), textRequest("hi"), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resps) != 1 {
		t.Fatalf("expected 1 response, got %d", len(resps))
	}
	if got := resps[0].Content.Parts[0].Text; got != "hello world" {
		t.Errorf("text = %q, want %q", got, "hello world")
	}
	if resps[0].Content.Role != "model" {
		t.Errorf("role = %q, want model", resps[0].Content.Role)
	}
}

func TestGenerateContent_ToolCalls(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","content":"","tool_calls":[{"id":"c1","type":"function","function":{"name":"knowledge_search","arguments":"{\"query\":\"营收\"}"}}]}}]}`)
	})

	resps, err := collect(t, m.GenerateContent(context.Background(), textRequest("查一下"), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fc := resps[0].Content.Parts[0].FunctionCall
	if fc == nil {
		t.Fatal("expected function call part")
	}
	if fc.Name != "knowledge_search" || fc.Args["query"] != "营收" {
		t.Errorf("function call = %+v", fc)
	}
}

func TestGenerateContent_ToolCallBadArgs(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"choices":[{"message":{"role":"assistant","tool_calls":[{"id":"c1","type":"function","function":{"name":"f","arguments":"not-json"}}]}}]}`)
	})
	resps, err := collect(t, m.GenerateContent(context.Background(), textRequest("x"), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fc := resps[0].Content.Parts[0].FunctionCall
	if fc.Args["_raw"] != "not-json" {
		t.Errorf("bad args should be preserved in _raw, got %v", fc.Args)
	}
}

func TestGenerateContent_Stream_Aggregates(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		flusher := w.(http.Flusher)
		chunks := []string{
			`{"choices":[{"index":0,"delta":{"role":"assistant","content":"Hello"},"finish_reason":""}]}`,
			`{"choices":[{"index":0,"delta":{"content":" World"},"finish_reason":""}]}`,
			`{"choices":[{"index":0,"delta":{"content":""},"finish_reason":"stop"}]}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
			flusher.Flush()
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
		flusher.Flush()
	})

	resps, err := collect(t, m.GenerateContent(context.Background(), textRequest("hi"), true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(resps) != 1 {
		t.Fatalf("expected 1 aggregated response, got %d", len(resps))
	}
	if got := resps[0].Content.Parts[0].Text; got != "Hello World" {
		t.Errorf("aggregated text = %q, want %q", got, "Hello World")
	}
}

func TestGenerateContent_Stream_ToolCallDeltas(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		chunks := []string{
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"id":"c1","type":"function","function":{"name":"stats_compute","arguments":"{\"method\":"}}]},"finish_reason":""}]}`,
			`{"choices":[{"index":0,"delta":{"tool_calls":[{"index":0,"function":{"arguments":"\"descriptive\",\"values\":[1,2]}"}}]},"finish_reason":""}]}`,
			`{"choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
		}
		for _, c := range chunks {
			fmt.Fprintf(w, "data: %s\n\n", c)
		}
		fmt.Fprint(w, "data: [DONE]\n\n")
	})

	resps, err := collect(t, m.GenerateContent(context.Background(), textRequest("stats"), true))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	fc := resps[0].Content.Parts[0].FunctionCall
	if fc == nil || fc.Name != "stats_compute" || fc.Args["method"] != "descriptive" {
		t.Errorf("streamed function call = %+v", fc)
	}
}

func TestGenerateContent_HTTPError(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusTooManyRequests)
		fmt.Fprint(w, `{"error":"rate limited"}`)
	})
	_, err := collect(t, m.GenerateContent(context.Background(), textRequest("hi"), false))
	if err == nil || !strings.Contains(err.Error(), "429") {
		t.Errorf("expected 429 error, got %v", err)
	}
}

func TestGenerateContent_InvalidJSON(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `not json`)
	})
	_, err := collect(t, m.GenerateContent(context.Background(), textRequest("hi"), false))
	if err == nil {
		t.Error("expected parse error")
	}
}

func TestGenerateContent_EmptyChoices(t *testing.T) {
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprint(w, `{"choices":[]}`)
	})
	_, err := collect(t, m.GenerateContent(context.Background(), textRequest("hi"), false))
	if err == nil || !strings.Contains(err.Error(), "no choices") {
		t.Errorf("expected no choices error, got %v", err)
	}
}

func TestGenerateContent_Unreachable(t *testing.T) {
	m := NewOpenAIModel(Backend{Model: "m", BaseURL: "http://127.0.0.1:1"})
	_, err := collect(t, m.GenerateContent(context.Background(), textRequest("hi"), false))
	if err == nil {
		t.Error("expected connection error")
	}
}

func TestBuildRequest_SystemInstructionAndConfig(t *testing.T) {
	var captured map[string]any
	_, m := newTestServer(t, func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &captured)
		fmt.Fprint(w, `{"choices":[{"message":{"content":"ok"}}]}`)
	})

	temp := float32(0.3)
	req := &model.LLMRequest{
		Model: "test-model",
		Contents: []*genai.Content{
			nil, // nil content must be skipped
			genai.NewContentFromText("hello", "user"),
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText("be helpful", "system"),
			MaxOutputTokens:   123,
			Temperature:       &temp,
			Tools: []*genai.Tool{
				nil,
				{FunctionDeclarations: []*genai.FunctionDeclaration{
					nil,
					{Name: "f1", Description: "d1", ParametersJsonSchema: map[string]any{"type": "object"}},
					{Name: "f2", Description: "d2"},
				}},
			},
		},
	}
	_, err := collect(t, m.GenerateContent(context.Background(), req, false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	msgs, _ := captured["messages"].([]any)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 messages (system+user), got %d", len(msgs))
	}
	if msgs[0].(map[string]any)["role"] != "system" {
		t.Errorf("first message should be system: %v", msgs[0])
	}
	if captured["max_tokens"].(float64) != 123 {
		t.Errorf("max_tokens = %v, want 123", captured["max_tokens"])
	}
	if captured["temperature"].(float64) < 0.29 || captured["temperature"].(float64) > 0.31 {
		t.Errorf("temperature = %v, want ~0.3", captured["temperature"])
	}
	tools, _ := captured["tools"].([]any)
	if len(tools) != 2 {
		t.Fatalf("expected 2 tools, got %d", len(tools))
	}
	f1 := tools[0].(map[string]any)["function"].(map[string]any)
	if f1["name"] != "f1" || f1["parameters"] == nil {
		t.Errorf("tool f1 malformed: %v", f1)
	}
}

func TestContentToMessages_FunctionCallAndResponse(t *testing.T) {
	content := &genai.Content{
		Role: "model",
		Parts: []*genai.Part{
			nil,
			{Text: "let me check"},
			{FunctionCall: &genai.FunctionCall{Name: "sql_validate", Args: map[string]any{"query": "SELECT 1"}}},
			{FunctionCall: &genai.FunctionCall{ID: "explicit-id", Name: "stats_compute", Args: nil}},
		},
	}
	msgs := contentToMessages(content)
	if len(msgs) != 1 {
		t.Fatalf("expected 1 message, got %d", len(msgs))
	}
	if msgs[0].Role != "assistant" || msgs[0].Content != "let me check" {
		t.Errorf("assistant message = %+v", msgs[0])
	}
	if len(msgs[0].ToolCalls) != 2 {
		t.Fatalf("expected 2 tool calls, got %d", len(msgs[0].ToolCalls))
	}
	if msgs[0].ToolCalls[0].ID != "call_sql_validate_0" {
		t.Errorf("generated call id = %q", msgs[0].ToolCalls[0].ID)
	}
	if msgs[0].ToolCalls[1].ID != "explicit-id" {
		t.Errorf("explicit id not preserved: %q", msgs[0].ToolCalls[1].ID)
	}

	// Function responses become tool messages after flushing assistant content.
	respContent := &genai.Content{
		Role: "user",
		Parts: []*genai.Part{
			{FunctionResponse: &genai.FunctionResponse{Name: "sql_validate", Response: map[string]any{"ok": true}}},
		},
	}
	toolMsgs := contentToMessages(respContent)
	if len(toolMsgs) != 1 || toolMsgs[0].Role != "tool" {
		t.Fatalf("expected 1 tool message, got %+v", toolMsgs)
	}
	if !strings.Contains(toolMsgs[0].Content, `"ok":true`) {
		t.Errorf("tool response content = %q", toolMsgs[0].Content)
	}
	if toolMsgs[0].ToolCallID != "call_sql_validate_0" {
		t.Errorf("tool call id = %q", toolMsgs[0].ToolCallID)
	}
}

func TestFunctionCallID(t *testing.T) {
	if got := functionCallID("x", "name", 0); got != "x" {
		t.Errorf("explicit id: %q", got)
	}
	if got := functionCallID("", "name", 2); got != "call_name_2" {
		t.Errorf("name-based id: %q", got)
	}
	if got := functionCallID("", "", 3); got != "call_3" {
		t.Errorf("index-based id: %q", got)
	}
}

func TestAggregateSSE_BadChunkSkipped(t *testing.T) {
	body := strings.NewReader("data: {bad json}\n\ndata: {\"choices\":[{\"delta\":{\"content\":\"ok\"},\"finish_reason\":\"stop\"}]}\n\ndata: [DONE]\n\n")
	resp, err := aggregateSSE(body)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resp.Content.Parts[0].Text != "ok" {
		t.Errorf("text = %q", resp.Content.Parts[0].Text)
	}
}

func TestConsumeSSE_ReadError(t *testing.T) {
	err := consumeSSE(&failReader{}, func(string) {})
	if err == nil {
		t.Error("expected read error")
	}
}

type failReader struct{}

func (r *failReader) Read([]byte) (int, error) { return 0, fmt.Errorf("boom") }

func TestFunctionSchemaJSON(t *testing.T) {
	// ParametersJsonSchema takes precedence.
	fd := &genai.FunctionDeclaration{
		ParametersJsonSchema: map[string]any{"type": "object"},
		Parameters:           &genai.Schema{Type: genai.TypeObject},
	}
	if raw := functionSchemaJSON(fd); raw == nil || !strings.Contains(string(raw), "object") {
		t.Errorf("json schema: %s", raw)
	}

	// genai.Schema fallback.
	fd2 := &genai.FunctionDeclaration{Parameters: &genai.Schema{Type: genai.TypeObject}}
	if raw := functionSchemaJSON(fd2); raw == nil {
		t.Error("schema fallback should marshal genai.Schema")
	}

	// No schema at all → nil.
	if raw := functionSchemaJSON(&genai.FunctionDeclaration{}); raw != nil {
		t.Errorf("empty schema should be nil, got %s", raw)
	}
}

// errAfterYieldLLM yields one response then fails — the caller must not fall back.
type errAfterYieldLLM struct{}

func (s *errAfterYieldLLM) Name() string { return "flaky" }

func (s *errAfterYieldLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if !yield(&model.LLMResponse{Content: genai.NewContentFromText("partial", "model")}, nil) {
			return
		}
		yield(nil, fmt.Errorf("stream broke"))
	}
}

func TestFallbackLLM_NoFallbackAfterYield(t *testing.T) {
	f, _ := NewFallbackLLM(&errAfterYieldLLM{}, &stubLLM{name: "b", content: "backup"})
	var texts []string
	_, err := collect(t, func(yield func(*model.LLMResponse, error) bool) {
		for resp, e := range f.GenerateContent(context.Background(), textRequest("hi"), false) {
			if e != nil {
				yield(nil, e)
				return
			}
			texts = append(texts, resp.Content.Parts[0].Text)
		}
	})
	if err == nil || !strings.Contains(err.Error(), "stream broke") {
		t.Errorf("mid-stream error should propagate, got %v", err)
	}
	if len(texts) != 1 || texts[0] != "partial" {
		t.Errorf("should have kept partial response without fallback, got %v", texts)
	}
}

func TestFallbackLLM_EarlyBreak(t *testing.T) {
	f, _ := NewFallbackLLM(&stubLLM{name: "a", content: "ok"})
	count := 0
	for range f.GenerateContent(context.Background(), textRequest("hi"), false) {
		count++
		break // consumer stops early
	}
	if count != 1 {
		t.Errorf("early break should stop iteration, count=%d", count)
	}
}

// ---- FallbackLLM ----

type stubLLM struct {
	name    string
	content string
	err     error
}

func (s *stubLLM) Name() string { return s.name }

func (s *stubLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if s.err != nil {
			yield(nil, s.err)
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(s.content, "model")}, nil)
	}
}

func TestNewFallbackLLM_Validation(t *testing.T) {
	if _, err := NewFallbackLLM(); err == nil {
		t.Error("empty chain should fail")
	}
	if _, err := NewFallbackLLM(&stubLLM{name: "a"}, nil); err == nil {
		t.Error("nil model should fail")
	}
	f, err := NewFallbackLLM(&stubLLM{name: "a"}, &stubLLM{name: "b"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.Name() != "a,b" {
		t.Errorf("Name() = %q, want a,b", f.Name())
	}
}

func TestFallbackLLM_PrimarySuccess(t *testing.T) {
	f, _ := NewFallbackLLM(&stubLLM{name: "a", content: "ok"}, &stubLLM{name: "b", content: "backup"})
	resps, err := collect(t, f.GenerateContent(context.Background(), textRequest("hi"), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resps[0].Content.Parts[0].Text != "ok" {
		t.Errorf("primary should answer, got %q", resps[0].Content.Parts[0].Text)
	}
}

func TestFallbackLLM_FallsBack(t *testing.T) {
	f, _ := NewFallbackLLM(
		&stubLLM{name: "a", err: fmt.Errorf("down")},
		&stubLLM{name: "b", content: "backup"},
	)
	resps, err := collect(t, f.GenerateContent(context.Background(), textRequest("hi"), false))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if resps[0].Content.Parts[0].Text != "backup" {
		t.Errorf("fallback should answer, got %q", resps[0].Content.Parts[0].Text)
	}
}

func TestFallbackLLM_AllFail(t *testing.T) {
	f, _ := NewFallbackLLM(
		&stubLLM{name: "a", err: fmt.Errorf("down-a")},
		&stubLLM{name: "b", err: fmt.Errorf("down-b")},
	)
	_, err := collect(t, f.GenerateContent(context.Background(), textRequest("hi"), false))
	if err == nil || !strings.Contains(err.Error(), "all 2 model backends failed") {
		t.Errorf("expected aggregated failure, got %v", err)
	}
	if !strings.Contains(err.Error(), "down-a") || !strings.Contains(err.Error(), "down-b") {
		t.Errorf("error should include both backends: %v", err)
	}
}
