# 分层语义纠正（一）：domain ID 解耦 / service SDK 清理 / middleware 解耦 / IMBind 补全

> **SPEC-056** | Status: 已实现 (PR #71, 2026-07-21)
>
> **范围说明**: 本 spec 原 5 块工作（domain 解耦 / logic 编排层 / service 扁平化 / main.go 迁移 / 覆盖率 98%）经评估风险等级为中-高，spec §5 自身建议分阶段。经 2026-07-21 晓军确认拆分：
> - **SPEC-056（本 spec）**: domain ID 解耦 + service SDK 清理 + middleware 解耦 + IMBind 补全 + infra converter 骨架（低-中风险快赢）
> - **SPEC-057**: domain model 全量移除 `bson` tag + infra 转换层全量改造（中风险，波及所有 repo 实现）
> - **SPEC-058**: logic 编排层 + chat 解耦 gin + service/agent 拆解 + main.go 迁移 1053→300 + 覆盖率恢复 98%（高风险）

## 1. 目标

纠正 SPEC-055 分层重构遗留的 SDK 泄漏与语义错误（低-中风险部分）：

1. **domain ID 解耦**：`domain/model/model.go` 的实体 ID 字段 `primitive.ObjectID` → `string`，移除 `primitive` import，`FixedRoles()` 不再调用 `primitive.NewObjectID()`。保证 `domain/` 包零 `mongo-driver` import。
2. **infra converter 齐骨架**：新增 `internal/infra/mongo/converter.go`，封装 domain↔bson 的 ID 转换（string↔ObjectID），为 SPEC-057 全量去 bson tag 铺路。
3. **service 层 SDK 泄漏清理**：`service/notification` 移除 `primitive.NewObjectID()`（ID 生成下沉 infra repo Create）；`service/audit`、`service/apireview` 移除 `bson.Marshal/Unmarshal`（转换移到 infra 或用 `map[string]any`）。保证 `service/` 包零 `bson`/`primitive` import。
4. **middleware 解耦 infra**：`middleware.AuditLogger` 改依赖 `repository.AuditRepository` 接口，不再直接持有 `*mongo.Collection`；`audit_test.go` 11 处 gomonkey → mockery。
5. **IMBind 补全**：实现 `infra/mongo/imbind_repository.go`（接口已定义未实现）+ `service/im/bind.go`，main.go 的 `getImBindHandler`/`updateImBindHandler` 接入 service，删除 stub。

> gomonkey 不可完全消除、`-race` 不恢复（gomonkey + race 会 panic，属既定工程约束，本 spec 不涉及）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-055 | ✅ | 分层骨架已落地（repository 接口零 SDK、infra 实现封装） |
| SPEC-045 | ✅ | Go UT 全覆盖规范（本 spec 覆盖率门禁沿用当前 80%，98% 恢复列 SPEC-058） |
| — | — | 无阻塞前置依赖，可立即开始 |

## 2. 背景

### 2.1 SPEC-055 遗留问题（本 spec 范围内）

SPEC-055 完成了 Controller→Service→Repository→Infra 骨架，但遗留三类 SDK 泄漏：

**(A) domain 被当成 repo 数据载体**

`internal/domain/model/model.go` 的实体直接耦合 mongo-driver：

```go
type User struct {
    ID           primitive.ObjectID `bson:"_id,omitempty" json:"id"`
    Username     string             `bson:"username" json:"username"`
    PasswordHash string             `bson:"password_hash" json:"-"`
    // ...
}
```

后果：
- domain 实体携带 `primitive.ObjectID` → repository 接口返回 domain 实体时被迫带入 mongo 类型 → `handler/notification_test.go`、`handler/audit_test.go` 被迫 `import primitive` 生成测试 ID。
- 违反 SPEC-055 §4.2 "repository 输入/输出使用纯 Go 类型，不使用 primitive.ObjectID"。
- `FixedRoles()` 直接调用 `primitive.NewObjectID()` 4 次。

> **本 spec 范围**: ID 字段类型 `primitive.ObjectID` → `string` + 移除 `primitive` import + `FixedRoles` 用 string ID。struct 的 `bson` tag **保留**（移除需全量改造 infra 转换，列 SPEC-057）。

**(B) service 层 SDK 泄漏**

- `service/notification` import `bson/primitive`（用 `primitive.NewObjectID()` 生成 ID，2 处）
- `service/audit` import `bson`（`bson.Marshal/Unmarshal` 做 raw→struct，1 处）
- `service/apireview` import `bson`（`bson.Marshal/Unmarshal`，4 处）

**(C) middleware 直依赖 infra**

- `middleware.AuditLogger` 直接持有 `*mongo.Collection`（main.go 注入），`AuditLogEntry` struct 用 `primitive.ObjectID` + bson tag，`AuditMiddleware` 调用 `primitive.NewObjectID()` + `a.coll.InsertOne`。
- `audit_test.go` 用 11 处 `gomonkey.ApplyMethodReturn(coll, "InsertOne", ...)` mock，依赖 `time.Sleep(200ms)` 等异步。

### 2.2 正确的分层语义（本 spec 涉及部分）

| 层 | 职责 | 禁止 |
|----|------|------|
| **domain/** | 领域实体（纯业务，无持久化标签）、值对象、领域契约、领域事件 | import 任何外部 SDK、infra、service |
| **service/** | 单一业务能力的实现，依赖 repository 接口与 domain 类型 | import 其他 service 包、infra、gin、`bson`/`primitive` |
| **repository/** | 数据访问接口（输入输出用 domain 实体或 DTO） | import 外部 SDK |
| **infra/** | 实现 repository，负责 domain ↔ mongo document 转换 | import handler、service |

**关键纠正**：domain 不是"传给 repo 的数据载体"，而是**领域层**。mongo 的 `ObjectID`/`bson` 是 infra 层的持久化细节，应封装在 infra 的转换函数里。service 层不应直接操作 `bson`/`primitive`。

## 3. 架构概述

### 3.1 本 spec 目标状态

```
┌──────────────────────────────────────────────────────────────┐
│              domain/ (领域层 — 纯业务)                        │
│   实体(User.ID 为 string，无 primitive import)、值对象        │
│   零外部 SDK 依赖（bson tag 暂留，SPEC-057 移除）             │
└──────────────────────────────────────────────────────────────┘
                   │
                   ▼
┌──────────────────────────────────────────────────────────────┐
│              repository/ (数据访问接口)                       │
│   输入输出 = domain 实体，零 SDK（已合规）                    │
└──────────────────────────────────────────────────────────────┘
        ▲ 实现
        │
┌──────────────────────────────────────────────────────────────┐
│              infra/ (持久化实现)                              │
│  converter.go: domain.ID(string) ↔ ObjectID 转换             │
│  imbind_repository.go: 实现 IMBindRepository（补全）         │
│  ObjectID 生成在此层，对上不可见                              │
└──────────────────────────────────────────────────────────────┘

┌──────────────────────────────────────────────────────────────┐
│   service/ (扁平)       │   middleware/                       │
│  notification 零 primitive │  AuditLogger 持 repository.Audit  │
│  audit 零 bson          │   Repository 接口（不持 Collection）│
│  apireview 零 bson      │                                     │
│  im/bind.go 接 IMBindRepo│                                     │
└──────────────────────────────────────────────────────────────┘
```

### 3.2 与 SPEC-055 现状的对比（本 spec 范围）

| 对比项 | SPEC-055 现状 | SPEC-056 目标 |
|--------|--------------|---------------|
| domain/model ID 类型 | `primitive.ObjectID` | `string` |
| domain/model primitive import | 有 | 零 |
| domain/model bson tag | 有 | 保留（SPEC-057 移除） |
| service/notification primitive | `primitive.NewObjectID()` | 零（ID 生成下沉 infra） |
| service/audit bson | `bson.Marshal/Unmarshal` | 零 |
| service/apireview bson | `bson.Marshal/Unmarshal` | 零 |
| middleware.AuditLogger | 持 `*mongo.Collection` | 持 `repository.AuditRepository` |
| audit_test gomonkey | 11 处 | 0 处（改 mockery） |
| IMBindRepository 实现 | 不存在（接口已定义） | infra 实现 + service 接入 |
| main.go IMBind handler | stub（返回空） | 接 service 实现 |

## 4. 详细设计

### 4.1 domain 层 ID 解耦

**范围**：`domain/model/model.go` 的 ID 字段类型 `primitive.ObjectID` → `string`，移除 `primitive` import，`FixedRoles()` 改用 string ID。

涉及 struct（ID 字段改 string）：
- `User.ID` (L28)
- `Invite.ID` (L55)
- `Role.ID` (L70)
- `AuditLog.ID` (L127)
- `Notification.ID` (L140)
- `SystemConfig.ID` (L152)

`FixedRoles()` (L188-232)：4 处 `primitive.NewObjectID()` 改为 string ID 生成。固定角色用确定性 ID（如 `"role_admin"`、`"role_user"`、`"role_developer"`、`"role_viewer"`），保证幂等 upsert 语义不变。

**保留**：struct 的 `bson:"..."` tag 暂留（移除需全量改造 infra 转换 + 所有 repo 实现，列 SPEC-057）。bson tag 是字符串字面量，不引入 import，不影响"domain 零 mongo-driver import"目标。

**infra 转换层**（新增 `internal/infra/mongo/converter.go`）：
```go
// infra 负责 domain.ID(string) ↔ ObjectID 转换
func ObjectIDFromDomain(id string) primitive.ObjectID {
    oid, err := primitive.ObjectIDFromHex(id)
    if err != nil {
        return primitive.NewObjectID()
    }
    return oid
}
func DomainIDFromObjectID(oid primitive.ObjectID) string {
    return oid.Hex()
}
```

**infra repo 实现适配**：现有 infra repo 实现中 `primitive.ObjectIDFromHex(...)` / `.Hex()` 的调用点，改用 converter 函数（保持行为不变，集中转换逻辑）。`notification_repository.go` 的 `Create` 方法返回带生成 ID 的实体（替代 service 层 `primitive.NewObjectID()`）。

> **波及面**: handler/service/test 中所有 `primitive.ObjectID` 字面量构造测试 ID 的地方，改用 `primitive.NewObjectID().Hex()` 或 converter。需全局排查 `primitive.ObjectID{}` / `primitive.NewObjectID()` 在 `internal/api/handler/`、`internal/service/` 测试文件中的用法。

### 4.2 service 层 SDK 泄漏清理

| 文件 | 现状 | 改为 |
|------|------|------|
| `service/notification/service.go` | L9 import `bson/primitive`；L22/L36 `primitive.NewObjectID()` 生成 ID | 移除 import；ID 生成下沉到 infra repo `Create`（repo 返回带 ID 的实体），service 用零值 ID 传入 repo |
| `service/audit/service.go` | L10 import `bson`；L60-61 `bson.Marshal/Unmarshal` 做 raw→struct | 移除 import；raw 用 `map[string]any`，struct 转换在 infra repo（repo 返回 `[]model.AuditLog`，service 不再做 bson 转换） |
| `service/apireview/service.go` | L10 import `bson`；L37-38/L53-54/L69-70/L95-96 `bson.Marshal/Unmarshal` | 移除 import；`Create` 传 domain struct 给 repo 由 repo 转换；`ListAll`/`Approve`/`Reject` 由 repo 返回 domain struct |

**原则**：service 只处理 domain 类型与纯 Go 类型（`map[string]any`、`string`、`time.Time`），所有 `bson` 序列化移到 infra repo 实现。

### 4.3 middleware.AuditLogger 解耦

```go
// Before:
func NewAuditLogger(coll *mongo.Collection) *AuditLogger
// After:
func NewAuditLogger(repo repository.AuditRepository) *AuditLogger
```

- `AuditLogEntry` struct 的 `ID` 字段 `primitive.ObjectID` → `string`，移除 bson tag（用 json tag）；ID 由 repo Create 生成。
- `AuditMiddleware` 调用 `a.repo.Create(ctx, entry)` 替代 `a.coll.InsertOne`；移除 `primitive.NewObjectID()` 调用。
- main.go 改为 `deps.auditLogger = middleware.NewAuditLogger(mongoinfra.NewAuditRepository(mongoClient.DB()))`（AuditRepository 实现已存在于 `infra/mongo/audit_repository.go`）。
- `audit_test.go`：11 处 `gomonkey.ApplyMethodReturn(coll, "InsertOne", ...)` → `mocks.NewAuditRepository(t)`（mockery）。移除 `time.Sleep(200ms)` 等异步（repo mock 同步返回）。

### 4.4 IMBind 补全（SPEC-055 P2 遗留）

- `infra/mongo/imbind_repository.go`（**New**）：实现 `repository.IMBindRepository`（Get/Upsert/Delete，操作 `im_binds` 集合）。
- `service/im/bind.go`（**New**）：`BindService` 接 `IMBindRepository`，提供 Get/Upsert/Delete 业务方法。
- main.go `getImBindHandler`/`updateImBindHandler`：接入 `service/im.BindService`，删除 stub（当前返回空 binds / ok 占位）。
- handler 层：IMBind 路由保留在 main.go setup（main.go 全量迁移列 SPEC-058），仅把 stub 逻辑替换为 service 调用。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（im_binds 集合已规划，本 spec 落地实现） |
| 是否影响现有 API | No（契约不变，仅内部重构） |
| 是否影响现有 UI | No |
| 性能影响 | 无 |
| 是否需要新增 Skill | No |
| 风险等级 | 低-中 — domain ID 类型变更波及测试文件，但生产代码路径稳定 |
| gomonkey 能否全消除 | No — ADK 内部 + 标准库函数保留（既定约束） |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/domain/model/model.go` | 6 个 struct ID 改 string + FixedRoles 去 primitive + 移除 primitive import | Modify |
| `internal/domain/model/model_test.go` | 测试 ID 断言改 string | Modify |
| `internal/infra/mongo/converter.go` | domain.ID ↔ ObjectID 转换层 | **New** |
| `internal/infra/mongo/imbind_repository.go` | 实现 IMBindRepository | **New** |
| `internal/infra/mongo/notification_repository.go` | Create 返回带生成 ID 的实体 | Modify |
| `internal/infra/mongo/audit_repository.go` | 返回 `[]model.AuditLog`（替代 service 层 bson 转换） | Modify |
| `internal/infra/mongo/apireview_repository.go` | Create/ListAll/Approve/Reject 返回 domain struct | Modify |
| `internal/infra/mongo/interface_checks.go` | 补 IMBindRepository 接口检查 | Modify |
| `internal/service/notification/service.go` | 移除 primitive import + NewObjectID 调用 | Modify |
| `internal/service/audit/service.go` | 移除 bson import + Marshal/Unmarshal | Modify |
| `internal/service/apireview/service.go` | 移除 bson import + Marshal/Unmarshal | Modify |
| `internal/service/im/bind.go` | BindService 接 IMBindRepository | **New** |
| `internal/api/middleware/audit.go` | AuditLogger 改收 AuditRepository + AuditLogEntry ID 改 string | Modify |
| `internal/api/middleware/audit_test.go` | 11 处 gomonkey → mockery | Modify |
| `cmd/server/main.go` | auditLogger 注入改 repo + IMBind handler 接 service | Modify |
| `internal/api/handler/*_test.go` | primitive.ObjectID 测试 ID → string | Modify |
| `internal/service/notification/*_test.go` | primitive 断言 → string | Modify |

## 7. 测试策略

1. **Unit tests**：service 测试用 mockery mock repository（消除 service 内 gomonkey 用于 mongo 的部分）；覆盖率维持当前基线（≥80%，98% 恢复列 SPEC-058）。
2. **不恢复 `-race`**：gomonkey + race 会 panic，属既定约束。
3. **gomonkey 保留范围**：仅 ADK 内部、标准库函数（hmac/io/rand/bcrypt/jwt）、package-level 函数（SystemStats/genShortID）。本 spec 清除 middleware/audit 的 11 处 gomonkey。
4. **审计**：用 `.agent/skills/go-ut-audit` 审查。

## 8. UI Test / E2E 验收规则

> 纯后端重构，API 契约不变。

- [ ] **必须** 现有 E2E 全部通过（无 UI 变更）
- [ ] **必须** CI sonar-check + ui-tests 通过才可合并
- [ ] **严禁** 降级已有测试

## 9. Go Unit Test 验收规则

> 重构后 UT 必须全部可本地运行（无需 Docker/网络）。

### 覆盖率底线（本 spec 维持当前门禁）

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | domain、infra converter 纯函数 | **100%** |
| L2 | service（依赖 repository 接口） | **≥95%** |
| L3 | handler、middleware | **≥90%** |
| Overall | 全量 | ≥80%（当前门禁，98% 恢复列 SPEC-058） |

### 关键验证

- [ ] `go test ./internal/...` 无需 MongoDB 全部通过
- [ ] `go test -gcflags=all=-l`（不使用 `-race`，gomonkey 约束）
- [ ] `service/audit`、`service/apireview`、`service/notification` 测试不再 import `bson`/`primitive`
- [ ] `middleware/audit` 测试不再用 gomonkey mock `mongo.Collection.InsertOne`，改用 mockery mock `AuditRepository`
- [ ] gomonkey 仅保留在 ADK / 标准库函数 / package 函数

## 10. 验证标准

1. `go test ./internal/...` 全部通过（本地，无 Docker/网络）
2. CI `ut-workflow.yml`（80% 门禁）+ `sonarqube` + `golangci-lint` + `ui-tests` 全部通过
3. `domain/` 包零 `mongo-driver` import（`grep -r "go.mongodb.org/mongo-driver" internal/domain/` 为空）
4. `service/` 包零 `bson`/`primitive` import（`grep -rn "go.mongodb.org/mongo-driver" internal/service/` 为空，测试文件同）
5. `middleware.AuditLogger` 不再持 `*mongo.Collection`
6. IMBind 不再是 stub（接 service + repo 实现）
7. `infra/mongo/converter.go` 存在且被 repo 实现引用
8. `FixedRoles()` 不再调用 `primitive.NewObjectID()`，角色 ID 为确定性 string

## 11. 不在本 spec 范围（拆分到后续 spec）

- **SPEC-057**: `domain/model` 所有 struct 全量移除 `bson` tag + infra 转换层全量改造（本 spec 仅做 ID 类型 + converter 骨架，bson tag 暂留）
- **SPEC-058**: logic 编排层（`logic/agent/orchestrator`）+ domain/chat/task 契约 + chat.Service 解耦 gin（HandleChat→Process）+ service/agent 拆解 + main.go 迁移 1053→300 行 + UT 覆盖率门禁恢复 98%
- gomonkey 在 ADK / 标准库的彻底移除 → 不做（既定约束）
- `-race` 恢复 → 不做（gomonkey 约束）
