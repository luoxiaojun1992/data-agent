# LLM 全链路 Token 统计与 Redis 缓存

> **SPEC-051** | Status: ✅ 已实现 | Date: 2026-07-18 | Phase: P12

## 1. 目标

1. **Token 全口径统计**：系统中**所有** LLM 调用点统一计入 token 统计——Chat（ADK runner）、提示词增强（`/chat/enhance`）、Session 摘要压缩、embedding 调用、记忆事实提取；按模型 `token_multiplier`（SPEC-049）折算计费 token
2. **Redis 缓存**：embedding 结果与提示词增强结果接入 Redis 缓存，降低重复调用成本与延迟

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-009 | ✅ | Redis 任务队列基础设施（Redis client 复用） |
| SPEC-010 | ✅ | 统计监控（token trend 展示端） |
| SPEC-048 | ⚠️ **阻塞依赖** | ADK 迁移（LLM 调用点：runner/enhance/summarizer/embedding）必须合入 |
| SPEC-049 | ⚠️ **阻塞依赖** | 模型 token_multiplier 与 embedding 统一配置 |

## 2. 背景

当前 token 统计只有两个缺口问题：

1. **口径不全**：`chat/enhance`（提示词增强）直接 HTTP 调 LLM 无统计；SPEC-048 新增的 session 摘要压缩、embedding、记忆提取全部绕过统计——Dashboard 的 Token 趋势失真
2. **重复调用浪费**：相同文本的 embedding（KB 检索高频重复查询）、相同输入的提示词增强，每次全价调 LLM/embedding 服务

mockllm 不返回 `usage` 字段，因此统计必须支持**估算模式**（4 字符≈1 token）与**真实 usage 模式**（OpenAI 兼容端点返回 usage 时优先采用）。

## 3. 架构概述

```
┌───────────────────────── LLM 调用点 ─────────────────────────┐
│ chat (ADK runner)  │ enhance │ 摘要压缩 │ embedding │ deriver │
└────────┬───────────┴────┬────┴────┬────┴────┬──────┴────┬────┘
         │                │         │         │           │
         ▼                ▼         ▼         ▼           ▼
┌──────────────────────────────────────────────────────────────┐
│  internal/infra/llmstats — 统一埋点                            │
│  Record(usage{model, prompt, completion}, multiplier)         │
│    • usage 缺失 → 估算（4 chars ≈ 1 token）                   │
│    • 计费 token = (prompt+completion) × token_multiplier      │
│    • 写 MongoDB llm_usage 集合 + Redis 计数器（实时）           │
└──────────────────────────────────────────────────────────────┘
                              │
                              ▼
┌──────────────────────────────────────────────────────────────┐
│  Redis 缓存层（internal/infra/llmcache）                       │
│  • embedding:  key=emb:{model}:{sha256(text)}  无 TTL         │
│  • enhance:    key=enh:{model}:{sha256(input)} TTL=1h         │
│  • 命中 → 跳过 LLM 调用，记 cache_hit 指标                     │
└──────────────────────────────────────────────────────────────┘
```

## 4. 详细设计

### 4.1 llmstats 埋点（`internal/infra/llmstats/`）

```go
// Usage 一次 LLM 调用的 token 消耗
type Usage struct {
    CallPoint        string    `bson:"call_point"`        // chat | enhance | compaction | embedding | memory_derive
    Model            string    `bson:"model"`
    PromptTokens     int       `bson:"prompt_tokens"`
    CompletionTokens int       `bson:"completion_tokens"`
    Multiplier       float64   `bson:"multiplier"`
    BilledTokens     int       `bson:"billed_tokens"`     // (prompt+completion) × multiplier
    Estimated        bool      `bson:"estimated"`         // usage 为估算值
    UserID           string    `bson:"user_id,omitempty"`
    SessionID        string    `bson:"session_id,omitempty"`
    CacheHit         bool      `bson:"cache_hit"`
    CreatedAt        time.Time `bson:"created_at"`
}

// Recorder 统一记录入口（Mongo 落库 + Redis 累加）
type Recorder struct { coll *mongo.Collection; redis *redis.Client }

func (r *Recorder) Record(ctx context.Context, u Usage) error
func EstimateTokens(text string) int  // (len+3)/4
```

