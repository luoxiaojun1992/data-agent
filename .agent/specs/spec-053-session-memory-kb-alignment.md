# 会话存储、记忆压缩与知识库索引逻辑对齐

> **SPEC-053** | Status: 设计中

## 1. 目标

明确 chat session 和 hermes session 的聊天记录存储、LLM session history 压缩、KB 索引和删除/恢复策略，确保当前代码实现与预期行为对齐。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 (Agent 引擎) | ✅ | ADK session 管理已就绪 |
| SPEC-006 (知识库) | ✅ | KB 索引基础设施就绪 |
| SPEC-048 (ADK 迁移) | ✅ | ADK 版 session/history 机制生效 |
| — | — | 无阻塞依赖 |

## 3. 架构概述

### 3.1 Chat Session 存储双轨模型

```
┌──────────────────────────────────────────┐
│              Chat Request                │
└──────────────────┬───────────────────────┘
                   ▼
          ┌────────────────┐
          │  ADK Runner    │
          │  (ReAct loop)  │
          └───────┬────────┘
                  │
     ┌────────────┼────────────┐
     ▼                         ▼
┌──────────────┐    ┌──────────────────────┐
│ 前端聊天记录  │    │  LLM Session History  │
│ (Chat Record)│    │  (压缩后上下文)        │
│              │    │                      │
│ 原始对话     │    │ LLM 摘要压缩           │
│ 不压缩       │    │ 用于后续 LLM context  │
│ 前端展示用   │    │ 不传给前端            │
└──────┬───────┘    └──────────┬───────────┘
       │                       │
       └───────┬───────────────┘
               │ 删除/恢复时联动清除
               ▼
        ┌──────────────┐
        │  Session 管理  │
        │  Create/Delete │
        │  Restore       │
        └──────────────┘
```

### 3.2 KB 索引流程

```
Chat 完成
  → scheduleMemoryWrite (异步 goroutine, 30s timeout)
    → memoryService.AddSessionToMemory
      → LLM 对对话记录切片 → (统计 token)
        → Embedding 向量化 (Redis 缓存) → (统计 token)
          → 写入 Qdrant
```
```
Session 压缩
  → LLM 摘要生成 → (统计 token)
    → 写入 ADK Session History (LLM session state)
```
```
提示词增强
  → callEnhanceLLM (POST /chat/enhance)
    → LLM 调用 (非流式)
      → Redis 缓存: enhance:{model}:{prompt_hash} (TTL 1h)
    → recordEnhanceTokens → (统计 token)
```

### 3.3 Session 压缩流程

```
Chat 完成
  → scheduleMemoryWrite (同上异步链路)
    → LLM 摘要生成
      → 存入 LLM Session History (ADK session)
        → 不影响前端聊天记录
```

### 3.4 Hermes Session 存储

```
┌──────────────────────────┐
│  Hermes Chat Request      │
└──────────┬───────────────┘
           ▼
    ┌──────────────┐
    │ Hermes Session │  (独立存储)
    │ 原始聊天记录   │
    │ 无压缩         │
    │ 无 KB 索引     │
    └──────┬───────┘
           │
           ▼
    ┌──────────────┐
    │ 删除: 整 session │
    │ 联动删除 Hermes  │
    │ session          │
    │ 不支持恢复       │
    └──────────────────┘
```

## 5. 详细设计

### 5.1 数据存储表

| 存储 | 内容 | 压缩 | 删除粒度 | 恢复 |
|------|------|:---:|----------|:---:|
| 前端聊天记录 (Chat Record) | 原始对话 | ❌ | 整 session | ✅ |
| LLM Session History | 摘要后上下文 | ✅ | 整 session | ✅ |
| KB 索引 (Qdrant) | 切片向量 | ❌ (向量) | 整 session | ✅ (重建) |
| Hermes Chat Record | Hermes 原始对话 | ❌ | 整 session | ❌ |

### 5.2 删除行为

- **Chat Session**: 联动删除 Chat Record + LLM Session History + KB 向量
- **Hermes Session**: 联动删除 Hermes Chat Record + Hermes Session
- **都不支持**部分删除聊天记录后继续用同一个 session
- **Chat Session 支持恢复**: 恢复时重建 Chat Record + LLM History（KB 索引需重新触发）

### 5.3 Chat vs Hermes 行为对比

