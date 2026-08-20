# 配置存储拆分：system_configs 去 namespace，模型配置每模型一条文档

> **SPEC-065** | Status: 设计中

## 目标

清理配置存储的混乱现状并解决并发写竞态：

1. `system_configs` 集合只保留系统配置，**移除 namespace 维度**（每 key 一条文档）。
2. 模型配置从「一个 `models` 大 JSON 数组」拆分为 **`model_configs` 集合每模型一条文档**，消除 read-modify-write 竞态。
3. skill 配置迁入独立集合 `skill_configs`（结构不变，仅换集合）。
4. 对外 REST API 与前端行为保持不变。

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
| `system_configs`（带 s）| `namespace=system`（11 条，系统配置）、`namespace=model`（9 条，模型配置）、`namespace=models`（2 条，死数据 api_url/model_name） |
| `system_config`（无 s）| `ns=skill`（22 条，skill 配置）—— `skill_config_repo.go` 硬编码集合名 |
| `rbac_*` / `feishu_configs` / `api_collections` 等 | 已独立，无需变更 |

### 核心问题：模型配置的 read-modify-write 竞态

所有模型写操作都是「读全量 `models` JSON → 内存改一项 → 写回整条 JSON」，路径为 `Provider.SetModels → repo.Upsert(cfgNS, "models", 整条JSON)`。经代码核实，以下 7 个写方法**全部**走这条读-改-写链路：

| 方法 | 触发端点 | 竞态表现 |
|------|---------|---------|
| `AddModel` | `POST /admin/models` | 并发新增两个模型，后写覆盖先写 |
| `UpdateModel` | `PATCH /admin/models/:id` | 并发编辑不同模型，互相覆盖 |
| `DeleteModel` | `DELETE /admin/models/:id` | 删除与编辑并发，删后复活 |
| `SetDefaultModel` | `PATCH /admin/models/:id/default` | 设默认与其他写并发，丢失 |
| `SetDefaultEmbedding` | 同上（embedding）| 同上 |
| `SetModels` | `PUT /admin/models`（legacy 全量）| 同上 |
| `SetEmbedding` | legacy embedding 写 | 同上 |

两个管理员并发编辑**不同模型**时，MongoDB 里 `models` 是一条文档，`Upsert` 原子只保证单文档完整性，不保证业务上的 read-modify-write 隔离——丢失更新必然发生。**每模型一条文档**可让单模型 CRUD 退化为 MongoDB 单文档原子操作，从根上消除此类竞态。

### 调研到的完整读写面（不遗漏清单）

**模型配置读**（Provider）：`models()`、`modelsFromDB()`、`embeddingFromDB()`、`EmbeddingConfig()`、`legacyCfgValue()`、`legacyConfig()`、`GetModelByID`、`GetModelByUseCase`、`DefaultModel`、`ListLLMModels`、`ListEmbeddingModels`、`ListAllModels`、`GetDefaultEmbeddingModel`、`GetRawModelConfig`、`DecryptModelAPIKey`。

**模型配置写**（Provider）：上文 7 个 + `BuildLLM`/`BuildLLMByID`（只读构建）。

**embedding 双轨**（需统一）：
- 轨 A（主）：`models` 数组里 `Type==embedding` 的条目，`GetDefaultEmbeddingModel`/`SetDefaultEmbedding` 读写
- 轨 B（legacy fallback）：独立 `embedding` key 的 `EmbeddingEntry`，`EmbeddingConfig()`/`embeddingFromDB()`/`SetEmbedding()` 读写
- 消费点：`cmd/server/wire.go` `buildEmbedFn` 先取轨 A（`GetDefaultEmbeddingModel`），空则 fallback 轨 B（`EmbeddingConfig()`）

**embedding 实际消费链**：`buildEmbedFn → knowledge.Service.WithVectorIndex / adkmemory.NewService`（KB 索引 + memory 向量化）。

**Vault 引用**：`ModelAPIKeyVaultPath(modelID) = data-agent/models/{id}/api_key`；`DecryptModelAPIKey` / `GET /vault/decrypt`。

**Registry 热更新**：`ConfigHash(ModelEntry)` sha256 fingerprint，`GetOrCreate` 每次比对后 rebuild。

