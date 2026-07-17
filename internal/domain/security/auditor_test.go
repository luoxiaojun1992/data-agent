package security

import (
	"regexp"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
)

type mockAlertLogger struct {
	alerts []string
}

func (m *mockAlertLogger) LogAlert(level, category, message string, details map[string]interface{}) {
	m.alerts = append(m.alerts, level+":"+category+":"+message)
}

func TestDefaultRules(t *testing.T) {
	rules := DefaultRules()
	if len(rules.InputRules) != 6 {
		t.Errorf("InputRules length: got %d, want 6", len(rules.InputRules))
	}
	if len(rules.OutputRules) != 3 {
		t.Errorf("OutputRules length: got %d, want 3", len(rules.OutputRules))
	}
}

func TestConfig_Compile(t *testing.T) {
	cfg := &Config{
		InputRules: []Rule{
			{Name: "test_regex", Type: "regex", Pattern: `\d{3}`, Action: "block"},
			{Name: "test_keyword", Type: "keyword", Pattern: "DROP TABLE", Action: "block"},
		},
	}
	cfg.Compile()

	if cfg.InputRules[0].compiled == nil {
		t.Error("regex rule should have compiled regex")
	}
	if cfg.InputRules[1].compiled != nil {
		t.Error("keyword rule should NOT have compiled regex")
	}
}

func TestAuditor_AuditInput(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})

	tests := []struct {
		name    string
		input   string
		wantErr bool
	}{
		{"safe text", "SELECT * FROM users", false},
		{"drop table blocked", "DROP TABLE users", true},
		{"delete from blocked", "DELETE FROM users WHERE id=1", true},
		{"insert into alerts", "INSERT INTO users VALUES (1)", false}, // alert only, no error
		{"script tag blocked", "<script>alert(1)</script>", true},
		{"empty input", "", false},
		{"normal query", "SELECT id, name FROM products WHERE price > 100", false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := a.AuditInput(tt.input)
			if tt.wantErr && err == nil {
				t.Errorf("AuditInput(%q) should return error", tt.input)
			}
			if !tt.wantErr && err != nil {
				t.Errorf("AuditInput(%q) unexpected error: %v", tt.input, err)
			}
		})
	}
}

func TestAuditor_AuditOutput(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})

	tests := []struct {
		name      string
		input     string
		wantPart  string // should be in output
		wantNotPart string // should NOT be in output
	}{
		{
			name:      "phone sanitized",
			input:     "call 13812345678 for help",
			wantPart:  "138****567",
			wantNotPart: "13812345678",
		},
		{
			name:      "id card sanitized",
			input:     "ID: 110101199001011234",
			wantPart:  "110***********123",
			wantNotPart: "110101199001011234",
		},
		{
			name:      "api key sanitized",
			input:     "key: sk-" + strings.Repeat("a", 32),
			wantPart:  "sk-a****",
			wantNotPart: "sk-" + strings.Repeat("a", 32),
		},
		{
			name:     "normal text unchanged",
			input:    "The analysis shows growth",
			wantPart: "The analysis shows growth",
		},
		{
			name:    "empty input",
			input:   "",
			wantPart: "",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			output, err := a.AuditOutput(tt.input)
			if err != nil {
				t.Fatalf("AuditOutput unexpected error: %v", err)
			}
			if tt.wantPart != "" && !strings.Contains(output, tt.wantPart) {
				t.Errorf("output should contain %q, got %q", tt.wantPart, output)
			}
			if tt.wantNotPart != "" && strings.Contains(output, tt.wantNotPart) {
				t.Errorf("output should NOT contain %q, got %q", tt.wantNotPart, output)
			}
		})
	}
}

