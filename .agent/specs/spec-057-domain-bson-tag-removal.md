# domain model 全量去 bson tag + infra 转换层全量改造

> **SPEC-057** | Status: 已实现
>
> **来源**: 从原 SPEC-056 拆分（2026-07-21）。SPEC-056 完成 ID 类型解耦 + converter 骨架，本 spec 完成全量 bson tag 移除 + infra 转换层。

## 1. 目标

移除 domain 层所有 struct 的 `bson:"..."` tag，使 domain 实体彻底成为纯业务对象（不感知持久化）。infra 层负责 domain ↔ bson document 的手动转换（`converter.go` 全量改造）。

**SPEC-056 已完成**：domain ID `primitive.ObjectID` → `string` + `converter.go` 骨架（`NewDomainID`）+ domain 零 `mongo-driver` import。
**本 spec 完成**：domain struct 全量去 bson tag + infra repo 改用手动转换。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-056 | ✅ | domain ID 改 string + converter 骨架 + domain 零 mongo import |
| SPEC-055 | ✅ | 分层骨架（repository 接口零 SDK，infra 实现） |
| — | — | 无阻塞前置依赖 |

## 2. 背景

### 2.1 SPEC-056 遗留

SPEC-056 §4.1 明确："struct 的 `bson:"..."` tag **保留**（移除需全量改造 infra 转换，列 SPEC-057）"。当前 domain struct 仍带 bson tag，mongo-driver 通过 tag 自动序列化 domain struct。这导致：
- domain 实体仍"知道"持久化字段名（bson tag 是持久化细节）
- 违反"domain 是纯业务对象"的 DDD 原则
- infra 无法独立控制字段映射（如字段重命名、敏感字段过滤需改 domain struct）

### 2.2 现状（2026-07-21 调研）

domain 层 bson tag 统计：
| 文件 | bson tag 数 | 主要 struct |
|------|:-----------:|------------|
| `domain/model/model.go` | 54 | User, Invite, Role, AuditLog, Notification, SystemConfig |
| `domain/task/model.go` | 31 | Task, TaskResult 等 |
| `domain/knowledge/model.go` | 35 | KB, KBDocument 等 |
| `domain/apireview/model.go` | 14 | APIReview |
| `domain/artifact/model.go` | 11 | Artifact |
| `service/chat/session.go` | 9 | Session（在 service 层，非 domain） |

`infra/mongo/converter.go` 现状：仅 `NewDomainID()` + 预留的 `ObjectIDFromDomainID`/`DomainIDFromObjectID`（SPEC-056 骨架），无 struct 级转换函数。

infra repo 现状：直接 `coll.InsertOne(ctx, domainStruct)` / `cursor.All(ctx, &domainStructs)`，依赖 bson tag 自动序列化。

## 3. 架构概述

```
domain/ (纯业务对象，零 bson tag，零 mongo-driver)
  User{ID string, Username string, ...}  ← 只有 json tag + 业务方法

infra/mongo/converter.go (全量转换层)
  userToDoc(u *domain.User) bson.M       ← domain → bson.M (InsertOne/UpdateOne)
  docToUser(d bson.M) *domain.User       ← bson.M → domain (FindOne/Decode)
  taskToDoc / docToTask / ...

infra/mongo/*_repository.go (改用手动转换)
  Create: coll.InsertOne(ctx, userToDoc(user))         ← 不再直接传 struct
  FindBy: doc := bson.M{}; FindOne().Decode(&doc); return docToUser(doc)
```

## 4. 详细设计

### 4.1 domain struct 去 bson tag

移除所有 domain struct 的 `bson:"..."` tag，保留 `json:"..."` tag（API 响应需要）。

示例（`domain/model/model.go`）：
```go
// Before (SPEC-056 现状):
type User struct {
    ID              string `bson:"_id,omitempty" json:"id"`
    Username        string `bson:"username" json:"username"`
    // ...
}

// After (SPEC-057):
type User struct {
    ID              string `json:"id"`
    Username        string `json:"username"`
    // ...
}
```

涉及文件（全量去 bson tag）：
- `internal/domain/model/model.go`（6 struct）
- `internal/domain/task/model.go`
- `internal/domain/knowledge/model.go`
- `internal/domain/apireview/model.go`
- `internal/domain/artifact/model.go`

### 4.2 `service/chat/session.go` Session 处理

`Session` struct 在 service 层但带 bson tag。两种方案：
- **方案 A（推荐）**：`Session` 移除 bson tag，infra `session_repository.go` 改用手动转换（`sessionToRecord`/`recordToSession` 已存在于 session.go L106-127，改造为 infra 转换函数）
- 方案 B：`Session` 改为 repository DTO（已有 `repository.SessionRecord`，`Session` 改为纯 domain）

本 spec 采用方案 A：Session 去 bson tag，infra session_repository 改用手动转换。

### 4.3 infra `converter.go` 全量改造

为每个 domain struct 新增 `toDoc` / `docTo` 转换函数：

```go
// internal/infra/mongo/converter.go

// ── User ──
func userToDoc(u *model.User) bson.M {
    return bson.M{
        "_id":             u.ID,  // string _id (SPEC-056)
        "username":        u.Username,
        "password_hash":   u.PasswordHash,
        "role":            u.Role,
        "status":          u.Status,
        // ... 全字段
    }
}
func docToUser(d bson.M) *model.User {
    return &model.User{
        ID:           getStr(d, "_id"),
        Username:     getStr(d, "username"),
        PasswordHash: getStr(d, "password_hash"),
        // ...
    }
}

// ── Task / Invite / Role / AuditLog / Notification / SystemConfig / Artifact / KB / APIReview / Session ──
// 每个 struct 一对 toDoc/docTo
```

