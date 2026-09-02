package modelcfg

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/stretchr/testify/mock"

	mockrepo "github.com/luoxiaojun1992/data-agent/internal/repository/mocks"
	"github.com/luoxiaojun1992/data-agent/internal/domain/modelconfig"
)

// fakeVault is an in-memory VaultStore used to test API key encryption paths.
type fakeVault struct {
	stored map[string]string
}

func newFakeVault() *fakeVault { return &fakeVault{stored: map[string]string{}} }

func (f *fakeVault) Store(_ context.Context, path, value string) error {
	f.stored[path] = value
	return nil
}

func (f *fakeVault) Retrieve(_ context.Context, path string) (string, error) {
	v, ok := f.stored[path]
	if !ok {
		return "", fmt.Errorf("secret not found")
	}
	return v, nil
}

var errNotFound = errors.New("not found")

// newProviderWithModels builds a Provider backed by a mock
// ModelConfigRepository serving the given entries. The returned
// ModelDefaultRepository mock is NOT pre-stubbed — tests must register their
// own default-repo expectations (see stubNoDefaults / stubDefaults) so that
// test-specific expectations always match first.
func newProviderWithModels(t *testing.T, entries []modelconfig.ModelEntry) (*Provider, *mockrepo.ModelDefaultRepository) {
	t.Helper()
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Return(func(_ context.Context, typ modelconfig.ModelType, skip, limit int64) []modelconfig.ModelEntry {
			out, _ := paginate(entries, skip, limit)
			if typ == "" {
				return out
			}
			filtered := make([]modelconfig.ModelEntry, 0, len(out))
			for _, e := range out {
				if e.Type == typ {
					filtered = append(filtered, e)
				}
			}
			return filtered
		}, func(_ context.Context, typ modelconfig.ModelType, _, _ int64) int64 {
			if typ == "" {
				return int64(len(entries))
			}
			var n int64
			for _, e := range entries {
				if e.Type == typ {
					n++
				}
			}
			return n
		}, nil).Maybe()
	repo.On("Get", mock.Anything, mock.Anything).
		Return(func(_ context.Context, id string) *modelconfig.ModelEntry {
			for i := range entries {
				if entries[i].ID == id {
					return &entries[i]
				}
			}
			return nil
		}, func(_ context.Context, id string) error {
			for i := range entries {
				if entries[i].ID == id {
					return nil
				}
			}
			return errNotFound
		}).Maybe()
	// Write-side defaults; tests asserting specific write behavior build the
	// provider manually instead.
	repo.On("Delete", mock.Anything, mock.Anything).Return(nil).Maybe()
	repo.On("Insert", mock.Anything, mock.Anything).Return(nil).Maybe()
	repo.On("Update", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	return NewProvider(repo, defRepo, nil), defRepo
}

func paginate(entries []modelconfig.ModelEntry, skip, limit int64) ([]modelconfig.ModelEntry, int64) {
	total := int64(len(entries))
	if skip >= total {
		return nil, total
	}
	end := skip + limit
	if end > total {
		end = total
	}
	return entries[skip:end], total
}

// stubNoDefaults registers generic default-repo behavior: no default records
// exist, so every Get falls back and List is empty. Register AFTER any
// test-specific expectations so those match first.
func stubNoDefaults(t *testing.T, defRepo *mockrepo.ModelDefaultRepository) {
	t.Helper()
	defRepo.On("Get", mock.Anything, mock.Anything).Return(nil, errNotFound).Maybe()
	defRepo.On("List", mock.Anything).Return([]modelconfig.ModelDefault{}, nil).Maybe()
	defRepo.On("Set", mock.Anything, mock.Anything, mock.Anything).Return(nil).Maybe()
	defRepo.On("Delete", mock.Anything, mock.Anything).Return(nil).Maybe()
}

// stubDefaults registers use-case → model defaults so GetModelByUseCase and
// attachDefaults resolve them. Register BEFORE stubNoDefaults.
func stubDefaults(t *testing.T, defRepo *mockrepo.ModelDefaultRepository, items []modelconfig.ModelDefault) {
	t.Helper()
	for i := range items {
		d := items[i]
		defRepo.On("Get", mock.Anything, d.UseCase).Return(&d, nil).Maybe()
	}
	defRepo.On("List", mock.Anything).Return(items, nil).Maybe()
}

func TestDefaultModel_FromDefaultRepo(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "a", Name: "A", Type: ModelTypeLLM},
		{ID: "b", Name: "B", Type: ModelTypeLLM},
	})
	stubDefaults(t, defRepo, []modelconfig.ModelDefault{{UseCase: string(UseCaseChat), ModelID: "b"}})
	stubNoDefaults(t, defRepo)
	dm, err := p.DefaultModel(context.Background())
	if err != nil {
		t.Fatalf("DefaultModel: %v", err)
	}
	if dm.ID != "b" {
		t.Errorf("DefaultModel ID = %q, want b (model_defaults)", dm.ID)
	}
}

