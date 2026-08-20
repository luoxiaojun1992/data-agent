# 配置存储拆分：system_configs 去 namespace，模型配置每模型一条文档 + 独立默认配置

> **SPEC-065** | Status: 设计中

## 目标

清理配置存储的混乱现状并解决并发写竞态：

1. `system_configs` 集合只保留系统配置，**移除 namespace 维度**（每 key 一条文档）。
2. 模型配置从「一个 `models` 大 JSON 数组」拆分为 **`model_configs` 集合每模型一条文档**，支持 DB 分页，消除 read-modify-write 竞态。
3. 模型「默认」语义从模型文档字段剥离，改为**独立 `model_defaults` 集合**（use_case 唯一索引），use case 可随扩展自由增删。
4. skill 配置迁入独立集合 `skill_configs`。
5. 修复前端 embedding 模型默认设置缺失「取消默认」能力的问题。

对外 REST API 与前端行为（除 embedding 默认 toggle 外）保持不变。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-061（配置缓存 Redis+热更新）| ✅ | 缓存装饰器仅用于系统配置，本 spec 调整其 key 结构 |
| SPEC-062（多模型配置）| ✅ | Provider/ModelEntry/Vault 引用结构稳定，仅存储形态变化 |
| SPEC-063（异步任务执行器）| ✅ | worker 经 Registry 读模型，无直接 DB 读 |

## 背景

### 现状存储（线上实测 2026-08-20）

| 集合 | 内容 |
|------|------|
| `system_configs`（带 s）| `namespace=system`（11 条）、`namespace=model`（9 条）、`namespace=models`（2 条死数据 api_url/model_name） |
| `system_config`（无 s）| `ns=skill`（22 条）—— `skill_config_repo.go` 硬编码集合名 |
| `rbac_*` / `feishu_configs` / `api_collections` 等 | 已独立，无需变更 |

### 核心问题一：模型配置 read-modify-write 竞态

所有模型写操作都是「读全量 `models` JSON → 内存改一项 → 写回整条 JSON」，路径为 `Provider.SetModels → repo.Upsert(cfgNS, "models", 整条JSON)`。经代码核实，以下 7 个写方法**全部**走这条读-改-写链路：

`AddModel`、`UpdateModel`、`DeleteModel`、`SetDefaultModel`、`SetDefaultEmbedding`、`SetModels`、`SetEmbedding`。

两个管理员并发编辑**不同模型**时，`models` 是一条文档，`Upsert` 原子只保证单文档完整性，业务上的 read-modify-write 隔离缺失——丢失更新必然发生。**每模型一条文档**让单模型 CRUD 退化为 MongoDB 单文档原子操作。

### 核心问题二：IsDefault/IsDefaultFor 字段无法优雅支持 use case 扩展

当前模型文档内嵌 `IsDefault`（legacy 全局默认）与 `IsDefaultFor []string`（per-use-case 默认）。缺陷：

- 「每个 use case 恰好一个默认」是**跨文档约束**，内嵌字段 + 单文档原子更新无法保证（已实测 MongoDB standalone，多文档事务不可用）。
- 每新增一个 use case，都要改动所有模型文档的 `IsDefaultFor` 数组（读-改-写整列表），扩展成本 O(N) 且再次引入竞态。
- `IsDefault` 与 `IsDefaultFor` 语义重叠（全局默认 vs chat 默认）。

**独立 `model_defaults` 集合 + use_case 唯一索引**彻底解决：新增 use case 只需插入一条 `{use_case → model_id}` record，无需触碰任何模型文档。

### 核心问题三：模型列表需 DB 分页

当前 `ListLLMModels`/`ListEmbeddingModels` 是「读全量 → 内存切片」分页。模型拆为独立 collection 后，须改为真正的 DB 分页（skip/limit + count），否则独立 collection 的价值折损（几十条时差异小，但模型规模增长后内存分页不可持续）。

### 核心问题四：embedding 默认无法取消