func TestAuditor_AuditToolCall(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})

	t.Run("safe tool call", func(t *testing.T) {
		err := a.AuditToolCall("sql_executor", map[string]any{"query": "SELECT 1"})
		if err != nil {
			t.Errorf("safe tool call should pass: %v", err)
		}
	})

	t.Run("workspace exec to etc blocked", func(t *testing.T) {
		err := a.AuditToolCall("workspace_exec", map[string]any{"path": "/etc/passwd"})
		if err == nil {
			t.Error("workspace_exec to /etc should be blocked")
		}
	})

	t.Run("workspace write to root blocked", func(t *testing.T) {
		err := a.AuditToolCall("workspace_write", map[string]any{"path": "/root/.ssh"})
		if err == nil {
			t.Error("workspace_write to /root should be blocked")
		}
	})

	t.Run("workspace exec safe path", func(t *testing.T) {
		err := a.AuditToolCall("workspace_exec", map[string]any{"path": "/home/user/safe"})
		if err != nil {
			t.Errorf("safe path should pass: %v", err)
		}
	})

	t.Run("no path param", func(t *testing.T) {
		err := a.AuditToolCall("workspace_exec", map[string]any{"cmd": "ls"})
		if err != nil {
			t.Errorf("no path param should pass: %v", err)
		}
	})
}

func TestAuditor_UpdateRules(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	newRules := &Config{
		InputRules: []Rule{
			{Name: "custom", Type: "keyword", Pattern: "CUSTOM_BLOCK", Action: "block"},
		},
	}
	a.UpdateRules(newRules)

	err := a.AuditInput("CUSTOM_BLOCK this text")
	if err == nil {
		t.Error("should block CUSTOM_BLOCK after UpdateRules")
	}
}

// ── matchRule Tests ──

func TestAuditor_matchRule_KeywordMatch(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{Name: "test_keyword", Type: "keyword", Pattern: "DROP TABLE", Action: "block"}
	matched, match := a.matchRule(rule, "DROP TABLE users")
	if !matched {
		t.Error("keyword should match")
	}
	if match != "DROP TABLE" {
		t.Errorf("match: got %q, want %q", match, "DROP TABLE")
	}
}

func TestAuditor_matchRule_KeywordCaseInsensitive(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{Name: "test_keyword", Type: "keyword", Pattern: "DROP TABLE", Action: "block"}
	matched, _ := a.matchRule(rule, "drop table users")
	if !matched {
		t.Error("keyword should match case-insensitively")
	}
}

func TestAuditor_matchRule_KeywordNoMatch(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{Name: "test_keyword", Type: "keyword", Pattern: "DROP TABLE", Action: "block"}
	matched, match := a.matchRule(rule, "SELECT * FROM users")
	if matched {
		t.Error("keyword should not match")
	}
	if match != "" {
		t.Errorf("match should be empty: got %q", match)
	}
}

func TestAuditor_matchRule_RegexMatch(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{
		Name:     "phone",
		Type:     "regex",
		Pattern:  `1[3-9]\d{9}`,
		Action:   "sanitize",
		compiled: regexp.MustCompile(`1[3-9]\d{9}`),
	}
	matched, match := a.matchRule(rule, "call 13812345678 now")
	if !matched {
		t.Error("regex should match")
	}
	if match != "13812345678" {
		t.Errorf("match: got %q, want %q", match, "13812345678")
	}
}

func TestAuditor_matchRule_RegexNoMatch(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{
		Name:     "phone",
		Type:     "regex",
		Pattern:  `1[3-9]\d{9}`,
		Action:   "sanitize",
		compiled: regexp.MustCompile(`1[3-9]\d{9}`),
	}
	matched, match := a.matchRule(rule, "no phone here")
	if matched {
		t.Error("regex should not match")
	}
	if match != "" {
		t.Errorf("match should be empty: got %q", match)
	}
}

func TestAuditor_matchRule_RegexCompileOnDemand(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{Name: "test_regex", Type: "regex", Pattern: `abc\d+`, Action: "sanitize"}
	// compiled is nil, so matchRule should compile it on demand
	matched, match := a.matchRule(rule, "abc123")
	if !matched {
		t.Error("should compile regex on demand and match")
	}
	if match != "abc123" {
		t.Errorf("match: got %q, want %q", match, "abc123")
	}
}