**flat keys 消费结论**（迁移处置依据）：
- `api_url` / `model_name`（namespace=model 及 models 复数）：后端从 env/默认值读（`fillLegacyDefaults`），**死数据 → 丢弃**
- `hermes_url`：后端从 `os.Getenv("HERMES_URL")` 读（`wire.go:491`），model 里的 flat key 仅前端模型页顺带读写 → **丢弃**（前端模型页 hermes_url 字段一并移除）
- `api_key`（flat）：legacy 全局 key，已被 per-model Vault 替代，`legacyConfig()` 仅作 base_url 空时的兜底 → 拆文档后每模型自带 `base_url`，**丢弃**

### 已确认「已是每配置一条文档」的域

- **系统配置**：`namespace=system` 的 11 条本就每 key 一条（JWT_SECRET、REDIS_ADDR…），仅需去 namespace
- **skill 配置**：`SkillConfig` 每 skill 一条文档，仅需换集合名

## 架构概述

### 目标存储模型

```
data_agent
├── system_configs   # {_id: key, value, updated_at} 每配置一条，无 namespace
├── skill_configs    # SkillConfig 文档（结构不变）
├── model_configs    # ModelEntry 文档，_id = 模型 ID，每模型一条（含 LLM 与 embedding）
├── rbac_* / feishu_configs / api_collections / ...   # 不动
```

### 模型文档结构（ModelEntry 增加 bson tag）

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
    IsDefault       bool      `json:"is_default" bson:"is_default"`
    IsDefaultFor    []string  `json:"is_default_for" bson:"is_default_for,omitempty"`
    FallbackOrder   int       `json:"fallback_order" bson:"fallback_order"`
    EmbeddingDim    int       `json:"embedding_dim" bson:"embedding_dim,omitempty"`
}
```

> ⚠️ converter 铁律：新增 bson tag 必须同步序列化/反序列化（converter），Go 零值不会报错，漏字段静默失效。

### 并发设计（本次核心）

**单模型 CRUD**（增/删/改模型字段）：退化为 MongoDB 单文档原子操作（`InsertOne`/`UpdateOne`/`DeleteOne`），并发编辑不同模型互不覆盖。

**默认模型约束**（跨文档：每 use case 恰好一个默认）：单文档更新无法原子保证。**已实测线上 MongoDB 为 standalone（非 replica set），多文档事务不可用**，二选一：

| 方案 | 描述 | 取舍 |
|------|------|------|
| **B（推荐）引用式 defaults 文档** | 独立 `model_defaults` 文档存 `{use_case → model_id}` 映射，模型文档去掉 `IsDefault`/`IsDefaultFor`；`SetDefault` = 单文档原子更新 defaults；`GetModelByUseCase` = 查 defaults → 查 model | 彻底消除跨文档约束，语义最干净；改动较大（ModelEntry 去字段 + 前端 default chip 逻辑） |
| C（务实）两步 + 读路径兜底 | 保留 `IsDefaultFor` 在模型文档；`SetDefault` = 「设目标默认 → 清其他默认」两步（非原子）；读路径 `GetModelByUseCase` 取首个默认兜底 | 改动小；设默认窗口内可能短暂双默认（低频操作，可接受） |

> **决策点（待晓军拍板）**：方案 B vs C。两者都依赖「每模型一条文档」这一前提；方案 B 更彻底，方案 C 更省改动。若追求最小变更可先 C，后续再演进 B。

### 与现有模块对比

| 维度 | Before | After |
|------|--------|-------|
| 系统配置 | `system_configs` + namespace=system | `system_configs` 无 namespace |
| 模型配置 | `system_configs` namespace=model 的 `models` 大 JSON | `model_configs` 每模型一条 |
| embedding | `models` 数组条目 + 独立 `embedding` key 双轨 | 统一为 `model_configs` 中 Type==embedding 文档 |
| Skill 配置 | `system_config`（无 s） | `skill_configs` |
| 并发模型写 | read-modify-write 整条 JSON | 单文档原子 |
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

// 模型配置：结构化 CRUD，每模型一条文档（重写现有空壳接口）
type ModelConfigRepository interface {
    List(ctx) ([]ModelEntry, error)                    // 全量，含 LLM + embedding
    ListByType(ctx, t ModelType) ([]ModelEntry, error)
    Get(ctx, id string) (*ModelEntry, error)
    Insert(ctx, entry ModelEntry) error                // 单文档原子
    Update(ctx, id string, entry ModelEntry) error     // 单文档原子（整文档替换）
    Delete(ctx, id string) error
}
```

