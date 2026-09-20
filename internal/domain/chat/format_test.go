package chat

import (
	"strings"
	"testing"
)

func TestFormatPDFText(t *testing.T) {
	got := FormatPDFText(PdfAttachment{Name: "a.pdf", Text: "内容"})
	if !strings.Contains(got, "[PDF:a.pdf]") || !strings.Contains(got, "[/PDF:a.pdf]") || !strings.Contains(got, "内容") {
		t.Fatalf("unexpected PDF tag: %q", got)
	}
}

func TestFormatExcelText(t *testing.T) {
	got := FormatExcelText(ExcelAttachment{Name: "b.xlsx", Text: "单元格"})
	if !strings.Contains(got, "[Excel:b.xlsx]") || !strings.Contains(got, "[/Excel:b.xlsx]") || !strings.Contains(got, "单元格") {
		t.Fatalf("unexpected Excel tag: %q", got)
	}
}
