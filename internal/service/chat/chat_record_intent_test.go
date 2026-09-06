package chat

import (
	"context"
	"testing"
	"time"

	"github.com/agiledragon/gomonkey/v2"
	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	"github.com/luoxiaojun1992/data-agent/internal/domain/security"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/service/guard"
	"github.com/stretchr/testify/mock"
	adksession "google.golang.org/adk/session"
	"google.golang.org/genai"
)

// newIntentService builds a Service with an in-memory ADK session store and a
// guard service whose CheckIntent is stubbed per test.
func newIntentService(t *testing.T) (*Service, adksession.Service, *guard.Service) {
	t.Helper()
	adkSessions := adksession.InMemoryService()
	registry := adkruntime.NewRegistry(adkruntime.RegistryConfig{
		AppName:        "data-agent",
		SessionService: adkSessions,
	})
	sessionRepo := mockrepo.NewSessionRepository(t)
	sessionRepo.On("SetTitle", mock.Anything, mock.Anything, mock.Anything).Maybe().Return(nil)
	mgr := &Manager{repo: sessionRepo, ttl: time.Hour}
	cbReg := security.NewCircuitBreakerRegistry(security.DefaultCircuitBreakerConfig())

	g := &guard.Service{}
	svc := NewService(registry, nil, adkSessions, mgr, cbReg, g)
	return svc, adkSessions, g
}

// patchCheckIntent stubs the concrete *guard.Service.CheckIntent method (it is
// not an interface) so recordIntent does not need a live LLM provider.
func patchCheckIntent(t *testing.T, g *guard.Service, isTask, isPlan bool) {
	t.Helper()
	patches := gomonkey.NewPatches()
	t.Cleanup(patches.Reset)
	patches.ApplyMethodFunc(g, "CheckIntent",
		func(ctx context.Context, content *genai.Content) (bool, bool, error) {
			return isTask, isPlan, nil
		})
}

func systemEventTexts(t *testing.T, s adksession.Service) ([]string, int) {
	t.Helper()
	resp, err := s.Get(context.Background(), &adksession.GetRequest{
		AppName: "data-agent", UserID: "u1", SessionID: "s1",
	})
	if err != nil {
		t.Fatalf("get session: %v", err)
	}
	events := resp.Session.Events()
	var texts []string
	hidden := 0
	for i := 0; i < events.Len(); i++ {
		ev := events.At(i)
		if ev.Author != "system" {
			continue
		}
		if IsHiddenEvent(ev) {
			hidden++
		}
		if ev.Content != nil {
			for _, p := range ev.Content.Parts {
				if p != nil && p.Text != "" {
					texts = append(texts, p.Text)
				}
			}
		}
	}
	return texts, hidden
}

func TestRecordIntent_PlanInjectsHiddenHint(t *testing.T) {
	svc, adkSessions, g := newIntentService(t)
	patchCheckIntent(t, g, true, true)

	if _, err := adkSessions.Create(context.Background(), &adksession.CreateRequest{
		AppName: "data-agent", UserID: "u1", SessionID: "s1",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc.recordIntent(context.Background(), "u1", "s1", "data-agent",
		genai.NewContentFromText("帮我制定学习计划", "user"))

	texts, hidden := systemEventTexts(t, adkSessions)
	if len(texts) != 2 {
		t.Fatalf("system events = %v, want 2 ([intent] + [plan_hint])", texts)
	}
	if texts[0] != "[intent] is_task=true is_plan=true" {
		t.Errorf("intent text = %q", texts[0])
	}
	if texts[1] != planHintText {
		t.Errorf("plan hint = %q", texts[1])
	}
	if hidden != 2 {
		t.Errorf("hidden events = %d, want 2", hidden)
	}
}

func TestRecordIntent_NoPlanOnlyIntent(t *testing.T) {
	svc, adkSessions, g := newIntentService(t)
	patchCheckIntent(t, g, true, false)

	if _, err := adkSessions.Create(context.Background(), &adksession.CreateRequest{
		AppName: "data-agent", UserID: "u1", SessionID: "s1",
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	svc.recordIntent(context.Background(), "u1", "s1", "data-agent",
		genai.NewContentFromText("查询销售数据", "user"))

	texts, hidden := systemEventTexts(t, adkSessions)
	if len(texts) != 1 {
		t.Fatalf("system events = %v, want 1 ([intent] only)", texts)
	}
	if texts[0] != "[intent] is_task=true is_plan=false" {
		t.Errorf("intent text = %q", texts[0])
	}
	if hidden != 1 {
		t.Errorf("hidden events = %d, want 1", hidden)
	}
}
