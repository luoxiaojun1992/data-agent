// Package modelcfg provides a unified model configuration layer that reads
// LLM and embedding models from MongoDB system_config (admin model-config page)
// with environment variable fallbacks. It replaces the env-only model wiring
// in cmd/server/main.go with a config-driven Provider used by initServices.
package modelcfg

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"google.golang.org/adk/model"

	"github.com/ieshan/adk-go-pkg/model/openai"
	adkmodel "github.com/luoxiaojun1992/data-agent/internal/adk/model"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// ModelType distinguishes LLM and Embedding models.
type ModelType string

const (
	ModelTypeLLM       ModelType = "llm"
	ModelTypeEmbedding ModelType = "embedding"
)

// UseCase identifies the intended use for a model.
type UseCase string

const (
	UseCaseChat       UseCase = "chat"
	UseCaseTask       UseCase = "task"
	UseCaseEnhance    UseCase = "enhance"
	UseCaseCompaction UseCase = "compaction"
	UseCaseKBChunking UseCase = "kb_chunking"
	UseCaseEmbedding  UseCase = "embedding"
)

// ModelEntry describes one model in the admin config.
type ModelEntry struct {
	ID              string    `json:"id"`              // unique identifier (UUID or slug); backfilled from Name when empty (legacy compat)
	Name            string    `json:"name"`
	BaseURL         string    `json:"base_url"`
	APIKey          string    `json:"api_key,omitempty"` // On input: plaintext (from frontend). On persisted output: Vault reference path. Resolved to plaintext in memory by Provider.models() before use.
	Type            ModelType `json:"type"`
	Instruction     string    `json:"instruction"` // LLM only
	Capability      string    `json:"capability"`  // LLM only
	UseCases        []string  `json:"use_cases"`   // declared capabilities (informational)
	TokenMultiplier float64   `json:"token_multiplier"`
	Temperature     float64   `json:"temperature"` // LLM only
	MaxTokens       int       `json:"max_tokens"`  // LLM only
	IsDefault       bool      `json:"is_default"`      // legacy global default (backward compat); prefer IsDefaultFor
	IsDefaultFor    []string  `json:"is_default_for"`  // per-use-case default: ["chat","enhance","compaction","task"]; each use case has exactly one default model
	FallbackOrder   int       `json:"fallback_order"`
}

// VaultStore is the minimal Vault interface the Provider needs for per-model
// API key encryption. The concrete *vault.Client satisfies this; tests can
// inject a fake. Nil-safe: nil vault means plaintext fallback only.
type VaultStore interface {
	Store(ctx context.Context, path, value string) error
	Retrieve(ctx context.Context, path string) (string, error)
}

// EmbeddingEntry describes the embedding model config.
type EmbeddingEntry struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"` // Same dual semantics as ModelEntry.APIKey.
}

// Provider reads model configurations from system_config with env fallback.
// It is the single source of truth for building the ADK model.LLM chain and
// retrieving the agent's system instruction.
type Provider struct {
	repo  repository.SysConfigRepository
	vault VaultStore // optional; when non-nil, per-model API keys are stored/retrieved via Vault
	cfgNS string     // system_config namespace, default "model"
}

// NewProvider creates a config provider. Passing nil repo means "env only".
// Pass a non-nil vault to enable per-model API key encryption.
func NewProvider(repo repository.SysConfigRepository, vault VaultStore) *Provider {
	return &Provider{repo: repo, vault: vault, cfgNS: "model"}
}

// ModelAPIKeyVaultPath returns the Vault KV v2 path used to store the API key
// for the model with the given ID. Stable across reloads so the same model
// always points to the same Vault entry.
func ModelAPIKeyVaultPath(modelID string) string {
	return "data-agent/models/" + modelID + "/api_key"
}

// ---- LLM models ----

const defaultBaseURL = "https://api.openai.com/v1"

