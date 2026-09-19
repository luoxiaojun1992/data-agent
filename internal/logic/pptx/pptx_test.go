package pptx

import (
	"os"
	"path/filepath"
	"testing"
)

// readPPTXHeader reads the first 2 bytes of a generated file, which must be the
// ZIP local file header signature "PK" for a valid .pptx (OOXML is a ZIP).
func readHeader(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read generated file: %v", err)
	}
	if len(data) < 2 {
		t.Fatalf("generated file too small (%d bytes)", len(data))
	}
	return data[:2]
}

func TestGenerateMarkdown(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.pptx")

	md := "# Title\n\n- bullet one\n- bullet two\n"
	if err := Generate(md, out); err != nil {
		t.Fatalf("Generate() error: %v", err)
	}

	hdr := readHeader(t, out)
	if string(hdr) != "PK" {
		t.Fatalf("expected ZIP header PK, got %q", hdr)
	}
}

func TestGenerateHTML(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.pptx")

	html := `<h1 style="color:#1E3A5F;text-align:center">Quarterly Report</h1>
<p>Overview paragraph.</p>
<ul><li>Point one</li><li>Point two</li></ul>
<table><tr><th>Metric</th><th>Value</th></tr><tr><td>Revenue</td><td>100</td></tr></table>`
	if err := GenerateHTML(html, out); err != nil {
		t.Fatalf("GenerateHTML() error: %v", err)
	}

	hdr := readHeader(t, out)
	if string(hdr) != "PK" {
		t.Fatalf("expected ZIP header PK, got %q", hdr)
	}
}

func TestGenerateHTMLEmpty(t *testing.T) {
	dir := t.TempDir()
	out := filepath.Join(dir, "out.pptx")

	if err := GenerateHTML("", out); err == nil {
		t.Fatal("expected error for empty HTML content")
	}
	if err := GenerateHTML("   \n\t ", out); err == nil {
		t.Fatal("expected error for whitespace-only HTML content")
	}
}

func TestGenerateMkdirError(t *testing.T) {
	dir := t.TempDir()
	// A file occupying the parent path forces MkdirAll to fail.
	blocker := filepath.Join(dir, "blocker")
	if err := os.WriteFile(blocker, []byte("x"), 0o644); err != nil {
		t.Fatalf("write blocker: %v", err)
	}
	out := filepath.Join(blocker, "out.pptx")

	if err := Generate("# t", out); err == nil {
		t.Fatal("expected error when parent is a file")
	}
	if err := GenerateHTML("<h1>t</h1>", out); err == nil {
		t.Fatal("expected error when parent is a file")
	}
}

func TestGenerateWriteFileError(t *testing.T) {
	dir := t.TempDir()
	// outputPath pointing at an existing directory makes os.Create fail.
	if err := Generate("# t", dir); err == nil {
		t.Fatal("expected error when output path is a directory")
	}
	if err := GenerateHTML("<h1>t</h1>", dir); err == nil {
		t.Fatal("expected error when output path is a directory")
	}
}
