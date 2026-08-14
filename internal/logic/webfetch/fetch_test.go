package webfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestFetch_ExtractsTitleAndText(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			t.Errorf("expected GET, got %s", r.Method)
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<html><head><title>Hello Page</title></head><body><h1>Heading</h1><p>First paragraph.</p><p>Second paragraph.</p><script>var x = 1;</script></body></html>`))
	}))
	defer srv.Close()

	result, err := Fetch(context.Background(), srv.URL, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "" {
		t.Fatalf("unexpected result error: %s", result.Error)
	}
	if result.Title != "Hello Page" {
		t.Errorf("title = %q, want %q", result.Title, "Hello Page")
	}
	if !strings.Contains(result.Text, "Heading") {
		t.Errorf("text missing Heading: %q", result.Text)
	}
	if !strings.Contains(result.Text, "First paragraph") {
		t.Errorf("text missing First paragraph: %q", result.Text)
	}
	if strings.Contains(result.Text, "var x = 1") {
		t.Errorf("script content leaked into text: %q", result.Text)
	}
	if result.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", result.StatusCode)
	}
}

func TestFetch_RejectsNonHTTPURL(t *testing.T) {
	result, err := Fetch(context.Background(), "ftp://example.com", Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Error, "http") {
		t.Errorf("expected scheme error, got %q", result.Error)
	}
}

func TestFetch_RejectsEmptyURL(t *testing.T) {
	result, err := Fetch(context.Background(), "  ", Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error != "url is required" {
		t.Errorf("expected 'url is required', got %q", result.Error)
	}
}

func TestFetch_Non200(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	result, err := Fetch(context.Background(), srv.URL, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404", result.StatusCode)
	}
	if !strings.Contains(result.Error, "404") {
		t.Errorf("expected 404 error, got %q", result.Error)
	}
}

func TestConfig_Defaults(t *testing.T) {
	cfg := Config{}
	if cfg.maxChars() != 8000 {
		t.Errorf("maxChars default = %d, want 8000", cfg.maxChars())
	}
	if cfg.maxBodySize() != 512*1024 {
		t.Errorf("maxBodySize default = %d, want %d", cfg.maxBodySize(), 512*1024)
	}
	if cfg.timeout() == 0 {
		t.Error("timeout default should be non-zero")
	}
}
