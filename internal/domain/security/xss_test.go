package security

import "testing"

func TestValidateXSS(t *testing.T) {
	cases := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"plain text", "今天天气不错", false},
		{"english plain", "hello world", false},
		{"normal sentence with =", "x = 1 + 2", false},
		{"script tag", "<script>alert(1)</script>", true},
		{"script uppercase", "<SCRIPT>alert(1)</SCRIPT>", true},
		{"script with whitespace", "< script >alert(1)", true},
		{"img onerror", `<img src="x" onerror="alert(1)">`, true},
		{"svg onload", `<svg onload="alert(1)">`, true},
		{"javascript protocol", "javascript:alert(1)", true},
		{"javascript uppercase", "JAVASCRIPT:alert(1)", true},
		{"standalone onerror", `onerror="alert(1)"`, true},
		{"onload", `onload=alert(1)`, true},
		{"onclick", `onclick="x"`, true},
		{"code sample without xss", "print('hello') if x > 1", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := ValidateXSS(c.input)
			if (err != nil) != c.wantErr {
				t.Fatalf("ValidateXSS(%q) err=%v wantErr=%v", c.input, err, c.wantErr)
			}
		})
	}
}