前端 `app/admin/models/page.tsx` embedding 表格默认列（约 535-556 行）：当前默认模型渲染「✓ 默认 · 切换」下拉，只能切换到其他 embedding；非默认渲染「设为默认」。**无「取消默认」入口**——单 embedding 时显示纯「✓ 默认」不可操作，多 embedding 时也只能切换不能取消。后端 `SetDefaultEmbedding` 语义是「强制恰好一个默认」，同样不支持取消。

### 调研到的完整读写面（不遗漏清单）

**模型配置读**（Provider）：`models()`、`modelsFromDB()`、`embeddingFromDB()`、`EmbeddingConfig()`、`legacyCfgValue()`、`legacyConfig()`、`GetModelByID`、`GetModelByUseCase`、`DefaultModel`、`ListLLMModels`、`ListEmbeddingModels`、`ListAllModels`、`GetDefaultEmbeddingModel`、`GetRawModelConfig`、`DecryptModelAPIKey`。

**模型配置写**（Provider）：上文 7 个 + `BuildLLM`/`BuildLLMByID`（只读构建）。

**embedding 双轨**（需统一）：轨 A（主）`models` 数组里 `Type==embedding` 条目，`GetDefaultEmbeddingModel`/`SetDefaultEmbedding` 读写；轨 B（legacy）独立 `embedding` key 的 `EmbeddingEntry`，`EmbeddingConfig()`/`embeddingFromDB()`/`SetEmbedding()` 读写。消费点 `cmd/server/wire.go buildEmbedFn` 先 A 后 B。

**Vault 引用**：`ModelAPIKeyVaultPath(modelID) = data-agent/models/{id}/api_key`；`DecryptModelAPIKey` / `GET /vault/decrypt`。

**Registry 热更新**：`ConfigHash(ModelEntry)` sha256 fingerprint。

**flat keys 处置**（逐一核实）：`api_url`/`model_name` 死数据（后端从 env/默认值读）；`hermes_url` 后端从 `os.Getenv("HERMES_URL")` 读；flat `api_key` 已被 per-model Vault 替代 —— **全部丢弃**。

**已确认「每配置一条文档」的域**：系统配置（system namespace 每 key 一条）、skill 配置（每 skill 一条）——仅去 namespace/换集合。

## 架构概述

### 目标存储模型

```
data_agent
├── system_configs    # {_id: key, value, updated_at} 每配置一条，无 namespace
├── skill_configs     # SkillConfig 文档（结构不变）
├── model_configs     # ModelEntry 文档，_id = 模型 ID，每模型一条（LLM + embedding）
├── model_defaults    # {_id: use_case, model_id} use_case 唯一索引，每 use case 一条默认
├── rbac_* / feishu_configs / api_collections / ...    # 不动
```

### 模型文档结构（ModelEntry 去默认字段 + bson tag）

```go
type ModelEntry struct {
    ID              string    `json:"id" bson:"_id"`
    Name            string    `json:"name" bson:"name"`
    BaseURL         string    `json:"base_url" bson:"base_url"`
    APIKey          string    `json:"api_key,omitempty" bson:"api_key,omitempty"` // Vault path（解密后仅在内存）
    Type            ModelType `json:"type" bson:"type"`
    Instruction     string    `json:"instruction" bson:"instruction,omitempty"`
    Capability      string    `json:"capability" bson:"capability,omitempty"`
    UseCases        []string  `json:"use_cases" bson:"use_cases,omitempty"`
    TokenMultiplier float64   `json:"token_multiplier" bson:"token_multiplier"`
    Temperature     float64   `json:"temperature" bson:"temperature"`
    MaxTokens       int       `json:"max_tokens" bson:"max_tokens"`
    ContextLen      int       `json:"context_len" bson:"context_len"`
    FallbackOrder   int       `json:"fallback_order" bson:"fallback_order"`
    EmbeddingDim    int       `json:"embedding_dim" bson:"embedding_dim,omitempty"`
}
// ⚠️ 移除 IsDefault / IsDefaultFor 字段 —— 默认语义迁至 model_defaults
```