func TestDefaultModel_FallbackToFirstLLM(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "emb", Name: "Emb", Type: ModelTypeEmbedding},
		{ID: "a", Name: "A", Type: ModelTypeLLM},
		{ID: "b", Name: "B", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	dm, err := p.DefaultModel(context.Background())
	if err != nil {
		t.Fatalf("DefaultModel: %v", err)
	}
	if dm.ID != "a" {
		t.Errorf("DefaultModel ID = %q, want a (first LLM)", dm.ID)
	}
}

func TestDefaultModel_NoLLMError(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "emb", Name: "Emb", Type: ModelTypeEmbedding},
	})
	stubNoDefaults(t, defRepo)
	_, err := p.DefaultModel(context.Background())
	if err == nil {
		t.Error("expected error when no LLM models configured")
	}
}

func TestGetModelByID_Found(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
		{ID: "m2", Name: "M2", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	m, err := p.GetModelByID(context.Background(), "m2")
	if err != nil {
		t.Fatalf("GetModelByID: %v", err)
	}
	if m.ID != "m2" {
		t.Errorf("got %q, want m2", m.ID)
	}
}

func TestGetModelByID_EmptyReturnsDefault(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "def", Name: "Def", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	m, err := p.GetModelByID(context.Background(), "")
	if err != nil {
		t.Fatalf("GetModelByID empty: %v", err)
	}
	if m.ID != "def" {
		t.Errorf("got %q, want def (default)", m.ID)
	}
}

func TestGetModelByID_NotFound(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	_, err := p.GetModelByID(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent ID")
	}
}

func TestListLLMModels_Pagination(t *testing.T) {
	entries := []modelconfig.ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
		{ID: "l2", Name: "L2", Type: ModelTypeLLM},
		{ID: "l3", Name: "L3", Type: ModelTypeLLM},
		{ID: "emb", Name: "Emb", Type: ModelTypeEmbedding},
	}
	p, defRepo := newProviderWithModels(t, entries)
	stubNoDefaults(t, defRepo)
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
	models2, _, _ := p.ListLLMModels(context.Background(), 2, 2)
	if len(models2) != 1 {
		t.Errorf("page 2 size = %d, want 1", len(models2))
	}
	models3, _, _ := p.ListLLMModels(context.Background(), 5, 2)
	if len(models3) != 0 {
		t.Errorf("page 5 size = %d, want 0", len(models3))
	}
}

func TestListLLMModels_DefaultPageSize(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	models, total, err := p.ListLLMModels(context.Background(), 0, 0)
	if err != nil {
		t.Fatalf("ListLLMModels: %v", err)
	}
	if total != 1 || len(models) != 1 {
		t.Errorf("default pagination: total=%d len=%d, want 1/1", total, len(models))
	}
}

func TestConfigHash_StableAndSensitive(t *testing.T) {
	m1 := modelconfig.ModelEntry{ID: "a", Name: "A", Type: ModelTypeLLM}
	h1 := ConfigHash(m1)
	if h1 != ConfigHash(m1) {
		t.Error("same config should produce same hash")
	}
	m2 := modelconfig.ModelEntry{ID: "a", Name: "A-Changed", Type: ModelTypeLLM}
	if h1 == ConfigHash(m2) {
		t.Error("different config should produce different hash")
	}
	if h1 == "" {
		t.Error("hash should not be empty")
	}
}

