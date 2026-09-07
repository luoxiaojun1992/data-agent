package webimport

import (
	"net/url"
	"strings"
	"testing"
)

func TestExtractTitle(t *testing.T) {
	if got := extractTitle(`<html><head><title>My Page</title></head></html>`); got != "My Page" {
		t.Errorf("extractTitle = %q, want %q", got, "My Page")
	}
	if got := extractTitle(`<html><body>no title</body></html>`); got != "" {
		t.Errorf("extractTitle = %q, want empty", got)
	}
	if got := extractTitle(`<title>  padded  </title>`); got != "padded" {
		t.Errorf("extractTitle = %q, want trimmed", got)
	}
}

func TestExtractText(t *testing.T) {
	cases := []struct {
		name string
		html string
		want string
	}{
		{
			"strips script and style",
			`<html><body><script>var x=1;</script><style>.a{}</style><p>Hello</p></body></html>`,
			"Hello",
		},
		{
			"collapses whitespace",
			"<p>a   b</p><p>c</p>",
			"a b\nc",
		},
		{
			"block tags become newlines",
			"<div>one</div><div>two</div>",
			"one\ntwo",
		},
		{
			"decodes entities",
			"&amp; &lt;tag&gt; &nbsp;",
			"& <tag>",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := extractText(tc.html, 0)
			if strings.TrimSpace(got) != tc.want {
				t.Errorf("extractText = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestExtractText_Truncates(t *testing.T) {
	in := strings.Repeat("a", 100)
	got := extractText(in, 50)
	if len(got) != 50 {
		t.Errorf("extractText truncated len = %d, want 50", len(got))
	}
}

func TestExtractText_TruncateUTF8Boundary(t *testing.T) {
	// 3-byte runes; cutting at byte 4 must not split a rune.
	in := "中文中文中文"
	got := extractText(in, 4)
	if !strings.HasPrefix(in, got) {
		t.Errorf("truncated text %q is not a prefix of %q", got, in)
	}
}

func TestExtractImageURLs(t *testing.T) {
	html := `<img src="https://a.com/1.png">
		<img data-src="/2.jpg">
		<img srcset="https://a.com/3.png 1x, https://a.com/4.png 2x">
		<img src="data:image/png;base64,AAAA">
		<img src="https://a.com/1.png">` // duplicate

	got := extractImageURLs(html, "https://a.com/page")
	want := []string{
		"https://a.com/1.png",
		"https://a.com/2.jpg",
		"https://a.com/3.png",
	}
	if len(got) != len(want) {
		t.Fatalf("extractImageURLs len = %d, want %d: %v", len(got), len(want), got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("extractImageURLs[%d] = %q, want %q", i, got[i], want[i])
		}
	}
}

func TestExtractImageURLs_NoImages(t *testing.T) {
	if got := extractImageURLs(`<p>no images</p>`, "https://a.com"); len(got) != 0 {
		t.Errorf("expected no images, got %v", got)
	}
}

func TestResolveURL(t *testing.T) {
	base := mustURL(t, "https://a.com/sub/page.html")
	cases := []struct{ in, want string }{
		{"/abs.png", "https://a.com/abs.png"},
		{"rel.png", "https://a.com/sub/rel.png"},
		{"https://b.com/x.png", "https://b.com/x.png"},
		{"javascript:alert(1)", ""},
	}
	for _, tc := range cases {
		if got := resolveURL(base, tc.in); got != tc.want {
			t.Errorf("resolveURL(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

func TestDecodeEntities(t *testing.T) {
	got := decodeEntities("&amp;&lt;&gt;&quot;&#39;&nbsp;")
	if got != `&<>"' ` {
		t.Errorf("decodeEntities = %q", got)
	}
}

func mustURL(t *testing.T, s string) *url.URL {
	t.Helper()
	u, err := url.Parse(s)
	if err != nil {
		t.Fatalf("parse url %q: %v", s, err)
	}
	return u
}
