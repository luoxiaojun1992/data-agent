package adkmodel

import (
	"testing"

	"google.golang.org/genai"
)

func TestEnsureResponseParts(t *testing.T) {
	t.Run("nil", func(t *testing.T) {
		EnsureResponseParts(nil)
	})

	t.Run("nil_FunctionResponse", func(t *testing.T) {
		EnsureResponseParts(&genai.Part{})
	})

	t.Run("nil_Response", func(t *testing.T) {
		p := &genai.Part{FunctionResponse: &genai.FunctionResponse{
			ID: "call_1", Name: "test_fn",
		}}
		EnsureResponseParts(p)
		if len(p.FunctionResponse.Parts) != 0 {
			t.Error("expected no parts for nil Response")
		}
	})

	t.Run("already_has_parts", func(t *testing.T) {
		p := &genai.Part{FunctionResponse: &genai.FunctionResponse{
			Parts: []*genai.FunctionResponsePart{{}},
		}}
		EnsureResponseParts(p)
		if len(p.FunctionResponse.Parts) != 1 {
			t.Errorf("expected 1 part, got %d", len(p.FunctionResponse.Parts))
		}
	})

	t.Run("converts_Response", func(t *testing.T) {
		p := &genai.Part{FunctionResponse: &genai.FunctionResponse{
			ID:       "call_1",
			Name:     "test_fn",
			Response: map[string]any{"key": "value"},
		}}
		EnsureResponseParts(p)
		if len(p.FunctionResponse.Parts) != 1 {
			t.Fatalf("expected 1 part, got %d", len(p.FunctionResponse.Parts))
		}
		part := p.FunctionResponse.Parts[0]
		if part.InlineData == nil {
			t.Fatal("expected InlineData")
		}
		if part.InlineData.MIMEType != "application/json" {
			t.Errorf("expected application/json, got %s", part.InlineData.MIMEType)
		}
		if string(part.InlineData.Data) != `{"key":"value"}` {
			t.Errorf("unexpected data: %s", string(part.InlineData.Data))
		}
	})
}

func TestNewCompatLLM(t *testing.T) {
	_ = NewCompatLLM(nil) // Should not panic (inner=nil causes deferred panic on use)
}
