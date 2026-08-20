# 配置存储拆分：system_configs 去 namespace，skill/模型配置独立集合

> **SPEC-065** | Status: 设计中

## 目标

清理配置存储的混乱现状：`system_configs` 集合只保留系统配置并**移除 namespace 维度**；skill 配置与模型配置各自迁入独立集合（`skill_configs`、`model_configs`），消除 `system_config`（无 s）与 `system_configs`（带 s）双集合并存的历史遗留。对外 REST API 与前端行为保持不变。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-061（配置缓存 Redis+热更新）| ✅ | 缓存装饰器（SysConfigCacheRepo）已就位，本 spec 仅调整其 key 结构 |
| SPEC-062（多模型配置）| ✅ | Provider/ModelEntry/Vault key 引用结构已稳定，本 spec 不改变其语义 |
| SPEC-063（异步任务执行器）| ✅ | 模型配置读取路径由 Provider 收敛，worker 无直接 DB 读 |

## 背景

线上 `data_agent` 库实测（2026-08-20）存在三处混乱：

1. **`system_configs`（带 s）用 namespace 装了三类数据**：
   - `namespace=system`（11 条）：JWT_SECRET、REDIS_ADDR、WORKER_POOL_SIZE 等系统配置，走 Redis 缓存
   - `namespace=model`（9 条）：`models`（模型列表大 JSON）、hermes_url、api_key 等模型配置，直读 DB
   - `namespace=models`（复数，2 条）：api_url、model_name —— **死数据**，当前代码（`p.cfgNS = "model"`）已不再读取
2. **skill 配置散落在 `system_config`（无 s）集合**（22 条，ns=skill）：`internal/infra/mongo/skill_config_repo.go:31` 硬编码集合名 `"system_config"`，与主集合拼写不一致，mongosh 排障极易查错集合。
3. **语义混乱的接口**：`SysConfigRepository` 的 namespace 参数对"系统配置"这一用途而言是多余抽象；`ModelConfigRepository` 接口已定义（`internal/repository/config.go:35`）但**从未实现**。

RBAC（4 集合）、feishu_configs、api_collections 等已独立，无需变更。

## 架构概述

### 目标存储模型

```
data_agent
├── system_configs      # 仅系统配置：{key, value, updated_at}，key 唯一（无 namespace）
├── skill_configs       # SkillConfig 文档（结构不变，仅换集合）
├── model_configs       # 模型配置：{key, value, updated_at}，key 唯一（含 models 大 JSON 与 flat keys）
├── rbac_* / feishu_configs / api_collections / ...  # 不动
```

### 与现有模块对比

| 维度 | Before | After |
|------|--------|-------|
| 系统配置集合 | `system_configs` + namespace=system | `system_configs` 无 namespace |
| 模型配置集合 | `system_configs` + namespace=model（+models 死数据） | `model_configs` |
| Skill 配置集合 | `system_config`（无 s，硬编码） | `skill_configs` |
| 缓存边界 | 仅 namespace=system 走缓存（实际如此） | 明确：仅 system_configs 走 Redis 缓存，其余直读 DB |
| SysConfigRepository 接口 | 全方法带 namespace 参数 | 全方法去 namespace |

### 设计决策：model_configs 保持 key-value 形式

**方案 A（采纳）**：`model_configs` 存 `{key: "models", value: "<JSON 数组>"}` + `hermes_url`/`api_key` 等 flat key，与现状同构，仅换集合、去 namespace。Provider 改动集中在 repo 调用点。

**方案 B（否决）**：把 models 大 JSON 拆成每模型一条结构化 document。改动波及 ModelEntry 序列化、admin UI CRUD、Vault key 引用、单测全量重写，风险与收益不成比例；且当前 admin UI 的编辑/默认值设定均围绕"一个 models JSON + per-use-case 标记"设计，拆文档需重做 UI。

**遗留数据处置**：namespace=`models`（复数）的 api_url/model_name 经代码核实为死数据（Provider 只读 `cfgNS="model"`），迁移时**直接丢弃**，不在新集合保留。

## 详细设计

### 数据模型（Go struct）

```go
// internal/domain/model/model.go
// SystemConfig 去掉 Namespace 字段（API 响应随之不再返回 namespace）
type SystemConfig struct {
    Key       string    `json:"key" bson:"_id"`
    Value     string    `json:"value"`
    UpdatedAt time.Time `json:"updated_at"`
}

// 集合常量
const (
    CollSystemConfigs = "system_configs"
    CollSkillConfigs  = "skill_configs"   // 新增
    CollModelConfigs  = "model_configs"   // 新增
)
```

SkillConfig 结构不变，仅存储集合从 `system_config` 改为 `skill_configs`。

### Repository 接口变更

```go
// internal/repository/config.go
type SysConfigRepository interface {
    Get(ctx context.Context, key string) (*model.SystemConfig, error)
    GetAll(ctx context.Context) ([]model.SystemConfig, error)
    List(ctx context.Context, skip, limit int64) ([]model.SystemConfig, error)
    Count(ctx context.Context) (int64, error)
    Upsert(ctx context.Context, key, value string) error
    Delete(ctx context.Context, key string) error
}

// ModelConfigRepository：已定义，本次落地实现（key-value 同构）
type ModelConfigRepository interface {  // 接口语义微调为与 SystemConfig 同构
    Get(ctx context.Context, key string) (*model.SystemConfig, error)
    GetAll(ctx context.Context) ([]model.SystemConfig, error)
    Upsert(ctx context.Context, key, value string) error
    Delete(ctx context.Context, key string) error
}
```

### 模块改动

