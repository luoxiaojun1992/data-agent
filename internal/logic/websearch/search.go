// Package websearch provides free web search via DuckDuckGo Instant Answer API.
// No API key required. Returns structured results: abstract, heading, answer, related topics.
package websearch

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Result holds a single search result from DuckDuckGo.
type Result struct {
	Abstract       string `json:"Abstract"`       // short summary
	AbstractText   string `json:"AbstractText"`   // plain-text abstract
	AbstractSource string `json:"AbstractSource"` // source name (e.g. "Wikipedia")
	AbstractURL    string `json:"AbstractURL"`    // source URL
	Answer         string `json:"Answer"`         // instant answer (e.g. "42")
	AnswerType     string `json:"AnswerType"`     // type of instant answer
	Definition     string `json:"Definition"`     // dictionary definition
	DefinitionSource string `json:"DefinitionSource"`
	DefinitionURL  string `json:"DefinitionURL"`
	Entity         string `json:"Entity"`         // entity name (e.g. "OpenAI")
	Heading        string `json:"Heading"`        // main heading
	Image          string `json:"Image"`          // related image URL
	Redirect       string `json:"Redirect"`       // official site redirect
	Type           string `json:"Type"`           // "A" (article) or "D" (disambiguation)
	Infobox        json.RawMessage `json:"Infobox,omitempty"`
	RelatedTopics  []RelatedTopic  `json:"RelatedTopics"`
	Results        []RelatedTopic  `json:"Results"`
}

// RelatedTopic represents a related topic or search result link.
type RelatedTopic struct {
	Result   string   `json:"Result"`   // HTML snippet with link
	Text     string   `json:"Text"`     // plain text
	FirstURL string   `json:"FirstURL"` // URL
	Icon     IconInfo `json:"Icon"`
	Name     string   `json:"Name"`
	Topics   []RelatedTopic `json:"Topics,omitempty"` // nested topics
}

// IconInfo holds icon metadata.
type IconInfo struct {
	URL    string `json:"URL"`
	Width  string `json:"Width,omitempty"`
	Height string `json:"Height,omitempty"`
}

// SearchResult is the clean output returned to the LLM.
type SearchResult struct {
	Query    string `json:"query"`
	Abstract string `json:"abstract,omitempty"`
	Answer   string `json:"answer,omitempty"`
	Heading  string `json:"heading,omitempty"`
	Redirect string `json:"redirect,omitempty"`
	Definition string `json:"definition,omitempty"`
	Topics   []TopicItem `json:"topics,omitempty"`
	Error    string `json:"error,omitempty"`
}

// TopicItem is a flattened related topic.
type TopicItem struct {
	Text string `json:"text"`
	URL  string `json:"url,omitempty"`
}

var httpClient = &http.Client{Timeout: 10 * time.Second}

// Search performs a DuckDuckGo search for the given query.
// Returns structured results suitable for LLM consumption.
func Search(ctx context.Context, query string) (*SearchResult, error) {
	query = strings.TrimSpace(query)
	if query == "" {
		return nil, fmt.Errorf("web_search: query is empty")
	}

	// Build DuckDuckGo Instant Answer API URL
	apiURL := "https://api.duckduckgo.com/?" + url.Values{
		"q":     {query},
		"format": {"json"},
		"no_html": {"1"},
		"skip_disambig": {"1"},
	}.Encode()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("web_search: create request: %w", err)
	}
	req.Header.Set("User-Agent", "DataAgent/1.0 (web-search)")

	resp, err := httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("web_search: http request: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20)) // 1MB limit
	if err != nil {
		return nil, fmt.Errorf("web_search: read response: %w", err)
	}

	var ddg Result
	if err := json.Unmarshal(body, &ddg); err != nil {
		return nil, fmt.Errorf("web_search: parse response: %w", err)
	}

	out := &SearchResult{Query: query}

	// Extract the most useful fields
	if ddg.AbstractText != "" {
		out.Abstract = fmt.Sprintf("%s (source: %s)", ddg.AbstractText, ddg.AbstractURL)
	}
	if ddg.Answer != "" {
		out.Answer = ddg.Answer
	}
	if ddg.Heading != "" {
		out.Heading = ddg.Heading
	}
	if ddg.Redirect != "" {
		out.Redirect = ddg.Redirect
	}
	if ddg.Definition != "" {
		out.Definition = fmt.Sprintf("%s (source: %s)", ddg.Definition, ddg.DefinitionURL)
	}

	// Flatten related topics
	var topics []TopicItem
	for _, t := range ddg.RelatedTopics {
		flattenTopic(&topics, t)
	}
	for _, r := range ddg.Results {
		flattenTopic(&topics, r)
	}

	// Deduplicate and limit
	seen := make(map[string]bool)
	var filtered []TopicItem
	for _, t := range topics {
		key := t.Text
		if key == "" {
			key = t.URL
		}
		if key == "" || seen[key] {
			continue
		}
		seen[key] = true
		filtered = append(filtered, t)
		if len(filtered) >= 10 {
			break
		}
	}
	out.Topics = filtered

	// If nothing was found, return a useful message
	if out.Abstract == "" && out.Answer == "" && len(out.Topics) == 0 {
		out.Error = fmt.Sprintf("no results found for %q", query)
	}

	return out, nil
}

// flattenTopic recursively extracts RelatedTopic entries into a flat list.
func flattenTopic(out *[]TopicItem, t RelatedTopic) {
	if len(t.Topics) > 0 {
		for _, nested := range t.Topics {
			flattenTopic(out, nested)
		}
		return
	}
	if t.Text != "" || t.FirstURL != "" {
		// Strip HTML from Result field if present
		text := t.Text
		if text == "" && t.Result != "" {
			text = stripHTML(t.Result)
		}
		if text != "" {
			*out = append(*out, TopicItem{Text: text, URL: t.FirstURL})
		}
	}
}

// stripHTML removes basic HTML tags from a string.
func stripHTML(s string) string {
	var result strings.Builder
	inTag := false
	for _, c := range s {
		switch c {
		case '<':
			inTag = true
		case '>':
			inTag = false
		default:
			if !inTag {
				result.WriteRune(c)
			}
		}
	}
	return strings.TrimSpace(result.String())
}
