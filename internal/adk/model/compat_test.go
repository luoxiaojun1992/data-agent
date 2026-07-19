package adkmodel

import (
	"context"
	"iter"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

type mockLLM struct{ name string }

func (m *mockLLM) Name() string { return m.name }
func (m *mockLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {}
}

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
	inner := &mockLLM{name: "test-model"}
	w := NewCompatLLM(inner)
	if w.Name() != "test-model" {
		t.Errorf("expected test-model, got %s", w.Name())
	}
	// GenerateContent should not panic
	for _, err := range w.GenerateContent(context.Background(), &model.LLMRequest{}, false) {
		if err != nil {
			t.Errorf("unexpected error: %v", err)
		}
	}
}
