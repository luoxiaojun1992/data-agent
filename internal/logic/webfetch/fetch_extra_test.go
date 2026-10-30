package webfetch

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestConfig_ExplicitValues(t *testing.T) {
	cfg := Config{MaxChars: 100, MaxBodySize: 2048, TimeoutSec: 5}
	if cfg.maxChars() != 100 {
		t.Errorf("maxChars = %d, want 100", cfg.maxChars())
	}
	if cfg.maxBodySize() != 2048 {
		t.Errorf("maxBodySize = %d, want 2048", cfg.maxBodySize())
	}
	if cfg.timeout() != 5*time.Second {
		t.Errorf("timeout = %v, want 5s", cfg.timeout())
	}
}

func TestFetch_RequestError(t *testing.T) {
	// a space in the host makes http.NewRequest fail, even though the scheme
	// check passes.
	result, err := Fetch(context.Background(), "http://exa mple.com", Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected a request-build error")
	}
}

func TestFetch_DoError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	url := srv.URL
	srv.Close() // connection refused on next request

	result, err := Fetch(context.Background(), url, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Fatal("expected a connection error")
	}
}

func TestFetch_RedirectLimit(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/", http.StatusFound)
	}))
	defer srv.Close()

	result, err := Fetch(context.Background(), srv.URL, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Error, "redirect") {
		t.Errorf("expected redirect-limit error, got %q", result.Error)
	}
}

func TestFetch_ContentTypeHeader(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer srv.Close()

	result, err := Fetch(context.Background(), srv.URL, Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.ContentType != "text/html; charset=utf-8" {
		t.Errorf("ContentType = %q", result.ContentType)
	}
}

func TestExtractTitle_NoMatch(t *testing.T) {
	if got := extractTitle("<html><body>no title here</body></html>"); got != "" {
		t.Errorf("extractTitle = %q, want empty", got)
	}
}

func TestExtractText_Truncates(t *testing.T) {
	long := "<p>" + strings.Repeat("a", 100) + "</p>"
	got := extractText(long, 10)
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("expected truncation marker, got %q", got)
	}
	if len(got) > 10+len("\n...[truncated]") {
		t.Errorf("truncated text too long: %q", got)
	}
}

func TestExtractText_EntityDecoding(t *testing.T) {
	got := extractText("&amp; &lt; &gt; &quot; &#39; &nbsp;", 1000)
	for _, want := range []string{"&", "<", ">", `"`, "'", " "} {
		if !strings.Contains(got, want) {
			t.Errorf("entity not decoded to %q: %q", want, got)
		}
	}
}
