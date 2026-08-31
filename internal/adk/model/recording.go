package adkmodel

import (
	"context"
	"iter"
	"strings"

	"google.golang.org/adk/model"

	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

// recordingLLM wraps an LLM to record token usage (SPEC-072). It prefers the
// real usage from the response's UsageMetadata; when the backend does not
// return usage (streaming without stream_options.include_usage, or non-OpenAI
// providers like Ollama/mockllm that omit the usage field), it falls back to
// estimating from the request/response text so no call is left unrecorded.
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

// GenerateContent records usage exactly once per call: real usage when the
// backend populates UsageMetadata, otherwise an estimate built from the
// request prompt and the accumulated response text.
func (r *recordingLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	estPrompt := estimatePromptTokens(req)
	return func(yield func(*model.LLMResponse, error) bool) {
		var completion strings.Builder
		recorded := false
		for resp, err := range r.inner.GenerateContent(ctx, req, stream) {
			if resp != nil {
				if um := resp.UsageMetadata; um != nil && !recorded {
					_ = r.rec.Record(ctx, llmstats.Record{
						CallPoint:        r.callPoint,
						PromptTokens:     int(um.PromptTokenCount),
						CompletionTokens: int(um.CandidatesTokenCount),
						Multiplier:       1.0,
					})
					recorded = true
				}
				if resp.Content != nil {
					for _, p := range resp.Content.Parts {
						if p != nil && p.Text != "" {
							completion.WriteString(p.Text)
						}
					}
				}
			}
			if !yield(resp, err) {
				return
			}
		}
		// Fallback: the backend never reported usage → estimate so the call is
		// still counted (token_tokens + llm_calls).
		if !recorded {
			_ = r.rec.Record(ctx, llmstats.Record{
				CallPoint:        r.callPoint,
				PromptTokens:     estPrompt,
				CompletionTokens: llmstats.EstimateTokens(completion.String()),
				Multiplier:       1.0,
				Estimated:        true,
			})
		}
	}
}

// estimatePromptTokens estimates the prompt token count from the request
// contents' text parts (same 4-char≈1-token heuristic as llmstats.EstimateTokens).
func estimatePromptTokens(req *model.LLMRequest) int {
	if req == nil {
		return 0
	}
	n := 0
	for _, c := range req.Contents {
		if c == nil {
			continue
		}
		for _, p := range c.Parts {
			if p == nil {
				continue
			}
			n += llmstats.EstimateTokens(p.Text)
		}
	}
	return n
}
