package knowledge

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
)

type pureTestRedactor struct {
	fn func(ctx context.Context, text string) (string, error)
}

func (f *pureTestRedactor) Redact(ctx context.Context, text string) (string, error) {
	if f.fn != nil {
		return f.fn(ctx, text)
	}
	return text, nil
}

var _ security.Redactor = (*pureTestRedactor)(nil)

// ── splitBySentence / lastSentenceBoundary / isSentenceEnd ───────────

func TestSplitBySentence_ChineseBoundary(t *testing.T) {
	got := splitBySentence("第一句。第二句。第三句。", 5)
	if len(got) == 0 {
		t.Fatal("expected at least one segment")
	}
	joined := strings.Join(got, "")
	if joined != "第一句。第二句。第三句。" {
		t.Errorf("segments lost content: %q", joined)
	}
}

func TestSplitBySentence_NoBoundaryForcedYield(t *testing.T) {
	// A long string with no sentence boundary forces a yield every 3 windows.
	text := strings.Repeat("x", 30)
	got := splitBySentence(text, 10)
	if len(got) == 0 {
		t.Fatal("expected forced-yield segments")
	}
	if strings.Join(got, "") != text {
		t.Errorf("content lost: got %q", strings.Join(got, ""))
	}
}

func TestSplitBySentence_Empty(t *testing.T) {
	if got := splitBySentence("", 10); len(got) != 0 {
		t.Errorf("splitBySentence(empty) = %v, want empty", got)
	}
}

func TestLastSentenceBoundary(t *testing.T) {
	cases := []struct {
		name string
		buf  string
		want int
	}{
		{"chinese period", "你好。世界", 2},
		{"english period + space", "abc. def", 3},
		{"english period at end", "abc.", 3},
		{"period not followed by space", "abc.def", -1},
		{"no boundary", "abcdef", -1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := lastSentenceBoundary([]rune(tc.buf)); got != tc.want {
				t.Errorf("lastSentenceBoundary(%q) = %d, want %d", tc.buf, got, tc.want)
			}
		})
	}
}

func TestIsSentenceEnd(t *testing.T) {
	ends := []rune{' ', '\n', '\t', '\r', '。', '！', '？', '.', '!', '?'}
	for _, r := range ends {
		if !isSentenceEnd(r) {
			t.Errorf("isSentenceEnd(%q) = false, want true", r)
		}
	}
	if isSentenceEnd('a') || isSentenceEnd('中') {
		t.Error("isSentenceEnd should be false for letters")
	}
}

// ── extFromMime default ──────────────────────────────────────────────

func TestExtFromMime_Default(t *testing.T) {
	if got := extFromMime("application/pdf"); got != ".jpg" {
		t.Errorf("extFromMime(pdf) = %q, want .jpg default", got)
	}
}

// ── maybeRedact ──────────────────────────────────────────────────────

func TestMaybeRedact_NilRedactor(t *testing.T) {
	s := &Service{}
	got, err := s.maybeRedact(context.Background(), "hello")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want unchanged", got)
	}
}

func TestMaybeRedact_SwitchOff(t *testing.T) {
	s := &Service{redactor: &pureTestRedactor{}, redactionEnabled: func() bool { return false }}
	got, err := s.maybeRedact(context.Background(), "hello")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got != "hello" {
		t.Errorf("switch off should keep original, got %q", got)
	}
}

func TestMaybeRedact_Success(t *testing.T) {
	s := &Service{redactor: &pureTestRedactor{fn: func(_ context.Context, text string) (string, error) {
		return "REDACTED", nil
	}}}
	got, err := s.maybeRedact(context.Background(), "hello")
	if err != nil {
		t.Fatalf("error = %v", err)
	}
	if got != "REDACTED" {
		t.Errorf("got %q, want REDACTED", got)
	}
}

func TestMaybeRedact_Error(t *testing.T) {
	s := &Service{redactor: &pureTestRedactor{fn: func(_ context.Context, text string) (string, error) {
		return "", errors.New("redaction failed")
	}}}
	if _, err := s.maybeRedact(context.Background(), "hello"); err == nil {
		t.Fatal("expected fail-closed error")
	}
}

// ── CreateTextDoc validation ─────────────────────────────────────────

func TestCreateTextDoc_EmptyTitle(t *testing.T) {
	s := &Service{}
	if _, err := s.CreateTextDoc(context.Background(), "u1", "", "text"); err == nil {
		t.Fatal("expected title required error")
	}
}

// ── CreateFromText validation ────────────────────────────────────────

func TestCreateFromText_EmptyUserID(t *testing.T) {
	s := &Service{}
	if _, err := s.CreateFromText(context.Background(), "", "title", "title.txt", "text"); err == nil {
		t.Fatal("expected userID required error")
	}
}
