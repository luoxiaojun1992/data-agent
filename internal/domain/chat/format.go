package chat

import "fmt"

// FormatPDFText wraps a PDF's parsed text in the [PDF:name]…[/PDF:name] tag
// protocol (SPEC-077 §5.3) so the frontend can strip it from history rendering
// while the LLM still receives the text. Exported so the task path (SPEC-096
// D1 方案 B) reuses the exact same wire protocol as chat.
func FormatPDFText(pdf PdfAttachment) string {
	return fmt.Sprintf("[PDF:%s]\n%s\n[/PDF:%s]", pdf.Name, pdf.Text, pdf.Name)
}

// FormatExcelText wraps an Excel attachment's parsed text in the
// [Excel:name]…[/Excel:name] tag protocol (SPEC-096 D3), symmetric with
// FormatPDFText. Name is used by the frontend to render a 📊 card; Text is
// consumed by the LLM and never rendered.
func FormatExcelText(excel ExcelAttachment) string {
	return fmt.Sprintf("[Excel:%s]\n%s\n[/Excel:%s]", excel.Name, excel.Text, excel.Name)
}
