package adkmodel

import (
	"context"
	"encoding/json"
	"iter"
	"log"

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
	if p == nil {
		return
	}
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
	// Log request parts for debugging.
	for _, c := range req.Contents {
		for _, p := range c.Parts {
			if p.FunctionCall != nil {
				b, _ := json.Marshal(p.FunctionCall.Args)
				log.Printf("[DEBUG compat] req FunctionCall: name=%s args=%s", p.FunctionCall.Name, string(b))
			}
			if p.FunctionResponse != nil {
				respJSON, _ := json.Marshal(p.FunctionResponse.Response)
				log.Printf("[DEBUG compat] req FunctionResponse: id=%s name=%s response=%s",
					p.FunctionResponse.ID, p.FunctionResponse.Name, string(respJSON))
			}
			EnsureResponseParts(p)
		}
	}
	seq := m.inner.GenerateContent(ctx, req, stream)
	// Wrap sequence to log response parts too.
	return func(yield func(*model.LLMResponse, error) bool) {
		for resp, err := range seq {
			if resp != nil && resp.Content != nil {
				for _, p := range resp.Content.Parts {
					if p.FunctionCall != nil {
						b, _ := json.Marshal(p.FunctionCall.Args)
						log.Printf("[DEBUG compat] resp FunctionCall: name=%s args=%s", p.FunctionCall.Name, string(b))
					}
				}
			}
			if !yield(resp, err) {
				return
			}
		}
	}
}
