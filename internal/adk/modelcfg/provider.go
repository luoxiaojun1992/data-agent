// Package modelcfg provides a unified model configuration layer that reads
// LLM and embedding models from the model_configs collection (one document per
// model) with a separate model_defaults collection for per-use-case defaults.
package modelcfg

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"iter"
	"os"
	"strconv"
	"strings"

	"google.golang.org/adk/model"
	"google.golang.org/genai"

	"github.com/ieshan/adk-go-pkg/model/openai"
	adkmodel "github.com/luoxiaojun1992/data-agent/internal/adk/model"
	"github.com/luoxiaojun1992/data-agent/internal/domain/modelconfig"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
	"github.com/luoxiaojun1992/data-agent/internal/repository"
)

// Re-export the model-config domain types so the rest of the codebase keeps
// referencing modelcfg.* while the canonical definitions live in
// internal/domain/modelconfig (shared with the repository layer).
type (
	ModelType  = modelconfig.ModelType
	UseCase    = modelconfig.UseCase
	ModelEntry = modelconfig.ModelEntry
)

const (
	ModelTypeLLM       = modelconfig.ModelTypeLLM
	ModelTypeEmbedding = modelconfig.ModelTypeEmbedding

	UseCaseChat           = modelconfig.UseCaseChat
	UseCaseTask           = modelconfig.UseCaseTask
	UseCaseEnhance        = modelconfig.UseCaseEnhance
	UseCaseCompaction     = modelconfig.UseCaseCompaction
	UseCaseKBChunking     = modelconfig.UseCaseKBChunking
	UseCaseKBImage        = modelconfig.UseCaseKBImage
	UseCaseEmbedding      = modelconfig.UseCaseEmbedding
	UseCaseIntentCheck    = modelconfig.UseCaseIntentCheck
	UseCaseRelevanceCheck = modelconfig.UseCaseRelevanceCheck
)

// DefaultInstruction is the system prompt used when a model's Instruction
// field is empty. It lives here (not in runtime) so model config is the
// single source of truth for all model parameters.
const DefaultInstruction = `You are a data analysis agent. Help the user analyze data, query knowledge bases, compute statistics, and produce reports.
Use the available tools when they help answer the question. Answer in the same language the user uses.

## Data Analysis Workflow

When the user asks for data analysis or statistics:

1. **Query data with sql_executor** — Execute SQL SELECT queries to retrieve raw data from the database.
   - Start with exploratory queries (e.g. SELECT * FROM orders LIMIT 5) to understand the schema.
   - Use aggregations (COUNT, SUM, AVG, GROUP BY) to compute preliminary metrics.
   - Filter and sort as needed (WHERE, ORDER BY).

2. **Extract intermediate results** — Parse the rows returned by sql_executor. Each result has "columns" (field names) and "rows" (2D array of values). Identify the numeric columns you need for statistical analysis.

3. **Compute statistics with stats_compute** — Pass the extracted numeric arrays to stats_compute:
   - Use "descriptive" for summary statistics (mean, median, std_dev, quartiles).
   - Use "linear_regression" for relationships between two variables.
   - Use "time_series" for trend decomposition.

4. **Search knowledge base with knowledge_search** — When the user references terms or concepts you need background on.

## Important Rules

- Always validate your SQL query structure — use parameterized queries when possible.
- When stats_compute returns results, explain them in plain language for the user.
- Do NOT fabricate data — if the query returns empty results, tell the user.
- If sql_executor returns an error (e.g. table not found), adjust your query and retry.

## PPT Generation

When the user asks to create a PowerPoint presentation:
1. **Plan the slides** — Determine the slide structure (title slide, content slides, summary).
2. **Write markdown content** — Use # for slide titles, ## for subtitles, - for bullet points.
3. **Generate the PPTX** — Call pptx_generator with the markdown content and an optional file name.
4. **Save the result** — Call save_artifact with the relative file path to persist it.

## Saving Results

After generating files (PPTX, charts, reports), use save_artifact to persist them.
The tool packages the file into a zip, uploads it, and returns a download URL
that you can share with the user.`

// VaultStore is the minimal Vault interface the Provider needs for per-model
// API key encryption. The concrete *vault.Client satisfies this; tests can
// inject a fake. Nil-safe: nil vault means plaintext fallback only.
type VaultStore interface {
	Store(ctx context.Context, path, value string) error
	Retrieve(ctx context.Context, path string) (string, error)
}

