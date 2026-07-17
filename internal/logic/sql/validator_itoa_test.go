package sql

import "testing"

func TestItoa_Comprehensive(t *testing.T) {
	tests := []struct {
		name string
		n    int
		want string
	}{
		{"zero", 0, "0"},
		{"single digit 1", 1, "1"},
		{"single digit 5", 5, "5"},
		{"single digit 9", 9, "9"},
		{"two digits", 42, "42"},
		{"three digits", 100, "100"},
		{"four digits", 1234, "1234"},
		{"multi-digit", 99999, "99999"},
		{"negative", -1, ""},
		{"negative large", -123, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := itoa(tt.n)
			if got != tt.want {
				t.Errorf("itoa(%d) = %q, want %q", tt.n, got, tt.want)
			}
		})
	}
}
