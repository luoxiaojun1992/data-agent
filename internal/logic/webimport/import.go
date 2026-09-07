package webimport

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
)

// Import timeouts (SPEC-081 §4.3 — the single source of truth for URL import).
const (
	// ImportURLRenderTimeout is the end-to-end budget for rendering the target
	// page (headless load → final DOM), NOT a per-sub-request timeout.
	ImportURLRenderTimeout = 30 * time.Second
	// ImportURLTotalTimeout is the end-to-end budget for the whole import
	// (render + text extract + image download + doc creation).
	ImportURLTotalTimeout = 120 * time.Second
	// ImportURLImageDownloadTimeout is the whole-download budget for a single
	// image (from request start to the full image bytes).
	ImportURLImageDownloadTimeout = 10 * time.Second
	// maxHTMLBytes caps the rendered HTML read from the renderer to prevent
	// an unbounded allocation (SPEC-081 §2.3 "超大文件会打爆内存").
	maxHTMLBytes = 50 * 1024 * 1024 // 50 MB
)

// Image is a downloaded page image ready for GridFS upload.
type Image struct {
	Data     []byte
	MimeType string
}

// Result is the extracted page content for KB doc creation.
type Result struct {
	Text          string  // rendered page text (already ≤ MaxKBTextBytes)
	Images        []Image // downloaded images (≤ MaxKBImageCount, each ≤ MaxKBImageBytes)
	SkippedImages int     // images skipped (oversized / over-count / SSRF-blocked / failed)
}

// Importer fetches and extracts a web page for URL import (SPEC-081).
type Importer interface {
	Import(ctx context.Context, rawURL string) (*Result, error)
}

// Renderer renders a URL into final (JS-executed) HTML. The production
// implementation drives the render sidecar (zenika/alpine-chrome + Chromium).
type Renderer interface {
	Render(ctx context.Context, rawURL string) (string, error)
}

// importer is the default Importer: SSRF-check → render → extract → download.
type importer struct {
	renderer    Renderer
	imageClient *http.Client
	resolver    ipResolver
}

// NewImporter builds the default importer backed by the given renderer.
func NewImporter(renderer Renderer) Importer {
	return &importer{
		renderer:    renderer,
		imageClient: &http.Client{Timeout: ImportURLImageDownloadTimeout},
		resolver:    net.DefaultResolver,
	}
}

func (im *importer) Import(ctx context.Context, rawURL string) (*Result, error) {
	// Whole-import budget (SPEC-081 §4.3 ImportURLTotalTimeout).
	ctx, cancel := context.WithTimeout(ctx, ImportURLTotalTimeout)
	defer cancel()

	// 1. SSRF + scheme validation.
	if err := validateURL(ctx, rawURL, im.resolver); err != nil {
		return nil, err
	}

	// 2. Render (30s end-to-end budget).
	renderCtx, cancelRender := context.WithTimeout(ctx, ImportURLRenderTimeout)
	html, err := im.renderer.Render(renderCtx, rawURL)
	cancelRender()
	if err != nil {
		return nil, err
	}

	// 3. Extract text + image URLs.
	text := extractText(html, knowledge.MaxKBTextBytes)
	imgURLs := extractImageURLs(html, rawURL)

	// 4. Download images (count/size/SSRF limits).
	images, skipped := im.downloadImages(ctx, imgURLs)

	// 5. No usable content → 422.
	if strings.TrimSpace(text) == "" && len(images) == 0 {
		return nil, ErrNoContent
	}

	return &Result{Text: text, Images: images, SkippedImages: skipped}, nil
}

// downloadImages downloads up to MaxKBImageCount images, each ≤ MaxKBImageBytes.
// It returns the downloaded images and the number skipped (over-count,
// SSRF-blocked, oversized, or failed downloads).
func (im *importer) downloadImages(ctx context.Context, urls []string) ([]Image, int) {
	var images []Image
	skipped := 0
	for _, u := range urls {
		if len(images) >= knowledge.MaxKBImageCount {
			skipped++
			continue
		}
		// Images can also point at internal hosts — SSRF-check each one.
		if err := validateURL(ctx, u, im.resolver); err != nil {
			skipped++
			continue
		}
		data, mime, err := im.downloadImage(ctx, u)
		if err != nil || len(data) > knowledge.MaxKBImageBytes {
			skipped++
			continue
		}
		images = append(images, Image{Data: data, MimeType: mime})
	}
	return images, skipped
}

// downloadImage fetches a single image with a size cap (reads MaxKBImageBytes+1
// so an over-limit image is detected rather than silently truncated).
func (im *importer) downloadImage(ctx context.Context, rawURL string) ([]byte, string, error) {
	ctx, cancel := context.WithTimeout(ctx, ImportURLImageDownloadTimeout)
	defer cancel()

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, "", err
	}
	req.Header.Set("User-Agent", "DataAgentBot/1.0")

	resp, err := im.imageClient.Do(req)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, "", fmt.Errorf("http %d", resp.StatusCode)
	}

	data, err := io.ReadAll(io.LimitReader(resp.Body, knowledge.MaxKBImageBytes+1))
	if err != nil {
		return nil, "", err
	}
	mime := resp.Header.Get("Content-Type")
	if mime == "" {
		mime = "application/octet-stream"
	}
	return data, mime, nil
}

// HTTPRenderer renders pages by POSTing to a render sidecar's /content
// endpoint (deploy/renderer: Chromium --headless --dump-dom). The backend
// carries zero browser dependencies; the browser lives in a separate container.
type HTTPRenderer struct {
	baseURL string
	token   string
	client  *http.Client
}

// NewHTTPRenderer builds a renderer against the given sidecar base URL (e.g.
// "http://renderer:3000") and optional access token (empty = no auth). The
// token is passed as a `?token=` query param — the sidecar does NOT accept an
// Authorization header.
func NewHTTPRenderer(baseURL, token string) *HTTPRenderer {
	return &HTTPRenderer{
		baseURL: strings.TrimRight(baseURL, "/"),
		token:   token,
		client:  &http.Client{Timeout: ImportURLRenderTimeout},
	}
}

func (r *HTTPRenderer) Render(ctx context.Context, rawURL string) (string, error) {
	payload, _ := json.Marshal(map[string]string{"url": rawURL})
	endpoint := r.baseURL + "/content"
	if r.token != "" {
		sep := "?"
		if strings.Contains(endpoint, "?") {
			sep = "&"
		}
		endpoint += sep + "token=" + url.QueryEscape(r.token)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, endpoint, bytes.NewReader(payload))
	if err != nil {
		return "", ErrRenderFailed
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.client.Do(req)
	if err != nil {
		// Connection to the renderer itself failed → 503 (service unavailable).
		return "", ErrRenderUnavailable
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		// Renderer answered but the target site fetch failed → 502.
		return "", ErrRenderFailed
	}

	html, err := io.ReadAll(io.LimitReader(resp.Body, maxHTMLBytes))
	if err != nil {
		return "", ErrRenderFailed
	}
	return string(html), nil
}
