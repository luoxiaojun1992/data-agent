# Go 1.26 升级与 adk-go-memory 迁移（含记忆相似度合并）

> **SPEC-050** | Status: 设计中 | Date: 2026-07-18 | Phase: P12

## 1. 目标

1. 全仓库 Go 工具链升级 **1.25 → 1.26**（go.mod / CI / Dockerfile / lint 工具链）
2. 长期记忆从 SPEC-048 自实现（`internal/adk/memory`）迁移到社区库 `ieshan/adk-go-memory`（获得 LLM 事实提取 deriver、成熟的 compaction、DeltaMode 增量写入）
3. **记忆相似度合并**：新记忆写入前与存量记忆做 embedding 余弦相似度计算，≥ 阈值时合并/更新而非新增，防止记忆无限膨胀（**明确不做摘要缓存**）

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-048 | ⚠️ **阻塞依赖** | ADK 迁移（memory.Service 接口使用方、Ollama embedding 基础设施）必须合入 |
| SPEC-049 | ⚠️ **阻塞依赖** | embedding 模型统一配置（相似度计算依赖配置的 embedding 模型） |

## 2. 背景

SPEC-048 因 `ieshan/adk-go-memory` 全部版本要求 `go >= 1.26` 而自实现了 `internal/adk/memory`（MongoDB + embedding + 余弦检索，98% UT 覆盖，功能对齐 SPEC-046 验收点）。决策升级 go 1.26 后迁移到社区库，理由：

- **deriver**：LLM 驱动的事实提取（从对话抽取结构化记忆），比「整段消息文本入库」检索质量高
- **维护成本**：自实现 300+ 行及测试的长期维护转移给上游
- **compaction 对齐**：库的 compaction 包与 ADK 生态同步演进

风险与缓解：
- 库成熟度 v1.0.x → 保留 `internal/adk/memory` 作为编译期可切换的 fallback（构建标签或配置开关），一个版本周期后删除
- 库绑定 ADK v1.2.0 vs 我们 v1.5.0 → 升级后全量 UT + E2E 验证接口兼容性（`memory.Service` 接口稳定）

## 3. 架构概述

```
┌────────────────────────────────────────────────────────────┐
│ Go 1.26 升级面                                              │
│  go.mod (go 1.26) → CI setup-go 1.26 → golang:1.26-alpine  │
│  → golangci-lint 兼容版本 → gomonkey/mockery 兼容性验证      │
└────────────────────────────────────────────────────────────┘

┌────────────────── Before (SPEC-048) ───────────────────────┐
│ internal/adk/memory (自实现)                                │
│  AddSessionToMemory: 整段消息文本 + embedding 入库           │
│  SearchMemory: 余弦相似度 top5                               │
└────────────────────────────────────────────────────────────┘
                        ▼
┌────────────────── After (SPEC-050) ────────────────────────┐
│ ieshan/adk-go-memory Kit                                    │
│  • Deriver: LLM 事实提取（对话 → 关键事实）                  │
│  • Storage: MongoDB 适配器（库 adapter.Storage 接口）         │
│  • Embedding: 配置的 embedding 模型（SPEC-049）               │
│  • DeltaMode: 增量处理新事件                                  │
│  • ★ 相似度合并: 写入前 SearchMemory(query=新记忆)            │
│    similarity ≥ 0.92 → 更新已有记忆(merged) 而非 Insert       │
└────────────────────────────────────────────────────────────┘
```

## 4. 详细设计

### 4.1 Go 1.26 升级清单

| 文件 | 变更 |
|------|------|
| `go.mod` | `go 1.25.0` → `go 1.26.0`，`go mod tidy` 全量 |
| `Dockerfile` | `golang:1.25-alpine` → `golang:1.26-alpine` |
| `mockllm/Dockerfile` | 同上（如有） |
| `.github/workflows/*.yml` | `go-version: '1.25'` → `'1.26'`（lint-check / ut-workflow / ui-tests） |
| golangci-lint | 升级到兼容 go 1.26 的版本（CI image 同步） |
| 兼容性验证 | gomonkey（运行时 patch 对 go 版本敏感）、mockery、ginkgo 全量 UT 验证 |

### 4.2 MongoDB Storage 适配器

