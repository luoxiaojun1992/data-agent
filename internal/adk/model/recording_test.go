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

func newTestReq() *model.LLMRequest {
	return &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{{Text: "hello prompt"}}},
		},
	}
}

func TestNewRecordingLLM_RecordsUsage(t *testing.T) {
	fc := &fakeCounter{}
	rec := llmstats.NewRecorder(fc)
	inner := &fakeUsageLLM{responses: []*model.LLMResponse{
		{Content: genai.NewContentFromText("partial", "model")},
		{Content: genai.NewContentFromText("done", "model"), UsageMetadata: &genai.GenerateContentResponseUsageMetadata{
			PromptTokenCount:     100,
			CandidatesTokenCount: 50,
		}},
	}}
	wrapped := NewRecordingLLM(inner, rec, "chat")

	var got []*model.LLMResponse
	for resp, err := range wrapped.GenerateContent(context.Background(), newTestReq(), false) {
		if err != nil {
			t.Fatalf("unexpected err: %v", err)
		}
		got = append(got, resp)
	}
	if len(got) != 2 {
		t.Fatalf("response count = %d, want 2", len(got))
	}
	if fc.tokenDelta != 150 {
		t.Errorf("token delta = %d, want 150 (real usage)", fc.tokenDelta)
	}
	if fc.calls != 1 {
		t.Errorf("llm_calls = %d, want 1", fc.calls)
	}
}

func TestNewRecordingLLM_EstimatesWhenNoUsage(t *testing.T) {
	fc := &fakeCounter{}
	rec := llmstats.NewRecorder(fc)
	// Backend returns no usage (streaming without include_usage, or Ollama/mockllm).
	inner := &fakeUsageLLM{responses: []*model.LLMResponse{
		{Content: genai.NewContentFromText("done text", "model")},
	}}
	wrapped := NewRecordingLLM(inner, rec, "chat")

	for range wrapped.GenerateContent(context.Background(), newTestReq(), true) {
	}
	// Fallback estimation must still count the call exactly once.
	if fc.calls != 1 {
		t.Errorf("llm_calls = %d, want 1 (fallback estimate)", fc.calls)
	}
	if fc.tokenDelta <= 0 {
		t.Errorf("token delta = %d, want > 0 (fallback estimate)", fc.tokenDelta)
	}
}

func TestNewRecordingLLM_RecordsOnceWhenUsageInMiddle(t *testing.T) {
	fc := &fakeCounter{}
	rec := llmstats.NewRecorder(fc)
	// Usage appears on a middle chunk; only the first non-nil usage is recorded.
	inner := &fakeUsageLLM{responses: []*model.LLMResponse{
		{Content: genai.NewContentFromText("a", "model"), UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 10, CandidatesTokenCount: 5}},
		{Content: genai.NewContentFromText("b", "model"), UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 99, CandidatesTokenCount: 99}},
	}}
	wrapped := NewRecordingLLM(inner, rec, "chat")
	for range wrapped.GenerateContent(context.Background(), newTestReq(), true) {
	}
	if fc.calls != 1 {
		t.Errorf("llm_calls = %d, want 1 (dedup)", fc.calls)
	}
	if fc.tokenDelta != 15 {
		t.Errorf("token delta = %d, want 15 (first usage only)", fc.tokenDelta)
	}
}

func TestNewRecordingLLM_NilRecNoop(t *testing.T) {
	inner := &fakeUsageLLM{responses: []*model.LLMResponse{
		{Content: genai.NewContentFromText("done", "model"), UsageMetadata: &genai.GenerateContentResponseUsageMetadata{PromptTokenCount: 1}},
	}}
	wrapped := NewRecordingLLM(inner, nil, "chat")
	if wrapped != model.LLM(inner) {
		t.Fatal("nil recorder should return the inner LLM unwrapped")
	}
}