func TestGetModelByUseCase(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "chat", Name: "Chat", Type: ModelTypeLLM},
		{ID: "enh", Name: "Enh", Type: ModelTypeLLM},
	})
	stubDefaults(t, defRepo, []modelconfig.ModelDefault{{UseCase: string(UseCaseEnhance), ModelID: "enh"}})
	stubNoDefaults(t, defRepo)
	m, err := p.GetModelByUseCase(context.Background(), UseCaseEnhance)
	if err != nil {
		t.Fatalf("GetModelByUseCase: %v", err)
	}
	if m.ID != "enh" {
		t.Errorf("got %q, want enh", m.ID)
	}
}

func TestGetModelByUseCase_NoMatchFallback(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, UseCases: []string{"chat"}},
	})
	stubNoDefaults(t, defRepo)
	m, err := p.GetModelByUseCase(context.Background(), UseCaseCompaction)
	if err != nil {
		t.Fatalf("expected fallback to first model, got error: %v", err)
	}
	if m.ID != "m1" {
		t.Errorf("fallback model ID = %q, want m1", m.ID)
	}
}

func TestBuildLLMByID(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "gpt-4o", Type: ModelTypeLLM, BaseURL: "http://localhost:8082"},
	})
	stubNoDefaults(t, defRepo)
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
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	_, err := p.BuildLLMByID(context.Background(), "nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestListAllModels_KeepsEmbeddingRows(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
		{ID: "e1", Name: "E1", Type: ModelTypeEmbedding},
	})
	stubNoDefaults(t, defRepo)
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
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
		{ID: "m2", Name: "M2", Type: ModelTypeLLM},
	})
	defRepo.On("Set", mock.Anything, string(UseCaseChat), "m2").Return(nil).Once()
	if err := p.SetDefaultModel(context.Background(), "m2", nil); err != nil {
		t.Fatalf("SetDefaultModel: %v", err)
	}
	defRepo.AssertCalled(t, "Set", mock.Anything, string(UseCaseChat), "m2")
}

func TestSetDefaultModel_InvalidUseCaseRejected(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	err := p.SetDefaultModel(context.Background(), "m1", []string{"not-a-use-case"})
	if err == nil {
		t.Fatal("expected error for invalid use case")
	}
	defRepo.AssertNotCalled(t, "Set", mock.Anything, mock.Anything, mock.Anything)
}

func TestSetDefaultModel_NilDefaultRepo(t *testing.T) {
	p := NewProvider(nil, nil, nil)
	if err := p.SetDefaultModel(context.Background(), "x", nil); err == nil {
		t.Error("expected error with nil default repo")
	}
}

func TestSetDefaultEmbedding_Success(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "e1", Name: "E1", Type: ModelTypeEmbedding},
		{ID: "e2", Name: "E2", Type: ModelTypeEmbedding},
	})
	defRepo.On("Set", mock.Anything, string(UseCaseEmbedding), "e2").Return(nil).Once()
	if err := p.SetDefaultEmbedding(context.Background(), "e2"); err != nil {
		t.Fatalf("SetDefaultEmbedding: %v", err)
	}
	defRepo.AssertCalled(t, "Set", mock.Anything, string(UseCaseEmbedding), "e2")
}

func TestSetDefaultEmbedding_NotEmbeddingModel(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
	})
	if err := p.SetDefaultEmbedding(context.Background(), "l1"); err == nil {
		t.Error("expected error when target is not an embedding model")
	}
	defRepo.AssertNotCalled(t, "Set", mock.Anything, mock.Anything, mock.Anything)
}

func TestSetDefaultEmbedding_NotFound(t *testing.T) {
	p, _ := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "e1", Name: "E1", Type: ModelTypeEmbedding},
	})
	if err := p.SetDefaultEmbedding(context.Background(), "ghost"); err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestGetDefaultEmbeddingModel_FallbackFirst(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
		{ID: "e1", Name: "E1", Type: ModelTypeEmbedding},
	})
	stubNoDefaults(t, defRepo)
	m, err := p.GetDefaultEmbeddingModel(context.Background())
	if err != nil {
		t.Fatalf("GetDefaultEmbeddingModel: %v", err)
	}
	if m.ID != "e1" {
		t.Errorf("got %q, want e1", m.ID)
	}
}

func TestGetDefaultEmbeddingModel_None(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "l1", Name: "L1", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	if _, err := p.GetDefaultEmbeddingModel(context.Background()); err == nil {
		t.Error("expected error when no embedding model configured")
	}
}

