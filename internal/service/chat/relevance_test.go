package chat

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"testing"

	"github.com/agiledragon/gomonkey/v2"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	"github.com/luoxiaojun1992/data-agent/internal/service/guard"
)

// These cover the chat-path relevance retry loop (SPEC-092) plus the small
// pure helpers (truncateTitle / isSessionPersistenceError) and the system-event
// appenders that the Process/Stream tests do not reach.

type fakeSessionService struct {
	session.Service
	getFn    func(context.Context, *session.GetRequest) (*session.GetResponse, error)
	appendFn func(context.Context, session.Session, *session.Event) error
}

func (f *fakeSessionService) Get(ctx context.Context, req *session.GetRequest) (*session.GetResponse, error) {
	return f.getFn(ctx, req)
}

func (f *fakeSessionService) AppendEvent(ctx context.Context, sess session.Session, evt *session.Event) error {
	if f.appendFn != nil {
		return f.appendFn(ctx, sess, evt)
	}
	return nil
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
func (e eventSlice) Len() int                { return len(e) }
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

func newChatRuntime(t *testing.T) *adkruntime.Runtime {
	t.Helper()
	rt, err := adkruntime.New(adkruntime.Config{
		AppName:        "data-agent",
		Model:          &fakeLLM{text: "x"},
		SessionService: session.InMemoryService(),
	})
	if err != nil {
		t.Fatalf("runtime: %v", err)
	}
	return rt
}

// ── pure helpers ──

func TestTruncateTitle(t *testing.T) {
	if got := truncateTitle("  hello  ", 100); got != "hello" {
		t.Errorf("truncateTitle(short) = %q, want trimmed", got)
	}
	if got := truncateTitle("你好世界", 2); got != "你好" {
		t.Errorf("truncateTitle(4 runes, max 2) = %q, want 你好", got)
	}
}

func TestIsSessionPersistenceError(t *testing.T) {
	if isSessionPersistenceError(nil) {
		t.Error("nil should not be a persistence error")
	}
	if !isSessionPersistenceError(errors.New("failed to add event to session")) {
		t.Error("failed to add event → persistence error")
	}
	if !isSessionPersistenceError(errors.New("append event: context canceled")) {
		t.Error("append event context canceled → persistence error")
	}
	// third branch: errors.Is(err, context.Canceled) AND message contains "session"
	if !isSessionPersistenceError(fmt.Errorf("session: %w", context.Canceled)) {
		t.Error("wrapped context.Canceled mentioning session → persistence error")
	}
	// bare context.Canceled has no "session" in its message → not a persistence error
	if isSessionPersistenceError(context.Canceled) {
		t.Error("bare context.Canceled should not be a persistence error")
	}
	if isSessionPersistenceError(errors.New("some other error")) {
		t.Error("unrelated error should not be a persistence error")
	}
}

// ── appendSystemEvent / appendSystemEventHidden ──

func TestAppendSystemEvent_LoadErrorNoPanic(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return nil, errors.New("no session")
	}}
	svc.appendSystemEvent(context.Background(), "u1", "s1", "data-agent", "hello")
	svc.appendHiddenSystemEvent(context.Background(), "u1", "s1", "data-agent", "hello")
}

func TestAppendSystemEvent_WritesVisibleEvent(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	var appended *session.Event
	svc.adkSessions = &fakeSessionService{
		getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
			return respWithUserEvent("base"), nil
		},
		appendFn: func(_ context.Context, _ session.Session, evt *session.Event) error {
			appended = evt
			return nil
		},
	}
	svc.appendSystemEvent(context.Background(), "u1", "s1", "data-agent", "visible")
	if appended == nil || appended.Author != "system" {
		t.Fatalf("expected a system event, got %+v", appended)
	}
	if v, ok := appended.LLMResponse.CustomMetadata["hidden"]; ok && v == true {
		t.Error("visible system event should not carry hidden metadata")
	}
}

func TestAppendSystemEventHidden_WritesHiddenEvent(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	var appended *session.Event
	svc.adkSessions = &fakeSessionService{
		getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
			return respWithUserEvent("base"), nil
		},
		appendFn: func(_ context.Context, _ session.Session, evt *session.Event) error {
			appended = evt
			return nil
		},
	}
	svc.appendHiddenSystemEvent(context.Background(), "u1", "s1", "data-agent", "hidden")
	if appended == nil || appended.LLMResponse.CustomMetadata["hidden"] != true {
		t.Error("hidden system event should carry hidden=true metadata")
	}
}

// ── relevanceBase ──

func TestRelevanceBase_GetError(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return nil, errors.New("boom")
	}}
	if got := svc.relevanceBase(context.Background(), "data-agent", "u1", "s1"); got != "" {
		t.Errorf("relevanceBase = %q, want empty on error", got)
	}
}

func TestRelevanceBase_ReturnsUserText(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("用户的问题"), nil
	}}
	if got := svc.relevanceBase(context.Background(), "data-agent", "u1", "s1"); got != "用户的问题" {
		t.Errorf("relevanceBase = %q, want user text", got)
	}
}

// ── relevanceLoop ──

func TestRelevanceLoop_NilGuardReturnsFirst(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	rt := newChatRuntime(t)
	// svc.guard stays nil → returns firstText unchanged.
	if got := svc.relevanceLoop(context.Background(), rt, "u1", "s1", &genai.Content{}, adkruntime.RunConfig{}, "first"); got != "first" {
		t.Errorf("relevanceLoop = %q, want first", got)
	}
}

