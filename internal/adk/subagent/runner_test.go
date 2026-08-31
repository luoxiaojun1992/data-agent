package subagent

import (
	"context"
	"errors"
	"iter"
	"sync"
	"testing"

	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	adkruntime "github.com/luoxiaojun1992/data-agent/internal/adk/runtime"
	domainchat "github.com/luoxiaojun1992/data-agent/internal/domain/chat"
)

// ---- fake LLM ----

type fakeLLM struct {
	text string
	err  error
}

func (f *fakeLLM) Name() string { return "fake-subagent" }

func (f *fakeLLM) GenerateContent(_ context.Context, _ *model.LLMRequest, _ bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		if f.err != nil {
			yield(nil, f.err)
			return
		}
		yield(&model.LLMResponse{Content: genai.NewContentFromText(f.text, "model")}, nil)
	}
}

// ---- mock model provider ----

type mockProvider struct {
	entry      *modelcfg.ModelEntry
	llm        model.LLM
	buildCount int
	mu         sync.Mutex
}

func (m *mockProvider) GetModelByID(_ context.Context, id string) (*modelcfg.ModelEntry, error) {
	if m.entry == nil {
		return nil, errors.New("not found")
	}
	return m.entry, nil
}

func (m *mockProvider) GetModelByUseCase(_ context.Context, _ modelcfg.UseCase) (*modelcfg.ModelEntry, error) {
	return nil, errors.New("n/a")
}

func (m *mockProvider) BuildLLMByID(_ context.Context, _ string) (model.LLM, error) {
	m.mu.Lock()
	m.buildCount++
	m.mu.Unlock()
	return m.llm, nil
}

func (m *mockProvider) DefaultInstruction(context.Context) string { return "sub-instruction" }

// ---- mock ADK session service (tracks sub session create/delete) ----

type mockSessions struct {
	session.Service
	mu         sync.Mutex
	createdSub []string
	deleted    []string
}

func (m *mockSessions) CreateSubSession(ctx context.Context, req *session.CreateRequest, parent string) (*session.CreateResponse, error) {
	m.mu.Lock()
	m.createdSub = append(m.createdSub, parent)
	m.mu.Unlock()
	return m.Service.Create(ctx, req)
}

func (m *mockSessions) Delete(ctx context.Context, req *session.DeleteRequest) error {
	m.mu.Lock()
	m.deleted = append(m.deleted, req.SessionID)
	m.mu.Unlock()
	return m.Service.Delete(ctx, req)
}

// ---- mock parent resolver ----

type mockParent struct {
	sess *domainchat.Session
	err  error
}

func (m *mockParent) Get(string) (*domainchat.Session, error) { return m.sess, m.err }

// ---- helpers ----

func newTestRunner(llm model.LLM, parent *mockParent) (*Runner, *mockSessions, *mockProvider) {
	sess := &mockSessions{Service: session.InMemoryService()}
	prov := &mockProvider{
		entry: &modelcfg.ModelEntry{ID: "m1", Name: "m1", Type: modelcfg.ModelTypeLLM, Instruction: "sys"},
		llm:   llm,
	}
	reg := adkruntime.NewRegistry(adkruntime.RegistryConfig{
		Provider:       prov,
		SessionService: sess,
		AppName:        "test-agent",
	})
	return NewRunner(reg, sess, parent), sess, prov
}

// ---- tests ----

func TestRunner_resolveModel(t *testing.T) {
	cases := []struct {
		name    string
		parent  *mockParent
		want    string
		wantErr bool
	}{
		{"ok", &mockParent{sess: &domainchat.Session{ID: "p", ModelID: "m9"}}, "m9", false},
		{"empty model", &mockParent{sess: &domainchat.Session{ID: "p"}}, "", true},
		{"not found", &mockParent{err: errors.New("no session")}, "", true},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			r, _, _ := newTestRunner(&fakeLLM{text: "x"}, c.parent)
			got, err := r.resolveModel("p")
			if c.wantErr {
				if err == nil {
					t.Fatalf("expected error, got model %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("resolveModel: %v", err)
			}
			if got != c.want {
				t.Fatalf("model = %q, want %q", got, c.want)
			}
		})
	}
}

