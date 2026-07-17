package agent

import (
	"context"
	"fmt"
	"testing"
)

func TestNewRouter(t *testing.T) {
	r := NewRouter()
	if r == nil {
		t.Fatal("NewRouter should not return nil")
	}
	if r.providers == nil {
		t.Error("providers map should be initialized")
	}
	if r.models == nil {
		t.Error("models map should be initialized")
	}
}

func TestRegisterProvider(t *testing.T) {
	r := NewRouter()
	r.RegisterProvider("openai", &mockProvider{})
	if _, exists := r.providers["openai"]; !exists {
		t.Error("provider should be registered")
	}
}

func TestRegisterModel(t *testing.T) {
	r := NewRouter()
	cfg := &ModelConfig{Model: "gpt-4"}
	r.RegisterModel("gpt-4", cfg)
	ret, err := r.GetModel("gpt-4")
	if err != nil {
		t.Fatalf("GetModel: %v", err)
	}
	if ret.Model != "gpt-4" {
		t.Errorf("model: got %s", ret.Model)
	}
}

func TestGetModel_NotFound(t *testing.T) {
	r := NewRouter()
	_, err := r.GetModel("nonexistent")
	if err == nil {
		t.Error("should error for nonexistent model")
	}
}

func TestGetDefaultModel(t *testing.T) {
	r := NewRouter()
	r.RegisterModel("default-model", &ModelConfig{Model: "default-model", IsDefault: true})
	m, err := r.GetDefaultModel()
	if err != nil {
		t.Fatalf("GetDefaultModel: %v", err)
	}
	if m.Model != "default-model" {
		t.Errorf("got %s", m.Model)
	}
}

func TestGetDefaultModel_NotFound(t *testing.T) {
	r := NewRouter()
	_, err := r.GetDefaultModel()
	if err == nil {
		t.Error("should error when no default")
	}
}

func TestListModels(t *testing.T) {
	r := NewRouter()
	r.RegisterModel("m1", &ModelConfig{Model: "m1"})
	r.RegisterModel("m2", &ModelConfig{Model: "m2"})
	models := r.ListModels()
	if len(models) != 2 {
		t.Errorf("got %d models, want 2", len(models))
	}
}

func TestNewEngine(t *testing.T) {
	r := NewRouter()
	e := NewEngine(r, nil, nil)
	if e == nil {
		t.Error("NewEngine should not return nil")
	}
}

func TestNewSkillRegistryAdapter(t *testing.T) {
	a := NewSkillRegistryAdapter()
	if a == nil {
		t.Error("NewSkillRegistryAdapter should not return nil")
	}
}

func TestSkillRegistryAdapter_Get_Empty(t *testing.T) {
	a := NewSkillRegistryAdapter()
	_, err := a.Get("nonexistent")
	if err == nil {
		t.Error("should error for nonexistent skill")
	}
}

func TestSkillRegistryAdapter_List_Empty(t *testing.T) {
	a := NewSkillRegistryAdapter()
	list := a.List()
	if len(list) != 0 {
		t.Errorf("List should be empty, got %d items", len(list))
	}
}

// ===== Engine tests =====

func TestEngine_Run_WithProvider(t *testing.T) {
	r := NewRouter()
	mp := &mockProvider{}
	r.RegisterProvider("gpt-4", mp)
	r.RegisterModel("gpt-4", &ModelConfig{Model: "gpt-4"})
	r.RegisterModel("default-model", &ModelConfig{Model: "gpt-4", IsDefault: true})

	e := NewEngine(r, nil, nil)
	resp, err := e.Run(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Run: %v", err)
	}
	if resp.Content != "mock response" {
		t.Errorf("content: got %q", resp.Content)
	}
}

func TestEngine_Run_NoModel_NoDefault(t *testing.T) {
	r := NewRouter()
	e := NewEngine(r, nil, nil)

	_, err := e.Run(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Error("should error when no model and no default")
	}
}

func TestEngine_Run_WithDefault(t *testing.T) {
	r := NewRouter()
	mp := &mockProvider{}
	r.RegisterProvider("default-gpt", mp)
	r.RegisterModel("default-gpt", &ModelConfig{Model: "default-gpt", IsDefault: true})

	e := NewEngine(r, nil, nil)
	resp, err := e.Run(context.Background(), ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Run with default: %v", err)
	}
	if resp == nil {
		t.Error("should have response")
	}
}

func TestEngine_Run_ProviderError(t *testing.T) {
	r := NewRouter()
	mp := &mockProvider{failMode: true}
	r.RegisterProvider("bad-model", mp)
	r.RegisterModel("bad-model", &ModelConfig{Model: "bad-model"})

	e := NewEngine(r, nil, nil)
	_, err := e.Run(context.Background(), ChatRequest{
		Model: "bad-model", Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err == nil {
		t.Error("should error when provider fails")
	}
}

func TestEngine_RunStream(t *testing.T) {
	r := NewRouter()
	mp := &mockProvider{}
	r.RegisterProvider("gpt-4", mp)
	r.RegisterModel("gpt-4", &ModelConfig{Model: "gpt-4"})

	e := NewEngine(r, nil, nil)
	var chunks []string
	err := e.RunStream(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk string) error {
		chunks = append(chunks, chunk)
		return nil
	})
	if err != nil {
		t.Fatalf("RunStream: %v", err)
	}
	if len(chunks) == 0 {
		t.Error("should have received chunks")
	}
}

func TestRouter_Chat(t *testing.T) {
	r := NewRouter()
	mp := &mockProvider{}
	r.RegisterProvider("gpt-4", mp)
	r.RegisterModel("gpt-4", &ModelConfig{Model: "gpt-4"})

	resp, err := r.Chat(context.Background(), "gpt-4", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	})
	if err != nil {
		t.Fatalf("Router.Chat: %v", err)
	}
	if resp == nil {
		t.Error("should have response")
	}
}

func TestRouter_ChatStream(t *testing.T) {
	r := NewRouter()
	mp := &mockProvider{}
	r.RegisterProvider("gpt-4", mp)
	r.RegisterModel("gpt-4", &ModelConfig{Model: "gpt-4"})

	err := r.ChatStream(context.Background(), "gpt-4", ChatRequest{
		Messages: []Message{{Role: "user", Content: "hi"}},
	}, func(chunk string) error { return nil })
	if err != nil {
		t.Fatalf("Router.ChatStream: %v", err)
	}
}

func TestEngine_Run_AutoRegister(t *testing.T) {
	// Router auto-registers a provider when one isn't found
	r := NewRouter()
	r.RegisterModel("gpt-4", &ModelConfig{Model: "gpt-4", BaseURL: "https://api.example.com", APIKey: "sk-test"})

	e := NewEngine(r, nil, nil)
	// This will try to auto-register OpenAIProvider and make HTTP call → fails gracefully
	_, err := e.Run(context.Background(), ChatRequest{
		Model: "gpt-4", Messages: []Message{{Role: "user", Content: "hi"}},
	})
	t.Logf("auto-register result: %v", err)
}

// ===== mockProvider =====

type mockProvider struct {
	failMode bool
}

func (m *mockProvider) Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	if m.failMode {
		return nil, fmt.Errorf("provider failed")
	}
	return &ChatResponse{Content: "mock response"}, nil
}

func (m *mockProvider) ChatStream(ctx context.Context, req ChatRequest, callback func(chunk string) error) error {
	if m.failMode {
		return fmt.Errorf("provider failed")
	}
	_ = callback("chunk1")
	_ = callback("chunk2")
	return nil
}
