// Package websearch provides multi-engine web search via Bing/Baidu APIs.
// Both engines are called concurrently; results are merged (2×TopN).
// API keys are injected from skill config; empty key or error → engine returns empty.
package websearch

import (
	"bytes"
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
	"unicode/utf8"
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
		log.Printf("[websearch] no engine configured; returning empty results")
		return &SearchResult{Query: query}, nil
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

// ---- Baidu Search (requires API key from Baidu Qianfan 智能搜索) ----

func searchBaidu(ctx context.Context, query, apiKey string, topN int) ([]ResultItem, error) {
	// Baidu Qianfan 智能搜索生成 API (https://cloud.baidu.com/doc/qianfan/s/2mh4su4uy)
	apiURL := "https://qianfan.baidubce.com/v2/ai_search/web_search"

	body := map[string]interface{}{
		"messages": []map[string]string{
			{"role": "user", "content": query},
		},
		"search_source": "baidu_search_v2",
		"resource_type_filter": []map[string]interface{}{
			{"type": "web", "top_k": topN},
		},
	}
	bodyJSON, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal body: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, apiURL, bytes.NewReader(bodyJSON))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)
	req.Header.Set("Content-Type", "application/json")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("http: %w", err)
	}
	defer resp.Body.Close()

	respBody, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("read: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("http %d: %s", resp.StatusCode, string(respBody))
	}

	var br struct {
		References []struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
			Content string `json:"content"`
			Date    string `json:"date"`
		} `json:"references"`
	}
	if err := json.Unmarshal(respBody, &br); err != nil {
		return nil, fmt.Errorf("parse: %w", err)
	}

	var out []ResultItem
	for _, r := range br.References {
		snippet := r.Snippet
		if snippet == "" {
			snippet = r.Content
		}
		out = append(out, ResultItem{
			Title:   r.Title,
			URL:     r.URL,
			Snippet: truncate(snippet, 500),
			Engine:  "baidu",
		})
	}
	return out, nil
}

// truncate cuts a string to maxLen runes without breaking UTF-8.
func truncate(s string, maxLen int) string {
	if utf8.RuneCountInString(s) <= maxLen {
		return s
	}
	runes := []rune(s)
	return string(runes[:maxLen]) + "..."
}
