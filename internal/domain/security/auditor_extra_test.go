package security

import (
	"context"
	"errors"
	"strings"
	"testing"
)

type fakeRedactor struct {
	redactFn func(ctx context.Context, text string) (string, error)
}

func (f *fakeRedactor) Redact(ctx context.Context, text string) (string, error) {
	if f.redactFn != nil {
		return f.redactFn(ctx, text)
	}
	return text, nil
}

func TestAuditor_SetRedactor(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	r := &fakeRedactor{}
	a.SetRedactor(r)
	if a.redactor != r {
		t.Error("SetRedactor did not attach the redactor")
	}
}

func TestAuditor_AuditInput_WithRedactor(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	a.SetRedactor(&fakeRedactor{redactFn: func(_ context.Context, text string) (string, error) {
		return "REDACTED", nil
	}})

	out, err := a.AuditInput("SELECT * FROM users")
	if err != nil {
		t.Fatalf("AuditInput error = %v", err)
	}
	if out != "REDACTED" {
		t.Errorf("out = %q, want REDACTED (redactor service takes precedence)", out)
	}
}

func TestAuditor_AuditInput_RedactorErrorFallback(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	a.SetRedactor(&fakeRedactor{redactFn: func(_ context.Context, text string) (string, error) {
		return "", errors.New("service down")
	}})

	// Redactor fails → fall back to regex sanitize (phone rule).
	out, err := a.AuditInput("call 13812345678 now")
	if err != nil {
		t.Fatalf("AuditInput error = %v", err)
	}
	if !strings.Contains(out, "138****5678") {
		t.Errorf("fallback should sanitize phone, got %q", out)
	}
	if strings.Contains(out, "13812345678") {
		t.Errorf("raw phone should be redacted, got %q", out)
	}
}

func TestAuditor_AuditOutput_WithRedactor(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	a.SetRedactor(&fakeRedactor{redactFn: func(_ context.Context, text string) (string, error) {
		return "REDACTED", nil
	}})

	out, err := a.AuditOutput("call 13812345678")
	if err != nil {
		t.Fatalf("AuditOutput error = %v", err)
	}
	if out != "REDACTED" {
		t.Errorf("out = %q, want REDACTED", out)
	}
}

func TestAuditor_AuditOutput_RedactorErrorFallback(t *testing.T) {
	a := NewAuditor(&mockAlertLogger{})
	a.SetRedactor(&fakeRedactor{redactFn: func(_ context.Context, text string) (string, error) {
		return "", errors.New("service down")
	}})

	out, err := a.AuditOutput("call 13812345678")
	if err != nil {
		t.Fatalf("AuditOutput error = %v", err)
	}
	if !strings.Contains(out, "138****5678") {
		t.Errorf("fallback should sanitize phone, got %q", out)
	}
}

func TestSanitizeByType_XSS(t *testing.T) {
	if got := sanitizeByType("xss", "<script>alert(1)</script>"); got != "&lt;script&gt;alert(1)&lt;/script&gt;" {
		t.Errorf("sanitizeByType(xss) = %q, want escaped", got)
	}
}
