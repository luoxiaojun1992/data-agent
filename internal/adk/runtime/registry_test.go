package adkruntime

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"google.golang.org/adk/memory"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
	"iter"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
)

// fakeLLM implements model.LLM for registry tests.
type fakeLLM struct {
	name string
}

func (f *fakeLLM) Name() string { return f.name }

func (f *fakeLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	return func(yield func(*model.LLMResponse, error) bool) {
		yield(&model.LLMResponse{Content: genai.NewContentFromText("ok", "model")}, nil)
	}
}

// mockProvider is a hand-written ModelProvider mock for registry tests. It
// returns configurable entries and LLMs, and tracks BuildLLMByID call count
// to verify lazy-create + reuse semantics.
type mockProvider struct {
	entries    map[string]*modelcfg.ModelEntry
	useCaseMap map[modelcfg.UseCase]*modelcfg.ModelEntry
	buildCount int
	mu         sync.Mutex
}

func newMockProvider() *mockProvider {
	return &mockProvider{
		entries:    make(map[string]*modelcfg.ModelEntry),
		useCaseMap: make(map[modelcfg.UseCase]*modelcfg.ModelEntry),
	}
}

func (m *mockProvider) GetModelByID(ctx context.Context, modelID string) (*modelcfg.ModelEntry, error) {
	if modelID == "" {
		// default = first entry
		for _, e := range m.entries {
			return e, nil
		}
		return nil, fmt.Errorf("no models")
	}
	if e, ok := m.entries[modelID]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("model %q not found", modelID)
}

func (m *mockProvider) GetModelByUseCase(ctx context.Context, useCase modelcfg.UseCase) (*modelcfg.ModelEntry, error) {
	if e, ok := m.useCaseMap[useCase]; ok {
		return e, nil
	}
	return nil, fmt.Errorf("no model for use case %q", useCase)
}

func (m *mockProvider) BuildLLMByID(ctx context.Context, modelID string) (model.LLM, error) {
	m.mu.Lock()
	m.buildCount++
	m.mu.Unlock()
	return &fakeLLM{name: modelID}, nil
}

func (m *mockProvider) DefaultInstruction(ctx context.Context) string { return "test-instruction" }

func (m *mockProvider) buildCountVal() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.buildCount
}

func newTestRegistry(t *testing.T) (*Registry, *mockProvider) {
	t.Helper()
	mp := newMockProvider()
	r := NewRegistry(RegistryConfig{
		Provider:       mp,
		SessionService: session.InMemoryService(),
		AppName:        "test-agent",
	})
	return r, mp
}

func TestRegistry_GetOrCreate_LazyCreate(t *testing.T) {
	r, mp := newTestRegistry(t)
	mp.entries["m1"] = &modelcfg.ModelEntry{ID: "m1", Name: "Model1", Type: modelcfg.ModelTypeLLM, Instruction: "sys"}

	rt, err := r.GetOrCreate(context.Background(), "m1")
	if err != nil {
		t.Fatalf("GetOrCreate: %v", err)
	}
	if rt == nil {
		t.Fatal("expected non-nil Runtime")
	}
	if mp.buildCountVal() != 1 {
		t.Errorf("BuildLLMByID calls = %d, want 1 (lazy create)", mp.buildCountVal())
	}
}

func TestRegistry_GetOrCreate_ReuseOnHit(t *testing.T) {
	r, mp := newTestRegistry(t)
	mp.entries["m1"] = &modelcfg.ModelEntry{ID: "m1", Name: "Model1", Type: modelcfg.ModelTypeLLM}

	rt1, _ := r.GetOrCreate(context.Background(), "m1")
	rt2, _ := r.GetOrCreate(context.Background(), "m1")
	if rt1 != rt2 {
		t.Error("same modelID should return the same Runtime instance (reuse)")
	}
	if mp.buildCountVal() != 1 {
		t.Errorf("BuildLLMByID calls = %d, want 1 (reuse, no rebuild)", mp.buildCountVal())
	}
}

func TestRegistry_GetOrCreate_DefaultFallback(t *testing.T) {
	r, mp := newTestRegistry(t)
	mp.entries["default-model"] = &modelcfg.ModelEntry{ID: "default-model", Name: "Default", Type: modelcfg.ModelTypeLLM}

	rt, err := r.GetOrCreate(context.Background(), "")
	if err != nil {
		t.Fatalf("empty modelID should resolve to default: %v", err)
	}
	if rt == nil {
		t.Fatal("expected non-nil Runtime for default model")
	}
	if mp.buildCountVal() != 1 {
		t.Errorf("BuildLLMByID calls = %d, want 1", mp.buildCountVal())
	}
}