> ⚠️ converter 铁律：新增/移除 bson tag 必须同步 converter 序列化/反序列化，漏字段静默失效。

### 默认配置集合（model_defaults）

```go
type ModelDefault struct {
    UseCase string `json:"use_case" bson:"_id"`   // 唯一索引（天然唯一，use case 作主键）
    ModelID string `json:"model_id" bson:"model_id"`
}
```

- **唯一索引**：`_id = use_case`，MongoDB 主键天然唯一，保证「每 use case 恰好一个默认」。
- **修改默认**：先 `deleteOne({_id: use_case})` 再 `insertOne({_id: use_case, model_id})`（用户指定顺序；两步非事务，窗口内该 use case 短暂无默认，读路径 fallback 第一个模型）。
- **取消默认**：`deleteOne({_id: use_case})`，无补偿插入。
- **list 联动**：`ListModels` 一次 `find(model_defaults)` 取全量映射 `map[use_case]model_id`，反向组装 `is_default_for` 到每个模型响应（**响应结构不变，前端零改动**，除 embedding toggle）。
- **use case 扩展**：新增 use case 仅 `insertOne` 一条 default record，不触碰模型文档。

### 与现有模块对比

| 维度 | Before | After |
|------|--------|-------|
| 系统配置 | `system_configs` + namespace=system | `system_configs` 无 namespace |
| 模型配置 | `models` 大 JSON 一条 | `model_configs` 每模型一条 |
| 模型默认 | `IsDefault`/`IsDefaultFor` 内嵌字段 | `model_defaults` use_case 唯一索引 |
| 模型分页 | 内存切片 | DB skip/limit + count |
| embedding | 双轨（数组条目 + 独立 key）| 统一 `model_configs` 文档 |
| embedding 默认 | 强制恰好一个，前端无法取消 | 可设可取消（删 use_case=embedding record）|
| Skill 配置 | `system_config`（无 s）| `skill_configs` |
| SysConfigRepository | 全方法带 namespace | 全方法去 namespace |

## 详细设计

### Repository 接口

```go
// internal/repository/config.go

// 系统配置：去 namespace，每 key 一条
type SysConfigRepository interface {
    Get(ctx, key string) (*model.SystemConfig, error)
    GetAll(ctx) ([]model.SystemConfig, error)
    List(ctx, skip, limit int64) ([]model.SystemConfig, error)
    Count(ctx) (int64, error)
    Upsert(ctx, key, value string) error
    Delete(ctx, key string) error
}

// 模型配置：结构化 CRUD + DB 分页
type ModelConfigRepository interface {
    List(ctx, t ModelType, skip, limit int64) ([]ModelEntry, int64, error) // DB 分页 + count
    Get(ctx, id string) (*ModelEntry, error)
    Insert(ctx, entry ModelEntry) error              // 单文档原子
    Update(ctx, id string, entry ModelEntry) error   // 单文档原子
    Delete(ctx, id string) error
}

// 默认配置：use_case 唯一索引
type ModelDefaultRepository interface {
    List(ctx) ([]ModelDefault, error)                // 全量，联动组装用
    Get(ctx, useCase string) (*ModelDefault, error)
    Set(ctx, useCase, modelID string) error          // deleteOne + insertOne
    Delete(ctx, useCase string) error                // 取消默认
}
```

### 类型下移解耦

`ModelEntry`/`ModelType`/`UseCase`/`ModelDefault` 从 `internal/adk/modelcfg` 下移至 `internal/domain/model`（或新建 `internal/domain/modelconfig`），Provider 与 repo 共同引用，避免 repository 反向依赖 adk 包。

### Provider 写方法重写