> ModelEntry 与 ModelType 当前定义在 `internal/adk/modelcfg`，repository 层需依赖 domain 类型。为解耦，将 `ModelEntry`/`ModelType`/`UseCase` 下移至 `internal/domain/model`（或新建 `internal/domain/modelconfig`），Provider 与 repo 共同引用——避免 repo 反向依赖 adk 包。

### 模块改动清单（不遗漏）

| 模块 | 改动 |
|------|------|
| `internal/domain/model/model.go` | `SystemConfig` 去 Namespace；新增 `CollSkillConfigs`/`CollModelConfigs` 常量；迁入 `ModelEntry`/`ModelType`/`UseCase` |
| `internal/adk/modelcfg/provider.go` | repo 换 ModelConfigRepository；删除 `models`/`embedding` 大 JSON 编解码、`cfgNS`、`legacyCfgValue`/`legacyConfig`、`SetModels`/`SetEmbedding`/`embeddingFromDB`/`EmbeddingConfig`；写方法改单文档原子调用 |
| `internal/repository/config.go` | `SysConfigRepository` 去 namespace；`ModelConfigRepository` 落地结构化接口 |
| `internal/infra/mongo/system_config_repository.go` | 去 namespace（按 `_id=key`） |
| `internal/infra/mongo/model_config_repository.go`（新）| 结构化 CRUD 实现，集合 `model_configs` |
| `internal/infra/mongo/skill_config_repo.go` | 集合 `"system_config"` → `CollSkillConfigs`，移除 `ns` 过滤 |
| `internal/infra/cache/sysconfig_cache.go` | 缓存 key 去 namespace：`syscfg:{key}` / `syscfg:all` |
| `internal/service/config/` | interface + service 去 namespace |
| `internal/api/handler/modelconfig.go` | `legacyGet`/`Put`（key-value 路径）移除或改走新接口；其余端点改调新 repo 方法 |
| `internal/api/handler/config.go` | 系统配置调用点去 namespace |
| `cmd/server/wire.go` | Provider 注入 ModelConfigRepository；`buildEmbedFn` 去掉 `EmbeddingConfig` fallback（统一走 `GetDefaultEmbeddingModel` + env fallback）；`rawRepo` 拆分 |
| 前端 `app/admin/models/page.tsx` | 移除 `hermes_url` 字段读写（后端已从 env 读）；其余 API 不变 |

### Provider 写方法重写（并发安全）

```go
func (p *Provider) AddModel(ctx, entry) (ModelEntry, error)      // Insert（唯一性靠 _id 冲突报错）
func (p *Provider) UpdateModel(ctx, id, entry) (ModelEntry, error) // Update(id) 单文档原子
func (p *Provider) DeleteModel(ctx, id) error                    // Delete(id)，默认补偿单独处理
func (p *Provider) SetDefaultModel(ctx, id, useCases) error      // 事务/引用式（见并发设计）
func (p *Provider) SetDefaultEmbedding(ctx, id) error            // 同上
```

`ListAllModels` / `ListLLMModels` / `ListEmbeddingModels` / `GetModelByID` / `GetModelByUseCase` / `GetDefaultEmbeddingModel` 改为按文档读 + 内存过滤（读全量数量级小，几十个模型，无性能问题）。

### 数据迁移（一次性脚本，幂等）

新增 `scripts/migrate_config_storage.js`（mongosh）：

