package adksession

import (
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

func textEvent(t *testing.T, text string) *session.Event {
	t.Helper()
	return &session.Event{
		Author: "assistant",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{Text: text}}},
		},
	}
}

func callEvent(t *testing.T, id, name, args string) *session.Event {
	t.Helper()
	return &session.Event{
		Author: "assistant",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{
				FunctionCall: &genai.FunctionCall{ID: id, Name: name, Args: map[string]any{"query": args}},
			}}},
		},
	}
}

func responseEvent(t *testing.T, id, result string) *session.Event {
	t.Helper()
	return &session.Event{
		Author: "tool",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{{
				FunctionResponse: &genai.FunctionResponse{ID: id, Response: map[string]any{"result": result}},
			}}},
		},
	}
}

// ---- 问题 1: estimateEventTokens 覆盖 tool 内容 ----

func TestEstimateEventTokens_TextOnly(t *testing.T) {
	ev := textEvent(t, "hello world 123456") // 18 chars
	got := estimateEventTokens([]*session.Event{ev})
	if got != 6 {
		t.Errorf("text-only estimate = %d, want 6", got)
	}
}

func TestEstimateEventTokens_WithToolContent(t *testing.T) {
	events := []*session.Event{
		callEvent(t, "c1", "sql_executor", "SELECT * FROM orders WHERE x=1"),
		responseEvent(t, "c1", "42 rows returned from database query result"),
		textEvent(t, "final answer"),
	}
	got := estimateEventTokens(events)
	textOnly := estimateEventTokens([]*session.Event{textEvent(t, "final answer")})
	if got <= textOnly {
		t.Errorf("tool-content estimate %d should exceed text-only %d", got, textOnly)
	}
	if got < 20 {
		t.Errorf("tool-content estimate %d suspiciously low for SQL+result payloads", got)
	}
}

func TestEstimateEventTokens_NilParts(t *testing.T) {
	ev := &session.Event{LLMResponse: model.LLMResponse{
		Content: &genai.Content{Parts: []*genai.Part{nil, {Text: "abc"}}},
	}}
	if got := estimateEventTokens([]*session.Event{ev}); got != 1 {
		t.Errorf("nil-part estimate = %d, want 1", got)
	}
}

// ---- 问题 2: latestDanglingCallIndex + adjustCutForDanglingCalls ----

func TestLatestDanglingCallIndex_NoCalls(t *testing.T) {
	events := []*session.Event{textEvent(t, "a"), textEvent(t, "b")}
	if got := latestDanglingCallIndex(events); got != -1 {
		t.Errorf("no-call index = %d, want -1", got)
	}
}

func TestLatestDanglingCallIndex_AllPaired(t *testing.T) {
	events := []*session.Event{
		callEvent(t, "c1", "search", "x"),
		callEvent(t, "c2", "sql", "y"),
		responseEvent(t, "c1", "r1"),
		responseEvent(t, "c2", "r2"),
	}
	if got := latestDanglingCallIndex(events); got != -1 {
		t.Errorf("all-paired index = %d, want -1", got)
	}
}

func TestLatestDanglingCallIndex_DanglingCall(t *testing.T) {
	events := []*session.Event{
		callEvent(t, "c1", "search", "x"),
		responseEvent(t, "c1", "r1"),
		callEvent(t, "c2", "sql", "y"), // response not yet arrived
	}
	if got := latestDanglingCallIndex(events); got != 2 {
		t.Errorf("dangling call index = %d, want 2", got)
	}
}

func TestLatestDanglingCallIndex_MultipleCallsInOneEvent(t *testing.T) {
	// One event carries two calls; only c1 has a response → the event is
	// dangling (event-granularity pairing).
	ev := &session.Event{
		Author: "assistant",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "model", Parts: []*genai.Part{
				{FunctionCall: &genai.FunctionCall{ID: "c1", Name: "search"}},
				{FunctionCall: &genai.FunctionCall{ID: "c2", Name: "sql"}},
			}},
		},
	}
	events := []*session.Event{ev, responseEvent(t, "c1", "r1")}
	if got := latestDanglingCallIndex(events); got != 0 {
		t.Errorf("multi-call event index = %d, want 0", got)
	}
}

func TestAdjustCutForDanglingCalls_MovesCutBack(t *testing.T) {
	// [call1][resp1][call2(dangling)][msg]; cut=2 would split call2 from its
	// future response → cut must move to 2 (index of the dangling call event).
	events := []*session.Event{
		callEvent(t, "c1", "search", "x"),
		responseEvent(t, "c1", "r1"),
		callEvent(t, "c2", "sql", "y"),
		textEvent(t, "final"),
	}
	if got := adjustCutForDanglingCalls(events, 2); got != 2 {
		t.Errorf("cut = %d, want 2 (dangling call index)", got)
	}
	// cut before the dangling call is untouched.
	if got := adjustCutForDanglingCalls(events, 1); got != 1 {
		t.Errorf("cut = %d, want 1 (unchanged)", got)
	}
}

func TestAdjustCutForDanglingCalls_AllPairedUnchanged(t *testing.T) {
	events := []*session.Event{
		callEvent(t, "c1", "search", "x"),
		responseEvent(t, "c1", "r1"),
		textEvent(t, "final"),
	}
	if got := adjustCutForDanglingCalls(events, 1); got != 1 {
		t.Errorf("all-paired cut = %d, want 1 (unchanged)", got)
	}
}
