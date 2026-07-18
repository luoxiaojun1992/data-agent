package modelcfg

import (
	"context"
	"testing"
)

func TestProvider_EnvOnly(t *testing.T) {
	t.Setenv("LLM_MODEL", "gpt-4")
	t.Setenv("LLM_BASE_URL", "http://api.test/v1")
	t.Setenv("LLM_FALLBACK_BASE_URLS", "http://fb1,http://fb2")
	t.Setenv("EMBEDDING_BASE_URL", "http://emb.test")
	t.Setenv("EMBEDDING_MODEL", "text-embed")
	t.Setenv("EMBEDDING_API_KEY", "ek")

	p := NewProvider(nil)
	llm, err := p.BuildLLM(context.Background())
	if err != nil || llm == nil {
		t.Fatalf("BuildLLM: %v", err)
	}
	if inst := p.DefaultInstruction(context.Background()); inst != "" {
		t.Errorf("env default instruction: %q", inst)
	}
	ec := p.EmbeddingConfig()
	if ec.BaseURL != "http://emb.test" || ec.APIKey != "ek" {
		t.Errorf("EmbeddingConfig = %+v", ec)
	}
}

func TestProvider_NilRepo(t *testing.T) {
	p := NewProvider(nil)
	if p.modelsFromDB() != nil {
		t.Error("nil repo should return nil")
	}
	if e := p.embeddingFromDB(); e.BaseURL != "" || e.Model != "" {
		t.Error("nil repo embedding should be empty")
	}
	if err := p.SetModels(context.Background(), nil); err == nil {
		t.Error("SetModels with nil repo should error")
	}
}

func TestEnvHelpers(t *testing.T) {
	t.Setenv("X", "v")
	if envOrDefault("X", "d") != "v" {
		t.Error("env pick")
	}
	if envOrDefault("Y", "d") != "d" {
		t.Error("default fallback")
	}
	parts := splitEnvList("a, b,,c")
	if len(parts) != 3 || parts[0] != "a" || parts[2] != "c" {
		t.Errorf("splitEnvList = %v", parts)
	}
}

func TestSortModels(t *testing.T) {
	entries := []ModelEntry{
		{FallbackOrder: 3, Name: "c"},
		{FallbackOrder: 1, Name: "a"},
		{FallbackOrder: 2, Name: "b"},
	}
	sortModels(entries)
	if entries[0].Name != "a" || entries[2].Name != "c" {
		t.Errorf("sort = %v", entries)
	}
	// Already sorted stays sorted.
	sortModels(entries)
	if entries[0].Name != "a" {
		t.Errorf("re-sort broke order")
	}
}

func TestFillLegacyDefaults(t *testing.T) {
	m := map[string]any{"api_url": "http://custom", "model_name": ""}
	fillLegacyDefaults(m)
	if m["model_name"] != "" && m["api_url"] != "http://custom" {
		t.Error("existing values preserved")
	}
	m2 := map[string]any{}
	fillLegacyDefaults(m2)
	if m2["api_url"] == nil {
		t.Error("defaults applied")
	}
}

func TestGetRawModelConfig_NilRepo(t *testing.T) {
	p := NewProvider(nil)
	cfg, err := p.GetRawModelConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRawModelConfig: %v", err)
	}
	if cfg["api_url"] == nil {
		t.Error("defaults should be filled")
	}
}

func TestNewProvider(t *testing.T) {
	p := NewProvider(nil)
	if p == nil || p.cfgNS != "model" {
		t.Error("bad provider init")
	}
}

func TestApplyEnvDefaults(t *testing.T) {
	t.Setenv("LLM_BASE_URL", "http://override")
	m := ModelEntry{}
	p := &Provider{}
	p.applyEnvDefaults(&m)
	if m.BaseURL != "http://override" {
		t.Errorf("env override: %s", m.BaseURL)
	}
	if m.Temperature != 0.7 || m.MaxTokens != 4096 || m.TokenMultiplier != 1.0 {
		t.Errorf("defaults not applied: %+v", m)
	}
}

func TestApplyEmbeddingDefaults(t *testing.T) {
	t.Setenv("EMBEDDING_BASE_URL", "http://emb")
	t.Setenv("EMBEDDING_MODEL", "nomic-text")
	e := EmbeddingEntry{APIKey: "existing"}
	p := &Provider{}
	p.applyEmbeddingDefaults(&e)
	if e.BaseURL != "http://emb" || e.Model != "nomic-text" || e.APIKey != "existing" {
		t.Errorf("applyEmbeddingDefaults = %+v", e)
	}
}
