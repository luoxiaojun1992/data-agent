package agent

import (
	"context"
	"errors"
	"iter"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	"github.com/luoxiaojun1992/data-agent/internal/service/guard"
)

// relevanceBase / relevanceLoop are the SPEC-092 relevance-gated retry loop.
// They are exercised here by faking the ADK session Service (relevanceBase)
// and stubbing the concrete guard.Service methods + runProtected (relevanceLoop),
// avoiding a real model/session backend.

type fakeSessionService struct {
	session.Service
	getFn func(context.Context, *session.GetRequest) (*session.GetResponse, error)
}

func (f *fakeSessionService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	return f.getFn(ctx, req)
}

type fakeSession struct {
	session.Session
	evs session.Events
}

func (f *fakeSession) Events() session.Events { return f.evs }

type eventSlice []*session.Event

func (e eventSlice) All() iter.Seq[*session.Event] {
	return func(yield func(*session.Event) bool) {
		for _, ev := range e {
			if !yield(ev) {
				return
			}
		}
	}
}
func (e eventSlice) Len() int               { return len(e) }
func (e eventSlice) At(i int) *session.Event { return e[i] }

func userEvent(text string) *session.Event {
	return &session.Event{
		Author: "user",
		LLMResponse: model.LLMResponse{
			Content: &genai.Content{Role: "user", Parts: []*genai.Part{{Text: text}}},
		},
	}
}

func respWithUserEvent(text string) *session.GetResponse {
	return &session.GetResponse{Session: &fakeSession{evs: eventSlice{userEvent(text)}}}
}

// ── relevanceBase ──

func TestRelevanceBase_GetError(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return nil, errors.New("boom")
	}}
	if got := te.exec.relevanceBase(context.Background(), "u1", "sess_1"); got != "" {
		t.Errorf("relevanceBase = %q, want empty on Get error", got)
	}
}

func TestRelevanceBase_NilResponse(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return nil, nil
	}}
	if got := te.exec.relevanceBase(context.Background(), "u1", "sess_1"); got != "" {
		t.Errorf("relevanceBase = %q, want empty on nil response", got)
	}
}

func TestRelevanceBase_NilSession(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return &session.GetResponse{Session: nil}, nil
	}}
	if got := te.exec.relevanceBase(context.Background(), "u1", "sess_1"); got != "" {
		t.Errorf("relevanceBase = %q, want empty on nil session", got)
	}
}

func TestRelevanceBase_ReturnsUserText(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("最近一条用户消息"), nil
	}}
	if got := te.exec.relevanceBase(context.Background(), "u1", "sess_1"); got != "最近一条用户消息" {
		t.Errorf("relevanceBase = %q, want user text", got)
	}
}

// ── relevanceLoop ──

func TestRelevanceLoop_EmptyBaseReturnsFirst(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return nil, errors.New("no session")
	}}
	te.exec.guard = guard.NewService(nil, nil, 2)
	got := te.exec.relevanceLoop(context.Background(), te.rt, sampleRun(), "sess_1",
		&genai.Content{}, adkruntime.RunConfig{}, "first")
	if got != "first" {
		t.Errorf("relevanceLoop = %q, want first", got)
	}
}

func TestRelevanceLoop_RelevantReturnsText(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	var cleared bool
	te.patches.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return true, nil })
	te.patches.ApplyMethodFunc(&guard.Service{}, "ClearRelevance",
		func(_ context.Context, _ string) { cleared = true })
	te.exec.guard = guard.NewService(nil, nil, 2)

	got := te.exec.relevanceLoop(context.Background(), te.rt, sampleRun(), "sess_1",
		&genai.Content{}, adkruntime.RunConfig{}, "text")
	if got != "text" {
		t.Errorf("relevanceLoop = %q, want text", got)
	}
	if !cleared {
		t.Error("ClearRelevance should be called when relevant")
	}
}

func TestRelevanceLoop_NoRetryLeftReturnsText(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	te.patches.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return false, nil })
	te.patches.ApplyMethodFunc(&guard.Service{}, "RecordAndShouldRetry",
		func(_ context.Context, _ string) (bool, error) { return false, nil })
	te.exec.guard = guard.NewService(nil, nil, 2)

	got := te.exec.relevanceLoop(context.Background(), te.rt, sampleRun(), "sess_1",
		&genai.Content{}, adkruntime.RunConfig{}, "text")
	if got != "text" {
		t.Errorf("relevanceLoop = %q, want text when no retry left", got)
	}
}

func TestRelevanceLoop_CheckErrorReturnsText(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	te.patches.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return false, errors.New("guard down") })
	te.exec.guard = guard.NewService(nil, nil, 2)

	got := te.exec.relevanceLoop(context.Background(), te.rt, sampleRun(), "sess_1",
		&genai.Content{}, adkruntime.RunConfig{}, "text")
	if got != "text" {
		t.Errorf("relevanceLoop = %q, want text on guard error", got)
	}
}

func TestRelevanceLoop_RetrySucceeds(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	checks := 0
	te.patches.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) {
			checks++
			return checks > 1, nil // first check irrelevant, second relevant
		})
	te.patches.ApplyMethodFunc(&guard.Service{}, "ClearRelevance", func(_ context.Context, _ string) {})
	te.patches.ApplyMethodFunc(&guard.Service{}, "RecordAndShouldRetry",
		func(_ context.Context, _ string) (bool, error) { return true, nil })
	// runProtected's runFn calls Runtime.RunAndCollectContent; stub it to
	// simulate a successful retry run yielding "retried text".
	te.patches.ApplyMethodFunc(te.rt, "RunAndCollectContent",
		func(_ context.Context, _, _ string, _ *genai.Content, _ adkruntime.RunConfig) (string, error) {
			return "retried text", nil
		})
	te.exec.guard = guard.NewService(nil, nil, 2)

	got := te.exec.relevanceLoop(context.Background(), te.rt, sampleRun(), "sess_1",
		&genai.Content{}, adkruntime.RunConfig{}, "first")
	if got != "retried text" {
		t.Errorf("relevanceLoop = %q, want retried text", got)
	}
	if checks != 2 {
		t.Errorf("CheckRelevance called %d times, want 2", checks)
	}
}

func TestRelevanceLoop_RetryRunErrorReturnsOriginal(t *testing.T) {
	te := newTestExecutor(t, &fakeLLM{text: "x"})
	te.exec.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	te.patches.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return false, nil })
	te.patches.ApplyMethodFunc(&guard.Service{}, "RecordAndShouldRetry",
		func(_ context.Context, _ string) (bool, error) { return true, nil })
	te.patches.ApplyMethodFunc(te.rt, "RunAndCollectContent",
		func(_ context.Context, _, _ string, _ *genai.Content, _ adkruntime.RunConfig) (string, error) {
			return "", errors.New("run boom")
		})
	te.exec.guard = guard.NewService(nil, nil, 2)

	got := te.exec.relevanceLoop(context.Background(), te.rt, sampleRun(), "sess_1",
		&genai.Content{}, adkruntime.RunConfig{}, "first")
	if got != "first" {
		t.Errorf("relevanceLoop = %q, want original text on retry failure", got)
	}
}
