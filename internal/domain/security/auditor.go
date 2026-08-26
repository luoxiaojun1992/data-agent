package security

import (
	"context"
	"fmt"
	"regexp"
	"strings"
	"sync"
)

// Rule represents a security rule.
type Rule struct {
	Name     string `json:"name" yaml:"name"`
	Type     string `json:"type" yaml:"type"` // "regex", "keyword", "path"
	Pattern  string `json:"pattern" yaml:"pattern"`
	Action   string `json:"action" yaml:"action"` // "block", "alert", "sanitize"
	Priority int    `json:"priority" yaml:"priority"`
	compiled *regexp.Regexp
}

// Config holds the security rules configuration.
type Config struct {
	InputRules  []Rule `json:"input_rules" yaml:"input_rules"`
	OutputRules []Rule `json:"output_rules" yaml:"output_rules"`
}

// Auditor is the security audit engine.
// It sanitizes inputs, sanitizes outputs, and audits tool calls.
type Auditor struct {
	mu       sync.RWMutex
	config   *Config
	alerts   AlertLogger
	redactor Redactor // optional PII redactor (pii-redaction service); nil = 降级 regex 规则
}

// Redactor is the single PII-redaction interface shared by input and output
// auditing (SPEC-068). The concrete implementation lives in the service layer
// (internal/service/pii) and is injected via SetRedactor; the domain layer
// never imports infra/service packages.
type Redactor interface {
	// Redact returns the PII-redacted text, or an error (switch off / service
	// failure) that signals the caller to fall back to regex rules.
	Redact(ctx context.Context, text string) (string, error)
}

// AlertLogger logs security alerts.
type AlertLogger interface {
	LogAlert(level, category, message string, details map[string]interface{})
}

// NewAuditor creates a new security auditor with default rules.
func NewAuditor(alerts AlertLogger) *Auditor {
	config := DefaultRules()
	config.Compile()
	return &Auditor{
		config: config,
		alerts: alerts,
	}
}

// SetRedactor injects the optional PII redactor. Safe to call before the
// auditor is shared across runtimes; the auditor itself is immutable at runtime.
func (a *Auditor) SetRedactor(r Redactor) {
	a.mu.Lock()
	defer a.mu.Unlock()
	a.redactor = r
}

// DefaultRules returns the default security rules.
func DefaultRules() *Config {
	return &Config{
		InputRules: []Rule{
			{Name: "sql_drop", Type: "keyword", Pattern: "DROP TABLE", Action: "block", Priority: 100},
			{Name: "sql_delete", Type: "keyword", Pattern: "DELETE FROM", Action: "block", Priority: 100},
			{Name: "sql_insert", Type: "keyword", Pattern: "INSERT INTO", Action: "alert", Priority: 50},
			{Name: "sql_update", Type: "keyword", Pattern: "UPDATE .* SET", Action: "block", Priority: 100, compiled: regexp.MustCompile("UPDATE .* SET")},
			{Name: "sql_alter", Type: "keyword", Pattern: "ALTER TABLE", Action: "block", Priority: 100},
			{Name: "xss_script", Type: "keyword", Pattern: "<script", Action: "block", Priority: 100},
			// SPEC-068: input-side PII sanitize rules — fallback when the
			// pii-redaction service is off or errors (输入侧降级兜底).
			{Name: "id_card", Type: "regex", Pattern: `\d{17}[\dXx]`, Action: "sanitize", Priority: 90},
			{Name: "phone", Type: "regex", Pattern: `1[3-9]\d{9}`, Action: "sanitize", Priority: 80},
			{Name: "api_key", Type: "regex", Pattern: `sk-[a-zA-Z0-9]{32,}`, Action: "sanitize", Priority: 90},
		},
		OutputRules: []Rule{
			{Name: "id_card", Type: "regex", Pattern: `\d{17}[\dXx]`, Action: "sanitize", Priority: 90},
			{Name: "phone", Type: "regex", Pattern: `1[3-9]\d{9}`, Action: "sanitize", Priority: 80},
			{Name: "api_key", Type: "regex", Pattern: `sk-[a-zA-Z0-9]{32,}`, Action: "sanitize", Priority: 90},
			// SPEC-068: output-side XSS check (sanitize). SQL is NOT checked on
			// output — SQL injection risk is input-side only.
			{Name: "xss", Type: "regex", Pattern: `(?i)<\s*script`, Action: "sanitize", Priority: 100},
		},
	}
}

// Compile compiles regex patterns in the rules.
func (c *Config) Compile() {
	for i := range c.InputRules {
		if c.InputRules[i].Type == "regex" && c.InputRules[i].compiled == nil {
			c.InputRules[i].compiled = regexp.MustCompile(c.InputRules[i].Pattern)
		}
	}
	for i := range c.OutputRules {
		if c.OutputRules[i].Type == "regex" && c.OutputRules[i].compiled == nil {
			c.OutputRules[i].compiled = regexp.MustCompile(c.OutputRules[i].Pattern)
		}
	}
}

