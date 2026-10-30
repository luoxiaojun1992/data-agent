package handler

import (
	"strings"
	"testing"
)

func TestValidateAuditQueryStrings(t *testing.T) {
	cases := []struct {
		name    string
		q, path string
		wantErr bool
	}{
		{"ok", "a", "b", false},
		{"q too long", strings.Repeat("a", 101), "", true},
		{"path too long", "", strings.Repeat("b", 101), true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateAuditQueryStrings(c.q, c.path)
			if (err != nil) != c.wantErr {
				t.Fatalf("validateAuditQueryStrings(%q, %q) err=%v, wantErr=%v", c.q, c.path, err, c.wantErr)
			}
		})
	}
}
