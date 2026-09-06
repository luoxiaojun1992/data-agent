// Package guard implements intent classification and output-relevance checks
// that gate LLM calls in chat / feishu / agent task. Both checks are internal
// one-shot LLM calls (BuildLLM + GenerateContent) that do NOT write session
// events, do NOT trigger compaction, and are not themselves guarded (no
// recursion). The relevance check drives a bounded retry loop via a Redis
// counter keyed by session ID.
package guard

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/luoxiaojun1992/data-agent/internal/adk/modelcfg"
	"github.com/luoxiaojun1992/data-agent/internal/infra/redis"
	"google.golang.org/adk/model"
	"google.golang.org/adk/session"
	"google.golang.org/genai"
)

// Service aggregates intent + relevance + retry-counting.
type Service struct {
	provider   *modelcfg.Provider
	redis      *redis.Client
	maxRetries int
	// maxRetriesFn optionally resolves the retry limit dynamically (e.g. from
	// system config `guard.max_retries`). nil → use maxRetries.
	maxRetriesFn func(ctx context.Context) int
}

// relevanceTTL bounds the retry counter lifetime so a session that dies
// mid-retry never leaves a stale guard:relevance:{sessionID} key behind.
const relevanceTTL = 10 * time.Minute

// NewService creates a guard service. maxRetries is the fallback retry limit
// (default 2) used when no resolver is set or the resolver returns <= 0.
func NewService(provider *modelcfg.Provider, redisClient *redis.Client, maxRetries int) *Service {
	if maxRetries <= 0 {
		maxRetries = 2
	}
	return &Service{provider: provider, redis: redisClient, maxRetries: maxRetries}
}

// SetMaxRetriesResolver installs a resolver that supplies the retry limit per
// call (returns <= 0 to fall back to the static default). Safe to call after
// construction; the resolver is read on every retry decision so config changes
// take effect without restart.
func (s *Service) SetMaxRetriesResolver(fn func(ctx context.Context) int) {
	s.maxRetriesFn = fn
}

// resolveMaxRetries returns the effective retry limit for the current call.
func (s *Service) resolveMaxRetries(ctx context.Context) int {
	if s.maxRetriesFn != nil {
		if v := s.maxRetriesFn(ctx); v > 0 {
			return v
		}
	}
	return s.maxRetries
}

// SetRedis injects the Redis client used for the relevance retry counter.
// Safe to call after construction (Redis connects later than the guard).
func (s *Service) SetRedis(redisClient *redis.Client) {
	s.redis = redisClient
}

// CheckIntent classifies the user content as task vs chat and, within tasks,
// whether the task needs planning (is_plan). Returns (isTask, isPlan, error).
// Images flow through as InlineData parts (caller is responsible for using a
// multimodal model — the backend does not validate model capability).
func (s *Service) CheckIntent(ctx context.Context, content *genai.Content) (bool, bool, error) {
	llm, err := s.provider.BuildLLM(ctx, modelcfg.UseCaseIntentCheck)
	if err != nil {
		return false, false, err
	}
	sys := "你是用户意图分类器。判断用户的输入是「任务」（需要数据分析、计算、查询、生成报告/文件、制定计划等）还是「聊天」（闲聊、问候、咨询）。若属于任务，进一步判断是否「需要规划」（用户要求制定计划/方案/路线图，或任务本身需要多步拆解后才能完成）。只输出 JSON：{\"is_task\": true, \"is_plan\": false} 形式（is_plan 仅在 is_task 为 true 时可为 true），不要输出其他内容。"
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{genai.NewPartFromText(sys)}},
			content,
		},
	}
	text, err := generateText(ctx, llm, req)
	if err != nil {
		return false, false, err
	}
	r := parseIntent(text)
	return r.IsTask, r.IsPlan, nil
}