| 特性 | Chat | Hermes |
|------|:---:|:---:|
| 前端聊天记录存储 | ✅ | ✅ 独立 |
| KB 索引 (Qdrant) | ✅ | ❌ |
| Session 压缩 (LLM 摘要) | ✅ | ❌ |
| 联动删除 | 三条链路 | Hermes 链路 |
| Session 恢复 | ✅ | ❌ |
| 部分删除聊天记录 | ❌ | ❌ |

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（使用现有 session/KB 集合） |
| 是否影响现有 API | 需确认：删除/恢复 API 是否已联动三条链路 |
| 性能影响 | 异步写入，无阻塞 |
| 是否需要新增 Skill | No |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/chat/chat_service.go` | chat SSE/非流式响应 + memory hook | Review |
| `cmd/server/main.go` | WithMemoryWrite 注册 | Review |
| `internal/adk/memory/` | AddSessionToMemory (KB 索引 + session 压缩) | Review |
| `internal/api/session/` | session 删除/恢复 API | Review |
| `internal/service/hermes/` | Hermes session 管理 | Review |
| `internal/infra/llmcache/` | Embedding + Enhance Redis 缓存 | Review |

## 9. UI Test / E2E 验收规则

- 无新增 UI 用例（纯后端逻辑对齐 spec）
- 若发现差异需修改代码，相关 UT 覆盖变更路径

## 10. Redis 缓存场景

以下场景使用 Redis 缓存，减少重复 LLM/Embedding 调用：

| 场景 | 缓存 Key | TTL | 备注 |
|------|---------|-----|------|
| KB 索引 Embedding 向量化 | `emb:{model}:{text_hash}` | 待确认 | 同一文本段落在不同 session 中复用向量，避免重复调 embedding 模型 |
| 提示词增强 (Enhance) | `enhance:{model}:{prompt_hash}` | 1h | `llmcache.SetEnhance` / `GetEnhance`，相同 prompt 跨 session/session 复用 |

> 两个缓存链路都在 `internal/infra/llmcache/` 中实现。

## 11. 统一 LLM/Embedding 调用路径

所有 LLM 调用**必须**通过 ADK 社区适配器（`openai.New` → `model.LLM`），Embedding 调用使用独立统一客户端：

| 场景 | 调用路径 | 适配器 | 备注 |
|------|---------|:---:|------|
| Chat 对话 | `handleStream/handleNonStreamChat` → `s.rt.Run()` | ADK | 流式/非流式 |
| Session 压缩摘要 | `adksession.NewLLMSummarizer(llm)` → `llm.GenerateContent` | ADK ✅ | 已统一 |
| 提示词增强 (Enhance) | `callEnhanceLLM` → **直接 HTTP** | ❌ 待统一 | 改为 `modelCfg.BuildLLM` + `GenerateContent(stream=false)` |
| KB 索引 Embedding | `buildEmbedFn` → `adkmemory.NewOpenAIEmbedding` | 独立 Embedding 客户端 | `/v1/embeddings` 端点不同，不能共用 chat 适配器 |
| KB 索引文本提取 | `extractTexts` → 纯文本截断 | N/A | 无 LLM 调用 |

> **待办**: 提示词增强改为走 `modelCfg.BuildLLM(context.Background())` 返回的 `model.LLM`，非流式 `GenerateContent`。统一后享受以下能力：
> - 模型路由/fallback 链（SPEC-052）
> - 统一连接池
> - Token 统计自动触发（SPEC-051）
> - Redis 缓存命中时跳过 LLM 调用

## 12. Token 消耗统计

所有 LLM 和 Embedding 调用**必须**统计 token 消耗，覆盖以下场景：

| 场景 | 调用类型 | Token 统计入口 | 备注 |
|------|----------|---------------|------|
| Chat 对话 | LLM (流式/非流式) | ADK runner 内置 | SPEC-051 已覆盖 |
| 工具调用 | LLM (ReAct loop) | ADK runner 内置 | 每次 LLM call 独立统计 |
| 提示词增强 | LLM (非流式) | `recordEnhanceTokens` | 增强请求/响应 token |
| Session 压缩摘要 | LLM | 待确认 | Memory deriver 内部调用需补统计 |
| KB 索引文本切片 | LLM | 待确认 | Memory deriver 内部调用需补统计 |
| KB 索引 Embedding | Embedding 模型 | 待确认 | 向量化 token 量统计 |
| Hermes 探索 | LLM (自由模式) | ADK runner | SPEC-012，需确认是否独立统计 |

> Token 统计写入 `token_usage` 集合（MongoDB），字段包含 model / prompt_tokens / completion_tokens / purpose / session_id / timestamp。
> Redis 缓存命中时不计 token（未实际调用 LLM/Embedding）。
