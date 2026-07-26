// Package enhance implements the prompt-enhancement service. It wraps the
// ADK model router with a Redis cache and token-usage recording, falling
// back to a direct OpenAI-compatible HTTP call when the ADK model is
// unavailable.
package enhance

import (
	"context"
	"os"

	"google.golang.org/adk/model"
	genai "google.golang.org/genai"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmcache"
	"github.com/luoxiaojun1992/data-agent/internal/infra/llmstats"
)

// defaultModel is the fallback model name for enhance/embedding.
const defaultModel = "gpt-4o"

// Service enhances user prompts into structured data-analysis prompts.
type Service struct {
	modelCfg    *modelcfg.Provider
	cache       *llmcache.Cache
	recorder    *llmstats.Recorder
}

// NewService creates an enhance service.
func NewService(modelCfg *modelcfg.Provider, cache *llmcache.Cache, recorder *llmstats.Recorder) *Service {
	return &Service{modelCfg: modelCfg, cache: cache, recorder: recorder}
}

// Enhance optimizes a prompt via the ADK model router (with Redis cache and
// token recording). Returns the original prompt on any failure so callers
// never hard-fail on enhancement.
func (s *Service) Enhance(ctx context.Context, prompt string) string {
	modelName := envOrDefault("LLM_MODEL", "default")

	// Redis cache lookup.
	if s.cache != nil {
		if cached, ok := s.cache.GetEnhance(ctx, modelName, prompt); ok {
			return cached
		}
	}

	enhanced := s.enhanceViaADK(ctx, prompt)
	if s.cache != nil {
		s.cache.SetEnhance(ctx, modelName, prompt, enhanced)
	}
	s.recordTokens(ctx, prompt, enhanced)
	return enhanced
}

// enhanceViaADK uses the ADK model router for prompt enhancement. On failure,
// returns the original prompt unchanged — no fallback to hardcoded API endpoints.
func (s *Service) enhanceViaADK(ctx context.Context, prompt string) string {
	if s.modelCfg == nil {
		return prompt
	}
	llm, lErr := s.modelCfg.BuildLLM(ctx, modelcfg.UseCaseEnhance)
	if lErr != nil {
		return prompt
	}
	sys := "你是提示词优化专家。把用户输入的模糊查询转化为结构化、可操作的数据分析提示词，包含具体指标、维度、时限和期望输出格式。直接输出优化后的提示词，不要解释。"
	temp := float32(0.3)
	adkReq := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{genai.NewPartFromText(sys)}},
			{Role: "user", Parts: []*genai.Part{genai.NewPartFromText(prompt)}},
		},
		Config: &genai.GenerateContentConfig{Temperature: &temp}, // MaxOutputTokens from model config via wrapper
	}
	for resp, err := range llm.GenerateContent(ctx, adkReq, false) {
		if err != nil {
			return prompt
		}
		if resp.Content != nil && len(resp.Content.Parts) > 0 {
			return resp.Content.Parts[0].Text
		}
	}
	return prompt
}

// callEnhanceLLM calls a plain OpenAI-compatible HTTP endpoint to enhance a
// prompt. Falls back to the original prompt on any error.
func callEnhanceLLM(ctx context.Context, prompt string) string {
	return prompt // removed — enhance uses ADK model router exclusively
}

// recordTokens records token usage for an enhance call.
func (s *Service) recordTokens(ctx context.Context, prompt, enhanced string) {
	if s.recorder == nil {
		return
	}
	modelName := envOrDefault("LLM_MODEL", defaultModel)
	_ = s.recorder.Record(ctx, llmstats.Record{
		CallPoint:        "enhance",
		Model:            modelName,
		PromptTokens:     llmstats.EstimateTokens(prompt),
		CompletionTokens: llmstats.EstimateTokens(enhanced),
		Estimated:        true,
	})
}

// envOrDefault reads an env var or returns the default when unset/empty.
func envOrDefault(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}