辅助函数：
```go
func getStr(d bson.M, key string) string {
    v, ok := d[key]
    if !ok { return "" }
    s, _ := v.(string)
    return s
}
func getTime(d bson.M, key string) time.Time { ... }
func getIntPtr(d bson.M, key string) *time.Time { ... }
```

### 4.4 infra repo 改造

每个 repo 的 CRUD 改用手动转换：

```go
// user_repository.go
func (r *UserRepository) Create(ctx context.Context, user *model.User) error {
    user.ID = NewDomainID()
    user.CreatedAt = time.Now()
    user.UpdatedAt = time.Now()
    _, err := r.coll.InsertOne(ctx, userToDoc(user))  // ← 不再传 user struct
    return err
}

func (r *UserRepository) FindByID(ctx context.Context, id string) (*model.User, error) {
    var d bson.M
    err := r.coll.FindOne(ctx, bson.M{"_id": id}).Decode(&d)
    if err != nil {
        if err == mongo.ErrNoDocuments { return nil, nil }
        return nil, err
    }
    return docToUser(d), nil  // ← 手动转换
}
```

涉及所有 repo：user/invite/role/audit/notification/apireview/system_config/session/task/artifact/kb/imbind。

### 4.5 `repository.SessionRecord` 处理

`repository.SessionRecord`（notification.go L44-55）有 bson tag，是 repo 层 DTO。本 spec 保留其 bson tag（repo 层 DTO 允许，非 domain）。或改为 infra 内部 struct。本 spec 保留现状（非 domain 层，不强制）。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（契约不变，仅内部转换） |
| 是否影响现有 UI | No |
| 性能影响 | 极低（手动构造 bson.M 与反射 tag 相当） |
| 风险等级 | 中 — 涉及所有 domain struct + 所有 infra repo，字段映射易遗漏 |
| 测试策略 | 每个 converter 函数单测 + repo 集成测试验证字段完整往返 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/domain/model/model.go` | 6 struct 去 bson tag | Modify |
| `internal/domain/task/model.go` | struct 去 bson tag | Modify |
| `internal/domain/knowledge/model.go` | struct 去 bson tag | Modify |
| `internal/domain/apireview/model.go` | struct 去 bson tag | Modify |
| `internal/domain/artifact/model.go` | struct 去 bson tag | Modify |
| `internal/service/chat/session.go` | Session 去 bson tag | Modify |
| `internal/infra/mongo/converter.go` | 全量 toDoc/docTo 转换函数 + 辅助函数 | **Major** |
| `internal/infra/mongo/converter_test.go` | 每个 converter 往返测试 | **Major** |
| `internal/infra/mongo/user_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/invite_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/role_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/audit_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/notification_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/apireview_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/system_config_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/session_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/task_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/artifact_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/kb_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/imbind_repository.go` | CRUD 改手动转换 | Modify |
| `internal/infra/mongo/*_test.go` | repo 测试适配 | Modify |

## 7. 测试策略

1. **Converter 单测**（L1，100%）：
   - 每个 `toDoc`/`docTo` 往返测试：构造 domain struct → toDoc → docTo → 验证字段一致
   - 边界：nil 指针、空切片、零值时间、可选字段（`omitempty`）
2. **Repo 测试**（L3，98%）：
   - Create → FindByID → 验证字段完整往返
   - Update → FindByID → 验证更新字段
   - List → 验证多个 struct 转换正确
3. **审计**：`.agent/skills/go-ut-audit` 审查字段映射完整性

> **关键风险**：字段遗漏（toDoc 漏字段导致写入缺失，docTo 漏字段导致读取为零值）。每个 converter 必须有完整往返测试覆盖所有字段。

## 8. UI Test / E2E 验收规则

> 纯后端重构，API 契约不变。

- [ ] **必须** 现有 E2E 全部通过（无 UI 变更）
- [ ] **必须** CI sonar-check + ui-tests 通过
- [ ] **严禁** 降级测试

## 9. Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | converter toDoc/docTo 纯函数 | **100%** |
| L3 | infra repo（依赖 mongo） | **98%** |

- [ ] 每个 converter 有往返测试（struct → doc → struct 字段一致）
- [ ] repo Create/Find/Update/List 验证字段完整
- [ ] Success 测试 ≥2 个行为验证断言

## 10. 验证标准

1. `grep -rn 'bson:"' internal/domain/` 为空（domain 零 bson tag）
2. `service/chat/session.go` Session struct 无 bson tag
3. `infra/mongo/converter.go` 含所有 domain struct 的 toDoc/docTo 函数
4. 所有 infra repo 的 CRUD 使用 converter（不直接传/接 domain struct）
5. `go test ./internal/...` 全通过，覆盖率 ≥98%
6. CI sonar-check + ui-tests 通过
7. 每个 converter 往返测试覆盖所有字段（无遗漏）

## 11. 不在本 spec 范围

- `repository.SessionRecord` 的 bson tag（repo 层 DTO，非 domain，保留）
- gomonkey 在 ADK / 标准库的彻底移除（既定约束）
- `-race` 恢复（gomonkey 约束）
