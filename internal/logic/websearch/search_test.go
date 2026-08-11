package websearch

import (
	"context"
	"os"
	"strings"
	"testing"
	"time"
)

func TestSearch_EmptyQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	_, err := Search(ctx, "")
	if err == nil || !strings.Contains(err.Error(), "empty") {
		t.Errorf("expected empty query error, got %v", err)
	}
}

func TestSearch_ValidQuery(t *testing.T) {
	// DuckDuckGo requires network access; skip in CI or when proxy unavailable.
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	if os.Getenv("CI") != "" {
		t.Skip("skipping: DuckDuckGo may be unreachable in CI")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := Search(ctx, "Go programming language")
	if err != nil {
		// In China, DuckDuckGo may be unreachable without proxy — not a code bug.
		if strings.Contains(err.Error(), "timeout") || strings.Contains(err.Error(), "no such host") {
			t.Skipf("network unavailable (expected without proxy): %v", err)
		}
		t.Fatalf("Search failed: %v", err)
	}
	if result.Query != "Go programming language" {
		t.Errorf("expected query 'Go programming language', got %q", result.Query)
	}
	// DuckDuckGo should return at least an abstract or heading for this query
	if result.Abstract == "" && result.Heading == "" && len(result.Topics) == 0 && result.Error == "" {
		t.Error("expected at least abstract, heading, topics, or error")
	}
	t.Logf("Abstract: %s", result.Abstract)
	t.Logf("Heading: %s", result.Heading)
	t.Logf("Topics: %d", len(result.Topics))
}

func TestStripHTML(t *testing.T) {
	tests := []struct {
		input    string
		expected string
	}{
		{`<a href="https://example.com">Example</a>`, "Example"},
		{"plain text", "plain text"},
		{"<b>bold</b> and <i>italic</i>", "bold and italic"},
	}
	for _, tt := range tests {
		got := stripHTML(tt.input)
		if got != tt.expected {
			t.Errorf("stripHTML(%q) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
