package agent

import (
	"context"
	"strings"
	"testing"
)

func TestInferImageMimeType(t *testing.T) {
	cases := []struct{ ext, want string }{
		{"a.png", "image/png"},
		{"a.jpg", "image/jpeg"},
		{"a.jpeg", "image/jpeg"},
		{"a.gif", "image/gif"},
		{"a.webp", "image/webp"},
		{"a.bmp", "image/bmp"},
		{"a.PNG", "image/png"}, // case-insensitive
		{"a.txt", "image/png"}, // unknown → default png
		{"", "image/png"},
	}
	for _, c := range cases {
		if got := inferImageMimeType(c.ext); got != c.want {
			t.Errorf("inferImageMimeType(%q) = %q, want %q", c.ext, got, c.want)
		}
	}
}

func TestTruncateForPrompt(t *testing.T) {
	if got := truncateForPrompt("short", 100); got != "short" {
		t.Errorf("truncateForPrompt(short) = %q, want short", got)
	}
	got := truncateForPrompt("hello world", 5)
	if got != "hello\n...[truncated]" {
		t.Errorf("truncateForPrompt(long) = %q", got)
	}
}

func TestBuildChunkPrompt(t *testing.T) {
	p := buildChunkPrompt("hello")
	if !strings.Contains(p, "---CHUNK---") {
		t.Error("prompt should contain the ---CHUNK--- marker")
	}
	if !strings.Contains(p, "hello") {
		t.Error("prompt should embed the text")
	}
	// long text is truncated to 8000 before embedding
	long := strings.Repeat("x", 9000)
	p = buildChunkPrompt(long)
	if strings.Contains(p, strings.Repeat("x", 9000)) {
		t.Error("long text should be truncated in the prompt")
	}
	if !strings.Contains(p, "[truncated]") {
		t.Error("truncated prompt should carry the truncation marker")
	}
}

func TestParseChunkResponse(t *testing.T) {
	got := parseChunkResponse("a\n---CHUNK---\nb")
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("parseChunkResponse = %v, want [a b]", got)
	}
	// empty parts (consecutive/leading/trailing markers) are dropped
	got = parseChunkResponse("---CHUNK---\n\n---CHUNK---\na\n---CHUNK---")
	if len(got) != 1 || got[0] != "a" {
		t.Errorf("parseChunkResponse = %v, want [a]", got)
	}
	if got := parseChunkResponse(""); len(got) != 0 {
		t.Errorf("parseChunkResponse(empty) = %v, want empty", got)
	}
	// no marker → whole text as one chunk
	if got := parseChunkResponse("plain"); len(got) != 1 || got[0] != "plain" {
		t.Errorf("parseChunkResponse(plain) = %v", got)
	}
}

func TestSplitParagraphs_MergesToTargetSize(t *testing.T) {
	// small paragraphs merge until they would exceed targetSize
	got := splitParagraphs("a\n\nb\n\nc", 100)
	if len(got) != 1 || got[0] != "a\n\nb\n\nc" {
		t.Errorf("splitParagraphs(merge) = %v, want single merged chunk", got)
	}
	// when targetSize is too small, they stay split
	got = splitParagraphs("a\n\nb", 3)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("splitParagraphs(split) = %v, want [a b]", got)
	}
	// empty paragraphs are skipped
	got = splitParagraphs("\n\na\n\n\n\nb\n\n", 100)
	if len(got) != 1 || got[0] != "a\n\nb" {
		t.Errorf("splitParagraphs(skip-empty) = %v, want [a b]", got)
	}
}

func TestBuildChunkFn_NilProviderFallsBackToParagraphSplit(t *testing.T) {
	e := &KBIndexExecutor{} // provider nil → paragraph fallback
	fn := e.buildChunkFn(context.Background())
	chunks, err := fn("first\n\nsecond")
	if err != nil {
		t.Fatalf("buildChunkFn fallback error = %v", err)
	}
	if len(chunks) != 1 || chunks[0] != "first\n\nsecond" {
		t.Errorf("fallback chunks = %v, want merged paragraph", chunks)
	}
}
