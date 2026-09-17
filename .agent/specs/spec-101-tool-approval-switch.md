# SPEC-101 Tool 用户批准执行开关（requires_approval DB 化）

> **SPEC-101** | Status: 设计中（立项）
> 日期：2026-09-17

## 0. 现状梳理（2026-09-17 调研实测）

| 项 | 现状 |
|---|---|
| tool 配置 collection | `skill_configs`（线上 **32 条**文档，字段 `name`/`value`(config JSON)/`display_name`/`description`/`enabled`） |
| hardcode 批准点 | `internal/adk/tools/tools.go` 的 `fileDelete()`/`dirDelete()` 函数体内 `if deps.HumanGate != nil { Confirm(...) }` —— **是否批准执行硬编码在函数体里，无法配置** |
| ask_user | 仅 `HumanGate != nil` 时注册；语义是「向用户提问」的 HITL 交互工具，**不属于「批准执行」开关范畴，不纳入本 spec** |
| seed 机制 | `predefinedSkills()` + `SeedSkills`（**幂等跳过已存在**）→ 线上存量文档**不会自动补新字段** |
| 线上实测 | `requires_approval` 字段存在数 = **0**；`file_delete` 文档无该字段 |

## 1. 目标

1. tool 增加「**是否需要用户批准执行**」开关（`requires_approval` bool），存 `skill_configs` 集合的**独立字段**（**不混入 config JSON（`value` 字段）**）。
2. `file_delete`/`dir_delete` 执行前改为**从 DB 读开关**决定是否走 `HumanGate.Confirm`，取代函数体内硬编码。
3. **一次性脚本**更新线上 DB：给全部存量 skill 文档补 `requires_approval` 字段，开关值与现状 hardcode 一致（`file_delete`/`dir_delete` = true，其余 false）。
4. **同步原始 seed 数据**：`predefinedSkills()` 加字段，默认值与现状 hardcode 一致。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-089 Human Channel（HumanGate confirm/ask + file_delete/dir_delete 挂授权） | ✅ | 已实现部署；`Deps.HumanGate` 接口、`Confirm`/`Ask` 就绪，wire.go 注入 `deps.humanGate` |
| SPEC-017/072 skill config 体系（`skill_configs` collection + `SeedSkills` + admin API） | ✅ | `SkillConfig` domain、`SkillConfigRepo`、`ConfigService` 就绪 |
| — | — | 无新增前置，可立即开始 |

## 2. 背景与动机

现状「哪些 tool 执行前需要用户批准」是**写死在代码里的**（`fileDelete`/`dirDelete` 函数体内 `if deps.HumanGate != nil`），无法配置：

- 运营/管理员无法新增或解除某个 tool 的批准要求（改开关要发版）。
- 开关状态不可见（DB 里无字段，前端 admin 页无从展示/编辑）。
- 与 skill 体系其余属性（`enabled` 等）不一致——`enabled` 是 DB 字段，批准要求却是代码常量。

**方向**：把「是否需要用户批准执行」提升为 `skill_configs` 的一等字段 `requires_approval`（独立 bson 字段，不塞进 `value` config JSON），执行路径改为读 DB 开关。

## 3. 架构概述

```
file_delete/dir_delete 执行时（tools.go）
        │
        ▼
Deps.SkillConfigs.RequiresApproval(ctx, "file_delete")   ← 查 skill_configs.requires_approval
        │
        ├── false ──────────────► 直接执行（现状 nil HumanGate 行为）
        └── true
                │
                ├── HumanGate == nil ──► ❌ 拒绝执行（fail-closed，见 §5.3 决策 1）
                └── HumanGate.Confirm ──► 同意 → 执行；拒绝 → 返回「用户拒绝」
```

与现有模块的关系：**不改** `HumanGate` 接口、Confirm/Ask 交互协议、前端授权弹窗、`ask_user` 注册逻辑；**改** `SkillConfig` domain/repo 加字段、`predefinedSkills()`、`Deps` 加查询依赖、`fileDelete`/`dirDelete` 函数体。

## 5. 详细设计（方向）

### 5.1 DB schema

`skill_configs` 文档新增独立字段（不进 `value` config JSON）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `requires_approval` | bool | 执行前是否需要用户批准；缺省 false |

### 5.2 代码变更

1. **domain** `internal/domain/skill/`：`SkillConfig` 加 `RequiresApproval bool`（`json:"requires_approval"`）。
2. **repo** `internal/infra/mongo/skill_config_repo.go`：`skillConfigDoc` 加 `bson:"requires_approval"`；`toSkillConfig()`/`Upsert` `$set` 同步该字段。
3. **seed** `internal/service/skill/config.go`：`predefinedSkills()` 里 `file_delete`/`dir_delete` 设 `RequiresApproval: true`，其余全部 false（Go 零值即默认，显式写 true 的两条即可）。
4. **tools.go**：`Deps` 加 skill config 查询依赖（接口如 `RequiresApproval(ctx, name string) (bool, error)`，由 `skill.ConfigService` 实现/适配）；`fileDelete`/`dirDelete` 函数体改为：
   - 查开关失败 → fail-closed 拒绝（同决策 1，避免「查不到=免批准」漏洞）。
   - `requires_approval == false` → 直接执行。
   - `requires_approval == true` → 走 `HumanGate.Confirm`；`HumanGate == nil` → fail-closed 拒绝。
