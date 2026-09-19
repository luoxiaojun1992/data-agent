// Package pptx provides PPTX file generation via genppt (pure Go).
// The agent generates markdown or HTML content; this package converts it to .pptx.
package pptx

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/CoolBanHub/genppt"
)

// Generate creates a .pptx file from markdown content and saves it at outputPath.
// The parent directory is created if it doesn't exist.
func Generate(markdown string, outputPath string) error {
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("pptx: create output dir: %w", err)
	}

	opts := genppt.DefaultMarkdownOptions()
	opts.TitleFontSize = 40
	opts.HeadingColor = "#1A1A2E"
	opts.BodyColor = "#333333"
	opts.SlideBackground = "#FFFFFF"
	opts.CodeBackground = "#F5F5F5"

	pres := genppt.FromMarkdownWithOptions(markdown, opts)
	if err := pres.WriteFile(outputPath); err != nil {
		return fmt.Errorf("pptx: write file: %w", err)
	}
	return nil
}

// GenerateHTML creates a .pptx file from HTML content and saves it at outputPath.
// HTML supports tables, images (with border-radius/float), and per-element inline
// styles — producing richer layouts than the markdown path. The parent directory
// is created if it doesn't exist.
func GenerateHTML(html string, outputPath string) error {
	if strings.TrimSpace(html) == "" {
		return fmt.Errorf("pptx: html content must not be empty")
	}
	dir := filepath.Dir(outputPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("pptx: create output dir: %w", err)
	}

	opts := genppt.DefaultHTMLOptions()
	opts.TitleColor = "#1E3A5F"
	opts.HeadingColor = "#1E3A5F"
	opts.BodyColor = "#333333"
	opts.SlideBackground = "#FFFFFF"

	pres := genppt.FromHTMLWithOptions(html, opts)
	if err := pres.WriteFile(outputPath); err != nil {
		return fmt.Errorf("pptx: write file: %w", err)
	}
	return nil
}
