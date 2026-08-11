package websearch

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestSearch_EmptyQuery(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := Search(ctx, "", Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error for empty query")
	}
}

func TestSearch_NoEngineConfigured(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	result, err := Search(ctx, "hello world", Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(result.Error, "no search engine configured") {
		t.Errorf("expected 'no search engine configured', got %q", result.Error)
	}
}

func TestSearch_BingBadKey(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	result, err := Search(ctx, "hello", Config{BingAPIKey: "invalid-key"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// Should return error message (graceful degradation) not panic
	if result.Error == "" && len(result.Results) == 0 {
		t.Error("expected either results or error message")
	}
	t.Logf("result: error=%q results=%d", result.Error, len(result.Results))
}
