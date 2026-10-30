package guard

import (
	"context"
	"errors"
	"iter"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// fakeLLM is a minimal model.LLM whose GenerateContent yields a fixed sequence
// of (response, error) pairs, enough to exercise generateText's loop without a
// real model backend.
type fakeLLM struct {
	name string
	seq  func(yield func(*model.LLMResponse, error) bool)
}

func (f *fakeLLM) Name() string { return f.name }

func (f *fakeLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return f.seq
}

func textResp(text string) *model.LLMResponse {
	return &model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{{Text: text}}}}
}

func TestGenerateText_Error(t *testing.T) {
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, errors.New("llm boom"))
	}}
	got, err := generateText(context.Background(), llm, &model.LLMRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if got != "" {
		t.Errorf("got %q, want empty on error", got)
	}
}

func TestGenerateText_FirstNonEmptyContent(t *testing.T) {
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		if !yield(textResp("   "), nil) { // whitespace only → skipped
			return
		}
		yield(textResp("hello"), nil)
	}}
	got, err := generateText(context.Background(), llm, &model.LLMRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello" {
		t.Errorf("got %q, want hello", got)
	}
}

func TestGenerateText_SkipsNilAndEmptyContent(t *testing.T) {
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, nil)                        // nil response
		yield(&model.LLMResponse{}, nil)       // nil content
		yield(textResp(""), nil)               // empty text
	}}
	got, err := generateText(context.Background(), llm, &model.LLMRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "" {
		t.Errorf("got %q, want empty", got)
	}
}

func TestGenerateText_ConcatenatesParts(t *testing.T) {
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: &genai.Content{Parts: []*genai.Part{
			{Text: "a"}, {Text: "b"}, {Text: "c"},
		}}}, nil)
	}}
	got, err := generateText(context.Background(), llm, &model.LLMRequest{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "abc" {
		t.Errorf("got %q, want abc", got)
	}
}

func TestStripCodeFence(t *testing.T) {
	cases := []struct{ in, want string }{
		{"plain", "plain"},
		{"```json\n{\"a\":1}\n```", "{\"a\":1}"},
		{"```\n{\"a\":1}\n```", "{\"a\":1}"},
		{"  {\"a\":1}  ", "{\"a\":1}"},
	}
	for _, c := range cases {
		if got := stripCodeFence(c.in); got != c.want {
			t.Errorf("stripCodeFence(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}

func TestParseIsRelevant(t *testing.T) {
	if !parseIsRelevant(`{"is_relevant": true}`) {
		t.Error("parseIsRelevant(true) should be true")
	}
	if parseIsRelevant(`{"is_relevant": false}`) {
		t.Error("parseIsRelevant(false) should be false")
	}
	// malformed / empty → default relevant (never blocks the flow)
	for _, s := range []string{"", "not json", "```json\n{\"is_relevant\": true}\n``` broken"} {
		if !parseIsRelevant(s) {
			t.Errorf("parseIsRelevant(%q) should default to true", s)
		}
	}
	// code-fenced false still parses correctly
	if parseIsRelevant("```json\n{\"is_relevant\": false}\n```") {
		t.Error("code-fenced false should parse to false")
	}
}

func TestNewService_DefaultRetries(t *testing.T) {
	s := NewService(nil, nil, 0)
	if s.maxRetries != 2 {
		t.Errorf("maxRetries = %d, want 2", s.maxRetries)
	}
	s = NewService(nil, nil, 5)
	if s.maxRetries != 5 {
		t.Errorf("maxRetries = %d, want 5", s.maxRetries)
	}
}

func TestResolveMaxRetries(t *testing.T) {
	s := NewService(nil, nil, 3)
	if got := s.resolveMaxRetries(context.Background()); got != 3 {
		t.Errorf("no resolver: got %d, want 3", got)
	}
	s.SetMaxRetriesResolver(func(ctx context.Context) int { return 7 })
	if got := s.resolveMaxRetries(context.Background()); got != 7 {
		t.Errorf("resolver=7: got %d, want 7", got)
	}
	// resolver returns <= 0 → fall back to static default
	s.SetMaxRetriesResolver(func(ctx context.Context) int { return 0 })
	if got := s.resolveMaxRetries(context.Background()); got != 3 {
		t.Errorf("resolver=0: got %d, want fallback 3", got)
	}
}

func TestRelevanceKey(t *testing.T) {
	if got := relevanceKey("sess_1"); got != "guard:relevance:sess_1" {
		t.Errorf("relevanceKey = %q", got)
	}
}

func TestRecordAndShouldRetry_NilRedis(t *testing.T) {
	s := NewService(nil, nil, 2)
	ok, err := s.RecordAndShouldRetry(context.Background(), "sess_1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("nil redis should report no retry")
	}
}
