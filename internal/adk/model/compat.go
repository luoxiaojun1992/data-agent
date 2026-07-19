package adkmodel

import (
	"context"
	"encoding/json"
	"iter"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// compatLLM wraps a model.LLM and converts FunctionResponse.Response → Parts.
// ADK runner stores tool results as Response (map[string]any), but adk-go-pkg
// expects Parts ([]*Part). This marshals Response to JSON text Part.
type compatLLM struct {
	inner model.LLM
}

// NewCompatLLM wraps an LLM to bridge ADK's Response format to Parts format.
func NewCompatLLM(inner model.LLM) model.LLM {
	return &compatLLM{inner: inner}
}

// EnsureResponseParts wraps a genai.Part's FunctionResponse to ensure Parts is populated.
func EnsureResponseParts(p *genai.Part) {
	fr := p.FunctionResponse
	if fr == nil || len(fr.Parts) > 0 || fr.Response == nil {
		return
	}
	b, err := json.Marshal(fr.Response)
	if err != nil {
		return
	}
	fr.Parts = []*genai.FunctionResponsePart{{
		InlineData: &genai.FunctionResponseBlob{
			MIMEType: "application/json",
			Data:     b,
		},
	}}
}

func (m *compatLLM) Name() string { return m.inner.Name() }

func (m *compatLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	// Convert FunctionResponse.Response → Parts for all contents in the request.
	for _, c := range req.Contents {
		for _, p := range c.Parts {
			EnsureResponseParts(p)
		}
	}
	return m.inner.GenerateContent(ctx, req, stream)
}