// EmbeddingEntry describes the embedding model config (legacy compat shape
// consumed by buildEmbedFn). Kept for backward compatibility.
type EmbeddingEntry struct {
	BaseURL string `json:"base_url"`
	Model   string `json:"model"`
	APIKey  string `json:"api_key,omitempty"`
}

// Provider reads model configurations from model_configs + model_defaults.
// It is the single source of truth for building the ADK model.LLM chain and
// retrieving the agent's system instruction.
type Provider struct {
	modelRepo   repository.ModelConfigRepository
	defaultRepo repository.ModelDefaultRepository
	vault       VaultStore         // optional; per-model API keys stored/retrieved via Vault
	auditor     TextAuditor        // optional; wraps internal (non-runtime) LLM calls with input/output text audit
	usageRec    *llmstats.Recorder // optional; records actual token usage for the runtime (chat) LLM (SPEC-072)
}

// SetAuditor injects the text auditor used to wrap internal LLM calls built
// via BuildLLM (compaction/enhance/intent/relevance/kb). It does NOT affect
// BuildLLMByID (runtime path), which applies its own audit callbacks. Safe to
// call after construction; nil clears auditing.
func (p *Provider) SetAuditor(a TextAuditor) {
	p.auditor = a
}

// SetUsageRecorder injects the token-usage recorder used to wrap the runtime
// (chat) LLM built via BuildLLMByID (SPEC-072). Internal LLM calls (BuildLLM
// useCase path) record their own usage at their call sites; the runtime path
// records actual UsageMetadata here. Safe to call after construction; nil
// disables recording.
func (p *Provider) SetUsageRecorder(rec *llmstats.Recorder) {
	p.usageRec = rec
}

// NewProvider creates a config provider. Passing nil repos means "env only".
func NewProvider(modelRepo repository.ModelConfigRepository, defaultRepo repository.ModelDefaultRepository, vault VaultStore) *Provider {
	return &Provider{modelRepo: modelRepo, defaultRepo: defaultRepo, vault: vault}
}

// ModelAPIKeyVaultPath returns the Vault KV v2 path used to store the API key
// for the model with the given ID. Stable across reloads so the same model
// always points to the same Vault entry.
func ModelAPIKeyVaultPath(modelID string) string {
	return "data-agent/models/" + modelID + "/api_key"
}

const defaultBaseURL = "https://api.openai.com/v1"

// models loads all models from the repository and decrypts their API keys.
// Empty Type is treated as "llm" (legacy compat).
func (p *Provider) models() []ModelEntry {
	entries := p.modelsFromRepo()
	ctx := context.Background()
	for i := range entries {
		if entries[i].Type == "" {
			entries[i].Type = ModelTypeLLM
		}
		p.applyEnvDefaults(&entries[i])
		if err := p.resolveAPIKey(ctx, &entries[i]); err != nil {
			entries[i].APIKey = "" // don't leak Vault path to API surface
		}
	}
	return entries
}

// modelsFromRepo loads models from the repository (large page, covers all).
func (p *Provider) modelsFromRepo() []ModelEntry {
	if p.modelRepo == nil {
		return nil
	}
	entries, _, err := p.modelRepo.List(context.Background(), "", 0, 1000)
	if err != nil {
		return nil
	}
	return entries
}

// resolveAPIKey transparently decrypts a Vault reference into plaintext.
func (p *Provider) resolveAPIKey(ctx context.Context, m *ModelEntry) error {
	if m.APIKey == "" {
		return nil
	}
	if !looksLikeVaultPath(m.APIKey) {
		return nil // already plaintext
	}
	if p.vault == nil {
		return fmt.Errorf("model %q: Vault not available, cannot decrypt API key", m.ID)
	}
	plain, err := p.vault.Retrieve(ctx, m.APIKey)
	if err != nil {
		return fmt.Errorf("model %q: Vault decrypt failed: %w", m.ID, err)
	}
	if plain == "" {
		return fmt.Errorf("model %q: empty API key from Vault", m.ID)
	}
	m.APIKey = plain
	return nil
}

