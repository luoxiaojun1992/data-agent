# 引擎层迁移 Google ADK — ReAct Loop / Session 压缩 / 模型路由

> **SPEC-048** | Status: ✅ 已实现 | Date: 2026-07-17 | Phase: P11

> **实现说明（2026-07-18）**：实现基于 ADK Go SDK v1.5.0，与本文档原始假设有以下偏差：
> 1. **LiteLlm 不存在于 Go SDK**（仅 Python ADK 有）。Go SDK 仅内置 `model/gemini`。模型路由由 `internal/adk/model` 自实现：OpenAI 兼容 `model.LLM` + `FallbackLLM` fallback 链（`LLM_FALLBACK_BASE_URLS`）。
> 2. **CompactionPlugin 不存在于 Go SDK 公开 API**。Session 压缩由 `internal/adk/session` 的 `WithCompaction` 实现：事件数/token 双阈值触发，`LLMSummarizer` 摘要压缩，保留最近 N 条。
> 3. **`ieshan/adk-go-memory` 全部版本要求 go ≥ 1.26**（项目锁定 go 1.25），改为自实现 `internal/adk/memory`：MongoDB 存储 + Ollama embedding（OpenAI 兼容 `/v1/embeddings`）+ 余弦相似度检索，写自动（chat 完成后 `AddSessionToMemory`）、读通过 `memory_search` tool。
> 4. 核心目标全部达成：ReAct loop（llmagent 内置）、Session 压缩（LLM 摘要）、模型路由（fallback 链）、Skill 绑定 session state（`tool.Context.State()` 注入 user_id/role/kb_id）。
> 5. 旧代码已删除：`engine.go` / `provider.go` / `skill_adapter.go` / `context_manager.go` / `skills/*` / `internal/domain/skill/`。

## 1. 目标

将 data-agent 手写 `Engine`（`internal/domain/agent/engine.go`）替换为 Google ADK Go SDK（`google.golang.org/adk`），一次性获得：
- **ReAct loop**（思考→调工具→观察→再思考，内置于 `llmagent`）
- **Session 压缩**（token 阈值触发 LLM 摘要压缩，Go SDK ADR-010）
- **模型路由**（`LiteLlm` 多 provider fallback 链）
- **Skill 强绑定 session/kb_id**（`tool.Context.State()` 自动注入，不依赖 LLM 传参）

> **核心原则**: 引擎层不应手写 loop 和路由。Google ADK 已原生覆盖全部 4 个能力，data-agent 只需写工具（tool）+ session/memory 适配。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 | ✅ | Agent 核心引擎（已实现但手写 Engine — **本 spec 替换其 Engine/Router/Provider 部分**） |
| SPEC-006 | ✅ | 知识库系统 |
| SPEC-008 | ✅ | Skill 实现层（4 个 skill，改写为 ADK tool） |
| SPEC-043 | ✅ | Mock Model Service（ADK 的 LiteLlm 连接到 mockllm） |
| SPEC-045 | ⚠️ | Go Service UT 全覆盖（非阻塞，但 ADK runner 引入后 service 层需适配） |
| SPEC-046 | ⚠️ | **本 spec 是 SPEC-046 的前置依赖** — SPEC-046 的工具调用 E2E 测试依赖 ADK ReAct loop |
| SPEC-047 | — | 无直接依赖 |

## 2. 背景

### 2.1 当前问题

`internal/domain/agent/engine.go` 的 `Run()` 方法只做单次 LLM 调用 + 审计：

```go
func (e *Engine) Run(ctx context.Context, req ChatRequest) (*ChatResponse, error) {
    resp, _ := e.router.Chat(ctx, modelName, req)  // 调一次
    e.auditToolCalls(resp.ToolCalls)                // 审计（只读）
    e.auditOutput(&resp.Content)                    // 脱敏
    return resp, nil                                 // ToolCalls 丢弃
}
```

缺失的功能：
- LLM 返回 `tool_calls` 后不执行、不回传结果 —— **ReAct loop 未实现**
- 上下文压缩仅 heuristic 截断（非 LLM 摘要）—— **session 压缩未实现**
- 模型路由仅按名字查 map —— **无 fallback/重试**
- Skill 不能读取 session 状态（`SkillExecute` 只收 params） —— **kbid/sid 依赖 LLM 传入**