func TestDeleteModel_Idempotent(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	if err := p.DeleteModel(context.Background(), "nonexistent"); err != nil {
		t.Errorf("idempotent delete should not error: %v", err)
	}
}

func TestDeleteModel_CleansDefaultRefs(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("Delete", mock.Anything, "m1").Return(nil).Once()
	defRepo.On("List", mock.Anything).Return([]modelconfig.ModelDefault{
		{UseCase: string(UseCaseChat), ModelID: "m1"},
	}, nil).Once()
	defRepo.On("Delete", mock.Anything, string(UseCaseChat)).Return(nil).Once()
	p := NewProvider(repo, defRepo, nil)
	if err := p.DeleteModel(context.Background(), "m1"); err != nil {
		t.Fatalf("DeleteModel: %v", err)
	}
	defRepo.AssertCalled(t, "Delete", mock.Anything, string(UseCaseChat))
}

func TestAddModel_AutoGenID(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	var inserted modelconfig.ModelEntry
	repo.On("Insert", mock.Anything, mock.Anything).Run(func(args mock.Arguments) {
		inserted = args.Get(1).(modelconfig.ModelEntry)
	}).Return(nil).Once()
	p := NewProvider(repo, defRepo, nil)
	saved, err := p.AddModel(context.Background(), modelconfig.ModelEntry{Name: "NewModel", Type: ModelTypeLLM})
	if err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if saved.ID == "" {
		t.Error("auto-generated ID should not be empty")
	}
	if inserted.ID == "" || inserted.Name != "NewModel" {
		t.Errorf("inserted entry wrong: %+v", inserted)
	}
	if inserted.Instruction == "" {
		t.Error("LLM entries should get the default instruction backfilled")
	}
}

func TestAddModel_InsertErrorPropagates(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("Insert", mock.Anything, mock.Anything).Return(errors.New("db down")).Once()
	p := NewProvider(repo, defRepo, nil)
	if _, err := p.AddModel(context.Background(), modelconfig.ModelEntry{ID: "x", Name: "X"}); err == nil {
		t.Error("expected insert error to propagate")
	}
}

func TestAddModel_PlaintextKeyRequiresVault(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	p := NewProvider(repo, defRepo, nil)
	_, err := p.AddModel(context.Background(), modelconfig.ModelEntry{ID: "x", Name: "X", APIKey: "sk-plaintext"})
	if err == nil {
		t.Error("expected error when plaintext API key provided without Vault")
	}
	repo.AssertNotCalled(t, "Insert", mock.Anything, mock.Anything)
}

func TestAddModel_VaultPathEncryption(t *testing.T) {
	repo := mockrepo.NewModelConfigRepository(t)
	vault := newFakeVault()
	repo.On("Insert", mock.Anything, mock.MatchedBy(func(e modelconfig.ModelEntry) bool {
		return e.APIKey == "data-agent/models/x/api_key"
	})).Return(nil).Once()
	p := NewProvider(repo, nil, vault)
	saved, err := p.AddModel(context.Background(), modelconfig.ModelEntry{ID: "x", Name: "X", APIKey: "sk-secret"})
	if err != nil {
		t.Fatalf("AddModel: %v", err)
	}
	if saved.APIKey != "data-agent/models/x/api_key" {
		t.Errorf("stored APIKey = %q, want Vault path", saved.APIKey)
	}
	if vault.stored["data-agent/models/x/api_key"] != "sk-secret" {
		t.Error("Vault should hold the plaintext secret")
	}
}

func TestUpdateModel_NotFound(t *testing.T) {
	p, _ := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	if _, err := p.UpdateModel(context.Background(), "ghost", modelconfig.ModelEntry{Name: "G"}); err == nil {
		t.Error("expected error for nonexistent model")
	}
}

