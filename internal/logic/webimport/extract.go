package webimport

import (
	"net/url"
	"regexp"
	"strings"
)

// Precompiled extractors (package-level for reuse across calls).
var (
	imgTagRe   = regexp.MustCompile(`(?is)<img\b[^>]*>`)
	scriptRe   = regexp.MustCompile(`(?is)<script[^>]*>.*?</script>`)
	styleRe    = regexp.MustCompile(`(?is)<style[^>]*>.*?</style>`)
	noscriptRe = regexp.MustCompile(`(?is)<noscript[^>]*>.*?</noscript>`)
	tagRe      = regexp.MustCompile(`(?s)<[^>]*>`)
	blockEndRe = regexp.MustCompile(`(?i)</(p|div|h[1-6]|li|tr|section|article|br)>`)
	brRe       = regexp.MustCompile(`(?i)<br\s*/?>`)

	srcRe     = regexp.MustCompile(`(?i)\bsrc\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	dataSrcRe = regexp.MustCompile(`(?i)\bdata-src\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
	srcsetRe  = regexp.MustCompile(`(?i)\bsrcset\s*=\s*("([^"]*)"|'([^']*)'|([^\s>]+))`)
)

// extractText strips scripts/styles/tags and collapses whitespace into a
// compact, readable plain-text body, truncated to maxBytes (SPEC-081 §5.2).
func extractText(html string, maxBytes int) string {
	html = scriptRe.ReplaceAllString(html, " ")
	html = styleRe.ReplaceAllString(html, " ")
	html = noscriptRe.ReplaceAllString(html, " ")
	// Block-level tags and <br> become newlines for readability.
	html = blockEndRe.ReplaceAllString(html, "\n")
	html = brRe.ReplaceAllString(html, "\n")
	// Strip remaining tags entirely (block boundaries are already newlines).
	text := tagRe.ReplaceAllString(html, "")
	text = decodeEntities(text)
	text = regexp.MustCompile(`[ \t]+`).ReplaceAllString(text, " ")
	text = regexp.MustCompile(`[ \t]*\n[ \t]*`).ReplaceAllString(text, "\n")
	text = regexp.MustCompile(`\n{3,}`).ReplaceAllString(text, "\n\n")
	text = strings.TrimSpace(text)

	if maxBytes > 0 && len(text) > maxBytes {
		// Truncate on a rune boundary to avoid splitting a UTF-8 sequence.
		text = truncateUTF8(text, maxBytes)
	}
	return text
}

// truncateUTF8 cuts s to at most n bytes without splitting a UTF-8 rune.
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	// Step back while the cut point lands in the middle of a multi-byte rune.
	for n > 0 && n < len(s) && (s[n]&0xC0) == 0x80 {
		n--
	}
	return s[:n]
}

// extractImageURLs returns absolute image URLs from <img> src / data-src /
// srcset (first candidate). It deduplicates, skips empty/data URIs, and
// resolves relative URLs against baseURL. The caller enforces the count limit.
func extractImageURLs(html, baseURL string) []string {
	base, _ := url.Parse(baseURL)
	seen := make(map[string]bool)
	var out []string
	for _, tag := range imgTagRe.FindAllString(html, -1) {
		src := attrValue(srcRe, tag)
		if src == "" {
			src = attrValue(dataSrcRe, tag)
		}
		if src == "" {
			src = firstSrcset(srcsetRe, tag)
		}
		src = strings.TrimSpace(src)
		if src == "" || strings.HasPrefix(src, "data:") {
			continue
		}
		abs := resolveURL(base, src)
		if abs == "" || seen[abs] {
			continue
		}
		seen[abs] = true
		out = append(out, abs)
	}
	return out
}

// attrValue extracts the value of an attribute matched by re (src / data-src).
// The regex's m[1] is the outer group (with surrounding quotes); m[2:]+ are the
// inner alternatives holding the unquoted value.
func attrValue(re *regexp.Regexp, tag string) string {
	m := re.FindStringSubmatch(tag)
	if len(m) < 3 {
		return ""
	}
	for _, g := range m[2:] {
		if g != "" {
			return g
		}
	}
	return ""
}

// firstSrcset returns the first URL in a srcset attribute ("a.jpg 1x, b.jpg 2x").
func firstSrcset(re *regexp.Regexp, tag string) string {
	raw := attrValue(re, tag)
	if raw == "" {
		return ""
	}
	first := strings.TrimSpace(strings.Split(raw, ",")[0])
	return strings.Fields(first)[0]
}

// resolveURL resolves a (possibly relative) URL against a base URL. Returns ""
// on failure or for non-http(s) schemes.
func resolveURL(base *url.URL, ref string) string {
	if base == nil {
		return ""
	}
	u, err := url.Parse(ref)
	if err != nil {
		return ""
	}
	abs := base.ResolveReference(u)
	if abs.Scheme != "http" && abs.Scheme != "https" {
		return ""
	}
	return abs.String()
}

// decodeEntities decodes a few common HTML entities (webfetch parity).
func decodeEntities(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&",
		"&lt;", "<",
		"&gt;", ">",
		"&quot;", `"`,
		"&#39;", "'",
		"&nbsp;", " ",
	)
	return replacer.Replace(s)
}
