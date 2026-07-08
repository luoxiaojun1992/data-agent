package sql

import (
	"strings"
)

// ValidationResult represents the result of SQL validation.
type ValidationResult struct {
	Allowed bool   `json:"allowed"`
	Reason  string `json:"reason,omitempty"`
}

// Validate checks if a SQL statement is safe to execute.
// Only SELECT, DESCRIBE, SHOW, EXPLAIN are allowed.
func Validate(sql string, params []interface{}) *ValidationResult {
	// Check for parameterized queries — no string concatenation allowed
	if len(params) > 0 {
		// Parameterized queries are safe when params are separate
		_ = params
	}

	upper := strings.ToUpper(strings.TrimSpace(sql))

	// Block dangerous statements
	dangerous := []string{
		"DROP", "DELETE", "UPDATE", "INSERT", "ALTER",
		"CREATE", "TRUNCATE", "GRANT", "REVOKE",
	}
	for _, d := range dangerous {
		if strings.HasPrefix(upper, d) || strings.Contains(upper, " "+d+" ") {
			return &ValidationResult{Allowed: false, Reason: "DML/DDL statements are not allowed: " + d}
		}
	}

	// Only allow read operations
	allowedPrefixes := []string{"SELECT", "DESCRIBE", "SHOW", "EXPLAIN", "WITH"}
	isAllowed := false
	for _, prefix := range allowedPrefixes {
		if strings.HasPrefix(upper, prefix) {
			isAllowed = true
			break
		}
	}

	if !isAllowed {
		return &ValidationResult{Allowed: false, Reason: "Only SELECT/DESCRIBE/SHOW/EXPLAIN statements are allowed"}
	}

	// Block subqueries deeper than 2 levels
	if depth := countSubqueryDepth(upper); depth > 2 {
		return &ValidationResult{Allowed: false, Reason: "Subquery depth exceeds maximum (max: 2, actual: " + itoa(depth) + ")"}
	}

	// Block dangerous functions
	dangerousFuncs := []string{"SLEEP(", "BENCHMARK(", "LOAD_FILE(", "INTO OUTFILE", "INTO DUMPFILE"}
	for _, df := range dangerousFuncs {
		if strings.Contains(upper, df) {
			return &ValidationResult{Allowed: false, Reason: "Dangerous function detected: " + df}
		}
	}

	return &ValidationResult{Allowed: true}
}

// countSubqueryDepth counts nested SELECT depth.
func countSubqueryDepth(sql string) int {
	maxDepth := 0
	currentDepth := 0
	for i := 0; i < len(sql)-7; i++ {
		word := sql[i : i+7]
		if word == "(SELECT" {
			currentDepth++
			if currentDepth > maxDepth {
				maxDepth = currentDepth
			}
		}
		if i < len(sql)-1 && sql[i] == ')' {
			if currentDepth > 0 {
				currentDepth--
			}
		}
	}
	return maxDepth
}

// SafeFields filters field names to prevent SQL injection via column names.
func SafeFields(fields []string) []string {
	allowed := make([]string, 0, len(fields))
	for _, f := range fields {
		cleaned := sanitizeField(f)
		if cleaned != "" {
			allowed = append(allowed, cleaned)
		}
	}
	return allowed
}

// sanitizeField removes non-alphanumeric chars (except underscore) from field names.
func sanitizeField(field string) string {
	var b strings.Builder
	for _, r := range field {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

func itoa(n int) string {
	result := ""
	if n == 0 {
		return "0"
	}
	for n > 0 {
		result = string(rune('0'+n%10)) + result
		n /= 10
	}
	return result
}
