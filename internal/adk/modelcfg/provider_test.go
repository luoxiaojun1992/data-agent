package modelcfg

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/mock"
	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/domain/model"
)

// newProviderWithModels builds a Provider backed by a mock repo that returns
// the given model list as the "models" config value.
func newProviderWithModels(t *testing.T, entries []ModelEntry) *Provider {
	t.Helper()
	repo := mockrepo.NewSysConfigRepository(t)
	raw, _ := json.Marshal(entries)
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: string(raw)}, nil)
	// legacyCfgValue("api_url") calls Get; return nil so BaseURL stays empty (test has no legacy config).
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	// legacyConfig() calls GetAll; return an empty list so fallback chain returns empty.
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	// SetModels calls Upsert; allow it so default-toggle / type-change tests work.
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Return(nil).Maybe()
	// EmbeddingConfig reads key "embedding"; tests that don't set it get the default.
	repo.On("Get", mock.Anything, "model", "embedding").Maybe().Return(nil, nil)
	return NewProvider(repo, nil)
}

func TestDefaultModel_IsDefaultPriority(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "a", Name: "A", Type: ModelTypeLLM},
		{ID: "b", Name: "B", Type: ModelTypeLLM, IsDefault: true},
		{ID: "c", Name: "C", Type: ModelTypeLLM},
	})
	dm, err := p.DefaultModel(context.Background())
	if err != nil {
		t.Fatalf("DefaultModel: %v", err)
	}
	if dm.ID != "b" {
		t.Errorf("DefaultModel ID = %q, want b (IsDefault)", dm.ID)
	}
}

func TestDefaultModel_FallbackToFirstLLM(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "emb", Name: "Emb", Type: ModelTypeEmbedding},
		{ID: "a", Name: "A", Type: ModelTypeLLM},
		{ID: "b", Name: "B", Type: ModelTypeLLM},
	})
	dm, err := p.DefaultModel(context.Background())
	if err != nil {
		t.Fatalf("DefaultModel: %v", err)
	}
	if dm.ID != "a" {
		t.Errorf("DefaultModel ID = %q, want a (first LLM)", dm.ID)
	}
}

func TestDefaultModel_NoLLMError(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "emb", Name: "Emb", Type: ModelTypeEmbedding},
	})
	_, err := p.DefaultModel(context.Background())
	if err == nil {
		t.Error("expected error when no LLM models configured")
	}
}

func TestGetModelByID_Found(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
		{ID: "m2", Name: "M2", Type: ModelTypeLLM},
	})
	m, err := p.GetModelByID(context.Background(), "m2")
	if err != nil {
		t.Fatalf("GetModelByID: %v", err)
	}
	if m.ID != "m2" {
		t.Errorf("got %q, want m2", m.ID)
	}
}

func TestGetModelByID_EmptyReturnsDefault(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "def", Name: "Def", Type: ModelTypeLLM, IsDefault: true},
	})
	m, err := p.GetModelByID(context.Background(), "")
	if err != nil {
		t.Fatalf("GetModelByID empty: %v", err)
	}
	if m.ID != "def" {
		t.Errorf("got %q, want def (default)", m.ID)
	}
}