### 2.2 原始设计要求

从 PRD 和 SPEC-004 的描述可知，原始设计要求使用 **Google ADK**。`engine.go` 的结构（`LLMProvider`、`SkillRegistry`、`SecurityAuditor`）也是朝 ADK 方向设计的，但实现时走了手写路线，导致 loop / 压缩 / 路由 / 状态绑定全部缺失。

## 3. 架构概述

### 3.1 替换前后对比

```
┌───────────────── Before ─────────────────┐   ┌──────────── After ────────────┐
│  手写 Engine                               │   │   Google ADK (google.golang.   │
│  ┌───────────┐  ┌───────────────────┐     │   │   org/adk)                     │
│  │  Router   │  │ SkillRegistry     │     │   │  ┌───────────────────────────┐ │
│  │(map lookup│  │(adapter→domain)   │     │   │  │ llmagent (ReAct built-in) │ │
│  │ no retry) │  │                   │     │   │  │  • Think→Act→Observe loop │ │
│  └─────┬─────┘  └────────┬──────────┘     │   │  │  • tool.Context.State()   │ │
│        │                 │                │   │  │  • StreamingMode SSE      │ │
│   ┌────▼─────────────────▼───────┐        │   │  └────────────┬──────────────┘ │
│   │         Engine.Run()         │        │   │               │                │
│   │  1. LLM call (single)        │        │   │  ┌────────────▼──────────────┐ │
│   │  2. audit tool_calls (只读)  │        │   │  │ runner                     │ │
│   │  3. audit output (脱敏)      │        │   │  │  • CompactionConfig(压缩)  │ │
│   │  4. return (tool_calls丢弃)  │        │   │  │  • SessionService(MongoDB) │ │
│   └──────────────────────────────┘        │   │  │  • MemoryService(Mem0)     │ │
└───────────────────────────────────────────┘   │  │  • LiteLlm(fallback路由)   │ │
                                                 │  └───────────────────────────┘ │
                                                 └────────────────────────────────┘
```

### 3.2 ADK 组件对应关系

| 手写组件 | ADK 替代 | 额外获得 |
|---------|---------|---------|
| `Engine.Run()` | `llmagent` 内置 ReAct | Tool call loop 零代码 |
| `Router.Chat()` | `LiteLlm` + `RunConfig.MaxLLMCalls` | 多 provider fallback / 重试 |
| `ContextManager.TruncateMessages()` | `CompactionConfig` (token阈值+LLM摘要) | 语义保留的上下文压缩 |
| `SkillRegistryAdapter` | ADK `tool.Tool` + `tool.Context.State()` | Session 状态自动注入 |
| `SecurityAuditor` | ADK `plugin.Plugin` / callback | 不改动，作为 plugin 注入 |
| 无 | `session.Service`（MongoDB 适配） | 会话持久化 |
| 无 | `memory.Service`（Mem0 适配） | 跨 session 长期记忆 |

## 4. 接口设计

### 4.1 Skill → ADK Tool 迁移

当前 4 个 skill 全部改写为 ADK tool：

| 当前 | 改写为 ADK tool | 关键变化 |
|------|----------------|---------|
| `sqlskill.SQLExecutor` | `sql_validate` tool | 从 `skill.SkillContext` 换为 `tool.Context`，`tc.State()` 读 session 归属 |
| `statsskill.StatsEngine` | `stats_compute` tool | 同上 |
| `kbskill.KnowledgeSearch` | `knowledge_search` tool | `tc.State()` 读 `kb_id`，强绑定索引目标 |
| `saveskill.SaveReport` | `save_report` tool | 同上 |

**示例 — knowledge_search 改写**：

```go
// Before: skill 手写接口，不感知 session
func (s *KnowledgeSearch) Execute(sc skill.SkillContext, params map[string]any) (any, error) {
    query := params["query"].(string)  // 依赖 LLM 传参
    // kb_id 需要 LLM 在参数里猜
}

// After: ADK tool，自动注入 session state
type KnowledgeSearchArgs struct {
    Query string `json:"query" jsonschema:"description=搜索关键词"`
    TopK  int    `json:"top_k" jsonschema:"description=返回结果数量"`
}
type KnowledgeSearchResult struct {
    Hits []SearchHit `json:"hits"`
}

func KnowledgeSearch(tc tool.Context, args KnowledgeSearchArgs) (KnowledgeSearchResult, error) {
    // kb_id 从 session state 读，不需要 LLM 传
    kbID, _ := tc.State().Get("kb_id")
    userID, _ := tc.State().Get("user_id")
    
    results := kbService.Search(kbID.(string), args.Query, args.TopK)
    return KnowledgeSearchResult{Hits: results}, nil
}
```

