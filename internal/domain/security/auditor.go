package security

import (
	"fmt"
	"log"
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
	mu     sync.RWMutex
	config *Config
	alerts AlertLogger
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
		},
		OutputRules: []Rule{
			{Name: "id_card", Type: "regex", Pattern: `\d{17}[\dXx]`, Action: "sanitize", Priority: 90},
			{Name: "phone", Type: "regex", Pattern: `1[3-9]\d{9}`, Action: "sanitize", Priority: 80},
			{Name: "api_key", Type: "regex", Pattern: `sk-[a-zA-Z0-9]{32,}`, Action: "sanitize", Priority: 90},
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

// AuditInput validates input content against security rules.
func (a *Auditor) AuditInput(input string) error {
	a.mu.RLock()
	defer a.mu.RUnlock()

	for _, rule := range a.config.InputRules {
		matched, match := a.matchRule(rule, input)
		if matched {
			switch rule.Action {
			case "block":
				a.logAlert("error", "input_blocked", fmt.Sprintf("Input blocked by rule %q", rule.Name), map[string]interface{}{
					"rule":    rule.Name,
					"pattern": rule.Pattern,
					"match":   match,
				})
				return fmt.Errorf("input blocked by security rule: %s", rule.Name)
			case "alert":
				a.logAlert("warn", "input_alert", fmt.Sprintf("Input triggered alert rule %q", rule.Name), map[string]interface{}{
					"rule":  rule.Name,
					"match": match,
				})
			}
		}
	}
	return nil
}

// AuditOutput sanitizes output content.
func (a *Auditor) AuditOutput(output string) (string, error) {
	log.Printf("[DEBUG security] AuditOutput: acquiring RLock, len=%d", len(output))
	a.mu.RLock()
	log.Printf("[DEBUG security] AuditOutput: RLock acquired, rules=%d", len(a.config.OutputRules))
	defer a.mu.RUnlock()

	result := output
	for i, rule := range a.config.OutputRules {
		log.Printf("[DEBUG security] AuditOutput: processing rule %d name=%q type=%q action=%q", i, rule.Name, rule.Type, rule.Action)
		matched, _ := a.matchRule(rule, result)
		log.Printf("[DEBUG security] AuditOutput: rule %d matched=%v", i, matched)
		if matched && rule.Action == "sanitize" {
			matches := rule.compiled.FindAllString(result, -1)
			for _, m := range matches {
				// Skip if this match is inside a longer already-masked sequence
				result = strings.Replace(result, m, sanitizeByType(rule.Name, m), 1)
			}
			log.Printf("[DEBUG security] AuditOutput: rule %d sanitized (%d matches)", i, len(matches))
		}
	}
	log.Printf("[DEBUG security] AuditOutput: done, len=%d", len(result))
	return result, nil
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
	}
	return "***"
}
