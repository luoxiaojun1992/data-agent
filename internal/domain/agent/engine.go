package agent

import (
	"context"
	"fmt"
	"sync"
	"time"
)

// LLMProvider defines the interface for language model providers.
type LLMProvider interface {
	// Chat sends a chat completion request and returns the response.
	Chat(ctx context.Context, req ChatRequest) (*ChatResponse, error)
	// ChatStream sends a chat completion request and streams the response via callback.
	ChatStream(ctx context.Context, req ChatRequest, callback func(chunk string) error) error
}

// ChatRequest represents a chat completion request.
type ChatRequest struct {
	Model       string        `json:"model"`
	Messages    []Message     `json:"messages"`
	Temperature float64       `json:"temperature,omitempty"`
	MaxTokens   int           `json:"max_tokens,omitempty"`
	Tools       []ToolDef     `json:"tools,omitempty"`
}

// Message represents a chat message.
type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

// ToolDef defines a tool/function that the LLM can call.
type ToolDef struct {
	Name        string                 `json:"name"`
	Description string                 `json:"description"`
	Parameters  map[string]interface{} `json:"parameters"`
}

// ChatResponse represents a chat completion response.
type ChatResponse struct {
	Content   string     `json:"content"`
	ToolCalls []ToolCall `json:"tool_calls,omitempty"`
	Usage     Usage      `json:"usage"`
}

// ToolCall represents a tool/function call from the LLM.
type ToolCall struct {
	ID        string                 `json:"id"`
	Name      string                 `json:"name"`
	Arguments map[string]interface{} `json:"arguments"`
}

// Usage tracks token consumption.
type Usage struct {
	PromptTokens     int `json:"prompt_tokens"`
	CompletionTokens int `json:"completion_tokens"`
	TotalTokens      int `json:"total_tokens"`
}

// ModelConfig defines configuration for a specific LLM model.
type ModelConfig struct {
	Model       string  `json:"model" bson:"model"`
	BaseURL     string  `json:"base_url" bson:"base_url"`
	APIKey      string  `json:"-" bson:"api_key"`
	MaxTokens   int     `json:"max_tokens" bson:"max_tokens"`
	Temperature float64 `json:"temperature" bson:"temperature"`
	TopP        float64 `json:"top_p" bson:"top_p"`
	IsDefault   bool    `json:"is_default" bson:"is_default"`
}

// Router manages multiple LLM providers and routes requests to the appropriate model.
type Router struct {
	mu        sync.RWMutex
	providers map[string]LLMProvider
	models    map[string]*ModelConfig
}

// NewRouter creates a new LLM router.
func NewRouter() *Router {
	return &Router{
		providers: make(map[string]LLMProvider),
		models:    make(map[string]*ModelConfig),
	}
}

// RegisterProvider registers an LLM provider under a name.
func (r *Router) RegisterProvider(name string, provider LLMProvider) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.providers[name] = provider
}

// RegisterModel registers a model configuration.
func (r *Router) RegisterModel(name string, cfg *ModelConfig) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.models[name] = cfg
}

// GetModel returns the model config for the given name.
func (r *Router) GetModel(name string) (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	cfg, exists := r.models[name]
	if !exists {
		return nil, fmt.Errorf("model %q not found", name)
	}
	return cfg, nil
}

// GetDefaultModel returns the default model config.
func (r *Router) GetDefaultModel() (*ModelConfig, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	for _, cfg := range r.models {
		if cfg.IsDefault {
			return cfg, nil
		}
	}
	return nil, fmt.Errorf("no default model configured")
}

// ListModels returns all registered model names.
func (r *Router) ListModels() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	names := make([]string, 0, len(r.models))
	for name := range r.models {
		names = append(names, name)
	}
	return names
}

// Engine is the core agent execution engine.
// It orchestrates the LLM ↔ Tool Call loop.
type Engine struct {
	router   *Router
	registry SkillRegistry
	security SecurityAuditor
}

// SkillRegistry defines the interface for skill lookup.
type SkillRegistry interface {
	Get(name string) (SkillExecutor, error)
	List() []string
}

// SkillExecutor defines the interface for executing a skill.
type SkillExecutor interface {
	Execute(ctx context.Context, params map[string]any) (any, error)
	Name() string
}

// SecurityAuditor defines the interface for security auditing.
type SecurityAuditor interface {
	AuditInput(input string) error
	AuditOutput(output string) (string, error)
	AuditToolCall(toolName string, params map[string]any) error
}

