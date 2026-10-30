package guard

import (
	"context"
	"errors"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
	"google.golang.org/adk/model"
	"google.golang.org/genai"
)

// These tests cover CheckIntent / CheckRelevance / ClearRelevance and the
// remaining contentText FunctionResponse branch. CheckIntent/CheckRelevance
// call the concrete *modelcfg.Provider.BuildLLM, which is stubbed with gomonkey
// to yield the existing fakeLLM (no real model backend needed).

func guardProvider() *modelcfg.Provider { return &modelcfg.Provider{} }

func patchBuildLLM(p *gomonkey.Patches, llm model.LLM) {
	p.ApplyMethodFunc(&modelcfg.Provider{}, "BuildLLM",
		func(_ context.Context, _ modelcfg.UseCase) (model.LLM, error) { return llm, nil })
}

func TestCheckIntent_BuildLLMError(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	p.ApplyMethodFunc(&modelcfg.Provider{}, "BuildLLM",
		func(_ context.Context, _ modelcfg.UseCase) (model.LLM, error) {
			return nil, errors.New("no model")
		})

	s := NewService(guardProvider(), nil, 2)
	_, _, err := s.CheckIntent(context.Background(), &genai.Content{})
	if err == nil {
		t.Fatal("expected error when BuildLLM fails")
	}
}

