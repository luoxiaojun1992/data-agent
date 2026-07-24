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
	ID              string    `json:"id"` // unique identifier (UUID or slug); backfilled from Name when empty (legacy compat)
	Name            string    `json:"name"`
	BaseURL         string    `json:"base_url"`
	APIKey          string    `json:"-"` // Vault encrypt
	Type            ModelType `json:"type"`
	Instruction     string    `json:"instruction"` // LLM only
	Capability      string    `json:"capability"`  // LLM only
	UseCases        []string  `json:"use_cases"`
	TokenMultiplier float64   `json:"token_multiplier"`
	Temperature     float64   `json:"temperature"` // LLM only
	MaxTokens       int       `json:"max_tokens"`  // LLM only
	IsDefault       bool      `json:"is_default"`
	FallbackOrder   int       `json:"fallback_order"`
}

// EmbeddingEntry describes the embedding model config.
type EmbeddingEntry struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"-"`
}

// Provider reads model configurations from system_config with env fallback.
// It is the single source of truth for building the ADK model.LLM chain and
// retrieving the agent's system instruction.
type Provider struct {
	repo  repository.SysConfigRepository
	cfgNS string // system_config namespace, default "model"
}

// NewProvider creates a config provider. Passing nil repo means "env only".
func NewProvider(repo repository.SysConfigRepository) *Provider {
	return &Provider{repo: repo, cfgNS: "model"}
}

// ---- LLM models ----

const defaultBaseURL = "https://api.openai.com/v1"

// models returns the configured LLM model list. DB has priority; env fallback.
// Empty IDs are backfilled from Name (legacy compat) so every entry has a
// stable identifier after read.
func (p *Provider) models() []ModelEntry {
	entries := p.modelsFromDB()
	if len(entries) > 0 {
		for i := range entries {
			p.applyEnvDefaults(&entries[i])
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
		Instruction:     "", // uses DefaultInstruction in runtime
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

// BuildLLM constructs an LLM from the configured models, filtered by UseCase.
// When useCase is empty (""), returns the first/default model (backward compat).
// Otherwise, filters by Type=llm and UseCases containing the given use case.
func (p *Provider) BuildLLM(ctx context.Context, useCase UseCase) (model.LLM, error) {
	models := p.models()
	if len(models) == 0 {
		return nil, fmt.Errorf("no LLM models configured")
	}
	selected := p.selectModelsByUseCase(models, useCase)
	backends := p.buildBackends(selected)
	return backends[0], nil
}

// selectModelsByUseCase returns the candidate model entries for a use case.
// Empty useCase returns all models. Filtered to LLM type with matching UseCases.
func (p *Provider) selectModelsByUseCase(models []ModelEntry, useCase UseCase) []ModelEntry {
	if useCase == "" {
		return models
	}
	var candidates []ModelEntry
	for _, m := range models {
		if m.Type != ModelTypeLLM {
			continue
		}
		if hasUseCase(m, useCase) {
			candidates = append(candidates, m)
		}
	}
	if len(candidates) == 0 {
		return models
	}
	sortModelsByCost(candidates)
	return candidates
}

// hasUseCase reports whether the model declares the given use case.
func hasUseCase(m ModelEntry, useCase UseCase) bool {
	for _, uc := range m.UseCases {
		if uc == string(useCase) {
			return true
		}
	}
	return false
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

// DefaultInstruction returns the system prompt of the default model.
func (p *Provider) DefaultInstruction(ctx context.Context) string {
	for _, m := range p.models() {
		if m.IsDefault && m.Instruction != "" {
			return m.Instruction
		}
	}
	return "" // caller falls back to runtime.DefaultInstruction
}

// DefaultModel returns the default LLM model entry: the first entry with
// IsDefault==true && Type==llm, or if none, the first Type==llm entry.
// Returns an error when no LLM models are configured.
func (p *Provider) DefaultModel(ctx context.Context) (*ModelEntry, error) {
	models := p.models()
	var firstLLM *ModelEntry
	for i := range models {
		if models[i].Type != ModelTypeLLM {
			continue
		}
		if firstLLM == nil {
			firstLLM = &models[i]
		}
		if models[i].IsDefault {
			return &models[i], nil
		}
	}
	if firstLLM != nil {
		return firstLLM, nil
	}
	return nil, fmt.Errorf("no LLM models configured")
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

// GetModelByUseCase returns the model entry selected for a given use case
// (cheapest matching LLM, or the first model when none match). Used by the
// Runtime registry's system-level path to build per-use-case Runtime
// instances (SPEC-062 §5.3.2).
func (p *Provider) GetModelByUseCase(ctx context.Context, useCase UseCase) (*ModelEntry, error) {
	models := p.models()
	if len(models) == 0 {
		return nil, fmt.Errorf("no models configured")
	}
	selected := p.selectModelsByUseCase(models, useCase)
	if len(selected) == 0 {
		return nil, fmt.Errorf("no model for use case %q", useCase)
	}
	return &selected[0], nil
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
func (p *Provider) AddModel(ctx context.Context, entry ModelEntry) (ModelEntry, error) {
	if p.repo == nil {
		return entry, fmt.Errorf("config repository not available")
	}
	if entry.ID == "" {
		entry.ID = "model_" + newUUID()
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

// DeleteModel removes the model with the given ID from the list. Idempotent:
// deleting a non-existent ID is a no-op (returns nil).
func (p *Provider) DeleteModel(ctx context.Context, id string) error {
	if p.repo == nil {
		return fmt.Errorf("config repository not available")
	}
	models := p.models()
	kept := make([]ModelEntry, 0, len(models))
	removed := false
	hadDefault := false
	for _, m := range models {
		if m.ID == id {
			removed = true
			if m.IsDefault {
				hadDefault = true
			}
			continue
		}
		kept = append(kept, m)
	}
	if !removed {
		return nil // idempotent delete
	}
	// If we removed the default LLM, promote the first remaining LLM.
	if hadDefault {
		ensureSingleDefault(kept)
	}
	return p.SetModels(ctx, kept)
}

// SetDefaultModel marks the model with the given ID as the sole default LLM,
// clearing IsDefault on every other LLM entry. Returns an error when the ID
// is not found or is not an LLM model.
func (p *Provider) SetDefaultModel(ctx context.Context, id string) error {
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
			models[i].IsDefault = true
			found = true
		} else {
			models[i].IsDefault = false
		}
	}
	if !found {
		return fmt.Errorf("LLM model %q not found", id)
	}
	return p.SetModels(ctx, models)
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
