package adkmodel

import (
	"context"
	"iter"
	"testing"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
	"github.com/luoxiaojun1992/data-agent/internal/infra/metrics"
)

// fakeUsageLLM yields a fixed sequence of responses (last one carries usage).
type fakeUsageLLM struct {
	responses []*model.LLMResponse
}

func (f *fakeUsageLLM) Name() string { return "fake-usage" }

func (f *fakeUsageLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		for _, r := range f.responses {
			if !yield(r, nil) {
				return
			}
		}
	}
}

// fakeCounter captures Incr calls for token_tokens / llm_calls.
type fakeCounter struct {
	tokenDelta int64
	calls      int64
}

func (f *fakeCounter) Incr(_ context.Context, m metrics.Metric, _ time.Time, delta int64) error {
	switch m {
	case metrics.MetricTokenTokens:
		f.tokenDelta += delta
	case metrics.MetricLLMCalls:
		f.calls += delta
	}
	return nil
}

func (f *fakeCounter) Stop() {}

func TestNewRecordingLLM_RecordsUsage(t *testing.T) {
	fc := &fakeCounter{}
	rec := llmstats.NewRecorder(fc)
	inner := &fakeUsageLLM{responses: []*model.LLMResponse{
		// Intermediate chunk without usage.
		{Content: genai.NewContentFromText("partial", "model")},
		// Final response carrying usage.
		{Content: genai.NewContentFromText("done", "model"), UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     100,
			CandidatesTokenCount: 50,
		}},
	}}
	wrapped := NewRecordingLLM(inner, rec, "chat")

	var got []*model.LLMResponse
	for resp, err := range wrapped.GenerateContent(context.Background(), nil, false) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		got = append(got, resp)
	}
	if len(got) != 2 {
		t.Fatalf("response count = %d, want 2", len(got))
	}
	// Billed = prompt(100) + completion(50) = 150; llm_calls = 1.
	if fc.tokenDelta != 150 {
		t.Errorf("token delta = %d, want 150", fc.tokenDelta)
	}
	if fc.calls != 1 {
		t.Errorf("llm_calls = %d, want 1", fc.calls)
	}
}

func TestNewRecordingLLM_NilRecNoop(t *testing.T) {
	inner := &fakeUsageLLM{responses: []*model.LLMResponse{
		{Content: genai.NewContentFromText("done", "model"), UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 1}},
	}}
	wrapped := NewRecordingLLM(inner, nil, "chat")
	// Nil recorder → returns inner unwrapped (same instance).
	if wrapped != model.LLM(inner) {
		t.Fatal("nil recorder should return the inner LLM unwrapped")
	}
}

func TestNewRecordingLLM_SkipsNilUsage(t *testing.T) {
	fc := &fakeCounter{}
	rec := llmstats.NewRecorder(fc)
	inner := &fakeUsageLLM{responses: []*model.LLMResponse{
		{Content: genai.NewContentFromText("no usage", "model")},
	}}
	wrapped := NewRecordingLLM(inner, rec, "chat")
	for range wrapped.GenerateContent(context.Background(), nil, false) {
	}
	if fc.calls != 0 {
		t.Errorf("no usage metadata should not record, got llm_calls=%d", fc.calls)
	}
}
