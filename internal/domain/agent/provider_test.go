package agent

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
)

func TestNewOpenAIProvider(t *testing.T) {
	p := NewOpenAIProvider("https://api.openai.com", "sk-test")
	if p == nil {
		t.Error("should not return nil")
	}
	if p.baseURL != "https://api.openai.com" {
		t.Errorf("baseURL: got %s", p.baseURL)
	}
	if p.httpClient == nil {
		t.Error("httpClient should be initialized")
	}
}

func TestParseSSEStream(t *testing.T) {
	t.Run("multiple chunks", func(t *testing.T) {
		input := "data: hello\n\ndata: world\n\ndata: [DONE]\n\n"
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
		err := parseSSEStream(bytes.NewReader([]byte("data: test\n\n")), func(chunk string) error {
			return fmt.Errorf("callback failed")
		})
		if err == nil {
			t.Error("should propagate callback error")
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
			t.Error("should not call callback for non-data lines")
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
			t.Error("body should not be empty")
		}
	})

	t.Run("http error", func(t *testing.T) {
		patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", (*http.Response)(nil), context.DeadlineExceeded)
		defer patches.Reset()

		_, err := p.doRequest(context.Background(), ChatRequest{Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}}}, false)
		if err == nil {
			t.Error("should error on HTTP failure")
		}
	})

	t.Run("non-200 status", func(t *testing.T) {
		respBody := io.NopCloser(bytes.NewReader([]byte(`{"error": "bad"}`)))
		resp := &http.Response{StatusCode: 400, Body: respBody}
		patches := gomonkey.ApplyMethodReturn(p.httpClient, "Do", resp, nil)
		defer patches.Reset()

		_, err := p.doRequest(context.Background(), ChatRequest{Model: "gpt-4", Messages: []Message{}}, false)
		if err == nil {
			t.Error("should error on non-200")
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

func TestOpenAIProvider_ChatStream(t *testing.T) {
	p := NewOpenAIProvider("https://api.example.com", "sk-test")

	sseData := "data: chunk1\n\ndata: chunk2\n\ndata: [DONE]\n\n"
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
		t.Error("should error on HTTP failure")
	}
}
