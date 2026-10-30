package webimport

import (
	"context"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// HTTPRenderer.Render error mapping (502 vs 503) and downloadImage/downloadImages
// defensive branches (non-200, default mime, SSRF on images) that the Import
// happy-path tests do not reach.

func TestHTTPRenderer_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "boom", http.StatusInternalServerError)
	}))
	defer srv.Close()

	renderer := NewHTTPRenderer(srv.URL, "")
	if _, err := renderer.Render(context.Background(), "https://example.com"); !errors.Is(err, ErrRenderFailed) {
		t.Errorf("err = %v, want ErrRenderFailed", err)
	}
}

func TestHTTPRenderer_NetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	url := srv.URL
	srv.Close() // connection refused next request

	renderer := NewHTTPRenderer(url, "")
	if _, err := renderer.Render(context.Background(), "https://example.com"); !errors.Is(err, ErrRenderUnavailable) {
		t.Errorf("err = %v, want ErrRenderUnavailable", err)
	}
}

func TestDownloadImage_NonOK(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer srv.Close()

	im := &importer{imageClient: &http.Client{}, resolver: alwaysPublicResolver{}}
	if _, _, err := im.downloadImage(context.Background(), srv.URL); err == nil {
		t.Error("expected error on non-200 image response")
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func TestDownloadImage_DefaultMime(t *testing.T) {
	// A response with no Content-Type header → default application/octet-stream.
	// (httptest would auto-detect text/plain, so use a raw transport.)
	im := &importer{
		imageClient: &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return &http.Response{
				StatusCode: http.StatusOK,
				Status:     "200 OK",
				Header:     http.Header{},
				Body:       io.NopCloser(strings.NewReader("img-bytes")),
				Request:    req,
			}, nil
		})},
		resolver: alwaysPublicResolver{},
	}
	_, mime, err := im.downloadImage(context.Background(), "https://example.com/img.png")
	if err != nil {
		t.Fatalf("downloadImage: %v", err)
	}
	if mime != "application/octet-stream" {
		t.Errorf("mime = %q, want application/octet-stream (default when no Content-Type)", mime)
	}
}

func TestDownloadImages_SSRFBlockedImageSkipped(t *testing.T) {
	im := &importer{imageClient: &http.Client{}, resolver: fakeResolver{ips: []net.IP{net.ParseIP("10.0.0.1")}}}
	images, skipped := im.downloadImages(context.Background(), []string{"http://10.0.0.1/img.png"})
	if len(images) != 0 {
		t.Errorf("images = %d, want 0 (SSRF-blocked)", len(images))
	}
	if skipped != 1 {
		t.Errorf("skipped = %d, want 1", skipped)
	}
}