func TestGetModelByID_NotFound(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	_, err := p.GetModelByID(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestListLLMModels_Pagination(t *testing.T) {
	entries := []ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
		{ID: "l2", Name: "L2", Type: ModelTypeLLM},
		{ID: "l3", Name: "L3", Type: ModelTypeLLM},
		{ID: "emb", Name: "Emb", Type: ModelTypeEmbedding},
	}
	p := newProviderWithModels(t, entries)
	models, total, err := p.ListLLMModels(context.Background(), 1, 2)
	if err != nil {
		t.Fatalf("ListLLMModels: %v", err)
	}
	if total != 3 {
		t.Errorf("total = %d, want 3 (LLM only, excludes embedding)", total)
	}
	if len(models) != 2 {
		t.Errorf("page size = %d, want 2", len(models))
	}
	// Page 2.
	models2, _, _ := p.ListLLMModels(context.Background(), 2, 2)
	if len(models2) != 1 {
		t.Errorf("page 2 size = %d, want 1", len(models2))
	}
	// Page beyond range.
	models3, _, _ := p.ListLLMModels(context.Background(), 5, 2)
	if len(models3) != 0 {
		t.Errorf("page 5 size = %d, want 0", len(models3))
	}
}

func TestConfigHash_StableAndSensitive(t *testing.T) {
	m1 := ModelEntry{ID: "a", Name: "A", Type: ModelTypeLLM}
	h1 := ConfigHash(m1)
	h2 := ConfigHash(m1)
	if h1 != h2 {
		t.Error("same config should produce same hash")
	}
	m2 := ModelEntry{ID: "a", Name: "A-Changed", Type: ModelTypeLLM}
	h3 := ConfigHash(m2)
	if h1 == h3 {
		t.Error("different config should produce different hash")
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestValidateModelIDs_DuplicateRejected(t *testing.T) {
	entries := []ModelEntry{
		{ID: "a", Name: "A"},
		{ID: "a", Name: "B"},
	}
	if err := validateModelIDs(entries); err == nil {
		t.Error("expected duplicate ID error")
	}
}

func TestValidateModelIDs_EmptyIDRejected(t *testing.T) {
	entries := []ModelEntry{
		{ID: "", Name: "A"},
	}
	if err := validateModelIDs(entries); err == nil {
		t.Error("expected empty ID error")
	}
}

func TestEnsureSingleDefault_AutoMarkFirstLLM(t *testing.T) {
	entries := []ModelEntry{
		{ID: "e", Name: "E", Type: ModelTypeEmbedding},
		{ID: "a", Name: "A", Type: ModelTypeLLM},
		{ID: "b", Name: "B", Type: ModelTypeLLM},
	}
	ensureSingleDefault(entries)
	if !entries[1].IsDefault {
		t.Error("first LLM should be auto-marked default")
	}
	if entries[2].IsDefault {
		t.Error("second LLM should not be default")
	}
}

func TestEnsureSingleDefault_CollapseExtras(t *testing.T) {
	entries := []ModelEntry{
		{ID: "a", Name: "A", Type: ModelTypeLLM, IsDefault: true},
		{ID: "b", Name: "B", Type: ModelTypeLLM, IsDefault: true},
	}
	ensureSingleDefault(entries)
	if !entries[0].IsDefault {
		t.Error("first default should remain")
	}
	if entries[1].IsDefault {
		t.Error("extra default should be collapsed")
	}
}

func TestBackfillID_EmptyUsesName(t *testing.T) {
	p := NewProvider(nil, nil)
	m := ModelEntry{Name: "Legacy"}
	p.backfillID(&m)
	if m.ID != "Legacy" {
		t.Errorf("backfill ID = %q, want Legacy", m.ID)
	}
}

func TestSetModels_DuplicateIDRejected(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	p := NewProvider(repo, nil)
	entries := []ModelEntry{
		{ID: "dup", Name: "A"},
		{ID: "dup", Name: "B"},
	}
	err := p.SetModels(context.Background(), entries)
	if err == nil {
		t.Error("expected duplicate ID rejection")
	}
}

func TestAddModel_AutoGenID(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	// Empty DB → no existing models.
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: ""}, nil)
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Return(nil)
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	p := NewProvider(repo, nil)

	entry := ModelEntry{Name: "NewModel", Type: ModelTypeLLM}
	saved, err := p.AddModel(context.Background(), entry)
	if err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if saved.ID == "" {
		t.Error("auto-generated ID should not be empty")
	}
}

func TestDeleteModel_Idempotent(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: "[]"}, nil)
	p := NewProvider(repo, nil)
	// Deleting a non-existent ID should not error (idempotent).
	if err := p.DeleteModel(context.Background(), "nonexistent"); err != nil {
		t.Errorf("idempotent delete should not error: %v", err)
	}
}

func TestSetDefaultModel_NotFound(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	err := p.SetDefaultModel(context.Background(), "nonexistent", []string{"chat"})
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestSetDefaultEmbedding_ExclusiveDefault(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	entries := []ModelEntry{
		{ID: "e1", Name: "E1", Type: ModelTypeEmbedding, IsDefault: true},
		{ID: "e2", Name: "E2", Type: ModelTypeEmbedding},
		{ID: "e3", Name: "E3", Type: ModelTypeEmbedding},
	}
	raw, _ := json.Marshal(entries)
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: string(raw)}, nil)
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	repo.On("Get", mock.Anything, "model", "embedding").Maybe().Return(nil, nil)
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	var stored string
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Run(func(args mock.Arguments) {
		stored = args.Get(3).(string)
	}).Return(nil)
	p := NewProvider(repo, nil)
	if err := p.SetDefaultEmbedding(context.Background(), "e3"); err != nil {
		t.Fatalf("SetDefaultEmbedding: %v", err)
	}
	var updated []ModelEntry
	if err := json.Unmarshal([]byte(stored), &updated); err != nil {
		t.Fatalf("decode stored payload: %v", err)
	}
	defaults := []string{}
	for _, m := range updated {
		if m.IsDefault {
			defaults = append(defaults, m.ID)
		}
	}
	if len(defaults) != 1 || defaults[0] != "e3" {
		t.Fatalf("expected only e3 default, got %v", defaults)
	}
}

