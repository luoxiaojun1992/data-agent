package webimport

import (
	"net/url"
	"testing"
)

func TestTruncateUTF8_Short(t *testing.T) {
	if got := truncateUTF8("abc", 10); got != "abc" {
		t.Errorf("truncateUTF8(short) = %q, want abc (unchanged)", got)
	}
}

func TestTruncateUTF8_MultibyteCut(t *testing.T) {
	// "中文中文" = 12 bytes; cutting at byte 4 lands mid-rune and must step back.
	got := truncateUTF8("中文中文", 4)
	if got != "中" {
		t.Errorf("truncateUTF8(multibyte) = %q, want 中 (no split rune)", got)
	}
}

func TestAttrValue_NoMatch(t *testing.T) {
	if got := attrValue(srcRe, `<div>no src</div>`); got != "" {
		t.Errorf("attrValue(no match) = %q, want empty", got)
	}
}

func TestAttrValue_EmptyValue(t *testing.T) {
	// matched (m[1] present) but every inner group is empty → return "".
	if got := attrValue(srcRe, `<img src="">`); got != "" {
		t.Errorf("attrValue(empty value) = %q, want empty", got)
	}
}

func TestAttrValue_Unquoted(t *testing.T) {
	if got := attrValue(srcRe, `<img src=img.png>`); got != "img.png" {
		t.Errorf("attrValue(unquoted) = %q, want img.png", got)
	}
}

func TestFirstSrcset_Empty(t *testing.T) {
	if got := firstSrcset(srcsetRe, `<div>no srcset</div>`); got != "" {
		t.Errorf("firstSrcset(no match) = %q, want empty", got)
	}
}

func TestResolveURL_NilBase(t *testing.T) {
	if got := resolveURL(nil, "https://a.com/x.png"); got != "" {
		t.Errorf("resolveURL(nil base) = %q, want empty", got)
	}
}

func TestResolveURL_NonHTTPScheme(t *testing.T) {
	base, _ := url.Parse("https://example.com/page")
	if got := resolveURL(base, "javascript:alert(1)"); got != "" {
		t.Errorf("resolveURL(javascript:) = %q, want empty", got)
	}
	if got := resolveURL(base, "data:image/png;base64,AAAA"); got != "" {
		t.Errorf("resolveURL(data:) = %q, want empty", got)
	}
}

func TestResolveURL_Relative(t *testing.T) {
	base, _ := url.Parse("https://example.com/a/b.html")
	if got := resolveURL(base, "../img/x.png"); got != "https://example.com/img/x.png" {
		t.Errorf("resolveURL(relative) = %q, want resolved absolute URL", got)
	}
}

func TestResolveURL_Malformed(t *testing.T) {
	base, _ := url.Parse("https://example.com/page")
	// "%zz" is an invalid URL escape → url.Parse returns an error.
	if got := resolveURL(base, "%zz"); got != "" {
		t.Errorf("resolveURL(malformed) = %q, want empty", got)
	}
}
