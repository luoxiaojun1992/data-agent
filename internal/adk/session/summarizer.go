package adksession

import (
	"context"
	"fmt"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// LLMSummarizer implements Summarizer by asking an LLM to compress
// the transcript of old events into a dense summary.
type LLMSummarizer struct {
	llm model.LLM
	// MaxInputChars bounds the transcript sent to the LLM.
	MaxInputChars int
}

// NewLLMSummarizer creates a summarizer backed by the given model.
func NewLLMSummarizer(llm model.LLM) *LLMSummarizer {
	return &LLMSummarizer{llm: llm, MaxInputChars: 16000}
}

const summarizeInstruction = `You are a conversation compactor. Summarize the following conversation transcript into a dense, factual summary (max 500 words) preserving: user goals, key decisions, important data points, file/artifact names, and unresolved questions. Write in the same language as the transcript.`

// Summarize renders the events as a transcript and returns an LLM-generated summary.
// On LLM failure it falls back to a truncated extractive transcript so that
// compaction never loses the conversation entirely.
func (s *LLMSummarizer) Summarize(ctx context.Context, events []*session.Event) (string, error) {
	transcript := transcriptOf(events)
	if transcript == "" {
		return "(no content)", nil
	}
	if len(transcript) > s.MaxInputChars {
		transcript = transcript[:s.MaxInputChars]
	}

	if text := s.callLLM(ctx, transcript); text != "" {
		return text, nil
	}
	return fallbackSummary(transcript), nil
}

// callLLM requests the summary from the model, returning "" on any failure.
func (s *LLMSummarizer) callLLM(ctx context.Context, transcript string) string {
	req := &model.LLMRequest{
		Model: s.llm.Name(),
		Contents: []*genai.Content{
			genai.NewContentFromText(transcript, "user"),
		},
		Config: &genai.GenerateContentConfig{
			SystemInstruction: genai.NewContentFromText(summarizeInstruction, "system"),
		},
	}

	for resp, err := range s.llm.GenerateContent(ctx, req, false) {
		if err != nil {
			return ""
		}
		if text := responseText(resp); text != "" {
			return text
		}
	}
	return ""
}

// responseText extracts trimmed text from an LLM response.
func responseText(resp *model.LLMResponse) string {
	if resp == nil || resp.Content == nil {
		return ""
	}
	var sb strings.Builder
	for _, p := range resp.Content.Parts {
		if p != nil {
			sb.WriteString(p.Text)
		}
	}
	return strings.TrimSpace(sb.String())
}

// fallbackSummary keeps the head and tail of a long transcript.
func fallbackSummary(transcript string) string {
	const head = 2000
	const tail = 1000
	if len(transcript) <= head+tail {
		return transcript
	}
	return transcript[:head] + "\n...[truncated]...\n" + transcript[len(transcript)-tail:]
}

// transcriptOf renders events as "author: text" lines for summarization input.
func transcriptOf(events []*session.Event) string {
	var sb strings.Builder
	for _, e := range events {
		if line := eventLine(e); line != "" {
			sb.WriteString(line)
			sb.WriteString("\n")
		}
	}
	return sb.String()
}

// eventLine renders one event as an "author: text" line ("" when skippable).
func eventLine(e *session.Event) string {
	if e == nil || e.Content == nil {
		return ""
	}
	var text strings.Builder
	for _, p := range e.Content.Parts {
		if p == nil {
			continue
		}
		if p.Text != "" {
			text.WriteString(p.Text)
		}
		if p.FunctionCall != nil {
			fmt.Fprintf(&text, "[tool call: %s]", p.FunctionCall.Name)
		}
	}
	line := strings.TrimSpace(text.String())
	if line == "" {
		return ""
	}
	return eventAuthor(e) + ": " + line
}

// eventAuthor resolves the display author of an event.
func eventAuthor(e *session.Event) string {
	if e.Author != "" {
		return e.Author
	}
	return e.Content.Role
}