func TestUpdateModel_KeepsVaultPathWhenEmptyKey(t *testing.T) {
	entries := []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, APIKey: "data-agent/models/m1/api_key"},
	}
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	vault := newFakeVault()
	vault.stored["data-agent/models/m1/api_key"] = "decrypted-secret"
	repo.On("Get", mock.Anything, "m1").Return(func(_ context.Context, _ string) *modelconfig.ModelEntry {
		cp := entries[0]
		return &cp
	}, nil).Maybe()
	repo.On("Update", mock.Anything, "m1", mock.MatchedBy(func(e modelconfig.ModelEntry) bool {
		return e.APIKey == "data-agent/models/m1/api_key" && e.Name == "Renamed"
	})).Return(nil).Once()
	p := NewProvider(repo, defRepo, vault)
	updated, err := p.UpdateModel(context.Background(), "m1", modelconfig.ModelEntry{Name: "Renamed"})
	if err != nil {
		t.Fatalf("UpdateModel: %v", err)
	}
	if updated.APIKey != "data-agent/models/m1/api_key" {
		t.Errorf("APIKey = %q, want existing Vault path kept", updated.APIKey)
	}
}

func TestDecryptModelAPIKey_NilVault(t *testing.T) {
	p, _ := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	if _, err := p.DecryptModelAPIKey(context.Background(), "m1"); err == nil {
		t.Error("expected error when vault is unavailable")
	}
}

func TestUnsetDefault_InvalidUseCase(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	if err := p.UnsetDefault(context.Background(), []string{"bogus"}); err == nil {
		t.Error("expected error for invalid use case")
	}
	defRepo.AssertNotCalled(t, "Delete", mock.Anything, mock.Anything)
}

func TestUnsetDefault_Success(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	defRepo.On("Delete", mock.Anything, string(UseCaseChat)).Return(nil).Once()
	if err := p.UnsetDefault(context.Background(), []string{string(UseCaseChat)}); err != nil {
		t.Fatalf("UnsetDefault: %v", err)
	}
	defRepo.AssertCalled(t, "Delete", mock.Anything, string(UseCaseChat))
}

func TestCompactionMaxTokens_Default(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, ContextLen: 0},
	})
	stubNoDefaults(t, defRepo)
	if got := p.CompactionMaxTokens(context.Background()); got != 4000 {
		t.Errorf("CompactionMaxTokens = %d, want 4000 (fallback)", got)
	}
}

func TestCompactionMaxTokens_HalfContext(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, ContextLen: 128000},
	})
	stubNoDefaults(t, defRepo)
	if got := p.CompactionMaxTokens(context.Background()); got != 64000 {
		t.Errorf("CompactionMaxTokens = %d, want 64000 (half context)", got)
	}
}

func TestCompactionMaxTokens_NilProvider(t *testing.T) {
	var p *Provider
	if got := p.CompactionMaxTokens(context.Background()); got != 4000 {
		t.Errorf("nil provider CompactionMaxTokens = %d, want 4000", got)
	}
}

func TestDefaultInstruction(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, Instruction: "You are helpful."},
	})
	stubNoDefaults(t, defRepo)
	if inst := p.DefaultInstruction(context.Background()); inst != "You are helpful." {
		t.Errorf("DefaultInstruction = %q, want 'You are helpful.'", inst)
	}
}

func TestDefaultInstruction_NoneSet(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM, Instruction: ""},
	})
	stubNoDefaults(t, defRepo)
	if inst := p.DefaultInstruction(context.Background()); inst != "" {
		t.Errorf("DefaultInstruction with no instruction = %q, want empty", inst)
	}
}

func TestEmbeddingConfig_FromDefaultEmbedding(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "e1", Name: "text-embed", Type: ModelTypeEmbedding, BaseURL: "http://emb:8082"},
	})
	stubNoDefaults(t, defRepo)
	cfg := p.EmbeddingConfig()
	if cfg.BaseURL != "http://emb:8082" {
		t.Errorf("BaseURL = %q, want http://emb:8082", cfg.BaseURL)
	}
	if cfg.Model != "text-embed" {
		t.Errorf("Model = %q, want text-embed", cfg.Model)
	}
}

func TestEmbeddingConfig_NilRepo(t *testing.T) {
	p := NewProvider(nil, nil, nil)
	cfg := p.EmbeddingConfig()
	if cfg.BaseURL != "" {
		t.Errorf("BaseURL = %q, want empty (no repo, no env)", cfg.BaseURL)
	}
}

