package security

import (
	"fmt"
	"regexp"
)

// xssPatterns are the XSS payload patterns blocked by ValidateXSS (SPEC-081
// §4.4, shared with SPEC-077 §4.4). They cover:
//   - <script (with case / whitespace variants)
//   - javascript: protocol
//   - on*= event handler attributes (onerror/onload/onclick/…)
//
// The check is block-only (no escaping, no content mutation): the fields it
// guards (chat user prompt, KB title) are plain-text labels/inputs that should
// not contain HTML at all. PII redaction is deliberately NOT performed here —
// that belongs to AuditInput/AuditOutput.
var xssPatterns = []*regexp.Regexp{
	regexp.MustCompile(`(?i)<\s*script`),     // <script / < script / <Script
	regexp.MustCompile(`(?i)javascript\s*:`), // javascript: / JavaScript :
	regexp.MustCompile(`(?i)\bon[a-z]+\s*=`), // onerror= / onload= / onclick= / …
}

// ValidateXSS reports whether s contains an XSS payload. It returns a
// descriptive error on the first match and nil when the input is clean. It is
// a single-purpose guard — it does NOT do PII redaction and does NOT mutate
// the input (unlike AuditInput/AuditOutput).
func ValidateXSS(s string) error {
	for _, re := range xssPatterns {
		if m := re.FindString(s); m != "" {
			return fmt.Errorf("input contains illegal content (xss): %q", m)
		}
	}
	return nil
}
