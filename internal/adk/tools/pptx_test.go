package adktools

import (
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	pptxpkg "github.com/luoxiaojun1992/data-agent/internal/logic/pptx"
)

// TestPPTXGeneratorFormatDispatch verifies the format routing in pptxGenerator:
// empty/markdown → Generate; html (any case) → GenerateHTML; unknown → error.
func TestPPTXGeneratorFormatDispatch(t *testing.T) {
	tests := []struct {
		name          string
		format        string
		wantHTML      bool
		wantMarkdown  bool
		wantErr       bool
	}{
		{name: "empty defaults to markdown", format: "", wantMarkdown: true},
		{name: "explicit markdown", format: "markdown", wantMarkdown: true},
		{name: "explicit html", format: "html", wantHTML: true},
		{name: "html case-insensitive", format: "HTML", wantHTML: true},
		{name: "unknown format errors", format: "pdf", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			var genCalled, genHTMLCalled bool
			patches := gomonkey.NewPatches()
			defer patches.Reset()
			patches.ApplyFunc(pptxpkg.Generate, func(string, string) error {
				genCalled = true
				return nil
			})
			patches.ApplyFunc(pptxpkg.GenerateHTML, func(string, string) error {
				genHTMLCalled = true
				return nil
			})

			fn := pptxGenerator(&Deps{})
			_, err := fn(newToolContext("s1"), PPTXGeneratorArgs{Content: "# t", Format: tt.format})

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error for format %q, got nil", tt.format)
				}
				if genCalled || genHTMLCalled {
					t.Fatal("expected no generation on unsupported format")
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if tt.wantHTML && !genHTMLCalled {
				t.Fatal("expected GenerateHTML to be called")
			}
			if tt.wantMarkdown && !genCalled {
				t.Fatal("expected Generate to be called")
			}
			if tt.wantHTML && genCalled {
				t.Fatal("expected Generate NOT to be called for html")
			}
			if tt.wantMarkdown && genHTMLCalled {
				t.Fatal("expected GenerateHTML NOT to be called for markdown")
			}
		})
	}
}

// TestPPTXGeneratorEmptyContent verifies empty content is rejected before any
// generation happens.
func TestPPTXGeneratorEmptyContent(t *testing.T) {
	fn := pptxGenerator(&Deps{})
	if _, err := fn(newToolContext("s1"), PPTXGeneratorArgs{Content: "   "}); err == nil {
		t.Fatal("expected error for empty content")
	}
}