func TestRunner_resolveModel_nilResolver(t *testing.T) {
	sess := &mockSessions{Service: session.InMemoryService()}
	prov := &mockProvider{entry: &modelcfg.ModelEntry{ID: "m1", Type: modelcfg.ModelTypeLLM}, llm: &fakeLLM{text: "x"}}
	reg := adkruntime.NewRegistry(adkruntime.RegistryConfig{Provider: prov, SessionService: sess, AppName: "test"})
	r := NewRunner(reg, sess, nil)
	if _, err := r.resolveModel("p"); err == nil {
		t.Fatal("expected error with nil resolver")
	}
}

func TestRunner_Run_CreatesRunsDeletes(t *testing.T) {
	parent := &mockParent{sess: &domainchat.Session{ID: "parent1", ModelID: "m1", UserID: "u1"}}
	r, sess, prov := newTestRunner(&fakeLLM{text: "sub-agent-done"}, parent)

	text, err := r.Run(context.Background(), "parent1", "u1", "do a task",
		map[string]any{"user_id": "u1", "session_id": "parent1"})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if text != "sub-agent-done" {
		t.Fatalf("text = %q, want sub-agent-done", text)
	}
	// Sub session created with the parent binding.
	sess.mu.Lock()
	created := append([]string(nil), sess.createdSub...)
	deleted := append([]string(nil), sess.deleted...)
	sess.mu.Unlock()
	if len(created) != 1 || created[0] != "parent1" {
		t.Fatalf("createdSub = %v, want [parent1]", created)
	}
	if len(deleted) != 1 {
		t.Fatalf("deleted = %v, want exactly 1 sub session deleted", deleted)
	}
	if prov.buildCount != 1 {
		t.Fatalf("BuildLLMByID calls = %d, want 1", prov.buildCount)
	}
}

func TestRunner_Run_DeletesOnRunError(t *testing.T) {
	parent := &mockParent{sess: &domainchat.Session{ID: "parent1", ModelID: "m1", UserID: "u1"}}
	r, sess, _ := newTestRunner(&fakeLLM{err: errors.New("model down")}, parent)

	_, err := r.Run(context.Background(), "parent1", "u1", "task", map[string]any{"user_id": "u1"})
	if err == nil {
		t.Fatal("expected run error")
	}
	// Sub session must still be destroyed on failure (no residual).
	sess.mu.Lock()
	deleted := len(sess.deleted)
	sess.mu.Unlock()
	if deleted != 1 {
		t.Fatalf("deleted count = %d, want 1 (cleanup on error)", deleted)
	}
}

func TestRunner_Run_ResolveModelError(t *testing.T) {
	parent := &mockParent{err: errors.New("parent gone")}
	r, sess, _ := newTestRunner(&fakeLLM{text: "x"}, parent)

	_, err := r.Run(context.Background(), "parent1", "u1", "task", map[string]any{"user_id": "u1"})
	if err == nil {
		t.Fatal("expected error when parent cannot be resolved")
	}
	// No sub session created when model resolution fails.
	sess.mu.Lock()
	created := len(sess.createdSub)
	sess.mu.Unlock()
	if created != 0 {
		t.Fatalf("createdSub count = %d, want 0 (resolve fails before create)", created)
	}
}

// TestNewTool_Name verifies the tool registers as invoke_subagent and builds.
func TestNewTool_Name(t *testing.T) {
	parent := &mockParent{sess: &domainchat.Session{ID: "p", ModelID: "m1"}}
	r, _, _ := newTestRunner(&fakeLLM{text: "x"}, parent)
	tool, err := NewTool(r)
	if err != nil {
		t.Fatalf("NewTool: %v", err)
	}
	if tool.Name() != "invoke_subagent" {
		t.Fatalf("tool name = %q, want invoke_subagent", tool.Name())
	}
}
