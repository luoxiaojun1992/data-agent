# 统一模型配置与多模型能力体系

> **SPEC-049** | Status: 设计中 | Date: 2026-07-18 | Phase: P12

## 1. 目标

将 LLM/Embedding 模型配置从「env 单模型」升级为「管理后台统一配置的多模型体系」，并为每个模型支持独立的**系统提示词、能力描述、Token 计费倍率**，同时让知识库索引/检索真正使用配置的 Embedding 模型（实装 `semanticSearch`，当前为占位返回 nil）。

具体能力：
1. **多模型注册**：model-config 页支持模型列表（主模型 + fallback 链），每模型独立 `base_url` / `api_key`(Vault) / `temperature` / `max_tokens`
2. **每模型元数据**：`instruction`（系统提示词）、`capability`（能力描述，供 Agent 选择模型）、`token_multiplier`（Token 计费倍率）
3. **引擎消费配置**：ADK runtime 从 MongoDB `system_config` 读取模型配置构建 `FallbackLLM`（env 作为兜底默认值）
4. **KB Embedding 索引**：文档 chunk 写入时调用配置的 embedding 模型生成向量 → 存 Qdrant；`semanticSearch` 实装向量检索并与全文检索 RRF 融合

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | system_config 存储 + Vault |
| SPEC-006 | ✅ | 知识库（kb_chunks 集合 + Qdrant client） |
| SPEC-025 | ✅ | 模型配置页 UI（需扩展模型列表） |
| SPEC-048 | ⚠️ **阻塞依赖** | ADK 迁移（FallbackLLM / memory embedding / chat enhance 端点）必须合入 |

## 2. 背景

SPEC-048 迁移后，模型配置仍是 env 驱动（`LLM_MODEL`/`LLM_BASE_URL`/`LLM_FALLBACK_BASE_URLS`），存在三个缺口：

1. **管理后台 model-config 页与引擎脱节**：配置写入 `system_config` 但引擎从不读取，管理员改配置不生效
2. **无每模型个性化**：fallback 链共享同一提示词和参数，无法表达「主模型强推理、备用模型快且便宜」
3. **KB 语义检索是占位符**：`semanticSearch()` 直接 `return nil`，知识库只有全文检索，embedding 基础设施（SPEC-048 的 Ollama）未用于 KB

## 3. 架构概述

```
┌─────────────────────────────────────────────────────────────┐
│  管理后台 model-config 页（扩展）                              │
│    models: [                                                 │
│      { name, base_url, api_key(vault), instruction,          │
│        capability, token_multiplier, temperature,            │
│        max_tokens, is_default, fallback_order }, ...         │
│    embedding: { base_url, model, api_key(vault) }            │
│  ]                                                           │
└──────────────┬──────────────────────────────────────────────┘
               │ MongoDB system_config（env 兜底）
               ▼
┌──────────────────────────┐   ┌──────────────────────────────┐
│  ModelConfigProvider     │   │  KB Index Pipeline           │
│  (internal/adk/modelcfg) │   │  AddChunks → embed(chunk)    │
│  • Load() 读配置          │   │  → Qdrant upsert             │
│  • BuildLLM() → FallbackLLM│ │  Search → embed(query)       │
│  • Instruction 注入 agent │  │  → 向量检索 → RRF 融合        │
└──────────────────────────┘   └──────────────────────────────┘
```

## 4. API 设计

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/admin/model-config` | 扩展返回 `models[]` 数组 + `embedding` 对象（向后兼容旧字段） |
| PUT | `/api/v1/admin/model-config` | 支持整体写入 `models[]` 与 `embedding`；api_key 经 Vault 加密 |
| GET | `/api/v1/admin/model-config/models` | 列出模型（脱敏，含 capability/token_multiplier） |

权限：沿用 `model.PermUserManage`。

## 5. 详细设计

### 5.1 数据模型（system_config）

```go
// ModelEntry 单个 LLM 模型配置
type ModelEntry struct {
    Name            string  `json:"name" bson:"name"`
    BaseURL         string  `json:"base_url" bson:"base_url"`
    APIKey          string  `json:"-" bson:"api_key"`           // Vault 加密存储
    Instruction     string  `json:"instruction" bson:"instruction"`     // 系统提示词
    Capability      string  `json:"capability" bson:"capability"`       // 能力描述
    TokenMultiplier float64 `json:"token_multiplier" bson:"token_multiplier"` // Token 计费倍率，默认 1.0
    Temperature     float64 `json:"temperature" bson:"temperature"`
    MaxTokens       int     `json:"max_tokens" bson:"max_tokens"`
    IsDefault       bool    `json:"is_default" bson:"is_default"`
    FallbackOrder   int     `json:"fallback_order" bson:"fallback_order"` // 0=主模型, 1..N=fallback 顺序
}

// EmbeddingEntry embedding 模型配置（SPEC-048 已定义字段，纳入统一页）
type EmbeddingEntry struct {
    BaseURL string `json:"base_url" bson:"base_url"`
    Model   string `json:"model" bson:"model"`
    APIKey  string `json:"-" bson:"api_key"`
}
```

### 5.2 ModelConfigProvider（`internal/adk/modelcfg/`）

```go
// Provider 从 system_config 加载模型配置，env 兜底
type Provider struct { repo *mongoinfra.SystemConfigRepository; vault *vaultinfra.Client }

func (p *Provider) Models(ctx context.Context) ([]ModelEntry, error)
func (p *Provider) Embedding(ctx context.Context) (EmbeddingEntry, error)

