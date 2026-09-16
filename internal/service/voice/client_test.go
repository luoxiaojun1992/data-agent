package voice

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClient_TranscribeSuccess(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Internal-Token") != "tok" {
			t.Errorf("expected internal token header, got %q", r.Header.Get("X-Internal-Token"))
		}
		var req struct {
			Audio      string `json:"audio"`
			Lang       string `json:"lang"`
			SampleRate int    `json:"sample_rate"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Errorf("decode request: %v", err)
		}
		raw, _ := base64.StdEncoding.DecodeString(req.Audio)
		if string(raw) != "pcm" {
			t.Errorf("expected audio pcm, got %q", raw)
		}
		if req.Lang != "zh" || req.SampleRate != 16000 {
			t.Errorf("unexpected req %+v", req)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"text":"你好"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL + "/transcribe", Token: "tok"})
	text, err := c.Transcribe(context.Background(), []byte("pcm"), "zh")
	if err != nil {
		t.Fatalf("transcribe: %v", err)
	}
	if text != "你好" {
		t.Errorf("expected 你好, got %q", text)
	}
}

func TestClient_TranscribeUnconfigured(t *testing.T) {
	c := NewClient(ClientConfig{})
	if _, err := c.Transcribe(context.Background(), []byte("x"), "zh"); err != ErrUnavailable {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestClient_TranscribeNonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	if _, err := c.Transcribe(context.Background(), []byte("x"), "zh"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("expected ErrUnavailable, got %v", err)
	}
}

func TestClient_TranscribeErrorBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"error":"decode failed"}`))
	}))
	defer srv.Close()

	c := NewClient(ClientConfig{Endpoint: srv.URL})
	if _, err := c.Transcribe(context.Background(), []byte("x"), "zh"); err == nil || err.Error() != "decode failed" {
		t.Fatalf("expected decode failed, got %v", err)
	}
}