func TestGetRawModelConfig(t *testing.T) {
	p, defRepo := newProviderWithModels(t, []modelconfig.ModelEntry{
		{ID: "m1", Name: "M1", Type: ModelTypeLLM},
	})
	stubNoDefaults(t, defRepo)
	raw, err := p.GetRawModelConfig(context.Background())
	if err != nil {
		t.Fatalf("GetRawModelConfig: %v", err)
	}
	if raw == nil {
		t.Fatal("expected non-nil raw config")
	}
	models, ok := raw["models"].([]modelconfig.ModelEntry)
	if !ok {
		t.Fatalf("expected 'models' key of []ModelEntry, got %T", raw["models"])
	}
	if len(models) != 1 || models[0].ID != "m1" {
		t.Errorf("unexpected models payload: %+v", models)
	}
}

func TestListAllModels_AttachesDefaults(t *testing.T) {
	entries := []modelconfig.ModelEntry{{ID: "m1", Name: "M1", Type: ModelTypeLLM}}
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(entries, int64(1), nil)
	defRepo.On("List", mock.Anything).Return([]modelconfig.ModelDefault{
		{UseCase: string(UseCaseChat), ModelID: "m1"},
	}, nil).Once()
	p := NewProvider(repo, defRepo, nil)
	all := p.ListAllModels(context.Background())
	if len(all) != 1 {
		t.Fatalf("ListAllModels = %d, want 1", len(all))
	}
	if len(all[0].IsDefaultFor) != 1 || all[0].IsDefaultFor[0] != string(UseCaseChat) {
		t.Errorf("IsDefaultFor = %v, want [chat]", all[0].IsDefaultFor)
	}
}

func TestConfigHash_EmptyOnMarshal(t *testing.T) {
	if h := ConfigHash(modelconfig.ModelEntry{ID: "x"}); h == "" {
		t.Error("valid model should produce non-empty hash")
	}
}

// newProviderWithRepo builds a Provider whose model repo serves no entries;
// mutations are registered by individual tests.
func newProviderWithRepo(t *testing.T) (*Provider, *mockrepo.ModelConfigRepository) {
	t.Helper()
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	repo.On("List", mock.Anything, mock.Anything, mock.Anything, mock.Anything).Return(nil, int64(0), nil)
	repo.On("Get", mock.Anything, mock.Anything).Return(nil, errNotFound)
	return NewProvider(repo, defRepo, nil), repo
}

// newProviderWithSearchRepo builds a Provider backed by mocks whose Search is
// NOT pre-stubbed, returning both repos so tests register Search expectations.
func newProviderWithSearchRepo(t *testing.T) (*Provider, *mockrepo.ModelConfigRepository, *mockrepo.ModelDefaultRepository) {
	t.Helper()
	repo := mockrepo.NewModelConfigRepository(t)
	defRepo := mockrepo.NewModelDefaultRepository(t)
	return NewProvider(repo, defRepo, nil), repo, defRepo
}

// stringSetContains reports whether every want id is present in got.
func stringSetContains(got []string, want ...string) bool {
	seen := map[string]bool{}
	for _, g := range got {
		seen[g] = true
	}
	for _, w := range want {
		if !seen[w] {
			return false
		}
	}
	return true
}

func TestSearchLLMModels_PassesDefaultIDsAndLimit(t *testing.T) {
	p, repo, defRepo := newProviderWithSearchRepo(t)
	// chat defaults to m1, task defaults to m2 — the search contract
	// scopes defaultIDs to the chat use case only, so the LLM dropdown
	// surfaces the chat default at the top and never leaks the task
	// default up into the chat selector.
	defRepo.On("List", mock.Anything).Return([]modelconfig.ModelDefault{
		{UseCase: string(UseCaseChat), ModelID: "m1"},
		{UseCase: string(UseCaseTask), ModelID: "m2"},
	}, nil).Maybe()

	var gotQ string
	var gotLimit int64
	var gotType modelconfig.ModelType
	var gotDefaultIDs []string
	repo.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotType = args.Get(1).(modelconfig.ModelType)
			gotQ = args.Get(2).(string)
			gotDefaultIDs = args.Get(3).([]string)
			gotLimit = args.Get(5).(int64)
		}).
		Return([]modelconfig.ModelEntry{{ID: "m1", Name: "M1", Type: ModelTypeLLM}}, int64(1), nil).Once()

	models, total, err := p.SearchLLMModels(context.Background(), "gpt", 30)
	if err != nil {
		t.Fatalf("SearchLLMModels: %v", err)
	}
	if gotType != ModelTypeLLM {
		t.Errorf("type = %q, want llm", gotType)
	}
	if gotQ != "gpt" {
		t.Errorf("q = %q, want gpt", gotQ)
	}
	if gotLimit != 30 {
		t.Errorf("limit = %d, want 30", gotLimit)
	}
	// Chat-scoped defaults → only the chat default (m1) is promoted;
	// task default (m2) stays out of the LLM dropdown.
	if len(gotDefaultIDs) != 1 || gotDefaultIDs[0] != "m1" {
		t.Errorf("defaultIDs = %v, want [m1] (chat default only)", gotDefaultIDs)
	}
	if total != 1 || len(models) != 1 || models[0].ID != "m1" {
		t.Errorf("result = total %d len %d, want 1/1 m1", total, len(models))
	}
}