**接入点**：
| 调用点 | 接入方式 |
|--------|---------|
| chat | `internal/adk/model/openai.go` 解析 OpenAI usage（真实）；缺失时估算 request/response 文本；AfterModelCallback 处 Record |
| enhance | `chatEnhanceHandler` 包装调用，记录 prompt/completion |
| compaction | `adksession.LLMSummarizer.Summarize` 记录（模型 = 压缩用模型） |
| embedding | `adkmemory.NewOpenAIEmbedding` 包装 Record（completion=0） |
| memory_derive | SPEC-050 deriver 调用点（预留接口，随 SPEC-050 接入） |

### 4.2 llmcache 缓存（`internal/infra/llmcache/`）

```go
// Cache Redis 缓存封装
type Cache struct { redis *redis.Client }

func (c *Cache) GetEmbedding(ctx context.Context, model, text string) ([]float32, bool)
func (c *Cache) SetEmbedding(ctx context.Context, model, text string, vec []float32) error
func (c *Cache) GetEnhance(ctx context.Context, model, input string) (string, bool)
func (c *Cache) SetEnhance(ctx context.Context, model, input, output string, ttl time.Duration) error
```

- embedding 包装器 `CachedEmbeddingFunc`：先查缓存，未命中调原始 embedding func 并回填；命中记 `cache_hit` 指标（不调 Record 计费——**缓存命中不计 token**）
- enhance handler：命中缓存直接返回，响应头/JSON 标记 `cached: true`
- 缓存 key 含模型名，模型切换后旧缓存自然隔离
- Redis 不可用时：缓存层自动降级为直通（不影响功能）

### 4.3 统计读取

- `internal/service/monitor` token trend 改为从 `llm_usage` 聚合（替换现有基于 CallTrend 的估算）
- Dashboard token KPI = `sum(billed_tokens)`；趋势按小时/天 bucket

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes — `llm_usage`（MongoDB，带 created_at/model/call_point 索引） |
| 是否影响现有 API | No — enhance 响应增加可选 `cached` 字段（向后兼容） |
| 性能影响 | 缓存命中省一次 LLM 调用；埋点写 Mongo 异步化（goroutine + 超时）不阻塞主链路 |
| 是否需要新增 Skill | No |
| Redis 依赖 | 复用 SPEC-009 client；Redis 不可用全链路降级直通 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/llmstats/recorder.go` | 统一埋点 | New |
| `internal/infra/llmstats/recorder_test.go` | UT | New |
| `internal/infra/llmcache/cache.go` | Redis 缓存层 | New |
| `internal/infra/llmcache/cache_test.go` | UT | New |
| `internal/adk/model/openai.go` | usage 解析 + Record | Edit |
| `internal/adk/memory/embedding.go` | CachedEmbeddingFunc 包装 | Edit |
| `internal/adk/session/summarizer.go` | 压缩埋点 | Edit |
| `cmd/server/main.go` | enhance 埋点 + 缓存接入 + Recorder 装配 | Edit |
| `internal/service/monitor/trends.go` | token 统计改从 llm_usage 聚合 | Edit |

## 7. 测试策略

1. **Unit tests**（Go）: llmstats/llmcache 100%（gomonkey mock mongo/redis）；估算与真实 usage 双路径；缓存命中/未命中/Redis 故障降级；覆盖率基线见 SPEC-045
2. **Integration tests**: docker-compose Redis 真实缓存读写
3. **E2E tests**: 同一查询 KB 检索两次 → 第二次 embedding 命中缓存（响应时间显著下降 / admin 指标 `cache_hit`）；Dashboard token KPI 非 0 且与 llm_usage 聚合一致（UI-2XX 新增）
4. **审计**: `.agent/skills/go-ut-audit`

## 8. UI Test / E2E 验收规则

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 8.5. Go Unit Test 验收规则

| Tier | 目标 |
|:---:|------|
| L1 纯逻辑 | **100%** |
| L2 接口依赖 | **100%** |
| L3 集成依赖 | **98%** |
| Overall | ≥98%（CI `ut-workflow.yml` gate） |

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] `go test -race -gcflags=all=-l` 全部通过，`go vet` 无警告

## 9. 验证标准

| 指标 | 当前 | 目标 |
|------|------|------|
| Token 统计口径 | chat 单点估算 | ✅ 5 个调用点全口径（真实 usage 优先） |
| 计费倍率 | 无 | ✅ 按模型 token_multiplier 折算 |
| enhance 统计 | 无 | ✅ 计入且支持缓存命中标记 |
| embedding 缓存 | 无 | ✅ 命中不重复调用、不计费 |
| enhance 缓存 | 无 | ✅ TTL 1h，命中即返回 |
| Dashboard token 数据 | 估算失真 | ✅ 来自 llm_usage 真实聚合 |