func TestAuditor_matchRule_RegexCompileError(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{Name: "bad_regex", Type: "regex", Pattern: `[\d`, Action: "sanitize"}
	matched, match := a.matchRule(rule, "text")
	if matched {
		t.Error("invalid regex should not match")
	}
	if match != "" {
		t.Errorf("match should be empty: got %q", match)
	}
}

func TestAuditor_matchRule_DefaultNoMatch(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	rule := Rule{Name: "unknown", Type: "unknown_type", Pattern: "test", Action: "block"}
	// Unknown type falls through to return false, ""
	result, _ := regexp.Compile("test")
	_ = result
	matched, match := a.matchRule(rule, "test")
	if matched {
		t.Error("unknown rule type should not match")
	}
	if match != "" {
		t.Errorf("match should be empty: got %q", match)
	}
}

// ── sanitizeByType Tests ──

func TestSanitizeByType_Phone(t *testing.T) {
	tests := []struct {
		name     string
		ruleName string
		input    string
		want     string
	}{
		{"phone 11 digits", "phone", "13812345678", "138****5678"},
		{"phone non-11", "phone", "12345", "***"},
		{"phone empty", "phone", "", "***"},
		{"id_card 18 chars", "id_card", "110101199001011234", "110***********1234"},
		{"id_card non-18", "id_card", "12345", "***"},
		{"api_key valid", "api_key", "sk-" + strings.Repeat("a", 32), "sk-a****"},
		{"api_key short <= 8", "api_key", "sk-short", "***"},
		{"api_key over 8", "api_key", "sk-123456", "sk-1****"},
		{"default type", "email", "user@example.com", "***"},
		{"default type ssn", "ssn", "123-45-6789", "***"},
		{"default type credit_card", "credit_card", "4111111111111111", "***"},
		{"default type unknown", "unknown_rule", "anything", "***"},
		{"default type empty", "", "", "***"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := sanitizeByType(tt.ruleName, tt.input)
			if got != tt.want {
				t.Errorf("sanitizeByType(%q, %q) = %q, want %q", tt.ruleName, tt.input, got, tt.want)
			}
		})
	}
}

// ── AuditOutput Edge Cases ──

func TestAuditor_AuditOutput_MultipleMatches(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	input := "call 13812345678 or 13987654321 for info"
	output, err := a.AuditOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(output, "13812345678") {
		t.Error("first phone should be sanitized")
	}
	if strings.Contains(output, "13987654321") {
		t.Error("second phone should be sanitized")
	}
}

func TestAuditor_AuditOutput_EmptyInput(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	output, err := a.AuditOutput("")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != "" {
		t.Errorf("output should be empty: got %q", output)
	}
}

func TestAuditor_AuditOutput_NoSensitiveData(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	input := "The analysis shows a growth of 15%"
	output, err := a.AuditOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if output != input {
		t.Errorf("output should be unchanged: got %q, want %q", output, input)
	}
}

func TestAuditor_AuditOutput_PanicRecovery(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})

	// Mock ReplaceAllStringFunc to panic, covering the recover() path
	patches := gomonkey.ApplyMethodFunc(&regexp.Regexp{}, "ReplaceAllStringFunc",
		func(src string, repl func(string) string) string {
			panic("forced panic for coverage")
		})
	defer patches.Reset()

	// Use input that will match the phone rule (first sanitize rule)
	input := "call 13812345678 now"
	output, err := a.AuditOutput(input)
	if err != nil {
		t.Fatalf("AuditOutput should not error even on panic: %v", err)
	}
	// Output should be unchanged since the sanitize panic was recovered
	if !strings.Contains(output, "13812345678") {
		t.Errorf("output should contain original phone number after panic recovery: %q", output)
	}
}

func TestAuditor_AuditOutput_OnlyApiKey(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	input := "sk-" + strings.Repeat("b", 40)
	output, err := a.AuditOutput(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(output, "****") {
		t.Errorf("api key should be sanitized: got %q", output)
	}
}
