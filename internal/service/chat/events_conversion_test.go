package chat

import (
	"strings"
	"testing"
	"time"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	genai "google.golang.org/genai"

	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
)

// ChatEventsFromParts / ChatEventsFromADKEvent 是 SSE 与历史记录共用的
// 事件转换逻辑，属于核心业务转换，逐分支覆盖（含 image-only、image+text、
// tool_call/tool_result、nil part、compaction 过滤等边界）。

func TestChatEventsFromADKEvent_NilEvent(t *testing.T) {
	if got := ChatEventsFromADKEvent(nil); got != nil {
		t.Fatalf("nil event should yield nil, got %v", got)
	}
}

func TestChatEventsFromADKEvent_NilContent(t *testing.T) {
	if got := ChatEventsFromADKEvent(&session.Event{}); got != nil {
		t.Fatalf("nil content should yield nil, got %v", got)
	}
}

func TestChatEventsFromADKEvent_CompactionFiltered(t *testing.T) {
	ev := &session.Event{
		Author: "compaction",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "summary"}}},
		},
	}
	if got := ChatEventsFromADKEvent(ev); got != nil {
		t.Fatalf("compaction event must be filtered, got %v", got)
	}
}

func TestChatEventsFromADKEvent_EmptyAuthorDefaultsToAssistant(t *testing.T) {
	ts := time.Date(2026, 1, 2, 3, 4, 5, 0, time.UTC)
	ev := &session.Event{
		ID:        "ev1",
		Author:    "",
		Timestamp: ts,
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Parts: []*genai.Part{{Text: "hello"}}},
		},
	}
	got := ChatEventsFromADKEvent(ev)
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Role != "assistant" {
		t.Errorf("Role = %q, want assistant", got[0].Role)
	}
	if got[0].Timestamp != ts.UTC().Format(time.RFC3339) {
		t.Errorf("Timestamp = %q", got[0].Timestamp)
	}
}

func TestChatEventsFromParts_NilPartSkipped(t *testing.T) {
	got := ChatEventsFromParts("user", "e1", "ts", []*genai.Part{nil})
	if len(got) != 0 {
		t.Fatalf("nil part should be skipped, got %d events", len(got))
	}
}

func TestChatEventsFromParts_EmptyParts(t *testing.T) {
	if got := ChatEventsFromParts("user", "e1", "ts", nil); len(got) != 0 {
		t.Fatalf("empty parts should yield empty, got %v", got)
	}
}

func TestChatEventsFromParts_ToolCall(t *testing.T) {
	got := ChatEventsFromParts("assistant", "e1", "ts", []*genai.Part{{
		FunctionCall: &genai.FunctionCall{Name: "sql_executor", Args: map[string]any{"q": "1"}},
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Type != "tool_call" || got[0].Name != "sql_executor" {
		t.Errorf("tool_call event wrong: %+v", got[0])
	}
}

func TestChatEventsFromParts_ToolResult(t *testing.T) {
	got := ChatEventsFromParts("assistant", "e1", "ts", []*genai.Part{{
		FunctionResponse: &genai.FunctionResponse{Name: "sql_executor", Response: map[string]any{"rows": 3}},
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 event, got %d", len(got))
	}
	if got[0].Type != "tool_result" || got[0].Name != "sql_executor" {
		t.Errorf("tool_result event wrong: %+v", got[0])
	}
}

func TestChatEventsFromParts_Text(t *testing.T) {
	got := ChatEventsFromParts("user", "e1", "ts", []*genai.Part{{Text: "hi"}})
	if len(got) != 1 || got[0].Type != "text" || got[0].Content != "hi" {
		t.Errorf("text event wrong: %+v", got)
	}
}

func TestChatEventsFromParts_ImageOnly(t *testing.T) {
	got := ChatEventsFromParts("user", "e1", "ts", []*genai.Part{{
		InlineData: &genai.Blob{MIMEType: "image/png", Data: []byte{0x89, 0x50}},
	}})
	if len(got) != 1 {
		t.Fatalf("expected 1 image-only event, got %d", len(got))
	}
	if got[0].Type != "text" || got[0].Content != "" {
		t.Errorf("image-only event should be text with empty content: %+v", got[0])
	}
	if len(got[0].Images) != 1 || !strings.HasPrefix(got[0].Images[0], "data:image/png;base64,") {
		t.Errorf("image data URL wrong: %+v", got[0].Images)
	}
}

func TestChatEventsFromParts_ImageAttachedToText(t *testing.T) {
	got := ChatEventsFromParts("user", "e1", "ts", []*genai.Part{
		{Text: "see this"},
		{InlineData: &genai.Blob{MIMEType: "image/jpeg", Data: []byte{0xff, 0xd8}}},
	})
	if len(got) != 1 {
		t.Fatalf("image should attach to the text event, got %d events", len(got))
	}
	if got[0].Content != "see this" || len(got[0].Images) != 1 {
		t.Errorf("image not attached to text event: %+v", got[0])
	}
}

func TestChatEventsFromParts_NonImageInlineDataIgnored(t *testing.T) {
	got := ChatEventsFromParts("user", "e1", "ts", []*genai.Part{{
		InlineData: &genai.Blob{MIMEType: "application/pdf", Data: []byte{0x25, 0x50}},
	}})
	if len(got) != 0 {
		t.Fatalf("non-image inline data should be ignored, got %+v", got)
	}
}

func TestChatEventsFromParts_MixedPartsOrder(t *testing.T) {
	got := ChatEventsFromParts("assistant", "e1", "ts", []*genai.Part{
		{FunctionCall: &genai.FunctionCall{Name: "f", Args: nil}},
		{Text: "thinking"},
		{FunctionResponse: &genai.FunctionResponse{Name: "f", Response: nil}},
	})
	if len(got) != 3 {
		t.Fatalf("expected 3 events, got %d", len(got))
	}
	if got[0].Type != "tool_call" || got[1].Type != "text" || got[2].Type != "tool_result" {
		t.Errorf("part order not preserved: %+v", got)
	}
}

// 确保 domainchat 引用保持在包内（ChatEvent 断言用）。
var _ = domainchat.ChatEvent{}