// models returns the configured LLM model list. DB has priority; env fallback.
// Empty IDs are backfilled from Name (legacy compat) so every entry has a
// stable identifier after read. Per-model API keys are resolved from Vault
// references at load time. API keys are NOT shared — each model must have
// its own key. BaseURL falls back to the legacy flat config when empty.
// Empty Type is treated as "llm" (legacy compat).
func (p *Provider) models() []ModelEntry {
	entries := p.modelsFromDB()
	if len(entries) > 0 {
		legacyBaseURL := p.legacyCfgValue("api_url")
		ctx := context.Background()
		for i := range entries {
			if entries[i].Type == "" {
				entries[i].Type = ModelTypeLLM
			}
			p.applyEnvDefaults(&entries[i])
			p.resolveAPIKey(ctx, &entries[i])
			if entries[i].BaseURL == "" && legacyBaseURL != "" {
				entries[i].BaseURL = legacyBaseURL
			}
			p.backfillID(&entries[i])
		}
		return entries
	}
	entries = p.modelsFromEnv()
	for i := range entries {
		p.backfillID(&entries[i])
	}
	return entries
}

// resolveAPIKey transparently decrypts a Vault reference into plaintext.
// When the field already looks like plaintext (no prefix), it's left as-is
// so legacy callers/tests keep working. When vault is unavailable but the
// field is a Vault path, it stays as-is (will surface as an auth error at
// chat time, which is easier to diagnose than silently dropping).
func (p *Provider) resolveAPIKey(ctx context.Context, m *ModelEntry) {
	if m.APIKey == "" {
		return
	}
	if !looksLikeVaultPath(m.APIKey) {
		return // already plaintext
	}
	if p.vault == nil {
		return // Vault path stays — will fail at auth time
	}
	plain, err := p.vault.Retrieve(ctx, m.APIKey)
	if err == nil && plain != "" {
		m.APIKey = plain
	}
	// Decrypt failure keeps the path — auth error will surface at LLM call time.
}

// looksLikeVaultPath reports whether the string resembles one of our Vault
// KV v2 paths (e.g. "data-agent/models/<id>/api_key"). Used to distinguish
// references from plaintext so we only call Vault.Retrieve when needed.
func looksLikeVaultPath(s string) bool {
	return strings.HasPrefix(s, "data-agent/")
}

// legacyCfgValue returns a single legacy flat-config value from the same
// namespace. Returns "" when the key is not found or the repo is nil.
func (p *Provider) legacyCfgValue(key string) string {
	if p.repo == nil {
		return ""
	}
	cfg, err := p.repo.Get(context.Background(), p.cfgNS, key)
	if err != nil || cfg == nil {
		return ""
	}
	return cfg.Value
}

// legacyConfig returns the legacy flat config map (api_url/api_key/...) from
// the same namespace. Used as a fallback source when structured models have
// empty fields that the json:"-" tag prevented from persisting.
func (p *Provider) legacyConfig() map[string]string {
	out := map[string]string{}
	if p.repo == nil {
		return out
	}
	cfgs, err := p.repo.GetAll(context.Background(), p.cfgNS)
	if err != nil {
		return out
	}
	for _, c := range cfgs {
		if c.Key == "models" || c.Key == "embedding" {
			continue // skip the structured list JSON blobs
		}
		out[c.Key] = c.Value
	}
	return out
}

// backfillID sets ID = Name when ID is empty (legacy config compat). After
// admin edits and saves, a proper UUID is generated server-side.
func (p *Provider) backfillID(m *ModelEntry) {
	if m.ID == "" {
		m.ID = m.Name
	}
}

// modelsFromDB deserializes the "models" key from the config namespace.
func (p *Provider) modelsFromDB() []ModelEntry {
	if p.repo == nil {
		return nil
	}
	cfg, err := p.repo.Get(context.Background(), p.cfgNS, "models")
	if err != nil || cfg == nil || cfg.Value == "" {
		return nil
	}
	var entries []ModelEntry
	if json.Unmarshal([]byte(cfg.Value), &entries) != nil {
		return nil
	}
	return entries
}

// modelsFromEnv builds a single-model list from env, with optional fallback chain.
func (p *Provider) modelsFromEnv() []ModelEntry {
	primary := ModelEntry{
		Name:            envOrDefault("LLM_MODEL", "mock-gpt-4o"),
		BaseURL:         envOrDefault("LLM_BASE_URL", defaultBaseURL),
		APIKey:          os.Getenv("LLM_API_KEY"),
		Type:            ModelTypeLLM,
		UseCases:        []string{"chat", "task", "enhance", "compaction"},
		IsDefaultFor:    []string{"chat"},
		Instruction:     "",
		Capability:      "",
		TokenMultiplier: 1.0,
		Temperature:     0.7,
		MaxTokens:       4096,
		IsDefault:       true,
		FallbackOrder:   0,
	}
	entries := []ModelEntry{primary}
	if raw := os.Getenv("LLM_FALLBACK_BASE_URLS"); raw != "" {
		for i, u := range splitEnvList(raw) {
			entries = append(entries, ModelEntry{
				Name:            primary.Name,
				BaseURL:         u,
				APIKey:          primary.APIKey,
				Type:            ModelTypeLLM,
				UseCases:        primary.UseCases,
				TokenMultiplier: 1.0,
				Temperature:     primary.Temperature,
				MaxTokens:       primary.MaxTokens,
				FallbackOrder:   i + 1,
			})
		}
	}
	return entries
}