func TestRegistry_GetOrCreate_FingerprintRebuild(t *testing.T) {
	r, mp := newTestRegistry(t)
	mp.entries["m1"] = &modelcfg.ModelEntry{ID: "m1", Name: "Model1", Type: modelcfg.ModelTypeLLM, Instruction: "v1"}

	rt1, _ := r.GetOrCreate(context.Background(), "m1")
	if mp.buildCountVal() != 1 {
		t.Fatalf("initial build count = %d, want 1", mp.buildCountVal())
	}

	// Change the model config (fingerprint changes) → next call rebuilds.
	mp.entries["m1"] = &modelcfg.ModelEntry{ID: "m1", Name: "Model1", Type: modelcfg.ModelTypeLLM, Instruction: "v2-changed"}
	rt2, _ := r.GetOrCreate(context.Background(), "m1")
	if mp.buildCountVal() != 2 {
		t.Errorf("after config change, build count = %d, want 2 (rebuild)", mp.buildCountVal())
	}
	if rt1 == rt2 {
		t.Error("config change should produce a new Runtime instance")
	}
}

func TestRegistry_GetOrCreate_ConcurrentSingleCreate(t *testing.T) {
	r, mp := newTestRegistry(t)
	mp.entries["m1"] = &modelcfg.ModelEntry{ID: "m1", Name: "Model1", Type: modelcfg.ModelTypeLLM}

	var wg sync.WaitGroup
	runtimes := make([]*Runtime, 10)
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			rt, err := r.GetOrCreate(context.Background(), "m1")
			if err != nil {
				t.Errorf("goroutine %d: %v", idx, err)
				return
			}
			runtimes[idx] = rt
		}(i)
	}
	wg.Wait()

	// All goroutines should get the same Runtime instance (single create).
	first := runtimes[0]
	for i, rt := range runtimes {
		if rt != first {
			t.Errorf("goroutine %d got different Runtime instance", i)
		}
	}
	if mp.buildCountVal() != 1 {
		t.Errorf("concurrent calls should create Runtime once: build count = %d, want 1", mp.buildCountVal())
	}
}

func TestRegistry_GetOrCreateByUseCase_IndependentInstances(t *testing.T) {
	r, mp := newTestRegistry(t)
	mp.useCaseMap[modelcfg.UseCaseEnhance] = &modelcfg.ModelEntry{ID: "enhance-model", Name: "Enhance", Type: modelcfg.ModelTypeLLM}
	mp.useCaseMap[modelcfg.UseCaseCompaction] = &modelcfg.ModelEntry{ID: "compact-model", Name: "Compact", Type: modelcfg.ModelTypeLLM}

	rtEnhance, err := r.GetOrCreateByUseCase(context.Background(), modelcfg.UseCaseEnhance)
	if err != nil {
		t.Fatalf("enhance: %v", err)
	}
	rtCompact, err := r.GetOrCreateByUseCase(context.Background(), modelcfg.UseCaseCompaction)
	if err != nil {
		t.Fatalf("compaction: %v", err)
	}
	if rtEnhance == rtCompact {
		t.Error("different use cases should get different Runtime instances")
	}

	// Same use case reuses.
	rtEnhance2, _ := r.GetOrCreateByUseCase(context.Background(), modelcfg.UseCaseEnhance)
	if rtEnhance != rtEnhance2 {
		t.Error("same use case should reuse Runtime")
	}
}

func TestRegistry_GetOrCreateByUseCase_RebuildOnConfigChange(t *testing.T) {
	r, mp := newTestRegistry(t)
	mp.useCaseMap[modelcfg.UseCaseEnhance] = &modelcfg.ModelEntry{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeLLM, Instruction: "v1"}

	rt1, _ := r.GetOrCreateByUseCase(context.Background(), modelcfg.UseCaseEnhance)
	mp.useCaseMap[modelcfg.UseCaseEnhance] = &modelcfg.ModelEntry{ID: "e1", Name: "E1", Type: modelcfg.ModelTypeLLM, Instruction: "v2"}
	rt2, _ := r.GetOrCreateByUseCase(context.Background(), modelcfg.UseCaseEnhance)
	if rt1 == rt2 {
		t.Error("config change should rebuild sys Runtime")
	}
}

func TestRegistry_GetOrCreate_ModelNotFound(t *testing.T) {
	r, _ := newTestRegistry(t)
	_, err := r.GetOrCreate(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestRegistry_AppName(t *testing.T) {
	r, _ := newTestRegistry(t)
	if r.AppName() != "test-agent" {
		t.Errorf("AppName = %q, want test-agent", r.AppName())
	}
}

func TestRegistry_GetOrCreateByUseCase_UseCaseNotFound(t *testing.T) {
	r, _ := newTestRegistry(t)
	_, err := r.GetOrCreateByUseCase(context.Background(), modelcfg.UseCaseKBChunking)
	if err == nil {
		t.Error("expected error for unconfigured use case")
	}
}

// ensure memory.Service import is satisfied (registry supports nil memory).
var _ memory.Service = (memory.Service)(nil)
