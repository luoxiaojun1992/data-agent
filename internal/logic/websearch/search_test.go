package websearch

import (
	"context"
	"testing"
	"time"
)

func TestSearch_EmptyQuery(t *testing.T) {
	result, err := Search(context.Background(), "", Config{BingAPIKey: "x"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" {
		t.Error("expected error for empty query")
	}
}

func TestSearch_NoEngineConfigured(t *testing.T) {
	result, err := Search(context.Background(), "hello", Config{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Results) != 0 {
		t.Errorf("expected empty results, got %d", len(result.Results))
	}
}

func TestSearch_BothEnginesBadKeys(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping integration test in short mode")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	result, err := Search(ctx, "hello world", Config{
		BingAPIKey:  "invalid",
		BaiduAPIKey: "invalid",
		TopN:        3,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Error == "" && len(result.Results) == 0 {
		t.Error("expected either results or error message")
	}
	t.Logf("error=%q results=%d", result.Error, len(result.Results))
}

func TestConfig_TopNDefault(t *testing.T) {
	c := Config{}
	if c.topN() != 5 {
		t.Error("expected default topN=5")
	}
	c = Config{TopN: 0}
	if c.topN() != 5 {
		t.Error("expected topN=5 for 0")
	}
	c = Config{TopN: 10}
	if c.topN() != 10 {
		t.Error("expected topN=10")
	}
	c = Config{TopN: 100}
	if c.topN() != 20 {
		t.Error("expected topN capped at 20")
	}
}
