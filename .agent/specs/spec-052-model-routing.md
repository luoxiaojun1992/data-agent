# 多模型路由与用途关联

> **SPEC-052** | Status: 设计中 | Date: 2026-07-19 | Phase: P13

## 1. 目标

实现多模型配置与智能路由，允许管理员在后台配置多个 LLM 模型，并为不同使用场景（Chat、Agent Task、Embedding、会话压缩摘要）关联最合适的模型。模型选择支持自动（基于能力标签/消耗倍率）和手动指定两种方式。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施 — MongoDB 系统配置 |
| SPEC-025 | ✅ | UI — 模型配置页已有基础 |
| SPEC-048 | ✅ | ADK 迁移 — model.LLM 接口就绪 |
| SPEC-049 | ✅ | 统一模型配置 — Provider、ModelEntry 已有 Capability/TokenMultiplier 字段 |

## 2. 背景

### 2.1 现状

- `modelcfg.Provider.BuildLLM()` 只返回第一个模型，不区分使用场景
- 压缩摘要 (`session/summarizer.go`) 硬编码使用同一个 LLM
- Embedding 模型用独立 `EmbeddingEntry` + `embedding` key，与 LLM 列表割裂
- 前端模型配置页（SPEC-025）LLM 和 Embedding 在同一个页面但使用不同数据结构
- 管理/配置碎片化：模型类型不同、存储 key 不同、Provider API 不同

### 2.2 目标

统一为一套模型配置体系：

```
┌─────────────────────────────────────────────────────────┐
│                   Admin 模型配置 (同一页面)               │
│  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐     │
│  │ gpt-4o       │ │ gpt-4o-mini  │ │ nomic-embed  │     │
│  │ 类型: LLM    │ │ 类型: LLM    │ │ 类型: Embed  │     │
│  │ 关联: chat   │ │ 关联: enh    │ │ 关联: embed  │     │
│  │ 倍率: 1.0x   │ │ 倍率: 0.5x   │ │              │     │
│  └──────────────┘ └──────────────┘ └──────────────┘     │
│                  ↓ 统一存储到一个 key                    │
│         MongoDB system_config["model"]["models"]         │
└─────────────────────────────────────────────────────────┘
                     │
                     ▼
┌─────────────────────────────────────────────────────────┐
│              modelcfg.Provider                           │
│  BuildLLM(UseCaseChat)       → 匹配 Type=LLM + uc       │
│  BuildLLM(UseCaseCompaction) → 匹配 Type=LLM + uc       │
│  EmbeddingConfig()           → 匹配 Type=Embedding      │
└─────────────────────────────────────────────────────────┘
```

## 3. 架构概述

### 3.1 设计原则

- **Provider 是唯一入口**：所有模型构建通过 `modelcfg.Provider`
- **用途优先**：按 `UseCase` 选模型，不支持多模型 fallback 链
- **能力标签匹配**：模型声明 `Capability`（如 `"chat"`, `"code"`），请求按 `UseCase` 精确或模糊匹配
- **消耗倍率排序**：多个模型满足条件时，选 `TokenMultiplier` 最低的
- **索引独立**：`UseCaseEmbedding` 和 `UseCaseCompaction` 有独立配置 key，不与 LLM 列表混用

### 3.2 UseCase 枚举

| UseCase | 适用模型类型 | 用途 |
|---------|:---------:|------|
| `chat` | LLM | 对话（Chat / Hermes） |
| `task` | LLM | Agent 任务（数据分析） |
| `enhance` | LLM | 提示词增强 |
| `compaction` | LLM | 会话压缩摘要（summarization） |
| `kb_chunking` | LLM | KB 索引语义切片（SPEC-053，可选开关） |
| `embedding` | Embedding | 向量嵌入（KB 索引） |

Provider API：
```go
// 构建 LLM（按 UseCase 匹配 Type=llm 的模型）
func (p *Provider) BuildLLM(ctx context.Context, uc UseCase) (model.LLM, error)

// 构建 Embedding 模型（匹配 Type=embedding + UseCases 含 "embedding"）
func (p *Provider) EmbeddingConfig() EmbeddingConfig
```

> **Embedding 不走 BuildLLM**：Embedding 模型协议（文本→向量）与 Chat Completions 不同，`BuildLLM` 只匹配 `Type=llm`。Embedding 通过 `EmbeddingConfig()` 构建。

### 3.3 ModelEntry 统一结构

```go
// ModelType 区分 LLM 和 Embedding 模型
type ModelType string

const (
    ModelTypeLLM       ModelType = "llm"
    ModelTypeEmbedding ModelType = "embedding"
)

type ModelEntry struct {
    Name            string    `json:"name"`
    BaseURL         string    `json:"base_url"`
    APIKey          string    `json:"-"`              // Vault encrypt
    Type            ModelType `json:"type"`            // "llm" or "embedding"
    Instruction     string    `json:"instruction"`     // 仅 LLM
    Capability      string    `json:"capability"`      // 仅 LLM
    UseCases        []string  `json:"use_cases"`       // 强制：Type=llm → 选 ["chat","task","enhance","compaction","kb_chunking"], Type=embedding → 只能 ["embedding"]
    TokenMultiplier float64   `json:"token_multiplier"`
    Temperature     float64   `json:"temperature"`     // 仅 LLM
    MaxTokens       int       `json:"max_tokens"`      // 仅 LLM
    IsDefault       bool      `json:"is_default"`
}
```