1. **备份**：`system_configs` 与 `system_config` 各 duplicate `*_bak_20260820`
2. **skill**：`system_config`（无 s）22 条 → `skill_configs`（映射 SkillConfig 字段，丢 `ns`）
3. **model**：`system_configs` 中 `namespace=model` 的 `models` JSON 数组 → **逐条展开**为 `model_configs` 文档（`_id` = 各模型 `id`，其余字段原样；`api_key` 若为 Vault path 原样保留）；`embedding` key 若存在且数组内无 embedding 条目则合并/丢弃（以数组为准）
4. **system**：`system_configs` 中 `namespace=system` 11 条原地 `$unset namespace` + `_id` 改为 key
5. **死数据**：`namespace=models`（复数）2 条、`namespace=model` 的 `hermes_url`/`api_key`/`api_url`/`model_name` flat keys —— 丢弃（已核实后端从 env/默认值读）
6. 每步计数 + 日志；脚本可重复执行（幂等：先清目标再插）

### 部署顺序（一次性切换）

1. 低峰期执行迁移脚本（幂等）
2. 部署新 backend 镜像（前端仅 model 页 hermes_url 字段移除，需一并重建）
3. 验证；回滚 = 旧代码镜像 + 从 `*_bak` 恢复数据

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（skill_configs、model_configs 新增；system_config 无 s 下线） |
| 是否影响现有 API | No（REST 路径/响应语义不变；SystemConfig 响应去 namespace 字段，前端已确认不消费；model 页移除 hermes_url 是前端小改） |
| 并发改进 | Yes（模型 CRUD 单文档原子；默认约束按方案 A/B 处理） |
| 性能影响 | 读：几十条文档全量扫 vs 一条 JSON 解析，量级一致；缓存仅 system 配置 |
| 是否需要新增 Skill | No |
| 风险点 | ① MongoDB 是否 replica set（决定事务方案可行性）；② ModelEntry 迁 domain 的 import 面；③ 迁移窗口 backend 重启 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/model/model.go` | SystemConfig 去 Namespace + 集合常量 + 迁入 ModelEntry | Medium |
| `internal/adk/modelcfg/provider.go` | 大 JSON 编解码删除、写方法单文档化 | Large |
| `internal/repository/config.go` | 接口重构 | Medium |
| `internal/infra/mongo/system_config_repository.go` | 去 namespace | Medium |
| `internal/infra/mongo/model_config_repository.go` | New | New |
| `internal/infra/mongo/skill_config_repo.go` | 换集合 | Small |
| `internal/infra/cache/sysconfig_cache.go` | key 去 namespace | Small |
| `internal/service/config/` | 去 namespace | Medium |
| `internal/api/handler/modelconfig.go` | legacy 路径清理 + 新接口 | Medium |
| `internal/api/handler/config.go` | 去 namespace | Small |
| `cmd/server/wire.go` | 注入 + buildEmbedFn 单轨 | Medium |
| `frontend/app/admin/models/page.tsx` | 移除 hermes_url 字段 | Small |
| `scripts/migrate_config_storage.js` | New | New |
| `internal/domain/model/mocks/`、`internal/repository/mocks/` 等 | mock 再生成 | Small |

## 测试策略

1. **Unit tests**（Go）：
   - model_config_repository 结构化 CRUD（L2 100%）
   - Provider 各写方法单文档原子语义（L3 98%）：Add/Update/Delete/SetDefault 并发安全
   - system_config_repository 无 namespace
   - sysconfig_cache 新 key 格式
   - skill_config_repo 换集合
2. **Integration tests**：条件 Docker Compose 验证迁移脚本幂等 + 并发写不丢更新
3. **E2E tests**（条件）：admin 模型页 CRUD/设默认、设置页、skill 列表回归
4. **审计**：`.agent/skills/go-ut-audit`

## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
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

1. 线上集合分布：`system_configs` 仅 11 条且无 `namespace`；`skill_configs` 22 条；`model_configs` 每模型一条（3 条）；`system_config`（无 s）不再被代码引用
2. `GET /models/list`、`/admin/models`、`/admin/models/embedding` 返回与迁移前一致（qwen3-vl-plus 等字段完整、默认标记正确）
3. **并发回归**：并发编辑两个不同模型，两处改动均保留（不丢更新）
4. admin 模型页 CRUD/设默认、设置页、skill 列表正常；chat / agent 任务（含图片）/ KB 索引 E2E 全链路通过
5. 迁移脚本重复执行无报错；`*_bak` 备份集合存在
6. Redis 仅出现 `syscfg:{key}` 形式 key
