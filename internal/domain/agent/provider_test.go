package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
)

func TestNewOpenAIProvider(t *testing.T) {
	p := NewOpenAIProvider("https://api.openai.com", "sk-test")
	if p == nil {
		t.Fatal("should not return nil")
	}
	if p.baseURL != "https://api.openai.com" {
		t.Errorf("baseURL: got %s", p.baseURL)
	}
	if p.httpClient == nil {
		t.Fatal("httpClient should be initialized")
	}
}

func sseChunk(content string) string {
	data := map[string]interface{}{
		"choices": []map[string]interface{}{{"delta": map[string]string{"content": content}}},
	}
	b, _ := json.Marshal(data)
	return fmt.Sprintf("data: %s\n\n", string(b))
}

func TestParseSSEStream(t *testing.T) {
	t.Run("multiple chunks", func(t *testing.T) {
		input := sseChunk("hello") + sseChunk("world") + "data: [DONE]\n\n"
		var chunks []string
		err := parseSSEStream(bytes.NewReader([]byte(input)), func(chunk string) error {
			chunks = append(chunks, chunk)
			return nil
		})
		if err != nil {
			t.Fatalf("parseSSEStream: %v", err)
		}
		if len(chunks) != 2 {
			t.Errorf("got %d chunks, want 2: %v", len(chunks), chunks)
		}
	})

	t.Run("empty", func(t *testing.T) {
		err := parseSSEStream(bytes.NewReader([]byte("")), func(chunk string) error {
			return nil
		})
		if err != nil {
			t.Errorf("should handle empty stream: %v", err)
		}
	})

	t.Run("callback error", func(t *testing.T) {
		err := parseSSEStream(bytes.NewReader([]byte(sseChunk("test"))), func(chunk string) error {
			return fmt.Errorf("callback failed")
		})
		if err == nil {
			t.Fatal("should propagate callback error")
		}
	})

	t.Run("no data prefix", func(t *testing.T) {
		var called bool
		err := parseSSEStream(bytes.NewReader([]byte("other: stuff\n\n")), func(chunk string) error {
			called = true
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected: %v", err)
		}
		if called {
			t.Fatal("should not call callback for non-data lines")
		}
	})
}

func TestOpenAIProvider_DoRequest(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")

	t.Run("success", func(t *testing.T) {
		respBody := io.NopCloser(bytes.NewReader([]byte(`{"choices": [{"message": {"content": "hi"}}]}`)))
		resp := &http.Response{StatusCode: 200, Body: respBody}
		patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
		defer patches.Reset()

		body, err := p.doRequest(context.Background(), ChatRequest{Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}}}, false)
		if err != nil {
			t.Fatalf("doRequest: %v", err)
		}
		if len(body) == 0 {
			t.Fatal("body should not be empty")
		}
	})

	t.Run("http error", func(t *testing.T) {
		patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", (*http.Response)(nil), context.DeadlineExceeded)
		defer patches.Reset()

		_, err := p.doRequest(context.Background(), ChatRequest{Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}}}, false)
		if err == nil {
			t.Fatal("should error on HTTP failure")
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		respBody := io.NopCloser(bytes.NewReader([]byte(`{"error": "bad"}`)))
		resp := &http.Response{StatusCode: 400, Body: respBody}
		patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
		defer patches.Reset()

		_, err := p.doRequest(context.Background(), ChatRequest{Model: "gpt-4", Messages: []Message{}}, false)
		if err == nil {
			t.Fatal("should error on non-200")
		}
	})
}

func TestOpenAIProvider_Chat(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")

	respBody := io.NopCloser(bytes.NewReader([]byte(
		`{"choices": [{"message": {"role": "assistant", "content": "hello world"}}]}`)))
	resp := &http.Response{StatusCode: 200, Body: respBody}
	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
	defer patches.Reset()

	result, err := p.Chat(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Chat: %v", err)
	}
	if result.Content != "hello world" {
		t.Errorf("content: got %q", result.Content)
	}
}