// AuditInput validates input content against security rules and returns the
// PII-redacted input. Non-privacy rules (SQL/XSS block/alert) run first and
// may return an error; then PII redaction is applied (pii-redaction service
// first, falling back to regex sanitize rules).
func (a *Auditor) AuditInput(input string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, rule := range a.config.InputRules {
		matched, match := a.matchRule(rule, input)
		if !matched {
			continue
		}
		switch rule.Action {
		case "block":
			a.logAlert("error", "input_blocked", fmt.Sprintf("Input blocked by rule %q", rule.Name), map[string]interface{}{
				"rule":    rule.Name,
				"pattern": rule.Pattern,
				"match":   match,
			})
			return "", fmt.Errorf("input blocked by security rule: %s", rule.Name)
		case "alert":
			a.logAlert("warn", "input_alert", fmt.Sprintf("Input triggered alert rule %q", rule.Name), map[string]interface{}{
				"rule":  rule.Name,
				"match": match,
			})
		}
	}

	// PII redaction: pii-redaction service first, fall back to regex sanitize.
	if a.redactor != nil {
		if redacted, err := a.redactor.Redact(context.Background(), input); err == nil {
			return redacted, nil
		}
		// switch off / service error → fall through to regex sanitize rules
	}
	return sanitizeByRules(input, a.config.InputRules), nil
}

// AuditOutput sanitizes output content: pii-redaction service first (PII),
// falling back to regex sanitize rules (PII id_card/phone/api_key + XSS).
// SQL is intentionally NOT checked on output.
func (a *Auditor) AuditOutput(output string) (string, error) {
	a.mu.RLock()
	defer a.mu.RUnlock()

	if a.redactor != nil {
		if redacted, err := a.redactor.Redact(context.Background(), output); err == nil {
			return redacted, nil
		}
		// switch off / service error → fall through to regex sanitize rules
	}
	return sanitizeByRules(output, a.config.OutputRules), nil
}

// AuditToolCall validates a tool/skill call.
func (a *Auditor) AuditToolCall(toolName string, params map[string]any) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	// Block write operations to sensitive paths
	if toolName == "workspace_exec" || toolName == "workspace_write" {
		if path, ok := params["path"].(string); ok {
			sensitivePaths := []string{"/etc/", "/proc/", "/sys/", "/root/", "/var/", "/tmp/"}
			for _, sp := range sensitivePaths {
				if strings.HasPrefix(path, sp) {
					a.logAlert("error", "sensitive_path", fmt.Sprintf("Tool %q blocked from accessing %s", toolName, path), nil)
					return fmt.Errorf("access to sensitive path %q blocked", sp)
				}
			}
		}
	}
	return nil
}

// UpdateRules hot-reloads security rules.
func (a *Auditor) UpdateRules(config *Config) {
	config.Compile()
	a.mu.Lock()
	defer a.mu.Unlock()
	a.config = config
}

func (a *Auditor) matchRule(rule Rule, input string) (bool, string) {
	pattern := rule.Pattern
	switch rule.Type {
	case "keyword":
		upperInput := strings.ToUpper(input)
		upperPattern := strings.ToUpper(pattern)
		if strings.Contains(upperInput, upperPattern) {
			return true, pattern
		}
	case "regex":
		if rule.compiled == nil {
			compiled, err := regexp.Compile(pattern)
			if err != nil {
				return false, ""
			}
			rule.compiled = compiled
		}
		if loc := rule.compiled.FindStringIndex(input); loc != nil {
			return true, input[loc[0]:loc[1]]
		}
	}
	return false, ""
}

func (a *Auditor) logAlert(level, category, message string, details map[string]interface{}) {
	if a.alerts != nil {
		a.alerts.LogAlert(level, category, message, details)
	}
}

// sanitizeByRules applies all sanitize-action rules in order, replacing each
// regex match via sanitizeByType. Shared by input and output fallback paths.
// Each rule is wrapped in a recover so a single buggy rule can't crash the
// audit path.
func sanitizeByRules(text string, rules []Rule) string {
	result := text
	for _, rule := range rules {
		if rule.Action != "sanitize" || rule.compiled == nil {
			continue
		}
		func() {
			defer func() {
				_ = recover() // one bad rule must not fail the whole audit
			}()
			result = rule.compiled.ReplaceAllStringFunc(result, func(s string) string {
				return sanitizeByType(rule.Name, s)
			})
		}()
	}
	return result
}

func sanitizeByType(ruleName, s string) string {
	switch ruleName {
	case "phone":
		if len(s) == 11 {
			return s[:3] + "****" + s[7:]
		}
	case "id_card":
		if len(s) == 18 {
			return s[:3] + "***********" + s[14:]
		}
	case "api_key":
		if len(s) > 8 {
			return s[:4] + "****"
		}
	case "xss":
		// Escape angle brackets so the content renders inert (no script execution).
		return strings.ReplaceAll(strings.ReplaceAll(s, "<", "&lt;"), ">", "&gt;")
	}
	return "***"
}
