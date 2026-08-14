// Package webfetch fetches a single web page via HTTP GET and extracts
// readable text for the LLM. Only GET requests are supported (hard-coded);
// there is no way to inject other HTTP methods, headers, or a body.
package webfetch

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Config is the per-skill webfetch configuration, read from skill config JSON.
type Config struct {
	MaxChars    int `json:"max_chars"`    // max extracted text length, default 8000
	MaxBodySize int `json:"max_body_size"` // max raw HTML bytes to read, default 512*1024
	TimeoutSec  int `json:"timeout_sec"`  // HTTP timeout seconds, default 10
}

func (c Config) maxChars() int {
	if c.MaxChars <= 0 {
		return 8000
	}
	return c.MaxChars
}

func (c Config) maxBodySize() int64 {
	if c.MaxBodySize <= 0 {
		return 512 * 1024
	}
	return int64(c.MaxBodySize)
}

func (c Config) timeout() time.Duration {
	if c.TimeoutSec <= 0 {
		return 10 * time.Second
	}
	return time.Duration(c.TimeoutSec) * time.Second
}

// Result is the extracted page content returned to the LLM.
type Result struct {
	URL         string `json:"url"`
	StatusCode  int    `json:"status_code"`
	Title       string `json:"title"`
	ContentType string `json:"content_type"`
	Text        string `json:"text"` // extracted plain text (truncated)
	Error       string `json:"error,omitempty"`
}

// Fetch retrieves the given URL using HTTP GET only and extracts plain text.
// It never sends anything other than a GET request.
func Fetch(ctx context.Context, rawURL string, cfg Config) (*Result, error) {
	rawURL = strings.TrimSpace(rawURL)
	if rawURL == "" {
		return &Result{Error: "url is required"}, nil
	}
	if !strings.HasPrefix(rawURL, "http://") && !strings.HasPrefix(rawURL, "https://") {
		return &Result{Error: "url must start with http:// or https://"}, nil
	}

	client := &http.Client{
		Timeout: cfg.timeout(),
		// Never follow redirects to keep the request a pure GET on the given URL.
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 3 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return &Result{Error: err.Error()}, nil
	}
	// Browsers send a UA; some sites reject default Go client UA.
	req.Header.Set("User-Agent", "DataAgentBot/1.0 (+https://example.com/bot)")
	req.Header.Set("Accept", "text/html,application/xhtml+xml,text/plain;q=0.9,*/*;q=0.8")

	resp, err := client.Do(req)
	if err != nil {
		return &Result{URL: rawURL, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, cfg.maxBodySize())
	body, err := io.ReadAll(limited)
	if err != nil {
		return &Result{URL: rawURL, StatusCode: resp.StatusCode, Error: err.Error()}, nil
	}

	result := &Result{
		URL:         rawURL,
		StatusCode:  resp.StatusCode,
		ContentType: resp.Header.Get("Content-Type"),
	}
	if resp.StatusCode != http.StatusOK {
		result.Error = fmt.Sprintf("HTTP %d", resp.StatusCode)
		return result, nil
	}

	html := string(body)
	result.Title = extractTitle(html)
	result.Text = extractText(html, cfg.maxChars())
	return result, nil
}

var titleRe = regexp.MustCompile(`(?is)<title[^>]*>(.*?)</title>`)
var scriptRe = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
var styleRe = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
var noscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
var tagRe = regexp.MustCompile(`(?s)<[^>]*>`)

func extractTitle(html string) string {
	m := titleRe.FindStringSubmatch(html)
	if len(m) > 1 {
		return strings.TrimSpace(m[1])
	}
	return ""
}

// extractText strips scripts/styles/tags and collapses whitespace into a
// compact, readable plain-text body.
func extractText(html string, maxChars int) string {
	// Remove scripts/styles first.
	html = scriptRe.ReplaceAllString(html, " ")
	html = styleRe.ReplaceAllString(html, " ")
	html = noscriptRe.ReplaceAllString(html, " ")
	// Convert block-level tags and <br> to newlines for readability.
	html = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr|section|article|br)>`).ReplaceAllString(html, "\n")
	html = regexp.MustCompile(`(?i)<br\s*/?>`).ReplaceAllString(html, "\n")
	// Strip remaining tags.
	text := tagRe.ReplaceAllString(html, " ")
	// Decode a few common entities.
	text = strings.ReplaceAll(text, "&amp;", "&")
	text = strings.ReplaceAll(text, "&lt;", "<")
	text = strings.ReplaceAll(text, "&gt;", ">")
	text = strings.ReplaceAll(text, "&quot;", `"`)
	text = strings.ReplaceAll(text, "&#39;", "'")
	text = strings.ReplaceAll(text, "&nbsp;", " ")
	// Collapse whitespace.
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)

	if len(text) > maxChars {
		text = strings.TrimSpace(text[:maxChars]) + "\n...[truncated]"
	}
	return text
}
