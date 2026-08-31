package adksession

import (
	"context"
	"strings"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"google.golang.org/adk/session"
)

// TestFlushStreamBuffer_WritesBufferedTextToSessionEvents covers the
// SPEC-069 问题 4 follow-up: turn-end flush moves buffered streaming text
// into session_events even when the final LLM event still has Partial=true.
func TestFlushStreamBuffer_WritesBufferedTextToSessionEvents(t *testing.T) {
	svc := &Service{buf: map[string]*chunkBuffer{}}
	var b strings.Builder
	b.WriteString("你好，世界")
	svc.buf["s1"] = &chunkBuffer{author: "data_agent", eventID: "evt_x", text: b, size: len("你好，世界")}

	var flushed *session.Event
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(svc, "appendRawEvent",
		func(_ *Service, _ context.Context, _ session.Session, ev *session.Event) error {
			flushed = ev
			return nil
		})

	if err := svc.FlushStreamBuffer(context.Background(), "data-agent", "u1", "s1"); err != nil {
		t.Fatalf("FlushStreamBuffer: %v", err)
	}
	if flushed == nil {
		t.Fatal("expected buffered event to be flushed to session_events")
	}
	if flushed.Author != "data_agent" {
		t.Errorf("author = %q, want data_agent", flushed.Author)
	}
	if got := flushed.Content.Parts[0].Text; got != "你好，世界" {
		t.Errorf("flushed text = %q, want 你好，世界", got)
	}
	if _, ok := svc.buf["s1"]; ok {
		t.Error("buffer entry should be removed after flush")
	}
}

// TestFlushStreamBuffer_EmptyBufferIsNoop verifies an empty buffer flushes
// nothing (no session_events write) and returns nil.
func TestFlushStreamBuffer_EmptyBufferIsNoop(t *testing.T) {
	svc := &Service{buf: map[string]*chunkBuffer{}}

	wrote := false
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyPrivateMethod(svc, "appendRawEvent",
		func(_ *Service, _ context.Context, _ session.Session, _ *session.Event) error {
			wrote = true
			return nil
		})

	if err := svc.FlushStreamBuffer(context.Background(), "data-agent", "u1", "s1"); err != nil {
		t.Fatalf("FlushStreamBuffer: %v", err)
	}
	if wrote {
		t.Error("empty buffer must not write to session_events")
	}
}