5. **wire.go**：注入 skill config 查询依赖到 `Deps`（赋值在消费前）。
6. **admin skill config API**：domain 加字段后 Upsert/List 自然透传（无需新接口）；前端 admin 编辑页是否加开关控件，实现阶段评估（立项不展开）。

### 5.3 关键设计决策（待晓军拍板）

1. **HumanGate nil 且 requires_approval=true 时**：推荐 **fail-closed（拒绝执行并报错）**。现状 HumanGate nil → 直接执行（仅单测场景；生产 wire.go 恒注入）。fail-closed 更安全，避免「gate 未注入 = 免批准」漏洞。
2. **开关读取方式**：每次执行直查 DB（`file_delete` 低频操作 + 32 条小表，一次 FindOne 可接受），不做内存缓存（缓存引入失效问题，收益为零）。
3. **注册时机**：`file_delete`/`dir_delete` 注册不受开关影响（开关管「执行前是否批准」，不管「是否注册」）；`ask_user` 注册逻辑维持现状。

### 5.4 一次性脚本（线上 DB 迁移）

幂等 mongosh 脚本（实现时执行，仅补缺字段的文档）：

```js
// 仅 $set 缺失 requires_approval 的文档；file_delete/dir_delete=true，其余 false
db.skill_configs.updateMany(
  { requires_approval: { $exists: false }, name: { $in: ["file_delete", "dir_delete"] } },
  { $set: { requires_approval: true } }
);
db.skill_configs.updateMany(
  { requires_approval: { $exists: false } },
  { $set: { requires_approval: false } }
);
```

- 幂等可重入（`$exists: false` 条件）；线上 32 条存量文档一次补齐。
- 不改 `SeedSkills` 幂等跳过语义（seed 只保证新装环境；存量环境靠本脚本）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（`skill_configs` 加字段） |
| 是否影响现有 API | 影响小：skill config 的 Upsert/List 响应自动多出 `requires_approval` 字段（向前兼容） |
| 性能影响 | 无感：每次 file/dir 删除多一次 FindOne（小表 32 条） |
| 是否需要新增 Skill | No |
| 数据迁移 | 一次性 mongosh 脚本补 32 条存量文档字段（幂等） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/skill/model.go` | `SkillConfig` 加 `RequiresApproval` | Low |
| `internal/infra/mongo/skill_config_repo.go` | doc 加 bson 字段 + Upsert/toSkillConfig 同步 | Low |
| `internal/service/skill/config.go` | `predefinedSkills()` file_delete/dir_delete 设 true | Low |
| `internal/adk/tools/tools.go` | `Deps` 加查询依赖；`fileDelete`/`dirDelete` 读开关 | Medium |
| `cmd/server/wire.go` | 注入 skill config 查询依赖（消费前赋值） | Low |
| 一次性 mongosh 脚本 | 线上 32 条存量文档补字段（实现时执行一次） | — |

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 若前端 admin 新增开关控件则同步编写 E2E 用例（`tests/ui/`，编号 `UI-XXX`）；若纯后端改动（前端不新增交互），注明无 UI 变更并豁免
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5 Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | — |
| L2 | 依赖接口，可 mock | **100%** | `fileDelete`/`dirDelete` 开关分支、`RequiresApproval` 接口 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `skill_config_repo` Upsert 字段写入 |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** 验证 `fileDelete`/`dirDelete` 全分支：开关 false → 不调用 Confirm 直接执行；开关 true → Confirm 同意执行/拒绝报错/gate 错误报错；开关 true + HumanGate nil → fail-closed 拒绝；开关查询失败 → fail-closed 拒绝
- [ ] **必须** 验证 repo `Upsert` 写入 `requires_approval` 字段值正确（true/false 各一）
- [ ] **必须** 验证 `predefinedSkills()` 中仅 `file_delete`/`dir_delete` 为 true，其余为 false
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%；`go vet` 无警告

## 10. 验证标准

1. `skill_configs` 文档含 `requires_approval` 字段，且**不在 `value`（config JSON）内**。
2. 线上 32 条存量文档经一次性脚本后：`file_delete`/`dir_delete` = true，其余 30 条 = false；脚本重跑无副作用（幂等）。
3. 开关 true 时 file/dir 删除弹授权确认，同意执行、拒绝不执行；开关 false 时不弹确认直接执行。
4. 新装环境 seed 后开关默认值与现状 hardcode 一致（仅 file_delete/dir_delete 需批准）。
5. admin skill config 列表/详情响应含 `requires_approval`，读写正常。