func TestOpenAIProvider_Chat_Non200(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")
	respBody := io.NopCloser(bytes.NewReader([]byte(`{"error":"bad"}`)))
	resp := &http.Response{StatusCode: 500, Body: respBody}
	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
	defer patches.Reset()

	_, err := p.Chat(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Fatal("should error on non-200")
	}
}

func TestOpenAIProvider_Chat_WithTools(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")
	respBody := io.NopCloser(bytes.NewReader([]byte(`{"choices": [{"message": {"content": "ok"}}]}`)))
	resp := &http.Response{StatusCode: 200, Body: respBody}
	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
	defer patches.Reset()

	result, err := p.Chat(context.Background(), ChatRequest{
		Model: "gpt-4",
		Messages: []Message{{Role: "user", Content: "hi"}},
		Tools:    []ToolDef{{Name: "search"}},
	})
	if err != nil {
		t.Fatalf("Chat with tools: %v", err)
	}
	if result == nil {
		t.Fatal("should have response")
	}
}

func TestOpenAIProvider_DoRequest_StreamMode(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")
	respBody := io.NopCloser(bytes.NewReader([]byte(`{"choices": []}`)))
	resp := &http.Response{StatusCode: 200, Body: respBody}
	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
	defer patches.Reset()

	body, err := p.doRequest(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	}, true) // stream mode
	if err != nil {
		t.Fatalf("doRequest stream: %v", err)
	}
	if len(body) == 0 {
		t.Fatal("body should not be empty")
	}
}

func TestOpenAIProvider_DoRequest_WithTools(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")
	respBody := io.NopCloser(bytes.NewReader([]byte(`{}`)))
	resp := &http.Response{StatusCode: 200, Body: respBody}
	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
	defer patches.Reset()

	body, err := p.doRequest(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
		Tools: []ToolDef{{Name: "search"}},
	}, false)
	if err != nil {
		t.Fatalf("doRequest: %v", err)
	}
	if body == nil {
		t.Fatal("should have body")
	}
}

func TestOpenAIProvider_DoRequest_ReadError(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")
	errReader := &errorReader{err: io.ErrUnexpectedEOF}
	resp := &http.Response{StatusCode: 200, Body: io.NopCloser(errReader)}
	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
	defer patches.Reset()

	_, err := p.doRequest(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	}, false)
	if err == nil {
		t.Fatal("should error on read failure")
	}
}

func TestParseSSEStream_SkipNonDelta(t *testing.T) {
	// Data that isn't valid JSON or has empty delta content
	var called bool
	input := "data: {\"choices\":[{\"delta\":{\"content\":\"\"}}]}\n\n"
	err := parseSSEStream(bytes.NewReader([]byte(input)), func(chunk string) error {
		called = true
		return nil
	})
	if err != nil {
		t.Fatalf("parseSSEStream: %v", err)
	}
	if called {
		t.Fatal("should skip empty content")
	}
}

func TestParseSSEStream_InvalidJSON(t *testing.T) {
	var called bool
	_ = parseSSEStream(bytes.NewReader([]byte("data: not-json\n\n")), func(chunk string) error {
		called = true
		return nil
	})
	if called {
		t.Fatal("should skip invalid JSON")
	}
}

func TestOpenAIProvider_ChatStream(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")

	sseData := sseChunk("chunk1") + sseChunk("chunk2") + "data: [DONE]\n\n"
	respBody := io.NopCloser(bytes.NewReader([]byte(sseData)))
	resp := &http.Response{StatusCode: 200, Body: respBody}
	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
	defer patches.Reset()

	var chunks []string
	err := p.ChatStream(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("ChatStream: %v", err)
	}
	if len(chunks) != 2 {
		t.Errorf("got %d chunks, want 2: %v", len(chunks), chunks)
	}
}

func TestOpenAIProvider_ChatStream_RequestError(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")

	patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", (*http.Response)(nil), context.DeadlineExceeded)
	defer patches.Reset()

	err := p.ChatStream(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk string) error { return nil })
	if err == nil {
		t.Fatal("should error on HTTP failure")
	}
}

type errorReader struct{ err error }

func (e *errorReader) Read(p []byte) (int, error) { return 0, e.err }
