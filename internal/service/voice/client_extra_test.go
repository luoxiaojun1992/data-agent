package voice

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_TranscribeNilClient(t *testing.T) {
	var c *Client
	if _, err := c.Transcribe(context.Background(), []byte("x"), "zh"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable for nil client, got %v", err)
	}
}

func TestClient_TranscribeHTTPError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // connection refused afterwards

	c := NewClient(ClientConfig{Endpoint: url})
	if _, err := c.Transcribe(context.Background(), []byte("x"), "zh"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable wrapping http error, got %v", err)
	}
}

func TestClient_TranscribeInvalidJSON(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte("not-json"))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	_, err := c.Transcribe(context.Background(), []byte("x"), "zh")
	if err == nil {
		t.Fatal("expected JSON unmarshal error")
	}
	if errors.Is(err, ErrUnavailable) {
		t.Fatalf("invalid JSON should be a parse error, not ErrUnavailable: %v", err)
	}
}