// NewEngine creates a new Agent Engine.
func NewEngine(router *Router, registry SkillRegistry, auditor SecurityAuditor) *Engine {
	return &Engine{
		router:   router,
		registry: registry,
		security: auditor,
	}
}

// Run executes a chat completion with tool calls in a loop.
func (e *Engine) Run(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
	modelName := req.Model
	if modelName == "" {
		defaultModel, err := e.router.GetDefaultModel()
		if err != nil {
			return nil, fmt.Errorf("no model specified and no default: %w", err)
		}
		modelName = defaultModel.Model
	}

	// Security audit on input
	if e.security != nil {
		for _, msg := range req.Messages {
			if err := e.security.AuditInput(msg.Content); err != nil {
				return nil, fmt.Errorf("input audit failed: %w", err)
			}
		}
	}

	startTime := time.Now()
	resp, err := e.router.Chat(ctx, modelName, req)
	if err != nil {
		return nil, fmt.Errorf("chat failed: %w", err)
	}

	// Security audit on tool calls before execution
	if e.security != nil {
		for _, tc := range resp.ToolCalls {
			if err := e.security.AuditToolCall(tc.Name, tc.Arguments); err != nil {
				return nil, fmt.Errorf("tool call audit failed for %q: %w", tc.Name, err)
			}
		}
	}

	// Security audit on output
	if e.security != nil {
		sanitized, err := e.security.AuditOutput(resp.Content)
		if err != nil {
			return nil, fmt.Errorf("output audit failed: %w", err)
		}
		resp.Content = sanitized
	}

	_ = startTime // reserved for latency tracking
	return resp, nil
}

// RunStream executes a streaming chat completion.
func (e *Engine) RunStream(ctx context.Context, req ChatRequest, callback func(chunk string) error) error {
	return e.router.ChatStream(ctx, req.Model, req, callback)
}

// Chat sends a chat completion through the router.
func (r *Router) Chat(ctx context.Context, modelName string, req ChatRequest) (*ChatResponse, error) {
	cfg, err := r.GetModel(modelName)
	if err != nil {
		return nil, err
	}

	req.Model = cfg.Model
	if req.MaxTokens == 0 {
		req.MaxTokens = cfg.MaxTokens
	}
	if req.Temperature == 0 {
		req.Temperature = cfg.Temperature
	}

	provider, exists := r.providers[cfg.Model]
	if !exists {
		// Auto-register a default HTTP provider
		provider = NewOpenAIProvider(cfg.BaseURL, cfg.APIKey)
		r.RegisterProvider(cfg.Model, provider)
	}

	return provider.Chat(ctx, req)
}

// ChatStream sends a streaming chat completion through the router.
func (r *Router) ChatStream(ctx context.Context, modelName string, req ChatRequest, callback func(chunk string) error) error {
	cfg, err := r.GetModel(modelName)
	if err != nil {
		return err
	}

	req.Model = cfg.Model
	if req.MaxTokens == 0 {
		req.MaxTokens = cfg.MaxTokens
	}
	if req.Temperature == 0 {
		req.Temperature = cfg.Temperature
	}

	provider, exists := r.providers[cfg.Model]
	if !exists {
		provider = NewOpenAIProvider(cfg.BaseURL, cfg.APIKey)
		r.RegisterProvider(cfg.Model, provider)
	}

	return provider.ChatStream(ctx, req, callback)
}

// EstimateTotalTokens estimates the total token count for a list of messages.
func EstimateTotalTokens(messages []Message) int {
	total := 0
	for _, m := range messages {
		total += estimateTokens(m.Content)
	}
	return total
}

func estimateTokens(text string) int {
	if text == "" {
		return 0
	}
	// Rough heuristic: ~4 chars per token
	return (len(text) + 3) / 4
}

// SkillRegistryAdapter adapts a skill.Registry to the agent.SkillRegistry interface.
type SkillRegistryAdapter struct {
	// Placeholder for skill registry integration (SPEC-008)
}

// NewSkillRegistryAdapter creates a new adapter.
func NewSkillRegistryAdapter() *SkillRegistryAdapter {
	return &SkillRegistryAdapter{}
}

func (a *SkillRegistryAdapter) Get(name string) (SkillExecutor, error) {
	return nil, fmt.Errorf("skill %q not found (skills will be implemented in SPEC-008)", name)
}

func (a *SkillRegistryAdapter) List() []string {
	return nil
}
