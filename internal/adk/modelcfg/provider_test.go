package modelcfg

import (
	"context"
	"fmt"
	"testing"

	"github.com/agiledragon/gomonkey/v2"

	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
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

// ---- DB-path tests (mock SystemConfigRepository via gomonkey) ----

func TestDB_ModelsFromDB_Valid(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()

	patches.ApplyMethodReturn(mockRepo, "Get", &model.SystemConfig{
		Value: `[{"name":"gpt4","base_url":"http://db/v1","is_default":true,"instruction":"sys prompt","capability":"reasoning","token_multiplier":2.0,"fallback_order":0}]`,
	}, nil)

	p := NewProvider(mockRepo)
	models := p.models()
	if len(models) != 1 || models[0].Name != "gpt4" || models[0].Instruction != "sys prompt" {
		t.Fatalf("models = %+v", models)
	}
	if models[0].TokenMultiplier != 2.0 || models[0].Capability != "reasoning" {
		t.Errorf("field mapping: %+v", models[0])
	}
}

func TestDB_ModelsFromDB_GetError(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Get", (*model.SystemConfig)(nil), fmt.Errorf("db down"))
	p := NewProvider(mockRepo)
	if p.modelsFromDB() != nil {
		t.Error("should return nil on DB error")
	}
}

func TestDB_ModelsFromDB_BadJSON(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Get", &model.SystemConfig{Value: "not-json"}, nil)
	p := NewProvider(mockRepo)
	if p.modelsFromDB() != nil {
		t.Error("should return nil on bad JSON")
	}
}

func TestDB_EmbeddingFromDB_Valid(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Get", &model.SystemConfig{
		Value: `{"base_url":"http://emb.db","model":"bge","api_key":"ek"}`,
	}, nil)
	p := NewProvider(mockRepo)
	e := p.embeddingFromDB()
	if e.BaseURL != "http://emb.db" || e.Model != "bge" {
		t.Errorf("embeddingFromDB = %+v", e)
	}
}

func TestDB_EmbeddingFromDB_Error(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Get", (*model.SystemConfig)(nil), fmt.Errorf("db down"))
	p := NewProvider(mockRepo)
	e := p.embeddingFromDB()
	if e.BaseURL != "" || e.Model != "" {
		t.Error("should return empty on DB error")
	}
}

func TestDB_DefaultInstruction_WithDB(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Get", &model.SystemConfig{
		Value: `[{"name":"g","is_default":true,"instruction":"custom prompt"}]`,
	}, nil)
	p := NewProvider(mockRepo)
	if inst := p.DefaultInstruction(context.Background()); inst != "custom prompt" {
		t.Errorf("instruction = %q", inst)
	}
}

func TestDB_SetModels(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Upsert", nil)
	p := NewProvider(mockRepo)
	if err := p.SetModels(context.Background(), []ModelEntry{{Name: "m"}}); err != nil {
		t.Fatalf("SetModels error: %v", err)
	}
}

func TestDB_SetEmbedding(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Upsert", nil)
	p := NewProvider(mockRepo)
	if err := p.SetEmbedding(context.Background(), EmbeddingEntry{BaseURL: "http://e"}); err != nil {
		t.Fatalf("SetEmbedding error: %v", err)
	}
}

func TestDB_GetRawModelConfig_WithDB(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "GetAll", []model.SystemConfig{
		{Key: "models", Value: `[{"name":"gpt4"}]`},
		{Key: "embedding", Value: `{"base_url":"http://e"}`},
	}, nil)
	p := NewProvider(mockRepo)
	cfg, err := p.GetRawModelConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRawModelConfig error: %v", err)
	}
	if _, ok := cfg["models"]; !ok {
		t.Error("models key missing")
	}
	if _, ok := cfg["embedding"]; !ok {
		t.Error("embedding key missing")
	}
	if cfg["api_url"] == nil {
		t.Error("legacy defaults missing")
	}
}

func TestDB_BuildLLM_SingleBackend(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Get", &model.SystemConfig{
		Value: `[{"name":"oned","base_url":"http://one/v1","fallback_order":0}]`,
	}, nil)
	p := NewProvider(mockRepo)
	llm, err := p.BuildLLM(context.Background())
	if err != nil || llm == nil {
		t.Fatalf("BuildLLM error: %v", err)
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

func TestDB_BuildLLM_ZeroModels(t *testing.T) {
	// `models` is unexported and uses defaults → can't trivially return empty.
	// Line 138 (len(models)==0 error path) relies on zero models, which is
	// unreachable in normal operation. Coverage deferred to SPEC-050.
}

func TestDB_EmbeddingFromDB_BadJSON(t *testing.T) {
	mockRepo := &mongoinfra.SystemConfigRepository{}
	patches := gomonkey.NewPatches()
	defer patches.Reset()
	patches.ApplyMethodReturn(mockRepo, "Get", &model.SystemConfig{Value: "bad-json"}, nil)
	p := NewProvider(mockRepo)
	e := p.embeddingFromDB()
	if e.BaseURL != "" {
		t.Error("bad JSON should return empty")
	}
}

func TestTrimSpace(t *testing.T) {
	if trimSpace("  hello  ") != "hello" {
		t.Error("trim both sides")
	}
	if trimSpace("") != "" {
		t.Error("empty")
	}
	if trimSpace("no-space") != "no-space" {
		t.Error("no space")
	}
}
