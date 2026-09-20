package chat

import (
	"errors"
	"strings"
	"testing"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
)

// ── SPEC-077 PDF 附件 ──

func TestBuildUserContent_WithPDF(t *testing.T) {
	pdf := domainchat.PdfAttachment{Name: "doc.pdf", Text: "PDF 解析文字"}
	c, err := buildUserContent("用户输入", nil, []domainchat.PdfAttachment{pdf}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if c.Role != "user" || len(c.Parts) != 1 {
		t.Fatalf("role=%q parts=%d", c.Role, len(c.Parts))
	}
	text := c.Parts[0].Text
	if !strings.Contains(text, "[PDF:doc.pdf]") {
		t.Fatalf("missing open tag: %q", text)
	}
	if !strings.Contains(text, "[/PDF:doc.pdf]") {
		t.Fatalf("missing close tag: %q", text)
	}
	if !strings.Contains(text, "PDF 解析文字") {
		t.Fatalf("missing parsed text: %q", text)
	}
	// PDF 文字必须位于用户输入之前（SPEC-077 §5.2）。
	if strings.Index(text, "PDF 解析文字") > strings.Index(text, "用户输入") {
		t.Fatalf("PDF text must precede user input: %q", text)
	}
}

func TestBuildUserContent_WithPDFAndImages(t *testing.T) {
	img := domainchat.ImagePart{Data: "aGVsbG8=", MimeType: "image/png"}
	pdf := domainchat.PdfAttachment{Name: "a.pdf", Text: "文字"}
	c, err := buildUserContent("看", []domainchat.ImagePart{img}, []domainchat.PdfAttachment{pdf}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	// parts = [text(合并), inline image]
	if len(c.Parts) != 2 {
		t.Fatalf("parts=%d", len(c.Parts))
	}
	if c.Parts[1].InlineData == nil {
		t.Fatalf("missing inline image: %+v", c.Parts)
	}
	if !strings.Contains(c.Parts[0].Text, "[PDF:a.pdf]") {
		t.Fatalf("missing PDF tag: %q", c.Parts[0].Text)
	}
}

func TestBuildUserContent_EmptyPDFTextSkipped(t *testing.T) {
	// 纯扫描件 PDF（text 空）：不前置文字，只有用户输入。
	c, err := buildUserContent("只有文字", nil, []domainchat.PdfAttachment{{Name: "scan.pdf", Text: "  "}}, nil)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if len(c.Parts) != 1 {
		t.Fatalf("parts=%d", len(c.Parts))
	}
	if strings.Contains(c.Parts[0].Text, "[PDF:") {
		t.Fatalf("empty PDF text must not be tagged: %q", c.Parts[0].Text)
	}
	if c.Parts[0].Text != "只有文字" {
		t.Fatalf("unexpected text: %q", c.Parts[0].Text)
	}
}

func TestValidateChatTextSize(t *testing.T) {
	// 恰好 100KB 通过。
	if err := validateChatTextSize(strings.Repeat("a", domainchat.MaxChatTextBytes), nil, nil); err != nil {
		t.Fatalf("exact 100KB should pass: %v", err)
	}
	// 100KB + 1 字节拒绝。
	if err := validateChatTextSize(strings.Repeat("a", domainchat.MaxChatTextBytes+1), nil, nil); !errors.Is(err, domainchat.ErrChatTextTooLarge) {
		t.Fatalf("expected ErrChatTextTooLarge, got %v", err)
	}
	// 多 PDF 合并计数：两个各 50KB 恰好 100KB 通过。
	half := domainchat.MaxChatTextBytes / 2
	pdfs := []domainchat.PdfAttachment{
		{Name: "a.pdf", Text: strings.Repeat("a", half)},
		{Name: "b.pdf", Text: strings.Repeat("b", half)},
	}
	if err := validateChatTextSize("", pdfs, nil); err != nil {
		t.Fatalf("two PDFs at exactly 100KB should pass: %v", err)
	}
	// 用户提示词 + PDF 文字合并超限。
	pdfs2 := []domainchat.PdfAttachment{{Name: "a.pdf", Text: strings.Repeat("a", domainchat.MaxChatTextBytes)}}
	if err := validateChatTextSize("x", pdfs2, nil); !errors.Is(err, domainchat.ErrChatTextTooLarge) {
		t.Fatalf("expected ErrChatTextTooLarge, got %v", err)
	}
}

func TestHasPDFText(t *testing.T) {
	if hasPDFText(nil) {
		t.Error("nil pdfs should be false")
	}
	if hasPDFText([]domainchat.PdfAttachment{{Name: "a", Text: "   "}}) {
		t.Error("whitespace-only text should be false")
	}
	if !hasPDFText([]domainchat.PdfAttachment{{Name: "a", Text: "hello"}}) {
		t.Error("non-empty text should be true")
	}
}

// ── SPEC-096 Excel 附件 ──

func TestBuildUserContent_WithExcel(t *testing.T) {
	excel := domainchat.ExcelAttachment{Name: "data.xlsx", Text: "A\tB\n1\t2"}
	c, err := buildUserContent("分析数据", nil, nil, []domainchat.ExcelAttachment{excel})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if !strings.Contains(c.Parts[0].Text, "[Excel:data.xlsx]") || !strings.Contains(c.Parts[0].Text, "[/Excel:data.xlsx]") {
		t.Fatalf("expected Excel tag, got: %q", c.Parts[0].Text)
	}
	if !strings.Contains(c.Parts[0].Text, "分析数据") {
		t.Fatalf("expected user text preserved, got: %q", c.Parts[0].Text)
	}
}

func TestBuildUserContent_ExcelWhitespaceSkipped(t *testing.T) {
	c, err := buildUserContent("只有文字", nil, nil, []domainchat.ExcelAttachment{{Name: "empty.xlsx", Text: "   "}})
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	if strings.Contains(c.Parts[0].Text, "[Excel:") {
		t.Fatalf("whitespace-only Excel should be skipped, got: %q", c.Parts[0].Text)
	}
}

func TestHasExcelText(t *testing.T) {
	if hasExcelText(nil) {
		t.Error("nil excels should be false")
	}
	if hasExcelText([]domainchat.ExcelAttachment{{Name: "a", Text: "   "}}) {
		t.Error("whitespace-only text should be false")
	}
	if !hasExcelText([]domainchat.ExcelAttachment{{Name: "a", Text: "hello"}}) {
		t.Error("non-empty text should be true")
	}
}

func TestValidateChatTextSize_WithExcel(t *testing.T) {
	// Excel 恰好 100KB 通过。
	excels := []domainchat.ExcelAttachment{{Name: "a.xlsx", Text: strings.Repeat("x", domainchat.MaxChatTextBytes)}}
	if err := validateChatTextSize("", nil, excels); err != nil {
		t.Fatalf("exact 100KB Excel should pass: %v", err)
	}
	// 用户提示词 + Excel 文字合并超限。
	excels2 := []domainchat.ExcelAttachment{{Name: "a.xlsx", Text: strings.Repeat("x", domainchat.MaxChatTextBytes)}}
	if err := validateChatTextSize("y", nil, excels2); !errors.Is(err, domainchat.ErrChatTextTooLarge) {
		t.Fatalf("expected ErrChatTextTooLarge, got %v", err)
	}
	// PDF + Excel 合并计数超限。
	pdfs := []domainchat.PdfAttachment{{Name: "a.pdf", Text: strings.Repeat("p", domainchat.MaxChatTextBytes)}}
	excels3 := []domainchat.ExcelAttachment{{Name: "a.xlsx", Text: "x"}}
	if err := validateChatTextSize("", pdfs, excels3); !errors.Is(err, domainchat.ErrChatTextTooLarge) {
		t.Fatalf("expected ErrChatTextTooLarge (PDF+Excel), got %v", err)
	}
}