func TestRelevanceLoop_EmptyBaseFallsBackToImageMarker(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	rt := newChatRuntime(t)
	svc.guard = guard.NewService(nil, nil, 2)
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return nil, errors.New("no session")
	}}
	var gotBase string
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	p.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, base string) (bool, error) {
			gotBase = base
			return true, nil
		})
	p.ApplyMethodFunc(&guard.Service{}, "ClearRelevance", func(_ context.Context, _ string) {})

	if out := svc.relevanceLoop(context.Background(), rt, "u1", "s1", &genai.Content{}, adkruntime.RunConfig{}, "first"); out != "first" {
		t.Errorf("relevanceLoop = %q, want first", out)
	}
	if gotBase != "[图片]" {
		t.Errorf("base = %q, want [图片] fallback", gotBase)
	}
}

func TestRelevanceLoop_RelevantReturnsText(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	rt := newChatRuntime(t)
	svc.guard = guard.NewService(nil, nil, 2)
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	var cleared bool
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	p.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return true, nil })
	p.ApplyMethodFunc(&guard.Service{}, "ClearRelevance",
		func(_ context.Context, _ string) { cleared = true })

	if got := svc.relevanceLoop(context.Background(), rt, "u1", "s1", &genai.Content{}, adkruntime.RunConfig{}, "text"); got != "text" {
		t.Errorf("relevanceLoop = %q, want text", got)
	}
	if !cleared {
		t.Error("ClearRelevance should be called when relevant")
	}
}

func TestRelevanceLoop_IrrelevantRetriesAndRecovers(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	rt := newChatRuntime(t)
	svc.guard = guard.NewService(nil, nil, 2)
	checks := 0
	var appended bool
	svc.adkSessions = &fakeSessionService{
		getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
			return respWithUserEvent("base"), nil
		},
		appendFn: func(_ context.Context, _ session.Session, evt *session.Event) error {
			appended = true
			return nil
		},
	}
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	p.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) {
			checks++
			return checks > 1, nil // first irrelevant, second relevant
		})
	p.ApplyMethodFunc(&guard.Service{}, "ClearRelevance", func(_ context.Context, _ string) {})
	p.ApplyMethodFunc(&guard.Service{}, "RecordAndShouldRetry",
		func(_ context.Context, _ string) (bool, error) { return true, nil })
	p.ApplyMethodFunc(rt, "RunAndCollectContent",
		func(_ context.Context, _, _ string, _ *genai.Content, _ adkruntime.RunConfig) (string, error) {
			return "retried", nil
		})

	got := svc.relevanceLoop(context.Background(), rt, "u1", "s1", &genai.Content{}, adkruntime.RunConfig{}, "first")
	if got != "retried" {
		t.Errorf("relevanceLoop = %q, want retried", got)
	}
	if !appended {
		t.Error("appendSystemEvent should record a [relevance] hint on irrelevant")
	}
}

func TestRelevanceLoop_CheckErrorReturnsText(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	rt := newChatRuntime(t)
	svc.guard = guard.NewService(nil, nil, 2)
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	p.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return false, errors.New("guard down") })

	got := svc.relevanceLoop(context.Background(), rt, "u1", "s1", &genai.Content{}, adkruntime.RunConfig{}, "text")
	if got != "text" {
		t.Errorf("relevanceLoop = %q, want text on guard error", got)
	}
}

func TestRelevanceLoop_NoRetryLeftReturnsText(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	rt := newChatRuntime(t)
	svc.guard = guard.NewService(nil, nil, 2)
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	p.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return false, nil })
	p.ApplyMethodFunc(&guard.Service{}, "RecordAndShouldRetry",
		func(_ context.Context, _ string) (bool, error) { return false, nil })

	got := svc.relevanceLoop(context.Background(), rt, "u1", "s1", &genai.Content{}, adkruntime.RunConfig{}, "text")
	if got != "text" {
		t.Errorf("relevanceLoop = %q, want text when no retry left", got)
	}
}

func TestRelevanceLoop_RetryErrorReturnsOriginal(t *testing.T) {
	svc := newTestService(t, &fakeLLM{text: "x"})
	rt := newChatRuntime(t)
	svc.guard = guard.NewService(nil, nil, 2)
	svc.adkSessions = &fakeSessionService{getFn: func(context.Context, *session.GetRequest) (*session.GetResponse, error) {
		return respWithUserEvent("base"), nil
	}}
	p := gomonkey.NewPatches()
	t.Cleanup(p.Reset)
	p.ApplyMethodFunc(&guard.Service{}, "CheckRelevance",
		func(_ context.Context, _, _ string) (bool, error) { return false, nil })
	p.ApplyMethodFunc(&guard.Service{}, "RecordAndShouldRetry",
		func(_ context.Context, _ string) (bool, error) { return true, nil })
	p.ApplyMethodFunc(rt, "RunAndCollectContent",
		func(_ context.Context, _, _ string, _ *genai.Content, _ adkruntime.RunConfig) (string, error) {
			return "", errors.New("run boom")
		})

	got := svc.relevanceLoop(context.Background(), rt, "u1", "s1", &genai.Content{}, adkruntime.RunConfig{}, "first")
	if got != "first" {
		t.Errorf("relevanceLoop = %q, want original on retry failure", got)
	}
}
