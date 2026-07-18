package adksession

import (
	"context"
	"fmt"
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// scriptedLLM returns queued responses/errors in order.
type scriptedLLM struct {
	name string
	text string
	err  error
}

func (s *scriptedLLM) Name() string { return s.name }

func (s *scriptedLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if s.err != nil {
			yield(nil, s.err)
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(s.text, "model")}, nil)
	}
}

func TestNewLLMSummarizer(t *testing.T) {
	s := NewLLMSummarizer(&scriptedLLM{name: "m"})
	if s.llm == nil || s.MaxInputChars != 16000 {
		t.Errorf("summarizer not initialized: %+v", s)
	}
}

func TestSummarize_Success(t *testing.T) {
	s := NewLLMSummarizer(&scriptedLLM{name: "m", text: "用户讨论了营收数据"})
	events := []*session.Event{
		textEvent("user", "分析一下上季度营收"),
		textEvent("model", "营收增长 20%"),
	}
	summary, err := s.Summarize(context.Background(), events)
	if err != nil {
		t.Fatalf("Summarize failed: %v", err)
	}
	if summary != "用户讨论了营收数据" {
		t.Errorf("summary = %q", summary)
	}
}

func TestSummarize_LLMFailure_Fallback(t *testing.T) {
	s := NewLLMSummarizer(&scriptedLLM{name: "m", err: fmt.Errorf("llm down")})
	events := []*session.Event{textEvent("user", "hello")}
	summary, err := s.Summarize(context.Background(), events)
	if err != nil {
		t.Fatalf("fallback should not error: %v", err)
	}
	if !strings.Contains(summary, "hello") {
		t.Errorf("fallback summary should contain transcript: %q", summary)
	}
}

func TestSummarize_EmptyEvents(t *testing.T) {
	s := NewLLMSummarizer(&scriptedLLM{name: "m"})
	summary, err := s.Summarize(context.Background(), nil)
	if err != nil || summary != "(no content)" {
		t.Errorf("empty events = %q, %v", summary, err)
	}
}

func TestSummarize_EmptyLLMResponse(t *testing.T) {
	s := NewLLMSummarizer(&scriptedLLM{name: "m", text: ""})
	events := []*session.Event{textEvent("user", "hi")}
	summary, err := s.Summarize(context.Background(), events)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(summary, "hi") {
		t.Errorf("empty LLM response should fall back to transcript: %q", summary)
	}
}

func TestSummarize_TruncatesLongTranscript(t *testing.T) {
	var captured string
	llm := &captureLLM{text: "summary", captured: &captured}
	_ = llm
	s := NewLLMSummarizer(llm)
	s.MaxInputChars = 100

	long := strings.Repeat("x", 500)
	events := []*session.Event{textEvent("user", long)}
	if _, err := s.Summarize(context.Background(), events); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// The LLM should receive at most MaxInputChars of transcript.
	if len(captured) > 200 {
		t.Errorf("transcript not truncated, len=%d", len(captured))
	}
}

type captureLLM struct {
	text     string
	captured *string
}

func (s *captureLLM) Name() string { return "capture" }

func (s *captureLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		var sb strings.Builder
		for _, c := range req.Contents {
			for _, p := range c.Parts {
				sb.WriteString(p.Text)
			}
		}
		*s.captured = sb.String()
		yield(&model.LLMResponse{Content: genai.NewContentFromText(s.text, "model")}, nil)
	}
}

func TestFallbackSummary(t *testing.T) {
	short := "short text"
	if got := fallbackSummary(short); got != short {
		t.Errorf("short text unchanged: %q", got)
	}
	long := strings.Repeat("a", 5000)
	got := fallbackSummary(long)
	if len(got) >= len(long) {
		t.Errorf("long text should be truncated, len=%d", len(got))
	}
	if !strings.Contains(got, "[truncated]") {
		t.Errorf("should contain truncation marker")
	}
}

func TestTranscriptOf(t *testing.T) {
	events := []*session.Event{
		nil,
		{LLMResponse: model.LLMResponse{}}, // nil content
		textEvent("", "no author"),         // falls back to content role
		{
			Author: "agent",
			LLMResponse: model.LLMResponse{Content: &genai.Content{
				Role: "model",
				Parts: []*genai.Part{
					nil,
					{FunctionCall: &genai.FunctionCall{Name: "sql_validate"}},
				},
			}},
		},
	}
	out := transcriptOf(events)
	if !strings.Contains(out, "model: no author") {
		t.Errorf("missing role fallback: %q", out)
	}
	if !strings.Contains(out, "[tool call: sql_validate]") {
		t.Errorf("missing tool call marker: %q", out)
	}
}