### 4.2 Session State 注入流程

```
POST /api/v1/chat { session_id, message }
  │
  ▼
Runner.Run(userID, sessionID, message)
  │
  ├─► SessionService.Load(sessionID)        ← MongoDB 加载 session
  │     └─► State["kb_id"] = "abc123"       ← 预注入
  │     └─► State["user_id"] = userID
  │
  ▼
llmagent.run()
  │
  ├─► LLM call → tool_calls: [{name: "knowledge_search", args: {query: "营收"}}]
  │
  ▼
KnowledgeSearch(tc tool.Context, args)       ← ADK 自动注入 tc
  │
  ├─► tc.State().Get("kb_id")  →  "abc123" ← 不依赖 LLM 传入
  ├─► kbService.Search("abc123", "营收", 5)
  │
  ▼
  tool result → ADK 自动拼回 messages → 再调 LLM → final answer
```

### 4.3 chat 端点适配

`POST /api/v1/chat` 改为用 ADK Runner：

```go
// cmd/server/main.go
func handleChat(c *gin.Context) {
    req := parseChatRequest(c)
    
    // ADK Runner 替代手写 Engine
    evtStream := runner.Run(ctx, userID, req.SessionID, 
        genai.NewUserContent(req.Message),
        agent.RunConfig{
            StreamingMode: agent.StreamingModeSSE,
            MaxLLMCalls:   20,  // ReAct loop 上限
        },
    )
    
    // SSE 输出
    for evt := range evtStream {
        c.SSEvent("message", evt.Content)
    }
}
```

## 5. 详细设计

### 5.1 替换清单（按模块）

| 模块 | 当前文件 | 操作 | ADK 替代 |
|------|---------|:---:|---------|
| Engine | `internal/domain/agent/engine.go` | ❌ 删除 | `llmagent.New()` |
| Router | `internal/domain/agent/engine.go:82-142` | ❌ 删除 | `LiteLlm` |
| Provider | `internal/domain/agent/provider.go` | ❌ 删除 | `LiteLlm` |
| ContextManager | `internal/service/chat/context_manager.go` | ❌ 删除 | `CompactionConfig` |
| SkillRegistryAdapter | `internal/domain/agent/skill_adapter.go` | ❌ 删除 | ADK `tool.Tool` |
| SkillContext | `internal/domain/skill/context.go` | ❌ 删除 | `tool.Context` |
| ChatService | `internal/service/chat/chat_service.go` | ✏️ 重写 | `runner.Run()` |
| 4 skills | `skills/{sql,stats,kb,save}/` | ✏️ 改写 | ADK tool |
| SecurityAuditor | `internal/service/security/` | 🔄 保留 | ADK `plugin.Plugin` |
| JWT Middleware | `cmd/server/main.go` | 🔄 保留 | 从 gin.Context 提取 userID 注入 Runner |
| Session CRUD | `internal/service/chat/session.go` | ✏️ 适配 | 实现 `session.Service` 接口（MongoDB 后端） |

### 5.2 保留不改的部分

| 模块 | 原因 |
|------|------|
| `SecurityAuditor` (输入/输出/ToolCall 审计) | 作为 ADK plugin 或 callback 注入，逻辑不变 |
| JWT 认证中间件 | gin 层不变，提取 userID → 传 Runner |
| MongoDB Repositories | 数据层不变，被 `session.Service` 适配器调用 |
| 4 个 Skill 的核心逻辑 | 只改签名（`SkillContext` → `tool.Context`），不改业务逻辑 |
| Mock Model Service (mockllm) | 作为 LiteLlm 的 provider 之一，`baseURL=http://mockllm:8082` |

### 5.3 go.mod 新增依赖

```
google.golang.org/adk v0.x.x           // ADK Go SDK
github.com/ieshan/adk-go-memory v1.x.x // Mem0 Go 适配（社区）
```