```go
func (p *Provider) AddModel(ctx, entry) (ModelEntry, error)   // repo.Insert（_id 冲突报重复）
func (p *Provider) UpdateModel(ctx, id, entry) (ModelEntry, error) // repo.Update 单文档原子
func (p *Provider) DeleteModel(ctx, id) error                 // repo.Delete + 清理该 model 的 model_defaults 引用
func (p *Provider) SetDefaultModel(ctx, id, useCases) error   // 对每个 use_case: defaultRepo.Set
func (p *Provider) UnsetDefault(ctx, useCases) error          // 对每个 use_case: defaultRepo.Delete（新增，供取消默认）
func (p *Provider) SetDefaultEmbedding(ctx, id) error         // defaultRepo.Set(use_case=embedding, id)
```

`GetModelByUseCase` = `defaultRepo.Get(useCase) → modelRepo.Get(modelID)`，无 default record 时 fallback 第一个该类型模型。

`ListLLMModels`/`ListEmbeddingModels` = `modelRepo.List(type, skip, limit)` DB 分页 + `defaultRepo.List()` 联动填 `is_default_for`。

### 前端修复（embedding 默认 toggle）

`app/admin/models/page.tsx` embedding 表格默认列改为与 LLM 一致的交互：
- 非默认 → 「设为默认」按钮（`PATCH /admin/models/:id/default`，use_cases 缺省由后端识别为 embedding 类型）
- 默认 → 「✓ 默认」徽标 + 「取消默认」入口（调用新增取消接口）
- 后端 `SetDefaultEmbedding` 语义从「强制恰好一个」改为「设指定为默认」；新增取消路径（删 use_case=embedding record）

### 数据迁移（一次性脚本，幂等）

新增 `scripts/migrate_config_storage.js`（mongosh）：

1. **备份**：`system_configs`、`system_config` duplicate `*_bak_20260820`
2. **skill**：`system_config` 22 条 → `skill_configs`（映射字段，丢 `ns`）
3. **model**：`system_configs[namespace=model].models` JSON 数组 → 逐条展开为 `model_configs` 文档（`_id`=模型 `id`，其余原样）；`api_key` 为 Vault path 原样保留
4. **model_defaults**：展开时，每个模型 `is_default_for` 数组逐项 → `model_defaults{_id: use_case, model_id}`；`is_default=true` 的 LLM → `{_id: "chat", model_id}`；embedding `is_default=true` → `{_id: "embedding", model_id}`
5. **system**：`system_configs[namespace=system]` 11 条原地 `$unset namespace` + `_id` 改 key
6. **死数据**：`namespace=models`（复数）2 条、`namespace=model` 的 `hermes_url`/`api_key`/`api_url`/`model_name`/`embedding` —— 丢弃
7. 每步计数 + 日志；可重复执行（先清目标再插）

### 部署顺序（一次性切换）