// CheckRelevance checks whether the LLM output is relevant to the given base
// (the most recent user message or tool output). Returns true when relevant.
func (s *Service) CheckRelevance(ctx context.Context, llmOutput, base string) (bool, error) {
	llm, err := s.provider.BuildLLM(ctx, modelcfg.UseCaseRelevanceCheck)
	if err != nil {
		return false, err
	}
	sys := "你是回答相关性检查器。判断「回答」是否与「用户意图/工具结果」相关（直接回应、延续话题、完成所要求任务=相关；答非所问、明显跑偏、幻觉无关内容=不相关）。只输出 JSON：{\"is_relevant\": true} 或 {\"is_relevant\": false}，不要输出其他内容。"
	prompt := "用户意图/工具结果：\n" + base + "\n\n回答：\n" + llmOutput
	req := &model.LLMRequest{
		Contents: []*genai.Content{
			{Role: "user", Parts: []*genai.Part{genai.NewPartFromText(sys)}},
			{Role: "user", Parts: []*genai.Part{genai.NewPartFromText(prompt)}},
		},
	}
	text, err := generateText(ctx, llm, req)
	if err != nil {
		return false, err
	}
	return parseIsRelevant(text), nil
}

// LastRelevanceBase extracts the text of the most recent user message or tool
// output from the session's (compacted) events, to serve as the relevance
// check base (SPEC-092 §4.2). It walks the event list from newest to oldest
// and returns the first user-message or function-response text; assistant
// text, system hints ([intent]/[plan_hint]/[relevance]), and compaction
// summaries/hints are skipped. Returns "" when none is found (e.g. an
// image-only user message, or empty events).
func LastRelevanceBase(events session.Events) string {
	for i := events.Len() - 1; i >= 0; i-- {
		ev := events.At(i)
		if ev == nil || ev.Content == nil {
			continue
		}
		if hasFunctionResponse(ev.Content) {
			if t := contentText(ev.Content); t != "" {
				return t
			}
			continue
		}
		if ev.Author == "user" {
			if t := contentText(ev.Content); t != "" {
				return t
			}
		}
	}
	return ""
}

// hasFunctionResponse reports whether the content carries any FunctionResponse
// part (a tool output).
func hasFunctionResponse(c *genai.Content) bool {
	for _, p := range c.Parts {
		if p != nil && p.FunctionResponse != nil {
			return true
		}
	}
	return false
}

// contentText renders a content's parts into a single relevance-base string:
// text parts are concatenated, and function-response payloads are JSON-marshalled
// so tool outputs form a readable comparison base.
func contentText(c *genai.Content) string {
	var sb strings.Builder
	for _, p := range c.Parts {
		if p == nil {
			continue
		}
		switch {
		case p.Text != "":
			sb.WriteString(p.Text)
		case p.FunctionResponse != nil:
			if raw, err := json.Marshal(p.FunctionResponse.Response); err == nil {
				sb.WriteString(string(raw))
			}
		}
	}
	return strings.TrimSpace(sb.String())
}

// RecordAndShouldRetry increments the relevance-failure counter for a session
// and reports whether a retry is still allowed. On reaching maxRetries the key
// is deleted (reset) and the result is "stop retrying".
func (s *Service) RecordAndShouldRetry(ctx context.Context, sessionID string) (bool, error) {
	if s.redis == nil {
		return false, nil
	}
	key := relevanceKey(sessionID)
	n, err := s.redis.Incr(ctx, key)
	if err != nil {
		return false, err
	}
	// First increment sets a short TTL so a session that crashes mid-retry
	// never leaves a stale counter behind (SPEC-067 §3).
	if n == 1 {
		_ = s.redis.Expire(ctx, key, relevanceTTL)
	}
	if n >= int64(s.resolveMaxRetries(ctx)) {
		_ = s.redis.Del(ctx, key)
		return false, nil
	}
	return true, nil
}

// relevanceKey returns the Redis key for a session's relevance retry counter.
func relevanceKey(sessionID string) string {
	return "guard:relevance:" + sessionID
}

// ClearRelevance resets the relevance-failure counter for a session. Called
// when a relevance check passes (relevant=true) so the next user turn starts
// counting from zero (SPEC-067 §3: 用户下一次输入时计数从 0 重新开始). A
// previous turn that retried (n≥1) but never reached the limit would otherwise
// leak its counter into the next turn.
func (s *Service) ClearRelevance(ctx context.Context, sessionID string) {
	if s.redis == nil {
		return
	}
	_ = s.redis.Del(ctx, relevanceKey(sessionID))
}
