package adkmodel

import (
	"context"
	"iter"

	"google.golang.org/adk/model"

	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

// recordingLLM wraps an LLM to record actual token usage from the response's
// UsageMetadata (SPEC-072). It records exactly once per GenerateContent call,
// on the response that carries UsageMetadata (the final response — intermediate
// streaming chunks have nil UsageMetadata).
type recordingLLM struct {
	inner     model.LLM
	rec       *llmstats.Recorder
	callPoint string
}

// NewRecordingLLM wraps an LLM with token-usage recording. A nil rec returns
// the inner LLM unwrapped (no-op).
func NewRecordingLLM(inner model.LLM, rec *llmstats.Recorder, callPoint string) model.LLM {
	if rec == nil {
		return inner
	}
	return &recordingLLM{inner: inner, rec: rec, callPoint: callPoint}
}

func (r *recordingLLM) Name() string { return r.inner.Name() }

func (r *recordingLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		for resp, err := range r.inner.GenerateContent(ctx, req, stream) {
			if resp != nil && resp.UsageMetadata != nil {
				um := resp.UsageMetadata
				_ = r.rec.Record(ctx, llmstats.Record{
					CallPoint:        r.callPoint,
					PromptTokens:     int(um.PromptTokenCount),
					CompletionTokens: int(um.CandidatesTokenCount),
					Multiplier:       1.0,
				})
			}
			if !yield(resp, err) {
				return
			}
		}
	}
}