1. 低峰期执行迁移脚本（幂等）
2. 部署新 backend + frontend 镜像
3. 验证；回滚 = 旧代码镜像 + `*_bak` 恢复

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（skill_configs、model_configs、model_defaults 新增；system_config 无 s 下线） |
| 是否影响现有 API | 响应结构不变（is_default_for 由 list 联动组装）；新增取消默认接口 |
| 并发改进 | Yes（模型 CRUD 单文档原子；默认约束靠 use_case 唯一索引） |
| 分页改进 | Yes（DB skip/limit + count，替代内存切片） |
| 性能影响 | 读：model_defaults 全量（数量=use case 数，极小）+ 模型 DB 分页；缓存仅 system 配置 |
| 是否需要新增 Skill | No |
| 风险点 | ① ModelEntry 下移 domain 的 import 面；② 迁移窗口 backend 重启；③ 默认语义从内嵌字段迁引用式需回归 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/model/model.go` | SystemConfig 去 Namespace + 集合常量 + 迁入 ModelEntry/ModelType/UseCase/ModelDefault | Medium |
| `internal/adk/modelcfg/provider.go` | 大 JSON 编解码删除、写方法单文档化、默认读写改 model_defaults | Large |
| `internal/repository/config.go` | SysConfigRepository 去 namespace；ModelConfigRepository/ModelDefaultRepository 落地 | Medium |
| `internal/infra/mongo/system_config_repository.go` | 去 namespace | Medium |
| `internal/infra/mongo/model_config_repository.go` | New（DB 分页 CRUD） | New |
| `internal/infra/mongo/model_default_repository.go` | New（use_case 唯一索引） | New |
| `internal/infra/mongo/skill_config_repo.go` | 换集合 | Small |
| `internal/infra/cache/sysconfig_cache.go` | key 去 namespace | Small |
| `internal/service/config/` | 去 namespace | Medium |
| `internal/api/handler/modelconfig.go` | legacy 路径清理 + 新接口 + 取消默认端点 | Medium |
| `internal/api/handler/config.go` | 去 namespace | Small |
| `cmd/server/wire.go` | 注入 + buildEmbedFn 单轨 | Medium |
| `frontend/app/admin/models/page.tsx` | 移除 hermes_url 字段 + embedding 默认 toggle/取消默认 | Medium |
| `scripts/migrate_config_storage.js` | New | New |
| mocks 等 | mock 再生成 | Small |

## 测试策略

1. **Unit tests**（Go）：
   - model_config_repository 结构化 CRUD + DB 分页（L2 100%）
   - model_default_repository Set/Delete（先删后插语义，L2 100%）
   - Provider 各写方法单文档原子 + 默认读写改 model_defaults（L3 98%）
   - system_config_repository 无 namespace、sysconfig_cache 新 key、skill_config_repo 换集合
2. **Integration tests**：Docker Compose 验证迁移幂等 + 并发写不丢更新 + use_case 唯一索引约束
3. **E2E tests**（条件）：admin 模型页 CRUD/设默认/**取消 embedding 默认**、设置页、skill 列表回归
4. **审计**：`.agent/skills/go-ut-audit`

## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`；本次含 embedding 取消默认 UI）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `logic/sql`, `config` |
| L2 | 依赖接口，可 mock | **100%** | `repository/mocks`, `internal/infra/mongo/*` |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `service/*`, `api/handler/*` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc`（非 `ApplyMethodReturn`）验证 handler→service 参数传递正确性
- [ ] **必须** Service 测试的写操作（`UpdateOne`, `InsertOne` 等）验证写入内容的字段和值
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因并记录到 spec 中）
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

### 测试模式

- Handler: `httptest.NewRecorder` + `gin.CreateTestContext` + real handler → mock service
- Service: 直接注入 mock repository / 使用 `gomonkey` 模拟 MongoDB collection
- Logic (L1): 纯 table-driven test，无 mock 依赖

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考:
- `.agent/specs/spec-045-go-service-ut.md` — Go UT 全覆盖 spec
- `.agent/skills/go-ut-audit/SKILL.md` — UT 审计 skill
- `.github/workflows/ut-workflow.yml` — CI UT workflow

## 验证标准

1. 线上集合分布：`system_configs` 仅 11 条无 `namespace`；`skill_configs` 22 条；`model_configs` 每模型一条（3 条）；`model_defaults` 每 use case 一条；`system_config`（无 s）不再被代码引用
2. `GET /models/list`、`/admin/models`、`/admin/models/embedding` 返回与迁移前一致（含 `is_default_for` 联动组装正确）
3. **并发回归**：并发编辑两个不同模型，两处改动均保留（不丢更新）
4. **分页回归**：模型列表 DB 分页正确（total/page/page_size 与内存分页结果一致）
5. **embedding 默认**：前端可设默认、可取消默认；取消后 `GetDefaultEmbeddingModel` fallback 首个 embedding
6. chat / agent 任务（含图片）/ KB 索引 E2E 全链路通过；迁移脚本幂等；`*_bak` 备份存在
7. Redis 仅出现 `syscfg:{key}` 形式 key
