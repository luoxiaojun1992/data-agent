package modelcfg

import (
	"context"
	"iter"

	"google.golang.org/adk/model"
)

// TextAuditor is the minimal audit surface the model layer needs. It redacts
// text on input (block SQL/XSS + PII) and on output (XSS + PII). Tool-call
// auditing is intentionally NOT part of this interface — the internal LLM
// calls (compaction/enhance/intent/relevance/kb) never expose tools, so there
// is nothing to audit there. Keeping the surface minimal avoids side effects
// and unnecessary functionality.
type TextAuditor interface {
	AuditInput(input string) (string, error)
	AuditOutput(output string) (string, error)
}

// AuditedLLM wraps a model.LLM and applies text redaction on input and output.
// Only text parts are redacted; image parts (InlineData / FileData — e.g. image
// url or base64) pass through untouched, per SPEC-068.
type AuditedLLM struct {
	inner   model.LLM
	auditor TextAuditor
}

// NewAuditedLLM wraps inner with input/output text auditing. A nil auditor
// makes the wrapper a no-op passthrough (callers may still call it safely).
func NewAuditedLLM(inner model.LLM, auditor TextAuditor) *AuditedLLM {
	return &AuditedLLM{inner: inner, auditor: auditor}
}

// Compile-time assertion: AuditedLLM satisfies model.LLM.
var _ model.LLM = (*AuditedLLM)(nil)

// Name returns the inner model name.
func (a *AuditedLLM) Name() string {
	if a.inner == nil {
		return ""
	}
	return a.inner.Name()
}

// GenerateContent redacts text parts of the request, forwards to the inner
// LLM, then redacts text parts of each response. Input blocking (SQL/XSS)
// aborts the call with an error; output redaction is best-effort and never
// fails the call.
func (a *AuditedLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if err := a.redactRequest(req); err != nil {
		return func(yield func(*model.LLMResponse, error) bool) {
			yield(nil, err)
		}
	}
	return func(yield func(*model.LLMResponse, error) bool) {
		for resp, err := range a.inner.GenerateContent(ctx, req, stream) {
			if err == nil && resp != nil {
				a.redactResponse(resp)
			}
			if !yield(resp, err) {
				return
			}
		}
	}
}

// redactRequest redacts text parts of the request in place. Non-text parts
// (image/FileData) are skipped. Returns a block error so the caller aborts the
// LLM call instead of sending blocked content.
func (a *AuditedLLM) redactRequest(req *model.LLMRequest) error {
	if a.auditor == nil || req == nil {
		return nil
	}
	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		for _, p := range c.Parts {
			if p == nil || p.Text == "" {
				continue // image url / base64 (InlineData/FileData) not audited
			}
			redacted, err := a.auditor.AuditInput(p.Text)
			if err != nil {
				return err
			}
			p.Text = redacted
		}
	}
	return nil
}

// redactResponse redacts text parts of the response in place. Best-effort: on
// error keep the original text (never fail the call). Non-text parts skipped.
func (a *AuditedLLM) redactResponse(resp *model.LLMResponse) {
	if a.auditor == nil || resp == nil || resp.Content == nil {
		return
	}
	for _, p := range resp.Content.Parts {
		if p == nil || p.Text == "" {
			continue
		}
		if redacted, err := a.auditor.AuditOutput(p.Text); err == nil {
			p.Text = redacted
		}
	}
}
