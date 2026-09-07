package webimport

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/knowledge"
)

type fakeRenderer struct {
	html string
	err  error
}

func (f fakeRenderer) Render(_ context.Context, _ string) (string, error) {
	return f.html, f.err
}

// alwaysPublicResolver resolves any host to a public IP so Import tests can
// exercise the render/download path without touching real DNS or SSRF blocks.
type alwaysPublicResolver struct{}

func (alwaysPublicResolver) LookupIP(_ context.Context, _, _ string) ([]net.IP, error) {
	return []net.IP{net.ParseIP("8.8.8.8")}, nil
}

func TestImport_Success(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "image/png")
		_, _ = w.Write([]byte("fake-png-bytes"))
	}))
	defer imgServer.Close()

	renderer := fakeRenderer{html: `<html><body><p>Hello world</p><img src="` + imgServer.URL + `/img.png"></body></html>`}
	im := &importer{renderer: renderer, imageClient: &http.Client{}, resolver: alwaysPublicResolver{}}

	res, err := im.Import(context.Background(), "https://example.com/page")
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if !strings.Contains(res.Text, "Hello world") {
		t.Errorf("text = %q, want to contain 'Hello world'", res.Text)
	}
	if len(res.Images) != 1 {
		t.Fatalf("images len = %d, want 1", len(res.Images))
	}
	if string(res.Images[0].Data) != "fake-png-bytes" {
		t.Errorf("image data = %q", res.Images[0].Data)
	}
	if res.SkippedImages != 0 {
		t.Errorf("skipped = %d, want 0", res.SkippedImages)
	}
}

func TestImport_SSRFBlocked(t *testing.T) {
	blocked := fakeResolver{ips: []net.IP{net.ParseIP("10.0.0.1")}}
	im := &importer{renderer: fakeRenderer{html: "<p>x</p>"}, imageClient: &http.Client{}, resolver: blocked}

	_, err := im.Import(context.Background(), "https://example.com")
	if !errors.Is(err, ErrSSRFBlocked) {
		t.Errorf("err = %v, want ErrSSRFBlocked", err)
	}
}

func TestImport_NoContent(t *testing.T) {
	im := &importer{renderer: fakeRenderer{html: "<p>   </p>"}, imageClient: &http.Client{}, resolver: alwaysPublicResolver{}}

	_, err := im.Import(context.Background(), "https://example.com")
	if !errors.Is(err, ErrNoContent) {
		t.Errorf("err = %v, want ErrNoContent", err)
	}
}

func TestImport_RenderFailed(t *testing.T) {
	im := &importer{renderer: fakeRenderer{err: ErrRenderFailed}, imageClient: &http.Client{}, resolver: alwaysPublicResolver{}}

	_, err := im.Import(context.Background(), "https://example.com")
	if !errors.Is(err, ErrRenderFailed) {
		t.Errorf("err = %v, want ErrRenderFailed", err)
	}
}

func TestImport_ImageTooLargeSkipped(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(make([]byte, knowledge.MaxKBImageBytes+1))
	}))
	defer imgServer.Close()

	renderer := fakeRenderer{html: `<body><p>text</p><img src="` + imgServer.URL + `/big.png"></body>`}
	im := &importer{renderer: renderer, imageClient: &http.Client{}, resolver: alwaysPublicResolver{}}

	res, err := im.Import(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if len(res.Images) != 0 {
		t.Errorf("images len = %d, want 0 (oversized skipped)", len(res.Images))
	}
	if res.SkippedImages != 1 {
		t.Errorf("skipped = %d, want 1", res.SkippedImages)
	}
}

func TestImport_ImageCountLimit(t *testing.T) {
	imgServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write([]byte("img"))
	}))
	defer imgServer.Close()

	var imgs []string
	for i := 0; i < knowledge.MaxKBImageCount+3; i++ {
		imgs = append(imgs, fmt.Sprintf(`<img src="%s/i%d.png">`, imgServer.URL, i))
	}
	renderer := fakeRenderer{html: `<body><p>text</p>` + strings.Join(imgs, "") + `</body>`}
	im := &importer{renderer: renderer, imageClient: &http.Client{}, resolver: alwaysPublicResolver{}}

	res, err := im.Import(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Import error: %v", err)
	}
	if len(res.Images) != knowledge.MaxKBImageCount {
		t.Errorf("images len = %d, want %d", len(res.Images), knowledge.MaxKBImageCount)
	}
	if res.SkippedImages != 3 {
		t.Errorf("skipped = %d, want 3", res.SkippedImages)
	}
}

// TestBrowserlessRenderer_TokenInQuery guards the auth method: browserless/chrome
// v2 authenticates via `?token=` query param, NOT an Authorization header. A
// regression here would make every production URL import return 502.
func TestBrowserlessRenderer_TokenInQuery(t *testing.T) {
	var gotURL, gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<html><body>ok</body></html>"))
	}))
	defer srv.Close()

	renderer := NewBrowserlessRenderer(srv.URL, "secret-token")
	html, err := renderer.Render(context.Background(), "https://example.com")
	if err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if !strings.Contains(html, "ok") {
		t.Errorf("html = %q, want to contain 'ok'", html)
	}
	if !strings.Contains(gotURL, "token=secret-token") {
		t.Errorf("request URL %q, want token=secret-token query param", gotURL)
	}
	if gotAuth != "" {
		t.Errorf("Authorization header = %q, want empty (browserless rejects it)", gotAuth)
	}
}

// TestBrowserlessRenderer_NoTokenOmitsParam ensures an empty token sends no
// token query param at all (no trailing "token=").
func TestBrowserlessRenderer_NoTokenOmitsParam(t *testing.T) {
	var gotURL string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotURL = r.URL.String()
		w.Header().Set("Content-Type", "text/html")
		_, _ = w.Write([]byte("<p>ok</p>"))
	}))
	defer srv.Close()

	renderer := NewBrowserlessRenderer(srv.URL, "")
	if _, err := renderer.Render(context.Background(), "https://example.com"); err != nil {
		t.Fatalf("Render error: %v", err)
	}
	if strings.Contains(gotURL, "token") {
		t.Errorf("request URL %q, want no token param", gotURL)
	}
}

