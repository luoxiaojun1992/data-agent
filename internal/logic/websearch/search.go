// Package websearch provides multi-engine web search via Bing/Baidu APIs.
// Both engines are called concurrently; results are merged (2×TopN).
// API keys are injected from skill config; empty key or error → engine returns empty.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Config is the per-skill websearch configuration, read from skill config JSON.
type Config struct {
	BingAPIKey  string `json:"bing_api_key"`
	BaiduAPIKey string `json:"baidu_api_key"`
	TopN        int    `json:"top_n"` // per-engine result count, default 5
}

func (c Config) topN() int {
	if c.TopN <= 0 {
		return 5
	}
	if c.TopN > 20 {
		return 20
	}
	return c.TopN
}

// ResultItem is a single search result returned to the LLM.
type ResultItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
	Engine  string `json:"engine"` // "bing" or "baidu"
}

// SearchResult is the clean output returned to the LLM.
type SearchResult struct {
	Query   string       `json:"query"`
	Results []ResultItem `json:"results,omitempty"`
	Error   string       `json:"error,omitempty"`
}

var httpClient = &http.Client{Timeout: 12 * time.Second}

// Search calls Bing and Baidu concurrently, merges, and returns up to 2×TopN results.
func Search(ctx context.Context, query string, cfg Config) (*SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return &SearchResult{Error: "query is required"}, nil
	}

	if cfg.BingAPIKey == "" && cfg.BaiduAPIKey == "" {
		return &SearchResult{
			Query: query,
			Error: "no search engine configured; set bing_api_key or baidu_api_key in skill config",
		}, nil
	}

	n := cfg.topN()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var all []ResultItem

	// Run both engines concurrently
	for _, engine := range []struct {
		name string
		key  string
		fn   func(context.Context, string, string, int) ([]ResultItem, error)
	}{
		{"bing", cfg.BingAPIKey, searchBing},
		{"baidu", cfg.BaiduAPIKey, searchBaidu},
	} {
		if engine.key == "" {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			results, err := engine.fn(ctx, query, engine.key, n)
			if err != nil {
				log.Printf("[websearch] %s error: %v", engine.name, err)
				return
			}
			mu.Lock()
			all = append(all, results...)
			mu.Unlock()
		}()
	}
	wg.Wait()

	if len(all) == 0 {
		return &SearchResult{
			Query: query,
			Error: "all configured engines returned no results or failed",
		}, nil
	}

	return &SearchResult{Query: query, Results: all}, nil
}

// ---- Bing Web Search API (free tier: 1000 queries/month) ----

type bingResponse struct {
	WebPages struct {
		Value []struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"value"`
	} `json:"webPages"`
}

func searchBing(ctx context.Context, query, apiKey string, topN int) ([]ResultItem, error) {
	apiURL := "https://api.bing.microsoft.com/v7.0/search?" + url.Values{
		"q":     {query},
		"count": {fmt.Sprintf("%d", topN)},
		"mkt":   {"zh-CN"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	var br bingResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	var out []ResultItem
	for _, v := range br.WebPages.Value {
		out = append(out, ResultItem{
			Title:   v.Name,
			URL:     v.URL,
			Snippet: v.Snippet,
			Engine:  "bing",
		})
	}
	return out, nil
}

// ---- Baidu Search (requires API key from Baidu AI开放平台) ----

type baiduResponse struct {
	Items []struct {
		Title   string `json:"title"`
		URL     string `json:"url"`
		Summary string `json:"summary"`
	} `json:"items"`
}

func searchBaidu(ctx context.Context, query, apiKey string, topN int) ([]ResultItem, error) {
	apiURL := "https://qianfan.baidubce.com/v2/app/search?" + url.Values{
		"query": {query},
		"top_n": {fmt.Sprintf("%d", topN)},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(body))
	}

	var br baiduResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	var out []ResultItem
	for _, v := range br.Items {
		out = append(out, ResultItem{
			Title:   v.Title,
			URL:     v.URL,
			Snippet: v.Summary,
			Engine:  "baidu",
		})
	}
	return out, nil
}
