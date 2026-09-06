package guard

import (
	"iter"
	"strings"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// eventsList is a minimal session.Events backed by a slice, for testing
// LastRelevanceBase without pulling in the adksession package.
type eventsList []*session.Event

func (e eventsList) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, ev := range e {
			if !yield(ev) {
				return
			}
		}
	}
}
func (e eventsList) Len() int                { return len(e) }
func (e eventsList) At(i int) *session.Event { return e[i] }

func userEv(text string) *session.Event {
	return &session.Event{
		Author: "user",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: text}}},
		},
	}
}

func toolEv(result map[string]any) *session.Event {
	return &session.Event{
		Author: "data_agent",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "user", Parts: []*genai.Part{
				{FunctionResponse: &genai.FunctionResponse{ID: "c1", Name: "sql_executor", Response: result}},
			}},
		},
	}
}

func asstEv(text string) *session.Event {
	return &session.Event{
		Author: "data_agent",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: text}}},
		},
	}
}

func systemEv(text string) *session.Event {
	return &session.Event{
		Author: "system",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "system", Parts: []*genai.Part{{Text: text}}},
		},
	}
}

func compactionEv(text string) *session.Event {
	return &session.Event{
		Author: "compaction",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "system", Parts: []*genai.Part{{Text: text}}},
		},
	}
}

func TestLastRelevanceBase_EmptyEvents(t *testing.T) {
	if got := LastRelevanceBase(eventsList{}); got != "" {
		t.Errorf("empty events base = %q, want \"\"", got)
	}
}

func TestLastRelevanceBase_LastUserMessage(t *testing.T) {
	events := eventsList{
		userEv("first question"),
		asstEv("first answer"),
		userEv("second question"),
		asstEv("final answer"),
	}
	if got := LastRelevanceBase(events); got != "second question" {
		t.Errorf("base = %q, want %q", got, "second question")
	}
}

func TestLastRelevanceBase_ToolOutputOverridesEarlierUser(t *testing.T) {
	// tool output is more recent than the user message → base should be the
	// JSON of the tool result, not the user text.
	events := eventsList{
		userEv("帮我查一下订单"),
		asstEv("call tool"),
		toolEv(map[string]any{"result": "42 rows"}),
		asstEv("final answer"),
	}
	got := LastRelevanceBase(events)
	if got == "" || got == "帮我查一下订单" {
		t.Errorf("base = %q, want the tool result JSON", got)
	}
	if !strings.Contains(got, "42 rows") {
		t.Errorf("base = %q, want to contain tool result", got)
	}
}

func TestLastRelevanceBase_SkipsSystemAndCompaction(t *testing.T) {
	events := eventsList{
		userEv("question"),
		compactionEv("[conversation summary] old stuff"),
		systemEv("[intent] is_task=true"),
		asstEv("final answer"),
	}
	if got := LastRelevanceBase(events); got != "question" {
		t.Errorf("base = %q, want %q (skip compaction/system)", got, "question")
	}
}

func TestLastRelevanceBase_ImageOnlyUserReturnsEmpty(t *testing.T) {
	// user event with only an image part (no text) → skip, return "".
	events := eventsList{
		{
			Author: "user",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{Role: "user", Parts: []*genai.Part{
					{InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte("x")}},
				}},
			},
		},
		asstEv("final answer"),
	}
	if got := LastRelevanceBase(events); got != "" {
		t.Errorf("image-only base = %q, want \"\"", got)
	}
}

func TestLastRelevanceBase_NilEventsAndNilContent(t *testing.T) {
	events := eventsList{nil, {Author: "user"}, asstEv("answer")}
	if got := LastRelevanceBase(events); got != "" {
		t.Errorf("nil-event base = %q, want \"\"", got)
	}
}