func TestCheckIntent_TaskPlan(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(textResp(`{"is_task": true, "is_plan": true}`), nil)
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	isTask, isPlan, err := s.CheckIntent(context.Background(),
		&genai.Content{Role: "user", Parts: []*genai.Part{{Text: "制定计划"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !isTask || !isPlan {
		t.Errorf("got isTask=%v isPlan=%v, want true/true", isTask, isPlan)
	}
}

func TestCheckIntent_MalformedDefaultsToChat(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(textResp("not json at all"), nil)
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	isTask, isPlan, err := s.CheckIntent(context.Background(),
		&genai.Content{Role: "user", Parts: []*genai.Part{{Text: "你好"}}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if isTask || isPlan {
		t.Errorf("malformed response should default to chat, got isTask=%v isPlan=%v", isTask, isPlan)
	}
}

func TestCheckIntent_GenerateError(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, errors.New("generate boom"))
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	_, _, err := s.CheckIntent(context.Background(), &genai.Content{})
	if err == nil {
		t.Fatal("expected error when generateText fails")
	}
}

func TestCheckRelevance_BuildLLMError(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	p.ApplyMethodFunc(&modelcfg.Provider{}, "BuildLLM",
		func(_ context.Context, _ modelcfg.UseCase) (model.LLM, error) {
			return nil, errors.New("no model")
		})

	s := NewService(guardProvider(), nil, 2)
	_, err := s.CheckRelevance(context.Background(), "answer", "base")
	if err == nil {
		t.Fatal("expected error when BuildLLM fails")
	}
}

func TestCheckRelevance_Relevant(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(textResp(`{"is_relevant": true}`), nil)
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	ok, err := s.CheckRelevance(context.Background(), "回答内容", "用户问题")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("expected relevant=true")
	}
}

func TestCheckRelevance_MalformedDefaultsTrue(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(textResp("garbage"), nil)
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	ok, err := s.CheckRelevance(context.Background(), "answer", "base")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("malformed relevance should default to true (never blocks)")
	}
}

func TestCheckRelevance_Irrelevant(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(textResp("```json\n{\"is_relevant\": false}\n```"), nil)
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	ok, err := s.CheckRelevance(context.Background(), "answer", "base")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ok {
		t.Error("expected relevant=false")
	}
}

func TestClearRelevance_NilRedis(t *testing.T) {
	s := NewService(nil, nil, 2) // redis stays nil
	s.ClearRelevance(context.Background(), "sess_1") // must not panic
}

func TestClearRelevance_DeletesKey(t *testing.T) {
	rc := &redis.Client{}
	var delKey string
	p := gomonkey.NewPatches()
	defer p.Reset()
	p.ApplyMethodFunc(rc, "Del",
		func(_ context.Context, keys ...string) error {
			if len(keys) > 0 {
				delKey = keys[0]
			}
			return nil
		})

	s := NewService(nil, nil, 2)
	s.SetRedis(rc)
	s.ClearRelevance(context.Background(), "sess_9")
	if delKey != "guard:relevance:sess_9" {
		t.Errorf("Del key = %q, want guard:relevance:sess_9", delKey)
	}
}

func TestContentText_FunctionResponseMarshals(t *testing.T) {
	c := &genai.Content{Parts: []*genai.Part{
		{Text: "prefix "},
		{FunctionResponse: &genai.FunctionResponse{
			ID: "c1", Name: "sql_executor",
			Response: map[string]any{"rows": 42},
		}},
	}}
	got := contentText(c)
	if got == "" {
		t.Fatal("expected non-empty contentText")
	}
	if got[:7] != "prefix " {
		t.Errorf("got %q, want leading text prefix preserved", got)
	}
}

func TestContentText_FunctionResponseMarshalError(t *testing.T) {
	// a channel value cannot be JSON-marshalled → branch falls through silently.
	c := &genai.Content{Parts: []*genai.Part{
		{FunctionResponse: &genai.FunctionResponse{
			ID: "c1", Name: "bad", Response: map[string]any{"bad": make(chan int)},
		}},
	}}
	if got := contentText(c); got != "" {
		t.Errorf("unmarshalable response should yield empty, got %q", got)
	}
}

func TestCheckRelevance_GenerateError(t *testing.T) {
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(nil, errors.New("generate boom"))
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	_, err := s.CheckRelevance(context.Background(), "answer", "base")
	if err == nil {
		t.Fatal("expected error when generateText fails")
	}
}

func TestCheckRelevance_EmptyGenerateText(t *testing.T) {
	// generateText returns "" (no error) when every response is empty →
	// parseIsRelevant("") defaults to true so a silent model never blocks.
	p := gomonkey.NewPatches()
	defer p.Reset()
	llm := &fakeLLM{seq: func(yield func(*model.LLMResponse, error) bool) {
		yield(textResp("   "), nil) // whitespace-only → skipped, "" returned
	}}
	patchBuildLLM(p, llm)

	s := NewService(guardProvider(), nil, 2)
	ok, err := s.CheckRelevance(context.Background(), "answer", "base")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !ok {
		t.Error("empty relevance text should default to true")
	}
}

func TestContentText_NilPart(t *testing.T) {
	c := &genai.Content{Parts: []*genai.Part{nil, {Text: "keep"}}}
	if got := contentText(c); got != "keep" {
		t.Errorf("contentText with nil part = %q, want keep", got)
	}
}

func TestLastRelevanceBase_ToolOutputEmptyText(t *testing.T) {
	// a function response whose payload cannot be marshalled yields empty
	// contentText → skipped, falls through to the earlier user message.
	events := eventsList{
		userEv("用户的问题"),
		{
			Author: "data_agent",
			LLMResponse: model.LLMResponse{
				Content: &genai.Content{Role: "user", Parts: []*genai.Part{
					{FunctionResponse: &genai.FunctionResponse{
						ID: "c1", Name: "bad", Response: map[string]any{"x": make(chan int)},
					}},
				}},
			},
		},
	}
	if got := LastRelevanceBase(events); got != "用户的问题" {
		t.Errorf("base = %q, want fall back to user message", got)
	}
}

func TestHasFunctionResponse(t *testing.T) {
	if hasFunctionResponse(&genai.Content{Parts: []*genai.Part{{Text: "hi"}}}) {
		t.Error("text-only content should not report function response")
	}
	if !hasFunctionResponse(&genai.Content{Parts: []*genai.Part{
		{FunctionResponse: &genai.FunctionResponse{ID: "c1"}},
	}}) {
		t.Error("function-response content should report true")
	}
	if hasFunctionResponse(&genai.Content{Parts: []*genai.Part{nil}}) {
		t.Error("nil part should not report function response")
	}
}
