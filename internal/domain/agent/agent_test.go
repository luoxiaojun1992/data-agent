package agent

import (
	"testing"
)

func TestFirstN(t *testing.T) {
	tests := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 3, "hel"},
		{"", 5, ""},
		{"hi", 10, "hi"},
		{"exact", 5, "exact"},
		{"hello", 0, ""},
	}
	for _, tt := range tests {
		got := firstN(tt.input, tt.n)
		if got != tt.want {
			t.Errorf("firstN(%q, %d) = %q, want %q", tt.input, tt.n, got, tt.want)
		}
	}
}