**UseCases 约束**：
| Type | 可选 UseCase | 说明 |
|------|-------------|------|
| `llm` | `chat`, `task`, `enhance`, `compaction`, `kb_chunking` | 不能选 `embedding` |
| `embedding` | `embedding` | 只能选 `embedding`，不能选 LLM 用途 |

**废弃旧结构**：`EmbeddingEntry` 删除，`embedding` 系统配置 key 删除。所有模型统一存在 `system_config["model"]["models"]`。

### 3.4 Provider API 变更

```go
// BuildLLM 构建 LLM（按 UseCase 匹配 Type=llm 的模型）
func (p *Provider) BuildLLM(ctx context.Context, uc string) (model.LLM, error)

// EmbeddingConfig 返回 Embedding 模型配置（匹配 Type=embedding）
func (p *Provider) EmbeddingConfig() EmbeddingConfig
```

**LLM 选择算法**：
```
1. 过滤 models[]: Type=llm AND UseCases 包含 uc
2. 无匹配 → 过滤 IsDefault=true
3. 排序: TokenMultiplier 升序
4. 返回第一个
```

## 4. 数据库变更

### 4.1 system_config 数据

| Namespace | Key | Value (JSON) | 说明 |
|-----------|-----|-------------|------|
| model | `models` | `[{type:"llm",use_cases:["chat"],...}, {type:"embedding",use_cases:["embedding"],...}]` | **全局唯一模型列表** |

**删除旧 key**：`embedding` (embedding 配置)、`compaction` (压缩模型) — 全部合并到 `models` 列表。

## 5. 涉及的代码变更

### 5.1 modelcfg 包

| 文件 | 变更 |
|------|------|
| `internal/adk/modelcfg/provider.go` | 新增 `UseCase`、`UseCases` 字段、`BuildLLM(ctx, UseCase)` 路由逻辑、`BuildEmbeddingLLM`、`BuildCompactionLLM` |
| `internal/adk/modelcfg/provider_test.go` | 新增路由测试：Capability 匹配、UseCases 匹配、倍率排序、默认兜底 |

### 5.2 调用方变更

| 文件 | 当前 | 改为 |
|------|------|------|
| `cmd/server/main.go` — Chat | `deps.modelCfg.BuildLLM(ctx)` | `deps.modelCfg.BuildLLM(ctx, "chat")` |
| `cmd/server/main.go` — Agent | 同上 | `deps.modelCfg.BuildLLM(ctx, "task")` |
| `cmd/server/main.go` — Enhance | `buildEnhanceHandler` 用全局 LLM | `deps.modelCfg.BuildLLM(ctx, "enhance")` |
| `cmd/server/main.go` — Session | 无 compaction 模型配置 | `deps.modelCfg.BuildLLM(ctx, "compaction")` |
| `cmd/server/main.go` — KB Chunk | 新增 | `deps.modelCfg.BuildLLM(ctx, "kb_chunking")` （SPEC-053 开关开启时） |
| `internal/adk/session/summarizer.go` | 接受 `model.LLM` | 不变（已有参数，传入 compaction 模型即可） |
| Embedding 初始化 | 从 `EmbeddingEntry` 读 | 从 `models` 列表过滤 `Type="embedding"` |

### 5.3 UI 变更（同一页面）

| 变更 | 说明 |
|------|------|
| 模型配置页 | LLM 和 Embedding 合并为**同一个列表**，不再分区 |
| 模型卡片 | 新增"类型"下拉（LLM / Embedding），决定卡片显示哪些字段 |
| 关联用途 | 多选组件，选项根据 Type 动态过滤：LLM→chat/task/enhance/compaction，Embedding→仅 embedding |
| Embedding 卡片 | 不显示 Instruction、Temperature、MaxTokens、Capability（仅 LLM 相关） |
| LLM 卡片 | 不显示 embedding 相关选项 |

## 6. 验证标准

### 6.1 单元测试

- Capability 精确匹配
- UseCases 优先于 Capability
- TokenMultiplier 排序正确
- 无匹配时 IsDefault 兜底
- 空列表返回 error

### 6.2 E2E 测试

- 配置多个模型 → Chat 使用正确模型
- Agent Task 使用高能力低倍率模型
- KB 索引使用独立 embedding 模型
- 压缩摘要使用独立轻量模型

### 6.3 API 不变

- Admin GET/PUT `/api/v1/admin/model-config` 格式兼容，新增字段可忽略
- Chat SSE 行为不变

## 7. 实施顺序

| 阶段 | 内容 |
|------|------|
| 1 | modelcfg Provider 重构：新增 UseCase、路由逻辑 |
| 2 | 调用方适配：main.go、chat/agent service |
| 3 | 压缩摘要独立模型配置 |
| 4 | UI 用途关联选择 |
| 5 | E2E 测试 |