`LiteLlm` 内置于 ADK，不需要单独 dep。

### 5.4 Session 压缩配置

```go
runner.New(runner.Config{
    PluginConfig: runner.PluginConfig{
        Plugins: []*plugin.Plugin{
            // CompactionPlugin 自动触发摘要压缩
            plugin.NewCompactionPlugin(plugin.CompactionConfig{
                MaxEvents:   100,    // 超过 100 events 压缩
                MaxTokens:   4000,   // 超过 4000 tokens 压缩
                KeepRecent:  20,     // 保留最近 20 events
                Strategy:    plugin.StrategySummarization, // LLM 摘要策略
            }),
        },
    },
})
```

### 5.5 Mem0 集成

**调用方式**：Mem0 分两层，**写自动、读通过 skill**。

```
┌─────────────────────────────────────────────────────┐
│ Mem0 调用架构                                         │
│                                                       │
│  【写入 — 自动】                                      │
│  ADK Runner                                           │
│    │ 每次 session 结束自动调                           │
│    ▼                                                  │
│  MemoryService.AddSessionToMemory(session)             │
│    │ → 提取关键信息（LLM 驱动）                        │
│    │ → 生成 embedding                                  │
│    ▼                                                  │
│  Mem0 / pgvector 存储                                  │
│                                                       │
│  【读取 — 通过 Skill】                                │
│  LLM 决策需要查历史 → 调 memory_search tool             │
│    │                                                  │
│    ▼                                                  │
│  memory_search(tc tool.Context, args)                  │
│    │ → tc.State() 读 user_id（不依赖 LLM 传参）       │
│    │ → MemoryService.SearchMemory(ctx, query, userID)  │
│    ▼                                                  │
│  返回相关记忆片段 → LLM 拼入回答                        │
└─────────────────────────────────────────────────────┘
```

**实现**：

```go
// 1. 初始化 MemoryService（自动写）
import memory "github.com/ieshan/adk-go-memory"

storage, _ := sqlite.NewSQLiteStorage("/data/memory.db")
kit, _ := memory.New(memory.KitConfig{
    Storage:   storage,
    LLM:       modelLLM,
    DeltaMode: true,
})

runner.New(runner.Config{
    MemoryService: kit.Service,
    PluginConfig:  runner.PluginConfig{Plugins: []*plugin.Plugin{kit.Plugin}},
})

// 2. 注册 memory_search 为 ADK tool（供 LLM 调用读取）
type MemorySearchArgs struct {
    Query string `json:"query" jsonschema:"description=搜索关键词"`
    Limit int    `json:"limit" jsonschema:"description=返回结果数"`
}
type MemorySearchResult struct {
    Memories []string `json:"memories"`
}

func MemorySearch(tc tool.Context, m MemoryService, args MemorySearchArgs) (MemorySearchResult, error) {
    userID, _ := tc.State().Get("user_id")  // ← 从 session state 读，不靠 LLM
    results, err := m.SearchMemory(tc, args.Query, userID.(string), args.Limit)
    return MemorySearchResult{Memories: results}, err
}
```

**关键设计**：
- **写**是 ADK Runner 的副作用，不在 LLM 决策范围内
- **读**是 ADK tool（等同于一个 skill），LLM 在需要时主动调用
- `user_id` 从 `tool.Context.State()` 注入，**完全不需要 LLM 在参数里猜**

### 5.6 Embedding 模型（Ollama — 仅 docker-compose.ui-test.yml）

Mem0 语义搜索 + 未来 Qdrant 向量检索需要 embedding 模型。在当前架构中 Missing。

**设计原则**：完全复用 LLM 模型配置体系
- 环境变量兜底（CI 自动注入）
- 后台配置页覆盖（admin 手动修改）
- Vault 存 API key（如有远程 embedding 服务）
- 权限控制复用 `PermUserManage`

#### 5.6.1 基础设施（docker-compose.ui-test.yml）

```yaml
# 新增服务
ollama:
  build:
    context: docker/ollama
    dockerfile: Dockerfile
  expose:
    - "11434"
  volumes:
    - ollama-data:/root/.ollama
  deploy:
    resources:
      limits:
        memory: 1G
  healthcheck:
    test: ["CMD", "curl", "-f", "http://localhost:11434/api/tags"]
    interval: 5s
    timeout: 5s
    retries: 20
    start_period: 120s  # 首次需要 pull 模型，最多 2 分钟
```