func TestSearchLLMModels_DefaultLimitWhenZero(t *testing.T) {
	p, repo, defRepo := newProviderWithSearchRepo(t)
	defRepo.On("List", mock.Anything).Return([]modelconfig.ModelDefault{}, nil).Maybe()

	var gotLimit int64
	repo.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { gotLimit = args.Get(5).(int64) }).
		Return([]modelconfig.ModelEntry{}, int64(0), nil).Once()

	if _, _, err := p.SearchLLMModels(context.Background(), "", 0); err != nil {
		t.Fatalf("SearchLLMModels: %v", err)
	}
	if gotLimit != 20 {
		t.Errorf("limit = %d, want 20 (default)", gotLimit)
	}
}

func TestSearchLLMModels_LimitClampedTo100(t *testing.T) {
	p, repo, defRepo := newProviderWithSearchRepo(t)
	defRepo.On("List", mock.Anything).Return([]modelconfig.ModelDefault{}, nil).Maybe()

	var gotLimit int64
	repo.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) { gotLimit = args.Get(5).(int64) }).
		Return([]modelconfig.ModelEntry{}, int64(0), nil).Once()

	if _, _, err := p.SearchLLMModels(context.Background(), "", 500); err != nil {
		t.Fatalf("SearchLLMModels: %v", err)
	}
	if gotLimit != 100 {
		t.Errorf("limit = %d, want 100 (clamped)", gotLimit)
	}
}

func TestSearchEmbeddingModels_PassesEmbeddingType(t *testing.T) {
	p, repo, defRepo := newProviderWithSearchRepo(t)
	defRepo.On("List", mock.Anything).Return([]modelconfig.ModelDefault{
		{UseCase: string(UseCaseEmbedding), ModelID: "e1"},
	}, nil).Maybe()

	var gotType modelconfig.ModelType
	var gotDefaultIDs []string
	repo.On("Search", mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything, mock.Anything).
		Run(func(args mock.Arguments) {
			gotType = args.Get(1).(modelconfig.ModelType)
			gotDefaultIDs = args.Get(3).([]string)
		}).
		Return([]modelconfig.ModelEntry{{ID: "e1", Name: "E1", Type: ModelTypeEmbedding}}, int64(1), nil).Once()

	models, _, err := p.SearchEmbeddingModels(context.Background(), "nomic", 20)
	if err != nil {
		t.Fatalf("SearchEmbeddingModels: %v", err)
	}
	if gotType != ModelTypeEmbedding {
		t.Errorf("type = %q, want embedding", gotType)
	}
	if !stringSetContains(gotDefaultIDs, "e1") {
		t.Errorf("defaultIDs = %v, want contains e1", gotDefaultIDs)
	}
	if len(models) != 1 || models[0].ID != "e1" {
		t.Errorf("models = %+v, want single e1", models)
	}
}

func TestSearchLLMModels_NilRepo(t *testing.T) {
	p := NewProvider(nil, nil, nil)
	models, total, err := p.SearchLLMModels(context.Background(), "x", 20)
	if err != nil {
		t.Fatalf("SearchLLMModels nil repo should not error, got %v", err)
	}
	if len(models) != 0 || total != 0 {
		t.Errorf("nil repo result = len %d total %d, want 0/0", len(models), total)
	}
}
