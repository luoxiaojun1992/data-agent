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
	return NewProvider(repo)
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
	p := NewProvider(nil)
	m := ModelEntry{Name: "Legacy"}
	p.backfillID(&m)
	if m.ID != "Legacy" {
		t.Errorf("backfill ID = %q, want Legacy", m.ID)
	}
}

func TestSetModels_DuplicateIDRejected(t *testing.T) {
	repo := mockrepo.NewSysConfigRepository(t)
	p := NewProvider(repo)
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
	p := NewProvider(repo)

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
	p := NewProvider(repo)
	// Deleting a non-existent ID should not error (idempotent).
	if err := p.DeleteModel(context.Background(), "nonexistent"); err != nil {
		t.Errorf("idempotent delete should not error: %v", err)
	}
}

func TestSetDefaultModel_NotFound(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	err := p.SetDefaultModel(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent model")
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
	repo.On("Upsert", mock.Anything, "model", "models", mock.Anything).Return(nil)
	p := NewProvider(repo)

	if err := p.SetDefaultModel(context.Background(), "m2"); err != nil {
		t.Fatalf("SetDefaultModel: %v", err)
	}
}

func TestGetModelByUseCase(t *testing.T) {
	p := newProviderWithModels(t, []ModelEntry{
		{ID: "chat", Name: "Chat", Type: ModelTypeLLM, UseCases: []string{"chat"}},
		{ID: "enh", Name: "Enh", Type: ModelTypeLLM, UseCases: []string{"enhance"}, TokenMultiplier: 0.5},
	})
	m, err := p.GetModelByUseCase(context.Background(), UseCaseEnhance)
	if err != nil {
		t.Fatalf("GetModelByUseCase: %v", err)
	}
	if m.ID != "enh" {
		t.Errorf("got %q, want enh", m.ID)
	}
}