```dockerfile
# docker/ollama/Dockerfile
FROM ollama/ollama:latest
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT ["/entrypoint.sh"]
```

```bash
#!/bin/sh
# docker/ollama/entrypoint.sh
ollama serve &
OLLAMA_PID=$!

# 等待 Ollama 就绪
for i in $(seq 1 30); do
  if curl -s http://localhost:11434/api/tags > /dev/null 2>&1; then break; fi
  sleep 1
done

# 模型已存在则跳过，否则 pull
if ! ollama list 2>/dev/null | grep -q "nomic-embed-text"; then
  echo "[ollama] pulling nomic-embed-text (137MB)..."
  ollama pull nomic-embed-text
fi

wait $OLLAMA_PID
```

新增 volume：
```yaml
volumes:
  ollama-data:  # 持久化 embedding 模型，重启不重拉
```

#### 5.6.2 配置层（复用 LLM 模型配置体系）

```
配置来源优先级:
  env > MongoDB system_config > 默认值

env 变量:
  EMBEDDING_BASE_URL=http://ollama:11434/v1
  EMBEDDING_MODEL=nomic-embed-text
  EMBEDDING_API_KEY=            # Ollama 不需要，远程服务才需要
```

**后台配置页扩展**：在现有的 `GET/PUT /api/v1/admin/model-config` 中增加 embedding 字段：

```go
func fillModelConfigDefaults(result gin.H) {
    // ... existing LLM defaults ...

    // Embedding model defaults (same pattern)
    if result["embedding_base_url"] == nil {
        result["embedding_base_url"] = os.Getenv("EMBEDDING_BASE_URL")
        if result["embedding_base_url"] == nil || result["embedding_base_url"] == "" {
            result["embedding_base_url"] = "http://ollama:11434/v1"
        }
    }
    if result["embedding_model"] == nil {
        result["embedding_model"] = os.Getenv("EMBEDDING_MODEL")
        if result["embedding_model"] == nil || result["embedding_model"] == "" {
            result["embedding_model"] = "nomic-embed-text"
        }
    }
    if result["embedding_api_key"] == nil {
        result["embedding_api_key"] = os.Getenv("EMBEDDING_API_KEY")
    }
}
```

权限与 LLM 模型配置完全一致：`middleware.RequirePermission(model.PermUserManage)`。

#### 5.6.3 ADK 集成

