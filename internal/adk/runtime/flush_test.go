package adkruntime

import (
	"context"
	"iter"
	"sync"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

// flushTrackingSessionService wraps the ADK in-memory service and records
// FlushStreamBuffer calls (SPEC-069 问题 4 follow-up: the ieshan backend's
// final response still carries Partial=true, so the turn end must force-flush
// buffered streaming text or the last assistant message never lands in
// session_events).
type flushTrackingSessionService struct {
	adksession.Service
	mu    sync.Mutex
	calls []string
}

func (f *flushTrackingSessionService) FlushStreamBuffer(_ context.Context, appName, userID, sessionID string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls = append(f.calls, appName+"/"+userID+"/"+sessionID)
	return nil
}

func (f *flushTrackingSessionService) flushed() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, len(f.calls))
	copy(out, f.calls)
	return out
}

// partialFinalLLM mimics the ieshan openai backend: the last response carries
// the full text but still has Partial=true and an empty finish reason.
type partialFinalLLM struct {
	text string
}

func (p *partialFinalLLM) Name() string { return "partial-final" }

func (p *partialFinalLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: genai.NewContentFromText(p.text, "model"), Partial: true}, nil)
	}
}

// TestRunContent_FlushesStreamBufferAtTurnEnd verifies that when the run's
// event stream is exhausted (including mid-stream break), the runtime calls
// FlushStreamBuffer on the session service.
func TestRunContent_FlushesStreamBufferAtTurnEnd(t *testing.T) {
	inner := adksession.InMemoryService()
	flusher := &flushTrackingSessionService{Service: inner}

	rt, err := New(Config{
		AppName:        "data-agent",
		Model:          &partialFinalLLM{text: "你好"},
		SessionService: flusher,
	})
	require.NoError(t, err)

	_, err = flusher.Create(context.Background(), &adksession.CreateRequest{
		AppName: "data-agent", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	text, err := rt.RunAndCollectContent(context.Background(), "u1", "s1",
		genai.NewContentFromText("hi", "user"), RunConfig{})
	require.NoError(t, err)
	_ = text // RunAndCollectContent skips partial-only finals (IsFinalResponse=false)

	calls := flusher.flushed()
	require.Len(t, calls, 1, "FlushStreamBuffer must be called exactly once at turn end")
	require.Equal(t, "data-agent/u1/s1", calls[0])
}

// TestRunContent_FlushOnEarlyBreak verifies the flush also fires when the
// consumer breaks out of the event stream early.
func TestRunContent_FlushOnEarlyBreak(t *testing.T) {
	inner := adksession.InMemoryService()
	flusher := &flushTrackingSessionService{Service: inner}

	rt, err := New(Config{
		AppName:        "data-agent",
		Model:          &partialFinalLLM{text: "x"},
		SessionService: flusher,
	})
	require.NoError(t, err)

	_, err = flusher.Create(context.Background(), &adksession.CreateRequest{
		AppName: "data-agent", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)

	// Break after the first yielded event (does not exhaust the iterator).
	for _, err := range rt.RunContent(context.Background(), "u1", "s1",
		genai.NewContentFromText("hi", "user"), RunConfig{}) {
		require.NoError(t, err)
		break
	}

	calls := flusher.flushed()
	require.Len(t, calls, 1, "FlushStreamBuffer must fire on early break too")
}

// TestRunContent_NoFlushInterfaceIsNoop verifies runtimes whose session
// service does not implement FlushStreamBuffer (e.g. plain ADK in-memory)
// still work — the optional interface is skipped.
func TestRunContent_NoFlushInterfaceIsNoop(t *testing.T) {
	rt := newTestRuntime(t, &partialFinalLLM{text: "ok"})

	text, err := rt.RunAndCollectContent(context.Background(), "u1", "s1",
		genai.NewContentFromText("hi", "user"), RunConfig{})
	require.NoError(t, err)
	_ = text // partial-only final is skipped by IsFinalResponse
}
