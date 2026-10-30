package websearch

import (
	"context"
	"errors"
	"io"
	"net/http"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
)

// ── truncate ─────────────────────────────────────────────────────────

func TestTruncate_Short(t *testing.T) {
	if got := truncate("hello", 500); got != "hello" {
		t.Errorf("truncate(short) = %q, want hello", got)
	}
}

func TestTruncate_Long(t *testing.T) {
	s := strings.Repeat("a", 600)
	got := truncate(s, 500)
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncate(long) should end with '...', got suffix %q", got[len(got)-3:])
	}
	if len([]rune(got)) != 503 { // 500 runes + "..."
		t.Errorf("truncate(long) rune count = %d, want 503", len([]rune(got)))
	}
}

func TestTruncate_ChineseUTF8(t *testing.T) {
	// 10 Chinese chars = 30 bytes; truncate to 5 runes must not break UTF-8.
	s := "你好世界你好世界你好世界"
	got := truncate(s, 5)
	if strings.Count(got, "�") > 0 {
		t.Errorf("truncate(chinese) broke UTF-8: %q", got)
	}
	if !strings.HasSuffix(got, "...") {
		t.Errorf("truncate(chinese) should end with '...': %q", got)
	}
}

// ── Search merge / error logic ───────────────────────────────────────

func TestSearch_MergesBothEngines(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(searchBing, func(_ context.Context, q, key string, n int) ([]ResultItem, error) {
		return []ResultItem{{Title: "b", URL: "u1", Engine: "bing"}}, nil
	})
	patches.ApplyFunc(searchBaidu, func(_ context.Context, q, key string, n int) ([]ResultItem, error) {
		return []ResultItem{{Title: "d", URL: "u2", Engine: "baidu"}}, nil
	})

	res, err := Search(context.Background(), "q", Config{BingAPIKey: "b", BaiduAPIKey: "d"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Results) != 2 {
		t.Errorf("Results = %d, want 2", len(res.Results))
	}
	if res.Error != "" {
		t.Errorf("Error = %q, want empty", res.Error)
	}
}

func TestSearch_AllEnginesFail(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(searchBing, func(_ context.Context, q, key string, n int) ([]ResultItem, error) {
		return nil, errors.New("bing down")
	})
	patches.ApplyFunc(searchBaidu, func(_ context.Context, q, key string, n int) ([]ResultItem, error) {
		return nil, errors.New("baidu down")
	})

	res, err := Search(context.Background(), "q", Config{BingAPIKey: "b", BaiduAPIKey: "d"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Results) != 0 {
		t.Errorf("Results = %d, want 0", len(res.Results))
	}
	if res.Error == "" {
		t.Error("expected aggregated error when all engines fail")
	}
}

func TestSearch_PartialFail(t *testing.T) {
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyFunc(searchBing, func(_ context.Context, q, key string, n int) ([]ResultItem, error) {
		return []ResultItem{{Title: "b", Engine: "bing"}}, nil
	})
	patches.ApplyFunc(searchBaidu, func(_ context.Context, q, key string, n int) ([]ResultItem, error) {
		return nil, errors.New("baidu down")
	})

	res, err := Search(context.Background(), "q", Config{BingAPIKey: "b", BaiduAPIKey: "d"})
	if err != nil {
		t.Fatalf("Search() error = %v", err)
	}
	if len(res.Results) != 1 {
		t.Errorf("Results = %d, want 1 (only bing)", len(res.Results))
	}
	// Partial success → no error surfaced.
	if res.Error != "" {
		t.Errorf("Error = %q, want empty on partial success", res.Error)
	}
}

// ── searchBing / searchBaidu HTTP branches ───────────────────────────

func TestSearchBing_Success(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(httpClient, "Do",
		func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"webPages":{"value":[{"name":"t","url":"u","snippet":"s"}]}}`)),
				Header:     http.Header{},
			}, nil
		})
	t.Cleanup(patches.Reset)

	results, err := searchBing(context.Background(), "q", "key", 5)
	if err != nil {
		t.Fatalf("searchBing() error = %v", err)
	}
	if len(results) != 1 || results[0].Engine != "bing" || results[0].Title != "t" {
		t.Errorf("results = %+v", results)
	}
}

func TestSearchBing_Non200(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(httpClient, "Do",
		func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusUnauthorized,
				Body:       io.NopCloser(strings.NewReader("unauthorized")),
				Header:     http.Header{},
			}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := searchBing(context.Background(), "q", "key", 5); err == nil {
		t.Fatal("expected non-200 error")
	}
}

func TestSearchBing_HTTPError(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(httpClient, "Do",
		func(_ *http.Request) (*http.Response, error) {
			return nil, errors.New("connection refused")
		})
	t.Cleanup(patches.Reset)

	if _, err := searchBing(context.Background(), "q", "key", 5); err == nil {
		t.Fatal("expected http error")
	}
}

func TestSearchBaidu_Success(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(httpClient, "Do",
		func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"references":[{"title":"t","url":"u","snippet":"s","content":"c","date":"d"}]}`)),
				Header:     http.Header{},
			}, nil
		})
	t.Cleanup(patches.Reset)

	results, err := searchBaidu(context.Background(), "q", "key", 5)
	if err != nil {
		t.Fatalf("searchBaidu() error = %v", err)
	}
	if len(results) != 1 || results[0].Engine != "baidu" || results[0].Title != "t" {
		t.Errorf("results = %+v", results)
	}
}

func TestSearchBaidu_ContentFallback(t *testing.T) {
	// When snippet is empty, content is used as snippet.
	patches := gomonkey.ApplyMethodFunc(httpClient, "Do",
		func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Body:       io.NopCloser(strings.NewReader(`{"references":[{"title":"t","url":"u","snippet":"","content":"fallback-content"}]}`)),
				Header:     http.Header{},
			}, nil
		})
	t.Cleanup(patches.Reset)

	results, err := searchBaidu(context.Background(), "q", "key", 5)
	if err != nil {
		t.Fatalf("searchBaidu() error = %v", err)
	}
	if len(results) != 1 || results[0].Snippet != "fallback-content" {
		t.Errorf("Snippet = %q, want fallback-content", results[0].Snippet)
	}
}

func TestSearchBaidu_Non200(t *testing.T) {
	patches := gomonkey.ApplyMethodFunc(httpClient, "Do",
		func(_ *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusInternalServerError,
				Body:       io.NopCloser(strings.NewReader("err")),
				Header:     http.Header{},
			}, nil
		})
	t.Cleanup(patches.Reset)

	if _, err := searchBaidu(context.Background(), "q", "key", 5); err == nil {
		t.Fatal("expected non-200 error")
	}
}