// applyEnvDefaults fills zero values from env (per-model override).
func (p *Provider) applyEnvDefaults(m *ModelEntry) {
	if m.BaseURL == "" {
		m.BaseURL = envOrDefault("LLM_BASE_URL", defaultBaseURL)
	}
	if m.APIKey == "" {
		m.APIKey = os.Getenv("LLM_API_KEY")
	}
	if m.Temperature == 0 {
		m.Temperature = 0.7
	}
	if m.MaxTokens == 0 {
		m.MaxTokens = 4096
	}
	if m.TokenMultiplier == 0 {
		m.TokenMultiplier = 1.0
	}
}

// BuildLLM constructs an LLM from the model designated as default for the
// given use case. When useCase is empty, uses the chat default.
// System processes (enhance, compaction, memory, kb_chunking) MUST go through
// this path — they are not allowed to pick arbitrary models by their UseCases
// declaration field. Empty useCase falls back to chat.
func (p *Provider) BuildLLM(ctx context.Context, useCase UseCase) (model.LLM, error) {
	if useCase == "" {
		useCase = UseCaseChat
	}
	entry, err := p.GetModelByUseCase(ctx, useCase)
	if err != nil {
		return nil, err
	}
	backends := p.buildBackends([]ModelEntry{*entry})
	if len(backends) == 0 {
		return nil, fmt.Errorf("failed to build LLM for use case %q", useCase)
	}
	return backends[0], nil
}

// GetModelByUseCase returns the model designated as default for the given
// use case. Priority: 1) is_default_for contains use case, 2) legacy is_default,
// 3) first LLM model. Each use case MUST have exactly one default model.
func (p *Provider) GetModelByUseCase(ctx context.Context, useCase UseCase) (*ModelEntry, error) {
	models := p.models()
	if len(models) == 0 {
		return nil, fmt.Errorf("no models configured")
	}
	// 1. Per-use-case default: find model with this use case in is_default_for.
	for i := range models {
		if models[i].Type == ModelTypeLLM && isDefaultForUseCase(models[i], string(useCase)) {
			return &models[i], nil
		}
	}
	// 2. Legacy global default fallback.
	for i := range models {
		if models[i].IsDefault && models[i].Type == ModelTypeLLM {
			return &models[i], nil
		}
	}
	// 3. First LLM fallback.
	for i := range models {
		if models[i].Type == ModelTypeLLM {
			return &models[i], nil
		}
	}
	return nil, fmt.Errorf("no model for use case %q", useCase)
}

// isDefaultForUseCase reports whether the model is designated as the explicit
// default for the given use case.
func isDefaultForUseCase(m ModelEntry, useCase string) bool {
	for _, uc := range m.IsDefaultFor {
		if uc == useCase {
			return true
		}
	}
	return false
}

// selectModelsByUseCase returns candidates for a use case. Now delegates to
// the per-use-case default model via GetModelByUseCase.
func (p *Provider) selectModelsByUseCase(models []ModelEntry, useCase UseCase) []ModelEntry {
	entry, err := p.GetModelByUseCase(context.Background(), useCase)
	if err != nil {
		// If no model found for this use case, return all models as fallback.
		return filterLLMs(models)
	}
	return []ModelEntry{*entry}
}

// filterLLMs returns only LLM-type models from the list.
func filterLLMs(models []ModelEntry) []ModelEntry {
	var out []ModelEntry
	for _, m := range models {
		if m.Type == ModelTypeLLM {
			out = append(out, m)
		}
	}
	return out
}

// buildBackends creates the model.LLM chain sorted by FallbackOrder.
func (p *Provider) buildBackends(models []ModelEntry) []model.LLM {
	sortModels(models)
	backends := make([]model.LLM, 0, len(models))
	for _, m := range models {
		llm, err := openai.New(openai.Config{
			Model:   m.Name,
			BaseURL: m.BaseURL,
			APIKey:  m.APIKey,
		})
		if err != nil {
			continue
		}
		backends = append(backends, adkmodel.NewCompatLLM(llm))
	}
	return backends
}