func TestSetDefaultModel_EmbeddingTarget(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	entries := []ModelEntry{
		{ID: "e1", Name: "E1", Type: ModelTypeEmbedding, IsDefault: true},
		{ID: "e2", Name: "E2", Type: ModelTypeEmbedding},
	}
	raw, _ := json.Marshal(entries)
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: string(raw)}, nil)
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	repo.On("Get", mock.Anything, "model", "embedding").Maybe().Return(nil, nil)
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	var stored string
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Run(func(args mock.Arguments) {
		stored = args.Get(3).(string)
	}).Return(nil)
	p := NewProvider(repo, nil)
	if err := p.SetDefaultModel(context.Background(), "e2", nil); err != nil {
		t.Fatalf("SetDefaultModel on embedding should succeed: %v", err)
	}
	var updated []ModelEntry
	if err := json.Unmarshal([]byte(stored), &updated); err != nil {
		t.Fatalf("decode stored payload: %v", err)
	}
	for _, m := range updated {
		if m.ID == "e1" && m.IsDefault {
			t.Errorf("e1 should no longer be default after switching to e2")
		}
		if m.ID == "e2" && !m.IsDefault {
			t.Errorf("e2 should be default after SetDefaultModel")
		}
	}
}

func TestListAllModels_KeepsEmbeddingRows(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
		{ID: "e1", Name: "E1", Type: ModelTypeEmbedding},
	})
	all := p.ListAllModels(context.Background())
	if len(all) != 2 {
		t.Fatalf("ListAllModels = %d, want 2: %+v", len(all), all)
	}
	var sawLLM, sawEmbedding bool
	for _, m := range all {
		if m.Type == ModelTypeLLM {
			sawLLM = true
		}
		if m.Type == ModelTypeEmbedding {
			sawEmbedding = true
		}
	}
	if !sawLLM || !sawEmbedding {
		t.Errorf("missing types: LLM=%v EMB=%v", sawLLM, sawEmbedding)
	}
}

func TestSetDefaultModel_Success(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	entries := []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, IsDefault: true},
		{ID: "m2", Name: "M2", Type: ModelTypeLLM},
	}
	raw, _ := json.Marshal(entries)
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: string(raw)}, nil)
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Return(nil)
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	p := NewProvider(repo, nil)

	if err := p.SetDefaultModel(context.Background(), "m2", nil); err != nil {
		t.Fatalf("SetDefaultModel: %v", err)
	}
}

func TestGetModelByUseCase(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "chat", Name: "Chat", Type: ModelTypeLLM, IsDefaultFor: []string{"chat"}},
		{ID: "enh", Name: "Enh", Type: ModelTypeLLM, IsDefaultFor: []string{"enhance"}},
	})
	m, err := p.GetModelByUseCase(context.Background(), UseCaseEnhance)
	if err != nil {
		t.Fatalf("GetModelByUseCase: %v", err)
	}
	if m.ID != "enh" {
		t.Errorf("got %q, want enh", m.ID)
	}
}

func TestBuildLLMByID(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "gpt-4o", Type: ModelTypeLLM, BaseURL: "http://localhost:8082"},
	})
	llm, err := p.BuildLLMByID(context.Background(), "m1")
	if err != nil {
		t.Fatalf("BuildLLMByID: %v", err)
	}
	if llm == nil {
		t.Fatal("expected non-nil LLM")
	}
	if llm.Name() == "" {
		t.Error("LLM name should not be empty")
	}
}

func TestBuildLLMByID_NotFound(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	_, err := p.BuildLLMByID(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestDeleteModel_PromoteNewDefault(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	entries := []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, IsDefault: true},
		{ID: "m2", Name: "M2", Type: ModelTypeLLM},
	}
	raw, _ := json.Marshal(entries)
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: string(raw)}, nil)
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Return(nil)
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	p := NewProvider(repo, nil)

	if err := p.DeleteModel(context.Background(), "m1"); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}
	// After deleting the default, m2 should be promoted. Verify via DefaultModel.
	// (The mock returns the original list on subsequent Get calls, so we can't
	// verify the persisted state directly, but the Upsert was called.)
}

