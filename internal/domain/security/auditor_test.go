package security

import (
	"strings"
	"testing"
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
