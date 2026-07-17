package agent

import (
	"testing"
)

func TestEstimateTokens(t *testing.T) {
	tests := []struct {
		name  string
		text  string
		token int
	}{
		{"empty string", "", 0},
		{"ascii short", "hello", 2},
		{"ascii longer", "hello world this is a test", 7},
		{"cjk single", "你好", 2},
		{"cjk multi-char", "你好世界", 3},
		{"cjk sentence", "这是一段中文测试文本", 8},
		{"power of 4", "1234", 1},
		{"one char", "a", 1},
		{"mixed ascii cjk", "hello你好", 3},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := estimateTokens(tt.text)
			if got != tt.token {
				t.Errorf("estimateTokens(%q) = %d, want %d", tt.text, got, tt.token)
			}
		})
	}
}

func TestEstimateTotalTokens(t *testing.T) {
	tests := []struct {
		name     string
		messages []Message
		want     int
	}{
		{"empty messages", nil, 0},
		{"single ascii", []Message{{Content: "hello world"}}, 3},
		{"multiple messages", []Message{
			{Content: "hello"},
			{Content: "你好世界"},
			{Content: ""},
		}, 2 + 3 + 0},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := EstimateTotalTokens(tt.messages)
			if got != tt.want {
				t.Errorf("EstimateTotalTokens = %d, want %d", got, tt.want)
			}
		})
	}
}

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
