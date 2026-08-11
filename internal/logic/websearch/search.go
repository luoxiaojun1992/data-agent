// Package websearch provides multi-engine web search via Bing/Baidu APIs.
// API keys are injected from skill config; if empty, the engine is skipped.
// Errors are logged and returned as an Error field in the result — the LLM
// sees a graceful degradation rather than a tool-call exception.
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
	"time"
)

// Config is the per-skill websearch configuration, read from skill config JSON.
type Config struct {
	BingAPIKey  string `json:"bing_api_key"`
	BaiduAPIKey string `json:"baidu_api_key"`
}

// ResultItem is a single search result returned to the LLM.
type ResultItem struct {
	Title   string `json:"title"`
	URL     string `json:"url"`
	Snippet string `json:"snippet"`
}

// SearchResult is the clean output returned to the LLM.
type SearchResult struct {
	Query   string       `json:"query"`
	Total   int          `json:"total,omitempty"`
	Results []ResultItem `json:"results,omitempty"`
	Error   string       `json:"error,omitempty"`
}

var httpClient = &http.Client{Timeout: 12 * time.Second}

// Search performs a web search using configured engine APIs.
func Search(ctx context.Context, query string, cfg Config) (*SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return &SearchResult{Error: "query is required"}, nil
	}

	// Try Bing first
	if cfg.BingAPIKey != "" {
		result, err := searchBing(ctx, query, cfg.BingAPIKey)
		if err == nil {
			return result, nil
		}
		log.Printf("[websearch] bing error: %v", err)
	}

	// Fall back to Baidu
	if cfg.BaiduAPIKey != "" {
		result, err := searchBaidu(ctx, query, cfg.BaiduAPIKey)
		if err == nil {
			return result, nil
		}
		log.Printf("[websearch] baidu error: %v", err)
	}

	return &SearchResult{
		Query: query,
		Error: "no search engine configured or all engines failed; configure bing_api_key or baidu_api_key in skill config",
	}, nil
}

// ---- Bing Web Search API (free tier: 1000 queries/month) ----

type bingResponse struct {
	WebPages struct {
		TotalEstimatedMatches int `json:"totalEstimatedMatches"`
		Value                 []struct {
			Name    string `json:"name"`
			URL     string `json:"url"`
			Snippet string `json:"snippet"`
		} `json:"value"`
	} `json:"webPages"`
}

func searchBing(ctx context.Context, query, apiKey string) (*SearchResult, error) {
	apiURL := "https://api.bing.microsoft.com/v7.0/search?" + url.Values{
		"q":     {query},
		"count": {"10"},
		"mkt":   {"zh-CN"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("bing: create request: %w", err)
	}
	req.Header.Set("Ocp-Apim-Subscription-Key", apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("bing: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("bing: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("bing: http %d: %s", resp.StatusCode, string(body))
	}

	var br bingResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf("bing: parse response: %w", err)
	}

	out := &SearchResult{Query: query, Total: br.WebPages.TotalEstimatedMatches}
	for _, v := range br.WebPages.Value {
		out.Results = append(out.Results, ResultItem{
			Title:   v.Name,
			URL:     v.URL,
			Snippet: v.Snippet,
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
	Total int `json:"total"`
}

func searchBaidu(ctx context.Context, query, apiKey string) (*SearchResult, error) {
	// Baidu AI开放平台 搜索接口 (需要申请)
	apiURL := "https://qianfan.baidubce.com/v2/app/search?" + url.Values{
		"query": {query},
		"top_n": {"10"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("baidu: create request: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+apiKey)

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("baidu: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("baidu: read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("baidu: http %d: %s", resp.StatusCode, string(body))
	}

	var br baiduResponse
	if err := json.Unmarshal(body, &br); err != nil {
		return nil, fmt.Errorf("baidu: parse response: %w", err)
	}

	out := &SearchResult{Query: query, Total: br.Total}
	for _, v := range br.Items {
		out.Results = append(out.Results, ResultItem{
			Title:   v.Title,
			URL:     v.URL,
			Snippet: v.Summary,
		})
	}
	return out, nil
}