// BuildLLM 按 fallback_order 构建 FallbackLLM 链
func (p *Provider) BuildLLM(ctx context.Context) (model.LLM, error)

// DefaultInstruction 返回主模型的系统提示词（空则回退 runtime.DefaultInstruction）
func (p *Provider) DefaultInstruction(ctx context.Context) string
```

加载优先级：MongoDB `system_config.models` > env（`LLM_MODEL`/`LLM_BASE_URL`/`LLM_API_KEY`/`LLM_FALLBACK_BASE_URLS`）。服务启动时加载一次 + 管理后台 PUT 后热更新（重 build runner，或接受重启生效——MVP 接受重启生效，文档注明）。

### 5.3 ADK Runtime 适配

`adkruntime.Config` 增加 `Instruction` 已有；`initServices` 改为：
1. `modelcfg.Provider.BuildLLM()` → `FallbackLLM`
2. `Provider.DefaultInstruction()` → llmagent instruction（主模型独立提示词生效）
3. 每模型 `Capability`/`TokenMultiplier` 存入 `internal/adk/model.Backend` 扩展字段，供 SPEC-051 计费使用

### 5.4 KB Embedding 索引与检索

`internal/service/knowledge/service.go`：

```go
// AddChunks 扩展：每个 chunk 生成 embedding 后写 Mongo + Qdrant
func (s *Service) AddChunks(docID string, chunks []string) error
  // 1. embed(chunk) via adkmemory.NewOpenAIEmbedding(配置)
  // 2. kb_chunks 集合写入（含 embedding 维度元信息）
  // 3. Qdrant upsert (collection: kb_chunks, id: chunk_id, vector, payload: doc_id/user_id/text)

// semanticSearch 实装
func (s *Service) semanticSearch(query string, topK int) []knowledge.SearchResult
  // 1. embed(query) → Qdrant Search(topK*2)
  // 2. 映射回 SearchResult（score = 向量相似度）
  // 3. 与 fullTextSearch 结果 RRF 融合（现有 rrfFusion 复用）
```

embedding 模型配置读取：与 `modelcfg.Provider.Embedding()` 一致（env 兜底 `EMBEDDING_*`）。embedding 调用失败时降级：仅全文检索（不阻塞写入）。

### 5.5 前端 model-config 页扩展

- 模型列表编辑器（增删模型、拖拽 fallback 顺序）
- 每模型字段：instruction（textarea）、capability、token_multiplier（number）
- embedding 区块（SPEC-048 已加默认值回填，本 spec 加表单）

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No — 复用 system_config；Qdrant 新增 `kb_chunks` collection（向量库，非 Mongo） |
| 是否影响现有 API | 兼容 — GET model-config 保留旧字段，新增 models/embedding 字段 |
| 性能影响 | AddChunks 增加 N 次 embedding 调用（可批量）；检索增加 1 次 embedding + 1 次向量查询 |
| 是否需要新增 Skill | No |
| Qdrant client 就绪 | ✅ SPEC-003 已有 client |
| Embedding 基础设施 | ✅ SPEC-048 Ollama（docker-compose.ui-test.yml） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/modelcfg/provider.go` | 配置加载 + BuildLLM | New |
| `internal/adk/modelcfg/provider_test.go` | UT | New |
| `internal/adk/model/openai.go` | Backend 扩展 Capability/TokenMultiplier | Minor |
| `internal/service/knowledge/service.go` | AddChunks embedding + semanticSearch 实装 | Major |
| `internal/service/knowledge/knowledge_test.go` | UT 更新 | Major |
| `internal/infra/qdrant/qdrant.go` | 确认 Search/Upsert 能力（不足则补） | Minor |
| `cmd/server/main.go` | model-config handler 扩展 models[] + initServices 消费 Provider | Major |
| `frontend/app/admin/model-config/page.tsx` | 模型列表编辑器 + embedding 表单 | Major |
| `tests/ui/model-config.spec.ts` | 扩展 E2E | Major |

## 8. 测试策略

1. **Unit tests**（Go）: `modelcfg` 100%（配置加载/兜底/BuildLLM 链序）；KB embedding 索引与向量检索（gomonkey mock embedding func + qdrant client）；覆盖率基线见 SPEC-045
2. **Integration tests**: docker-compose 环境 Qdrant 真实向量写入/检索
3. **E2E tests**: 管理后台配置多模型 → 保存 → GET 回显一致；KB 上传文档 → 索引后语义检索命中（UI-2XX 新增用例）
4. **审计**: `.agent/skills/go-ut-audit`

## 9. UI Test / E2E 验收规则

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

| Tier | 目标 |
|:---:|------|
| L1 纯逻辑 | **100%** |
| L2 接口依赖 | **100%** |
| L3 集成依赖 | **98%** |
| Overall | ≥98%（CI `ut-workflow.yml` gate） |

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] `go test -race -gcflags=all=-l` 全部通过，`go vet` 无警告

## 10. 验证标准

| 指标 | 当前 | 目标 |
|------|------|------|
| 管理后台配置模型 → 引擎生效 | ❌ 不消费 | ✅ BuildLLM 按配置构建 |
| 每模型提示词/能力描述/倍率 | ❌ 无 | ✅ 配置化且引擎使用 |
| KB semanticSearch | ❌ 占位 nil | ✅ 向量检索 + RRF 融合 |
| KB 索引 embedding | ❌ 无向量 | ✅ chunk 写入即生成向量入 Qdrant |
| embedding 配置 | env only | ✅ 后台可配（env 兜底） |