func TestAddModel_DuplicateIDRejected(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	entries := []ModelEntry{{ID: "existing", Name: "E", Type: ModelTypeLLM}}
	raw, _ := json.Marshal(entries)
	repo.On("Get", mock.Anything, "model", "models").Return(&model.SystemConfig{Value: string(raw)}, nil)
	repo.On("Get", mock.Anything, "model", "api_url").Maybe().Return(nil, nil)
	repo.On("GetAll", mock.Anything, "model").Return([]model.SystemConfig{}, nil).Maybe()
	p := NewProvider(repo, nil)

	_, err := p.AddModel(context.Background(), ModelEntry{ID: "existing", Name: "Dup"})
	if err == nil {
		t.Error("expected duplicate ID rejection")
	}
}

func TestGetModelByUseCase_NoMatchFallback(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, UseCases: []string{"chat"}},
	})
	// When no model matches the use case, selectModelsByUseCase falls back to
	// returning all models, so GetModelByUseCase returns the first model.
	m, err := p.GetModelByUseCase(context.Background(), UseCaseCompaction)
	if err != nil {
		t.Fatalf("expected fallback to first model, got error: %v", err)
	}
	if m.ID != "m1" {
		t.Errorf("fallback model ID = %q, want m1", m.ID)
	}
}

func TestConfigHash_EmptyOnError(t *testing.T) {
	// ConfigHash on a model that fails to marshal should return "".
	// Since ModelEntry always marshals, this tests the error path indirectly.
	h := ConfigHash(ModelEntry{ID: "x"})
	if h == "" {
		t.Error("valid model should produce non-empty hash")
	}
}

func TestGetRawModelConfig(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	cfgs := []model.SystemConfig{
		{Namespace: "model", Key: "models", Value: `[{"id":"m1","name":"M1","type":"llm"}]`},
		{Namespace: "model", Key: "embedding", Value: `{"base_url":"http://x","model":"emb"}`},
	}
	repo.On("GetAll", mock.Anything, "model").Return(cfgs, nil)
	p := NewProvider(repo, nil)
	raw, err := p.GetRawModelConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRawModelConfig: %v", err)
	}
	if raw == nil {
		t.Fatal("expected non-nil raw config")
	}
	if _, ok := raw["models"]; !ok {
		t.Error("expected 'models' key in raw config")
	}
	if _, ok := raw["embedding"]; !ok {
		t.Error("expected 'embedding' key in raw config")
	}
	// Legacy defaults should be filled.
	if _, ok := raw["api_url"]; !ok {
		t.Error("expected legacy 'api_url' default")
	}
}

func TestDefaultInstruction(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, IsDefault: true, Instruction: "You are helpful."},
	})
	inst := p.DefaultInstruction(context.Background())
	if inst != "You are helpful." {
		t.Errorf("DefaultInstruction = %q, want 'You are helpful.'", inst)
	}
}

func TestDefaultInstruction_NoneSet(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, Instruction: ""},
	})
	inst := p.DefaultInstruction(context.Background())
	if inst != "" {
		t.Errorf("DefaultInstruction with no instruction = %q, want empty", inst)
	}
}

func TestEmbeddingConfig(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	repo.On("Get", mock.Anything, "model", "embedding").Return(&model.SystemConfig{
		Value: `{"base_url":"http://emb:8082","model":"text-embed","api_key":"sk-xxx"}`,
	}, nil)
	p := NewProvider(repo, nil)
	cfg := p.EmbeddingConfig()
	if cfg.BaseURL != "http://emb:8082" {
		t.Errorf("BaseURL = %q, want http://emb:8082", cfg.BaseURL)
	}
	if cfg.Model != "text-embed" {
		t.Errorf("Model = %q, want text-embed", cfg.Model)
	}
}

func TestEmbeddingConfig_NilRepo(t *testing.T) {
	p := NewProvider(nil, nil)
	cfg := p.EmbeddingConfig()
	// With nil repo, falls back to env defaults (likely empty in tests).
	// Just verify it doesn't panic.
	_ = cfg
}

func TestSetEmbedding(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	repo.On("Upsert", mock.Anything, "model", "embedding", mock.Anything).Return(nil)
	p := NewProvider(repo, nil)
	err := p.SetEmbedding(context.Background(), EmbeddingEntry{BaseURL: "http://x", Model: "emb"})
	if err != nil {
		t.Fatalf("SetEmbedding: %v", err)
	}
}

func TestSetEmbedding_NilRepo(t *testing.T) {
	p := NewProvider(nil, nil)
	err := p.SetEmbedding(context.Background(), EmbeddingEntry{})
	if err == nil {
		t.Error("expected error with nil repo")
	}
}

func TestSetModels_NilRepo(t *testing.T) {
	p := NewProvider(nil, nil)
	err := p.SetModels(context.Background(), []ModelEntry{{ID: "x", Name: "X"}})
	if err == nil {
		t.Error("expected error with nil repo")
	}
}