// looksLikeVaultPath reports whether the string resembles one of our Vault
// KV v2 paths (e.g. "data-agent/models/<id>/api_key").
func looksLikeVaultPath(s string) bool {
	return strings.HasPrefix(s, "data-agent/")
}

// modelsFromEnv builds a single-model list from env.
func (p *Provider) modelsFromEnv() []ModelEntry {
	primary := ModelEntry{
		Name:            envOrDefault("LLM_MODEL", "mock-gpt-4o"),
		BaseURL:         envOrDefault("LLM_BASE_URL", defaultBaseURL),
		APIKey:          os.Getenv("LLM_API_KEY"),
		Type:            ModelTypeLLM,
		UseCases:        []string{"chat", "task", "enhance", "compaction"},
		IsDefaultFor:    []string{"chat"},
		Instruction:     DefaultInstruction,
		Capability:      "",
		TokenMultiplier: 1.0,
		Temperature:     0.7,
		MaxTokens:       4096,
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
// API key is NEVER filled from env.
func (p *Provider) applyEnvDefaults(m *ModelEntry) {
	if m.BaseURL == "" {
		m.BaseURL = envOrDefault("LLM_BASE_URL", defaultBaseURL)
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
	if m.EmbeddingDim == 0 && m.Type == ModelTypeEmbedding {
		if v := os.Getenv("EMBEDDING_VECTOR_DIM"); v != "" {
			if n, err := strconv.Atoi(v); err == nil {
				m.EmbeddingDim = n
			}
		}
	}
}

// BuildLLM constructs an LLM from the model designated as default for the
// given use case. When useCase is empty, uses the chat default.
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
	backend := backends[0]
	// Internal (non-runtime) LLM calls get input/output text auditing here.
	// BuildLLMByID (runtime path) is intentionally left unwrapped — the runtime
	// applies its own audit callbacks via llmagent.
	if p.auditor != nil {
		backend = NewAuditedLLM(backend, p.auditor)
	}
	return backend, nil
}

// GetModelByUseCase returns the model designated as default for the given use
// case. Priority: 1) model_defaults record for use case, 2) first LLM model.
func (p *Provider) GetModelByUseCase(ctx context.Context, useCase UseCase) (*ModelEntry, error) {
	if p.defaultRepo != nil {
		if d, err := p.defaultRepo.Get(ctx, string(useCase)); err == nil && d != nil {
			if m, mErr := p.getModel(ctx, d.ModelID); mErr == nil && m != nil {
				return m, nil
			}
		}
	}
	// Fallback: first LLM model.
	models := p.models()
	for i := range models {
		if models[i].Type == ModelTypeLLM {
			return &models[i], nil
		}
	}
	return nil, fmt.Errorf("no model for use case %q", useCase)
}

// defaultCompactionMaxTokens is the fallback compaction token threshold used
// when the compaction model is unconfigured or lacks a context length.
const defaultCompactionMaxTokens = 4000

// CompactionMaxTokens derives the compaction trigger threshold from the
// compaction model's context length (50% of the context window). When the
// model is unconfigured or its ContextLen is missing it falls back to the
// default, so a fresh boot without a configured model still compacts safely.
func (p *Provider) CompactionMaxTokens(ctx context.Context) int {
	if p == nil {
		return defaultCompactionMaxTokens
	}
	m, err := p.GetModelByUseCase(ctx, UseCaseCompaction)
	if err != nil || m == nil || m.ContextLen <= 0 {
		return defaultCompactionMaxTokens
	}
	return m.ContextLen / 2
}

// getModel loads one model by ID (decrypted), with env fallback when no repo.
func (p *Provider) getModel(ctx context.Context, id string) (*ModelEntry, error) {
	if p.modelRepo == nil {
		return nil, fmt.Errorf("config repository not available")
	}
	entry, err := p.modelRepo.Get(ctx, id)
	if err != nil {
		return nil, err
	}
	if entry == nil {
		return nil, fmt.Errorf("model %q not found", id)
	}
	if entry.Type == "" {
		entry.Type = ModelTypeLLM
	}
	p.applyEnvDefaults(entry)
	if err := p.resolveAPIKey(ctx, entry); err != nil {
		entry.APIKey = ""
	}
	return entry, nil
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

// buildBackends creates the model.LLM chain sorted by FallbackOrder. Every
// backend is wrapped with a usage recorder (SPEC-072) so that ALL
// GenerateContent LLM calls — chat, enhance, compaction, intent, relevance,
// kb chunking/image — record their real token usage from UsageMetadata at the
// single ADK LLM boundary. Embedding is a separate EmbeddingFunc (not
// model.LLM) and records its own usage at its call sites.
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
		wrapped := adkmodel.NewCompatLLM(llm)
		if m.MaxTokens > 0 {
			wrapped = &maxTokensLLM{inner: wrapped, maxTokens: int32(m.MaxTokens)}
		}
		wrapped = adkmodel.NewRecordingLLM(wrapped, p.usageRec, "llm")
		backends = append(backends, wrapped)
	}
	return backends
}

// maxTokensLLM wraps a model.LLM to set a default MaxOutputTokens on every
// GenerateContent call.
type maxTokensLLM struct {
	inner     model.LLM
	maxTokens int32
}

func (m *maxTokensLLM) Name() string { return m.inner.Name() }
func (m *maxTokensLLM) GenerateContent(ctx context.Context, req *model.LLMRequest, stream bool) iter.Seq2[*model.LLMResponse, error] {
	if req.Config == nil || req.Config.MaxOutputTokens == 0 {
		cfg := req.Config
		if cfg == nil {
			cfg = &genai.GenerateContentConfig{}
		}
		cfg.MaxOutputTokens = m.maxTokens
		req.Config = cfg
	}
	return m.inner.GenerateContent(ctx, req, stream)
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
// for the chat use case.
func (p *Provider) DefaultInstruction(ctx context.Context) string {
	m, err := p.DefaultModel(ctx)
	if err == nil && m.Instruction != "" {
		return m.Instruction
	}
	return ""
}

// DefaultModel returns the model designated as default for the "chat" use case.
func (p *Provider) DefaultModel(ctx context.Context) (*ModelEntry, error) {
	return p.GetModelByUseCase(ctx, UseCaseChat)
}

// GetModelByID returns the model entry with the given ID. When modelID is
// empty, returns the default LLM model (backward compat).
func (p *Provider) GetModelByID(ctx context.Context, modelID string) (*ModelEntry, error) {
	if modelID == "" {
		return p.DefaultModel(ctx)
	}
	return p.getModel(ctx, modelID)
}

// BuildLLMByID constructs an LLM from the model entry matching modelID. Usage
// recording happens uniformly in buildBackends (SPEC-072), covering this
// runtime (chat) path as well as the useCase path.
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

// defaultMap loads the full use_case → model_id mapping from model_defaults.
func (p *Provider) defaultMap(ctx context.Context) map[string]string {
	m := map[string]string{}
	if p.defaultRepo == nil {
		return m
	}
	items, err := p.defaultRepo.List(ctx)
	if err != nil {
		return m
	}
	for _, d := range items {
		m[d.UseCase] = d.ModelID
	}
	return m
}

// attachDefaults assembles is_default_for onto each model entry from
// model_defaults (response-only field, not persisted).
func (p *Provider) attachDefaults(ctx context.Context, entries []ModelEntry) {
	dm := p.defaultMap(ctx)
	for i := range entries {
		var useCases []string
		for uc, mid := range dm {
			if mid == entries[i].ID {
				useCases = append(useCases, uc)
			}
		}
		entries[i].IsDefaultFor = useCases
	}
}

// ListEmbeddingModels returns paginated Type==embedding model entries.
func (p *Provider) ListEmbeddingModels(ctx context.Context, page, pageSize int) ([]ModelEntry, int, error) {
	if p.modelRepo == nil {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	skip := int64((page - 1) * pageSize)
	entries, total, err := p.modelRepo.List(ctx, ModelTypeEmbedding, skip, int64(pageSize))
	if err != nil {
		return nil, 0, err
	}
	for i := range entries {
		p.applyEnvDefaults(&entries[i])
		_ = p.resolveAPIKey(ctx, &entries[i])
	}
	p.attachDefaults(ctx, entries)
	return entries, int(total), nil
}

// SetDefaultEmbedding sets the embedding default for the given ID.
func (p *Provider) SetDefaultEmbedding(ctx context.Context, id string) error {
	if p.defaultRepo == nil {
		return fmt.Errorf("config repository not available")
	}
	// Verify the model exists and is an embedding model.
	m, err := p.getModel(ctx, id)
	if err != nil {
		return err
	}
	if m.Type != ModelTypeEmbedding {
		return fmt.Errorf("model %q is not an embedding model", id)
	}
	return p.defaultRepo.Set(ctx, string(UseCaseEmbedding), id)
}

// GetDefaultEmbeddingModel resolves the active embedding model, preferring the
// model_defaults record and falling back to the first embedding model.
func (p *Provider) GetDefaultEmbeddingModel(ctx context.Context) (*ModelEntry, error) {
	if p.defaultRepo != nil {
		if d, err := p.defaultRepo.Get(ctx, string(UseCaseEmbedding)); err == nil && d != nil {
			if m, mErr := p.getModel(ctx, d.ModelID); mErr == nil && m != nil {
				return m, nil
			}
		}
	}
	models := p.models()
	for i := range models {
		if models[i].Type == ModelTypeEmbedding {
			return &models[i], nil
		}
	}
	return nil, fmt.Errorf("no embedding model configured")
}

// ListAllModels returns every persisted model entry (decrypted) with
// is_default_for assembled from model_defaults.
func (p *Provider) ListAllModels(ctx context.Context) []ModelEntry {
	all := p.models()
	p.attachDefaults(ctx, all)
	return all
}

// ListLLMModels returns the Type==llm model entries (DB paginated).
func (p *Provider) ListLLMModels(ctx context.Context, page, pageSize int) ([]ModelEntry, int, error) {
	if p.modelRepo == nil {
		return nil, 0, nil
	}
	if page < 1 {
		page = 1
	}
	if pageSize < 1 {
		pageSize = 20
	}
	if pageSize > 100 {
		pageSize = 100
	}
	skip := int64((page - 1) * pageSize)
	entries, total, err := p.modelRepo.List(ctx, ModelTypeLLM, skip, int64(pageSize))
	if err != nil {
		return nil, 0, err
	}
	for i := range entries {
		p.applyEnvDefaults(&entries[i])
		_ = p.resolveAPIKey(ctx, &entries[i])
	}
	p.attachDefaults(ctx, entries)
	return entries, int(total), nil
}

// ConfigHash returns a sha256 hex digest of the JSON-serialized model entry.
func ConfigHash(m ModelEntry) string {
	raw, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	sum := sha256.Sum256(raw)
	return hex.EncodeToString(sum[:])
}

// AddModel inserts a single model entry, generating a pure UUID when empty.
// Plaintext API keys are encrypted to Vault (per-model path) when a Vault
// client is configured.
func (p *Provider) AddModel(ctx context.Context, entry ModelEntry) (ModelEntry, error) {
	if p.modelRepo == nil {
		return entry, fmt.Errorf("config repository not available")
	}
	if entry.ID == "" {
		entry.ID = newUUID()
	}
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
	if entry.Type == "" {
		entry.Type = ModelTypeLLM
	}
	if entry.Type == ModelTypeLLM && entry.Instruction == "" {
		entry.Instruction = DefaultInstruction
	}
	if err := p.modelRepo.Insert(ctx, entry); err != nil {
		return entry, err
	}
	return entry, nil
}

// DecryptModelAPIKey retrieves the plaintext API key for a model from Vault.
func (p *Provider) DecryptModelAPIKey(ctx context.Context, modelID string) (string, error) {
	if p.vault == nil {
		return "", fmt.Errorf("vault client not available")
	}
	m, err := p.getModel(ctx, modelID)
	if err != nil {
		return "", err
	}
	if m.APIKey == "" {
		return "", fmt.Errorf("model %q has no API key", modelID)
	}
	if !looksLikeVaultPath(m.APIKey) {
		return "", fmt.Errorf("model %q has a legacy plaintext API key — re-save the model to encrypt it to Vault", modelID)
	}
	return p.vault.Retrieve(ctx, m.APIKey)
}

// UpdateModel updates an existing model entry by ID. API key handling:
//   - Empty → keep existing Vault path.
//   - "data-agent/..." (Vault path) → keep as-is.
//   - Any other string → treat as new plaintext key, encrypt to Vault, replace.
func (p *Provider) UpdateModel(ctx context.Context, id string, entry ModelEntry) (ModelEntry, error) {
	if p.modelRepo == nil {
		return entry, fmt.Errorf("config repository not available")
	}
	existing, err := p.getModel(ctx, id)
	if err != nil {
		return entry, err
	}
	entry.ID = id
	if entry.APIKey == "" || looksLikeVaultPath(entry.APIKey) {
		// Keep the original persisted value (Vault path, or legacy plaintext).
		// getModel decrypts the key in memory, so re-read the raw record here —
		// persisting the decrypted plaintext back to MongoDB would violate the
		// "API keys live in Vault" invariant.
		entry.APIKey = existing.APIKey
		if raw, rawErr := p.modelRepo.Get(ctx, id); rawErr == nil && raw != nil {
			entry.APIKey = raw.APIKey
		}
	} else {
		if p.vault == nil {
			return entry, fmt.Errorf("vault client is required to store API keys; ensure VAULT_ADDR/VAULT_TOKEN are configured")
		}
		path := ModelAPIKeyVaultPath(id)
		if err := p.vault.Store(ctx, path, entry.APIKey); err != nil {
			return entry, fmt.Errorf("vault store api_key for %q: %w", id, err)
		}
		entry.APIKey = path
	}
	if entry.Type == "" {
		entry.Type = existing.Type
	}
	if err := p.modelRepo.Update(ctx, id, entry); err != nil {
		return entry, err
	}
	return entry, nil
}

// DeleteModel removes the model with the given ID and cleans up its
// model_defaults references. Idempotent.
func (p *Provider) DeleteModel(ctx context.Context, id string) error {
	if p.modelRepo == nil {
		return fmt.Errorf("config repository not available")
	}
	if err := p.modelRepo.Delete(ctx, id); err != nil {
		return err
	}
	// Clean up any default record pointing at this model.
	if p.defaultRepo != nil {
		items, _ := p.defaultRepo.List(ctx)
		for _, d := range items {
			if d.ModelID == id {
				_ = p.defaultRepo.Delete(ctx, d.UseCase)
			}
		}
	}
	return nil
}

// SetDefaultModel marks the model :id as default for the given use cases.
// Each use case value must be a valid enum (invalid → error). Embedding models
// route to SetDefaultEmbedding.
func (p *Provider) SetDefaultModel(ctx context.Context, id string, useCases []string) error {
	if p.defaultRepo == nil {
		return fmt.Errorf("config repository not available")
	}
	if len(useCases) == 0 {
		useCases = []string{string(UseCaseChat)}
	}
	for _, uc := range useCases {
		if !modelconfig.IsValidUseCase(uc) {
			return fmt.Errorf("invalid use case %q", uc)
		}
	}
	for _, uc := range useCases {
		if err := p.defaultRepo.Set(ctx, uc, id); err != nil {
			return err
		}
	}
	return nil
}

// UnsetDefault cancels the default for the given use cases (embedding included).
func (p *Provider) UnsetDefault(ctx context.Context, useCases []string) error {
	if p.defaultRepo == nil {
		return fmt.Errorf("config repository not available")
	}
	for _, uc := range useCases {
		if !modelconfig.IsValidUseCase(uc) {
			return fmt.Errorf("invalid use case %q", uc)
		}
		if err := p.defaultRepo.Delete(ctx, uc); err != nil {
			return err
		}
	}
	return nil
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

// ---- Embedding (legacy compat) ----

// EmbeddingConfig returns the embedding model config built from the default
// embedding model (single-track, no separate "embedding" key).
func (p *Provider) EmbeddingConfig() EmbeddingEntry {
	ctx := context.Background()
	if emb, err := p.GetDefaultEmbeddingModel(ctx); err == nil && emb != nil {
		e := EmbeddingEntry{
			BaseURL: emb.BaseURL,
			Model:   emb.Name,
			APIKey:  emb.APIKey,
		}
		p.applyEmbeddingDefaults(&e)
		return e
	}
	return EmbeddingEntry{}
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

// GetRawModelConfig returns the structured model list for the admin GET
// endpoint (legacy flat keys removed).
func (p *Provider) GetRawModelConfig(ctx context.Context) (map[string]any, error) {
	models := p.models()
	p.attachDefaults(ctx, models)
	return map[string]any{"models": models}, nil
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
