package workspace

import (
	"testing"
)

func TestSanitizePath(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{"normal.txt", "normal.txt"},
		{"../etc/passwd", "etc/passwd"},
		{"/root/file", "root/file"},
		{"a\\b\\c", "a/b/c"},
		{"", "unnamed"},
	}

	for _, tt := range tests {
		got := sanitizePath(tt.input)
		if got != tt.want {
			t.Errorf("sanitizePath(%q) = %q, want %q", tt.input, got, tt.want)
		}
	}
}

func TestSanitizePath_DoubleTraversal(t *testing.T) {
	// "../../../" → sanitize removes all ".." → "///" → TrimPrefix "/" → "//"
	got := sanitizePath("../../../")
	if got == "" || got == ".." || got == "/.." {
		t.Errorf("should not be empty or traversal: got %q", got)
	}
}
