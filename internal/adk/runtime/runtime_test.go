package adkruntime

import (
	"context"
	"fmt"
	"iter"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/adk/model"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

// collectLLM yields a single final response with the configured text (or
// error). Distinct from registry_test.go's fakeLLM (which is hardcoded "ok").
type collectLLM struct {
	text string
	err  error
}

func (f *collectLLM) Name() string { return "collect" }

func (f *collectLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(f.text, "model")}, nil)
	}
}

// collectQueueLLM yields a sequence of preset responses, draining one per call.
type collectQueueLLM struct {
	queue []*model.LLMResponse
}

func (q *collectQueueLLM) Name() string { return "collect-queue" }

func (q *collectQueueLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if len(q.queue) == 0 {
			yield(nil, fmt.Errorf("empty queue"))
			return
		}
		resp := q.queue[0]
		q.queue = q.queue[1:]
		yield(resp, nil)
	}
}

func newTestRuntime(t *testing.T, llm model.LLM) *Runtime {
	t.Helper()
	sessions := adksession.InMemoryService()
	rt, err := New(Config{
		AppName:        "data-agent",
		Model:          llm,
		SessionService: sessions,
	})
	require.NoError(t, err)
	// Pre-create the ADK session so Runtime.Run can find it (mirrors the
	// executor/chat prepareRun step that creates the session before running).
	_, err = sessions.Create(context.Background(), &adksession.CreateRequest{
		AppName: "data-agent", UserID: "u1", SessionID: "s1",
	})
	require.NoError(t, err)
	return rt
}

// TestRunAndCollect_SingleFinalResponse returns the final assistant text.
func TestRunAndCollect_SingleFinalResponse(t *testing.T) {
	rt := newTestRuntime(t, &collectLLM{text: "最终答案"})

	text, err := rt.RunAndCollect(context.Background(), "u1", "s1", "你好", RunConfig{})
	require.NoError(t, err)
	assert.Equal(t, "最终答案", text)
}

// TestRunAndCollect_SkipsNonFinalEvents collects only final-response text,
// consuming (ignoring) intermediate tool-call events.
func TestRunAndCollect_SkipsNonFinalEvents(t *testing.T) {
	llm := &collectQueueLLM{queue: []*model.LLMResponse{
		{Content: &genai.Content{Role: "model", Parts: []*genai.Part{
			{FunctionCall: &genai.FunctionCall{Name: "sql_validate", Args: map[string]any{}}},
		}}},
		{Content: genai.NewContentFromText("最终答案", "model")},
	}}
	rt := newTestRuntime(t, llm)

	text, err := rt.RunAndCollect(context.Background(), "u1", "s1", "hi", RunConfig{})
	require.NoError(t, err)
	assert.Equal(t, "最终答案", text)
}

// TestRunAndCollect_StreamError surfaces the first error and returns "".
func TestRunAndCollect_StreamError(t *testing.T) {
	rt := newTestRuntime(t, &collectLLM{err: fmt.Errorf("model down")})

	text, err := rt.RunAndCollect(context.Background(), "u1", "s1", "hi", RunConfig{})
	require.Error(t, err)
	assert.Equal(t, "model down", err.Error())
	assert.Empty(t, text)
}

// TestRunAndCollect_ConcatenatesMultipleFinalParts concatenates text from
// multiple parts of a single final-response event.
func TestRunAndCollect_ConcatenatesMultipleFinalParts(t *testing.T) {
	llm := &collectQueueLLM{queue: []*model.LLMResponse{
		{Content: &genai.Content{Role: "model", Parts: []*genai.Part{
			{Text: "第一段。"},
			{Text: "第二段。"},
		}}},
	}}
	rt := newTestRuntime(t, llm)

	text, err := rt.RunAndCollect(context.Background(), "u1", "s1", "hi", RunConfig{})
	require.NoError(t, err)
	assert.Equal(t, "第一段。第二段。", text)
}

// TestRunAndCollect_StateDeltaPropagated verifies StateDelta is accepted and
// does not break the run (the session receives the injected state).
func TestRunAndCollect_StateDeltaPropagated(t *testing.T) {
	rt := newTestRuntime(t, &collectLLM{text: "ok"})
	text, err := rt.RunAndCollect(context.Background(), "u1", "s1", "hi", RunConfig{
		StateDelta: map[string]any{"kb_id": "kb-1", "user_id": "u1"},
	})
	require.NoError(t, err)
	assert.Equal(t, "ok", text)
}
