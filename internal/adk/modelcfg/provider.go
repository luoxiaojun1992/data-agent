// Package modelcfg provides a unified model configuration layer that reads
// LLM and embedding models from MongoDB system_config (admin model-config page)
// with environment variable fallbacks. It replaces the env-only model wiring
// in cmd/server/main.go with a config-driven Provider used by initServices.
package modelcfg

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"google.golang.org/adk/model"

	"github.com/ieshan/adk-go-pkg/model/openai"
	adkmodel "github.com/luoxiaojun1992/data-agent/internal/adk/model"
	mongoinfra "github.com/luoxiaojun1992/data-agent/internal/infra/mongo"
)

// ModelEntry describes one LLM model in the admin config.
type ModelEntry struct {
	Name            string  `json:"name"`
	BaseURL         string  `json:"base_url"`
	APIKey          string  `json:"-"` // Vault encrypt
	Instruction     string  `json:"instruction"`
	Capability      string  `json:"capability"`
	TokenMultiplier float64 `json:"token_multiplier"`
	Temperature     float64 `json:"temperature"`
	MaxTokens       int     `json:"max_tokens"`
	IsDefault       bool    `json:"is_default"`
	FallbackOrder   int     `json:"fallback_order"`
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
	repo  *mongoinfra.SystemConfigRepository
	cfgNS string // system_config namespace, default "model"
}

// NewProvider creates a config provider. Passing nil repo means "env only".
func NewProvider(repo *mongoinfra.SystemConfigRepository) *Provider {
	return &Provider{repo: repo, cfgNS: "model"}
}

// ---- LLM models ----

const defaultBaseURL = "https://api.openai.com"

// models returns the configured LLM model list. DB has priority; env fallback.
func (p *Provider) models() []ModelEntry {
	entries := p.modelsFromDB()
	if len(entries) > 0 {
		for i := range entries {
			p.applyEnvDefaults(&entries[i])
		}
		return entries
	}
	return p.modelsFromEnv()
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

// BuildLLM constructs the FallbackLLM chain from the configured models.
func (p *Provider) BuildLLM(ctx context.Context) (model.LLM, error) {
	models := p.models()
	if len(models) == 0 {
		return nil, fmt.Errorf("no LLM models configured")
	}
	// Sort by FallbackOrder.
	sortModels(models)
	backends := make([]model.LLM, 0, len(models))
	for _, m := range models {
		llm, err := openai.New(openai.Config{
			Model:   m.Name,
			BaseURL: m.BaseURL,
			APIKey:  m.APIKey,
		})
		if err != nil {
			return nil, fmt.Errorf("create openai adapter for model %q: %w", m.Name, err)
		}
		backends = append(backends, llm)
	}
	if len(backends) == 1 {
		return backends[0], nil
	}
	return adkmodel.NewFallbackLLM(backends...)
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

// SetModels serializes and stores the model list (admin PUT).
func (p *Provider) SetModels(ctx context.Context, entries []ModelEntry) error {
	if p.repo == nil {
		return fmt.Errorf("config repository not available")
	}
	raw, err := json.Marshal(entries)
	if err != nil {
		return fmt.Errorf("marshal models: %w", err)
	}
	return p.repo.Upsert(ctx, p.cfgNS, "models", string(raw))
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