```go
// internal/adk/memoryx/mongo_storage.go
// 实现 adk-go-memory 的 adapter.Storage 接口，复用 memories 集合
type MongoStorage struct { coll *mongo.Collection }

func (s *MongoStorage) Save(ctx context.Context, m Memory) error        // upsert by ID（幂等）
func (s *MongoStorage) Get(ctx context.Context, id string) (Memory, error)
func (s *MongoStorage) Delete(ctx context.Context, id string) error     // 不存在不报错（幂等）
func (s *MongoStorage) List(ctx context.Context, userID string) ([]Memory, error)
func (s *MongoStorage) Search(ctx context.Context, userID string, vector []float32, topK int) ([]Memory, error)
```

### 4.3 相似度合并

写入路径（`AddSessionToMemory` → deriver 产出候选事实后）：

```go
// 合并策略（无摘要缓存，纯相似度）
const mergeThreshold = 0.92 // 余弦相似度

for _, fact := range derivedFacts {
    vec := embed(fact.Text)
    existing, _ := storage.Search(ctx, userID, vec, 1)
    if len(existing) > 0 && cosine(existing[0].Vector, vec) >= mergeThreshold {
        // 合并：更新已有记忆的文本（取信息更全者）+ 刷新时间戳 + 来源 session 列表追加
        storage.Save(ctx, mergeMemories(existing[0], fact))
    } else {
        storage.Save(ctx, newMemory(fact, vec))
    }
}
```

- 相似度计算复用 SPEC-048 `cosine` 实现（迁移到 `internal/adk/memoryx/`）
- **明确排除**：不缓存 LLM 摘要结果、不做压缩结果复用（用户明确要求）

### 4.4 切换与回退

- 配置开关 `MEMORY_BACKEND=adk-memory|legacy`（默认 `adk-memory`）
- `legacy` 保留 SPEC-048 自实现，一个版本周期（至 SPEC-052 前）后删除
- `/api/v1/admin/memory/search` 端点签名不变（SPEC-046 UI 测试不受影响）

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No — 复用 memories 集合（新增 merged_from 字段，向后兼容） |
| 是否影响现有 API | No — /admin/memory/search 签名不变 |
| go 1.26 可用性 | ✅ go1.26.5 toolchain 已验证可下载 |
| 库兼容性 | ⚠️ 需 UT 验证（adk-go-memory v1.0.8 绑定 adk v1.2.0，我们 v1.5.0） |
| CI 风险 | 中 — 工具链升级影响全部 workflow，需全绿才可合并 |
| SPEC-046 测试影响 | UI-219~222 用例语义不变，预期通过 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `go.mod` / `go.sum` | go 1.26 + adk-go-memory 依赖 | Edit |
| `Dockerfile` / `mockllm/Dockerfile` | 基础镜像升级 | Edit |
| `.github/workflows/lint-check.yml` / `ut-workflow.yml` / `ui-tests.yml` | go 版本升级 | Edit |
| `internal/adk/memoryx/mongo_storage.go` | adapter.Storage MongoDB 实现 | New |
| `internal/adk/memoryx/merge.go` | 相似度合并逻辑 | New |
| `internal/adk/memoryx/kit.go` | adk-go-memory Kit 装配 | New |
| `internal/adk/memory/` | legacy 保留（构建开关），下版本删除 | 保留 |
| `cmd/server/main.go` | MEMORY_BACKEND 开关 + Kit 初始化 | Minor |

## 7. 测试策略

1. **Unit tests**（Go）: MongoStorage 100%（gomonkey mock collection）；merge 逻辑 100%（含阈值边界 0.92±ε）；覆盖率基线见 SPEC-045
2. **兼容性 UT**: go 1.26 下全量 `go test -race -gcflags=all=-l`（gomonkey 重点）
3. **E2E tests**: SPEC-046 的 UI-219~222（Mem0 用例）回归必须通过；新增 UI-2XX（相似记忆合并：写两轮相似信息 → 记忆数 = 1）
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
| Go 工具链 | 1.25 | ✅ 1.26（go.mod/CI/Docker 一致） |
| 记忆后端 | 自实现 | ✅ adk-go-memory（legacy 可回退） |
| 记忆事实提取 | 整段文本 | ✅ LLM deriver 事实提取 |
| 相似记忆 | 重复入库 | ✅ ≥0.92 相似度自动合并 |
| 摘要缓存 | — | ❌ 明确不做（合规） |
| SPEC-046 Mem0 用例 | — | ✅ UI-219~222 回归通过 |