// sortModelsByCost sorts candidates by token cost (ascending).
func sortModelsByCost(entries []ModelEntry) {
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[j].TokenMultiplier < entries[i].TokenMultiplier {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

// DefaultInstruction returns the system prompt of the model that is default
// for the chat use case (or the global default if no per-use-case default is set).
func (p *Provider) DefaultInstruction(ctx context.Context) string {
	m, err := p.DefaultModel(ctx)
	if err == nil && m.Instruction != "" {
		return m.Instruction
	}
	return ""
}

// DefaultModel returns the model designated as default for the "chat" use case.
// Falls back to legacy is_default, then first LLM.
func (p *Provider) DefaultModel(ctx context.Context) (*ModelEntry, error) {
	return p.GetModelByUseCase(ctx, UseCaseChat)
}

// GetModelByID returns the model entry with the given ID. When modelID is
// empty, returns the default LLM model (backward compat). Returns an error
// when the ID is not found.
func (p *Provider) GetModelByID(ctx context.Context, modelID string) (*ModelEntry, error) {
	if modelID == "" {
		return p.DefaultModel(ctx)
	}
	models := p.models()
	for i := range models {
		if models[i].ID == modelID {
			return &models[i], nil
		}
	}
	return nil, fmt.Errorf("model %q not found", modelID)
}

// BuildLLMByID constructs an LLM from the model entry matching modelID.
// When modelID is empty, uses the default LLM model. This is the per-model
// construction path used by the Runtime registry (SPEC-062).
func (p *Provider) BuildLLMByID(ctx context.Context, modelID string) (model.LLM, error) {
	entry, err := p.GetModelByID(ctx, modelID)
	if err != nil {
		return nil, err
	}
	backends := p.buildBackends([]ModelEntry{*entry})
	if len(backends) == 0 {
		return nil, fmt.Errorf("failed to build LLM for model %q", entry.ID)
	}
	return backends[0], nil
}

// ListLLMModels returns the Type==llm model entries (paginated in memory).
// Returns (models, total, error) where total is the full LLM count. page
// starts at 1; pageSize is clamped to [1, 100].
func (p *Provider) ListLLMModels(ctx context.Context, page, pageSize int) ([]ModelEntry, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	all := p.models()
	var llmModels []ModelEntry
	for _, m := range all {
		if m.Type == ModelTypeLLM {
			llmModels = append(llmModels, m)
		}
	}
	total := len(llmModels)
	offset := (page - 1) * pageSize
	if offset >= total {
		return []ModelEntry{}, total, nil
	}
	end := offset + pageSize
	if end > total {
		end = total
	}
	return llmModels[offset:end], total, nil
}

// ConfigHash returns a sha256 hex digest of the JSON-serialized model entry.
// The Runtime registry uses this fingerprint to detect config changes and
// rebuild cached Runtime instances (hot-reload without Pub/Sub).
func ConfigHash(m ModelEntry) string {
	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// AddModel appends a single model entry, generating a UUID ID when empty,
// then persists the full list. Maintains the IsDefault invariant.
//
// When entry.APIKey is plaintext (e.g. from the admin POST body) and the
// Provider has a Vault client configured, the plaintext is stored in Vault
// at a per-model path and the entry's APIKey field is replaced with the
// Vault reference path. When vault is unavailable, the plaintext is kept
// as-is and persisted to MongoDB (legacy plaintext path) so users on a
// dev/test setup without Vault are not blocked.
func (p *Provider) AddModel(ctx context.Context, entry ModelEntry) (ModelEntry, error) {
	if p.repo == nil {
		return entry, fmt.Errorf("config repository not available")
	}
	if entry.ID == "" {
		entry.ID = "model_" + newUUID()
	}
	// Encrypt plaintext API key into a Vault reference.
	if entry.APIKey != "" && !looksLikeVaultPath(entry.APIKey) {
		if p.vault == nil {
			return entry, fmt.Errorf("vault client is required to store API keys; ensure VAULT_ADDR/VAULT_TOKEN are configured")
		}
		path := ModelAPIKeyVaultPath(entry.ID)
		if err := p.vault.Store(ctx, path, entry.APIKey); err != nil {
			return entry, fmt.Errorf("vault store api_key for %q: %w", entry.ID, err)
		}
		entry.APIKey = path
	}
	models := p.models()
	for _, m := range models {
		if m.ID == entry.ID {
			return entry, fmt.Errorf("model ID %q already exists", entry.ID)
		}
	}
	models = append(models, entry)
	if err := p.SetModels(ctx, models); err != nil {
		return entry, err
	}
	return entry, nil
}

// DecryptModelAPIKey retrieves the plaintext API key for a model from Vault.
// Reads directly from DB (without Vault resolution) to get the Vault path,
// then decrypts from Vault. Returns an error if no Vault-stored key exists.
func (p *Provider) DecryptModelAPIKey(ctx context.Context, modelID string) (string, error) {
	if p.vault == nil {
		return "", fmt.Errorf("vault client not available")
	}
	rawModels := p.modelsFromDB()
	for _, m := range rawModels {
		if m.ID == modelID {
			if m.APIKey == "" || !looksLikeVaultPath(m.APIKey) {
				return "", fmt.Errorf("model %q has no Vault-stored API key", modelID)
			}
			return p.vault.Retrieve(ctx, m.APIKey)
		}
	}
	return "", fmt.Errorf("model %q not found", modelID)
}

// UpdateModel updates an existing model entry by ID. When api_key is provided
// and is plaintext, it is stored in Vault and replaced with the Vault path.
// When api_key is empty, the existing Vault path is preserved from DB.
// The returned model NEVER exposes the plaintext API key.
func (p *Provider) UpdateModel(ctx context.Context, id string, entry ModelEntry) (ModelEntry, error) {
	if p.repo == nil {
		return entry, fmt.Errorf("config repository not available")
	}
	models := p.modelsFromDB()
	if models == nil {
		models = p.models()
	}
	found := false
	for i := range models {
		if models[i].ID == id {
			entry.ID = id
			if entry.APIKey == "" {
				entry.APIKey = models[i].APIKey
			} else if !looksLikeVaultPath(entry.APIKey) {
				if p.vault == nil {
					return entry, fmt.Errorf("vault client is required to store API keys; ensure VAULT_ADDR/VAULT_TOKEN are configured")
				}
				path := ModelAPIKeyVaultPath(id)
				if err := p.vault.Store(ctx, path, entry.APIKey); err != nil {
					return entry, fmt.Errorf("vault store api_key for %q: %w", id, err)
				}
				entry.APIKey = path
			}
			models[i] = entry
			found = true
			break
		}
	}
	if !found {
		return entry, fmt.Errorf("model %q not found", id)
	}
	if err := p.SetModels(ctx, models); err != nil {
		return entry, err
	}
	entry.APIKey = "••••••••••"
	return entry, nil
}

// DeleteModel removes the model with the given ID from the list. Idempotent:
// deleting a non-existent ID is a no-op (returns nil).
func (p *Provider) DeleteModel(ctx context.Context, id string) error {
	if p.repo == nil {
		return fmt.Errorf("config repository not available")
	}
	models := p.models()
	var removedUseCases []string
	var removedGlobalDefault bool
	kept := make([]ModelEntry, 0, len(models))
	for _, m := range models {
		if m.ID == id {
			removedGlobalDefault = m.IsDefault
			removedUseCases = append(removedUseCases, m.IsDefaultFor...)
			continue
		}
		kept = append(kept, m)
	}
	if len(kept) == len(models) {
		return nil // idempotent delete
	}
	// Promote defaults as needed.
	if removedGlobalDefault {
		ensureSingleDefault(kept)
	}
	ensurePerUseCaseDefaults(kept, removedUseCases)
	return p.SetModels(ctx, kept)
}

// SetDefaultModel marks the model with :id as default for the given use cases.
// When useCases is empty or contains "chat", it sets the legacy is_default flag
// for backward compat. For specific use cases, it sets is_default_for.
// Clearing defaults on other models is done per-use-case.
func (p *Provider) SetDefaultModel(ctx context.Context, id string, useCases []string) error {
	if p.repo == nil {
		return fmt.Errorf("config repository not available")
	}
	models := p.models()
	found := false
	for i := range models {
		if models[i].Type != ModelTypeLLM {
			continue
		}
		if models[i].ID == id {
			// Set legacy global default if requested or no specific use cases.
			if len(useCases) == 0 || containsStr(useCases, "chat") {
				models[i].IsDefault = true
			}
			// Set per-use-case defaults.
			for _, uc := range useCases {
				if !isDefaultForUseCase(models[i], uc) {
					models[i].IsDefaultFor = append(models[i].IsDefaultFor, uc)
				}
			}
			found = true
		} else {
			// Clear defaults on other models for these use cases.
			if len(useCases) == 0 || containsStr(useCases, "chat") {
				models[i].IsDefault = false
			}
			models[i].IsDefaultFor = removeStrAll(models[i].IsDefaultFor, useCases)
		}
	}
	if !found {
		return fmt.Errorf("LLM model %q not found", id)
	}
	return p.SetModels(ctx, models)
}

// SetDefaultModelPlain marks a model as the default for chat (legacy compat).
func (p *Provider) setDefaultModelPlain(ctx context.Context, id string) error {
	return p.SetDefaultModel(ctx, id, []string{"chat"})
}

// newUUID generates a UUID v4 string. Isolated so tests can stub it.
var newUUID = func() string {
	return generateUUID()
}

// generateUUID is the default UUID generator.
func generateUUID() string {
	b := make([]byte, 16)
	if _, err := readRand(b); err != nil {
		return fmt.Sprintf("%d", os.Getpid()) // fallback, unlikely path
	}
	b[6] = (b[6] & 0x0f) | 0x40 // version 4
	b[8] = (b[8] & 0x3f) | 0x80 // variant
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

// readRand reads random bytes (wraps crypto/rand.Read for testability).
var readRand = rand.Read

// ---- Embedding ----

// EmbeddingConfig returns the embedding model config, DB priority, env fallback.
func (p *Provider) EmbeddingConfig() EmbeddingEntry {
	cfg := p.embeddingFromDB()
	p.applyEmbeddingDefaults(&cfg)
	return cfg
}

func (p *Provider) embeddingFromDB() EmbeddingEntry {
	if p.repo == nil {
		return EmbeddingEntry{}
	}
	cfg, err := p.repo.Get(context.Background(), p.cfgNS, "embedding")
	if err != nil || cfg == nil || cfg.Value == "" {
		return EmbeddingEntry{}
	}
	var e EmbeddingEntry
	if json.Unmarshal([]byte(cfg.Value), &e) != nil {
		return EmbeddingEntry{}
	}
	return e
}

func (p *Provider) applyEmbeddingDefaults(e *EmbeddingEntry) {
	if e.BaseURL == "" {
		e.BaseURL = os.Getenv("EMBEDDING_BASE_URL")
	}
	if e.Model == "" {
		e.Model = envOrDefault("EMBEDDING_MODEL", "nomic-embed-text")
	}
	if e.APIKey == "" {
		e.APIKey = os.Getenv("EMBEDDING_API_KEY")
	}
}

// ---- Admin API helpers ----

// SetModels serializes and stores the model list (admin PUT). It validates
// ID uniqueness (after backfilling empty IDs from Name) and maintains the
// IsDefault invariant: exactly one LLM model has IsDefault==true when LLM
// models exist. The first LLM model is auto-marked default when none is.
func (p *Provider) SetModels(ctx context.Context, entries []ModelEntry) error {
	if p.repo == nil {
		return fmt.Errorf("config repository not available")
	}
	for i := range entries {
		p.backfillID(&entries[i])
	}
	if err := validateModelIDs(entries); err != nil {
		return err
	}
	ensureSingleDefault(entries)
	raw, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal models: %w", err)
	}
	return p.repo.Upsert(ctx, p.cfgNS, "models", string(raw))
}

// validateModelIDs rejects duplicate IDs within a model list.
func validateModelIDs(entries []ModelEntry) error {
	seen := make(map[string]bool, len(entries))
	for _, m := range entries {
		if m.ID == "" {
			return fmt.Errorf("model entry has empty ID after backfill")
		}
		if seen[m.ID] {
			return fmt.Errorf("duplicate model ID %q", m.ID)
		}
		seen[m.ID] = true
	}
	return nil
}

// ensureSingleDefault guarantees at most one LLM model is marked IsDefault.
// When no LLM model is default, the first LLM model is auto-marked.
func ensureSingleDefault(entries []ModelEntry) {
	firstLLM := -1
	defaultLLM := -1
	for i, m := range entries {
		if m.Type != ModelTypeLLM {
			continue
		}
		if firstLLM < 0 {
			firstLLM = i
		}
		if m.IsDefault {
			if defaultLLM >= 0 {
				entries[i].IsDefault = false // collapse extras
			} else {
				defaultLLM = i
			}
		}
	}
	if defaultLLM < 0 && firstLLM >= 0 {
		entries[firstLLM].IsDefault = true
	}
}

// SetEmbedding serializes and stores the embedding config (admin PUT).
func (p *Provider) SetEmbedding(ctx context.Context, e EmbeddingEntry) error {
	if p.repo == nil {
		return fmt.Errorf("config repository not available")
	}
	raw, err := json.Marshal(e)
	if err != nil {
		return fmt.Errorf("marshal embedding: %w", err)
	}
	return p.repo.Upsert(ctx, p.cfgNS, "embedding", string(raw))
}

// GetRawModelConfig returns all raw config for the admin GET endpoint,
// including legacy flat keys and the new structured models/embedding keys.
func (p *Provider) GetRawModelConfig(ctx context.Context) (map[string]any, error) {
	flat := map[string]any{}
	if p.repo != nil {
		cfgs, _ := p.repo.GetAll(ctx, p.cfgNS)
		for _, c := range cfgs {
			flat[c.Key] = c.Value
		}
	}
	// Decode structured values for the API response.
	if raw, ok := flat["models"]; ok {
		var models []ModelEntry
		if err := json.Unmarshal([]byte(raw.(string)), &models); err == nil {
			flat["models"] = models
		}
	}
	if raw, ok := flat["embedding"]; ok {
		var emb EmbeddingEntry
		if err := json.Unmarshal([]byte(raw.(string)), &emb); err == nil {
			flat["embedding"] = emb
		}
	}
	fillLegacyDefaults(flat)
	return flat, nil
}

// fillLegacyDefaults applies env/static defaults for flat keys (backward compat).
func fillLegacyDefaults(result map[string]any) {
	defaults := map[string]string{
		"api_url":      envOrDefault("LLM_BASE_URL", defaultBaseURL),
		"model_name":   envOrDefault("LLM_MODEL", "gpt-4o"),
		"context_len":  "128000",
		"max_output":   "16000",
		"temperature":  "0.7",
		"top_p":        "0.95",
		"hermes_url":   "http://hermes:8081",
		"hermes_model": "hermes-3-70b",
	}
	for k, v := range defaults {
		if _, ok := result[k]; !ok {
			result[k] = v
		}
	}
}

// ---- helpers ----

func sortModels(entries []ModelEntry) {
	for i := 0; i < len(entries)-1; i++ {
		for j := i + 1; j < len(entries); j++ {
			if entries[i].FallbackOrder > entries[j].FallbackOrder {
				entries[i], entries[j] = entries[j], entries[i]
			}
		}
	}
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func splitEnvList(raw string) []string {
	parts := []string{}
	for _, s := range splitByComma(raw) {
		s = trimSpace(s)
		if s != "" {
			parts = append(parts, s)
		}
	}
	return parts
}

func splitByComma(s string) []string {
	out := []string{}
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == ',' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

func trimSpace(s string) string {
	i := 0
	j := len(s) - 1
	for i <= j && s[i] == ' ' {
		i++
	}
	for j >= i && s[j] == ' ' {
		j--
	}
	return s[i : j+1]
}

// containsStr reports whether s is in the list.
func containsStr(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}

// removeStrAll removes all occurrences of given strings from the list.
func removeStrAll(list, toRemove []string) []string {
	if len(toRemove) == 0 {
		return list
	}
	removeSet := make(map[string]bool, len(toRemove))
	for _, s := range toRemove {
		removeSet[s] = true
	}
	var out []string
	for _, v := range list {
		if !removeSet[v] {
			out = append(out, v)
		}
	}
	return out
}

// ensurePerUseCaseDefaults promotes first LLM as default for orphaned use cases.
func ensurePerUseCaseDefaults(entries []ModelEntry, orphanedUseCases []string) {
	if len(orphanedUseCases) == 0 {
		return
	}
	firstLLM := -1
	for i, m := range entries {
		if m.Type == ModelTypeLLM && firstLLM < 0 {
			firstLLM = i
			break
		}
	}
	if firstLLM < 0 {
		return
	}
	for _, uc := range orphanedUseCases {
		if !isDefaultForUseCase(entries[firstLLM], uc) {
			entries[firstLLM].IsDefaultFor = append(entries[firstLLM].IsDefaultFor, uc)
		}
	}
}