```go
// 从 model-config 读取 embedding 配置
cfg := getModelConfig()  // 同一套 MongoDB + Vault

embeddingModel := memory.NewOpenAICompatibleEmbedding(
    memory.OpenAICompatibleEmbeddingConfig{
        BaseURL: cfg.EmbeddingBaseURL,
        Model:   cfg.EmbeddingModel,
        APIKey:  cfg.EmbeddingAPIKey,  // Vault 解密后
    },
)

kit, _ := memory.New(memory.KitConfig{
    Storage:        storage,
    LLM:            modelLLM,
    EmbeddingModel: embeddingModel,
    DeltaMode:      true,
})
```

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No — 复用 MongoDB sessions/tasks/kb_chunks |
| 是否影响现有 API | Yes — `POST /api/v1/chat` 改为 Runner 驱动，但请求/响应格式不变 |
| 性能影响 | ReAct loop 引入最多 `MaxLLMCalls` 次 LLM 调用，需限流（建议 20） |
| 是否需要新增 Skill | No — 4 个 skill 仅改写签名 |
| Go module 兼容性 | Go 1.25 满足 ADK 要求（≥ 1.22） |
| 测试通过率影响 | Engine/Router/Provider 相关 UT 需重写；service 层 UT 需适配 |
| 回滚成本 | 低 — skill 核心逻辑未变，仅 Engine 层替换 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/agent/engine.go` | ❌ 删除 | Delete |
| `internal/domain/agent/provider.go` | ❌ 删除 | Delete |
| `internal/domain/agent/skill_adapter.go` | ❌ 删除 | Delete |
| `internal/domain/agent/engine_test.go` | ❌ 删除 | Delete |
| `internal/domain/skill/context.go` | ❌ 删除 | Delete |
| `internal/service/chat/context_manager.go` | ❌ 删除 | Delete |
| `internal/service/chat/chat_service.go` | ✏️ 重写为 Runner 调用 | Major |
| `internal/service/chat/session.go` | ✏️ 实现 `session.Service` 接口 | Major |
| `cmd/server/main.go` | ✏️ 路由注册 + Runner 初始化 | Major |
| `skills/sql_executor/skill.go` | ✏️ 改写为 ADK tool | Medium |
| `skills/stats_engine/skill.go` | ✏️ 改写为 ADK tool | Medium |
| `skills/knowledge_search/skill.go` | ✏️ 改写为 ADK tool | Medium |
| `skills/save_report/skill.go` | ✏️ 改写为 ADK tool | Medium |
| `docker-compose.ui-test.yml` | ➕ Ollama 服务 + volume | Medium |
| `docker/ollama/Dockerfile` | ➕ Ollama entrypoint | New |
| `docker/ollama/entrypoint.sh` | ➕ Auto-pull 脚本 | New |
| `cmd/server/main.go` | ✏️ model-config 扩展 embedding 字段 | Minor |
| `go.mod` | ➕ `google.golang.org/adk`, `adk-go-memory` | Edit |
| `go.sum` | ➕ 依赖 checksums | Edit |
| `.agent/specs/INDEX.md` | ➕ SPEC-048 | Edit |
| `.agent/specs/spec-004-agent-engine.md` | ✏️ 标注"Engine 已迁移至 ADK" | Edit |

## 8. 测试策略

### 8.1 Go Unit Tests

- **删除**: `engine_test.go` / `skill_adapter_test.go`（过时，基于旧 Engine）
- **重写**: `chat_service_test.go`（基于 `llmagent` mock）
- **新增**: ADK tool UT（每个 skill 对应一个 `*_tool_test.go`）
- **保留**: Security Auditor UT（不改逻辑）
- 覆盖率底线：保持 ≥98%（SPEC-045）

### 8.2 E2E 测试

本 spec 完成后，**SPEC-046 的工具调用 E2E 测试才能执行**：
- UI-211~218（工具调用链）依赖 ADK ReAct loop
- UI-204~209（KB 索引）依赖 `tool.Context.State()` 注入 `kb_id`

### 8.3 审计

使用 `.agent/skills/go-ut-audit` 审查所有重构后的 UT 质量。

## 9. UI Test / E2E 验收规则

- [x] 本 spec 为纯后端引擎替换，不涉及前端变更
- [x] 完成后 CI sonar-check + ui-tests 必须通过
- [x] SPEC-046 的 E2E 测试建立在本 spec 之上

## 9.5. Go Unit Test 验收规则

- 覆盖率底线 ≥98%（SPEC-045）
- 每个 ADK tool 必须有 UT（含 Success + Error + SessionState 三个场景）
- Runner 层测试用 `llmagent` mock model

## 10. 验证标准

| 指标 | 当前 | 目标 |
|------|------|------|
| Engine 工具调用 | ❌ 不执行 | ✅ 完整 ReAct loop |
| Session 压缩 | ❌ 简单截断 | ✅ LLM 摘要压缩 |
| 模型路由 | ❌ 单 map 查名 | ✅ LiteLlm fallback 链 |
| Skill session 绑定 | ❌ LLM 传参 | ✅ `tool.Context.State()` |
| POST /api/v1/chat 行为 | 单次 LLM + 审计 | Runner 驱动 → 自动 ReAct → SSE 输出 |
| 请求/响应格式兼容 | — | ✅ 不变（仍收 JSON，仍出 SSE） |

## 11. 实施步骤

1. **第 1-2 天**: 新建 `internal/adk/` 包，实现 `session.Service` MongoDB 适配器 + `memory.Service` Mem0 适配器
2. **第 3-4 天**: 改写 4 个 skill 为 ADK tool（签名 + UT）
3. **第 5-6 天**: 重写 `chat_service.go` 为 Runner 调用，删旧 Engine/Router/Provider
4. **第 7 天**: `cmd/server/main.go` 初始化 Runner，跑 UT + E2E
5. **第 8 天**: Session 压缩 + Mem0 集成 + CI 联调

每步 CI 必须通过才能进入下一步。**禁止绕过 CI**（项目红线）。