| 模块 | 改动 |
|------|------|
| `internal/infra/mongo/system_config_repository.go` | 集合不变（system_configs），查询去 namespace（按 `_id=key`） |
| `internal/infra/mongo/skill_config_repo.go` | 集合 `"system_config"` → `CollSkillConfigs`；字段 `ns` 相关过滤移除 |
| `internal/infra/mongo/model_config_repository.go`（新） | 无 namespace 的 key-value 实现，集合 `CollModelConfigs` |
| `internal/adk/modelcfg/provider.go` | `p.repo` 换为 ModelConfigRepository；`cfgNS` 常量删除；`modelsFromDB`/`legacyCfgValue`/`legacyConfig`/`DeleteModel` 改调新接口 |
| `internal/infra/cache/sysconfig_cache.go` | 缓存 key 去 namespace 段：`syscfg:{key}` / `syscfg:all` |
| `internal/service/config/` | interface + service 去 namespace |
| `internal/api/handler/config.go` / `modelconfig.go` / `skill_config.go` | 调用点同步（`GetAll("model")` → 新接口） |
| `cmd/server/wire.go` | Provider 注入 ModelConfigRepository；`rawRepo` 拆分 |

### 数据迁移（一次性脚本，幂等）

新增 `scripts/migrate_config_storage.js`（mongosh），步骤：

1. **备份**：`system_configs` 与 `system_config` 各 duplicate 一份 `*_bak_20260820` 留存
2. **skill**：`system_config`（无 s）22 条 → `skill_configs`（按 SkillConfig 字段映射，`ns` 丢弃）
3. **model**：`system_configs` 中 `namespace=model` 的 9 条 → `model_configs`（`{_id: key, value, updated_at}`）
4. **system**：`system_configs` 中 `namespace=system` 的 11 条原地改写为 `{_id: key, value, updated_at}`（$unset namespace）
5. **死数据**：`namespace=models`（复数）2 条删除（已确认代码不读）
6. 每步打日志并计数；脚本可重复执行（先删目标集合已有同名 key 再插入）

### 部署顺序（一次性切换，无双读过渡）

1. 低峰期执行迁移脚本（幂等）
2. 部署新代码（backend 镜像重建；前端无改动，可不动）
3. 验证 + 必要时用备份集合回滚（回滚 = 恢复旧代码镜像 + 从 `*_bak` 恢复数据）

> 不做双读兼容期：三个集合拆分是纯内部存储变化，迁移窗口内短暂停机（backend 重启）即可，复杂度不值得引入双读分支。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（skill_configs、model_configs 新增；system_config 无 s 集合下线） |
| 是否影响现有 API | No（REST 路径/请求/响应字段语义不变；SystemConfig 响应去掉 namespace 字段属内部字段，前端已确认不消费） |
| 性能影响 | 无（读路径 DB 直读，与现状一致；缓存仅 system 配置，key 更短） |
| 是否需要新增 Skill | No |
| 风险点 | 迁移窗口内 backend 需重启；`*_bak` 备份保证可回滚 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/model/model.go` | SystemConfig 去 Namespace + 集合常量 | Small |
| `internal/repository/config.go` | SysConfigRepository 接口去 namespace；ModelConfigRepository 落地 | Medium |
| `internal/repository/skill_config.go` | 接口不变（语义已无 namespace） | None |
| `internal/infra/mongo/system_config_repository.go` | 去 namespace 查询 | Medium |
| `internal/infra/mongo/skill_config_repo.go` | 集合名替换 | Small |
| `internal/infra/mongo/model_config_repository.go` | New | New |
| `internal/adk/modelcfg/provider.go` | repo 替换、cfgNS 删除 | Medium |
| `internal/infra/cache/sysconfig_cache.go` | 缓存 key 去 namespace | Small |
| `internal/service/config/` | 去 namespace | Medium |
| `internal/service/skill/config.go` | 无（repo 层已隔离） | None |
| `internal/api/handler/{config,modelconfig,skill_config}.go` | 调用点同步 | Small |
| `cmd/server/wire.go` | 注入调整 | Small |
| `scripts/migrate_config_storage.js` | New | New |
| `internal/domain/model/mocks/`、`internal/repository/mocks/` 等 | mock 再生成 | Small |

## 测试策略

1. **Unit tests**（Go）：repo 层（L2）100%；service/provider 相关路径（L3）98%。重点：
   - system_config_repository 无 namespace CRUD
   - model_config_repository CRUD
   - skill_config_repo 换集合后 List/Get/Upsert/SearchByDescription
   - provider modelsFromDB/legacyConfig/DeleteModel 新 repo 调用
   - sysconfig_cache 新 key 格式
2. **Integration tests**：条件使用 Docker Compose（`go test -tags=integration`）验证迁移脚本幂等性
3. **E2E tests**（条件）：`UI-XXX` —— admin 设置页 CRUD、模型管理页列表/编辑/设默认、skill 列表。现有用例回归即可，无需新增 UI 交互
4. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量

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

1. 线上三个集合分布：`system_configs` 仅 11 条且无 `namespace` 字段；`skill_configs` 22 条；`model_configs` 9 条；`system_config`（无 s）集合不再被任何代码引用（可保留空集合或删除）
2. `GET /models/list` 返回模型列表与迁移前一致（qwen3-vl-plus 等字段完整）
3. admin 设置页读写正常；模型管理页 CRUD/设默认正常；skill 列表/搜索正常
4. Redis 中仅出现 `syscfg:{key}` 形式的缓存 key
5. 回归：chat 会话、agent 任务（含图片附件）、KB 索引链路 E2E 全部通过
6. 迁移脚本重复执行无报错（幂等）；`*_bak` 备份集合存在
