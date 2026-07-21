# 分层语义纠正：domain 为领域层 / logic 为编排层 / service 扁平化

> **SPEC-056** | Status: 设计中

## 1. 目标

纠正 SPEC-055 分层重构遗留的语义错误与架构违规：

1. **纠正 domain 语义**：domain 是 DDD 领域层（实体/值对象/领域契约），不是 repository 的数据载体。消除 `domain/model` 对 mongo-driver（`primitive.ObjectID`、`bson` tag）的耦合。
2. **建立 logic 编排层**：把跨多 service 的用例编排（如 agent 编排 chat+session+task）从 `service/agent` 上提到 `internal/logic/`，消除 service 层同层互相依赖。
3. **service 扁平化**：service 之间禁止互相 import；每个 service 只依赖 repository 接口与 domain 类型，由 logic 层组合。
4. **清理 service 层 SDK 泄漏**：`service/notification`、`service/audit`、`service/apireview` 移除 `bson`/`primitive` 依赖。
5. **middleware 解耦 infra**：`middleware.AuditLogger` 改依赖 `repository.AuditRepository` 接口，不再直接持有 `*mongo.Collection`。
6. **完成 main.go 迁移**：迁出残留 inline handler 与 enhance 逻辑，main.go 降至 ~300 行（SPEC-055 未达标项）。
7. **恢复 UT 覆盖率 98% 门禁**。

> gomonkey 不可完全消除、`-race` 不恢复（gomonkey + race 会 panic，属既定工程约束，本 spec 不涉及）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-055 | ✅ | 分层骨架已落地（repository 接口零 SDK、infra 实现封装） |
| SPEC-045 | ✅ | Go UT 全覆盖规范（本 spec 覆盖率门禁沿用） |
| — | — | 无阻塞前置依赖，可立即开始 |

## 2. 背景

### 2.1 SPEC-055 遗留问题

SPEC-055 完成了 Controller→Service→Repository→Infra 骨架，但存在三类语义错误：

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
- domain 实体携带 `primitive.ObjectID` + `bson` tag → repository 接口返回 domain 实体时被迫带入 mongo 类型 → `handler/notification_test.go`、`handler/audit_test.go` 被迫 `import primitive` 生成测试 ID。
- 违反 SPEC-055 §4.2 "repository 输入/输出使用纯 Go 类型，不使用 primitive.ObjectID"。

**(B) service 层同层互相依赖**

`agent.Service` 持有 `*chat.Service` 与 `*chat.Manager`：

```go
type Service struct {
    chatSvc  *chat.Service   // service 依赖 service（同层）
    sessions *chat.Manager   // service 依赖 service 内部组件
    // ...
}
```

后果：
- `agent` 包 import `chat` 包 → service 层内部耦合。
- 若抽 interface 让 agent 依赖 chat 定义的 `ChatService` → 仍同层依赖；若 interface 定义在 agent 包 → chat 反向依赖 agent（更糟）。
- 测试只能用 gomonkey `ApplyMethod(svc.chatSvc, "HandleChat", ...)`，无法用 mockery。

**(C) service 层 SDK 泄漏 + middleware 直依赖 infra**

- `service/notification` import `bson/primitive`（用 `primitive.NewObjectID()` 生成 ID）
- `service/audit`、`service/apireview` import `bson`（`bson.Marshal/Unmarshal`）
- `middleware.AuditLogger` 直接持有 `*mongo.Collection`（main.go L162），`audit_test.go` 用 gomonkey mock `coll.InsertOne`

### 2.2 正确的分层语义

| 层 | 职责 | 禁止 |
|----|------|------|
| **domain/** | 领域实体（纯业务，无持久化标签）、值对象、领域契约（跨 service 的 interface）、领域事件 | import 任何外部 SDK、infra、service |
| **logic/** | 用例编排（orchestrator）：组合多个 service 完成一个业务用例；纯函数工具 | import infra、gin |
| **service/** | 单一业务能力的实现（chat / task / knowledge / user ...），依赖 repository 接口与 domain 类型 | import 其他 service 包、infra、gin |
| **repository/** | 数据访问接口（输入输出用 domain 实体或 DTO） | import 外部 SDK |
| **infra/** | 实现 repository，负责 domain ↔ mongo document 转换 | import handler、service |
| **handler/** | gin 请求解析 → 调 logic/service → 写响应 | import infra |

**关键纠正**：domain 不是"传给 repo 的数据载体"，而是**领域层**。repo 的输入输出是 domain 实体，但 domain 实体本身不该知道它会被怎么持久化。mongo 的 `ObjectID`/`bson` 是 infra 层的持久化细节，应封装在 infra 的转换函数里。

## 3. 架构概述

### 3.1 目标架构

```
┌──────────────────────────────────────────────────────────────┐
│                       handler/ (gin)                          │
│   解析请求 → 调 logic 编排层 或 单 service → 写响应           │
└──────────────────────────────────────────────────────────────┘
        │                              │
        ▼                              ▼
┌─────────────────────┐     ┌─────────────────────────┐
│   logic/ (编排层)    │     │   service/ (扁平)       │
│  agent_orchestrator  │     │  chat  task  knowledge  │
│  组合 chat+task+mem  │     │  user  role  audit ...  │
│  依赖 domain 契约 +  │     │  依赖 repository 接口   │
│  service 接口        │     │  + domain 类型          │
└─────────────────────┘     └─────────────────────────┘
        │                              │
        └──────────┬───────────────────┘
                   ▼
┌──────────────────────────────────────────────────────────────┐
│              domain/ (领域层 — 纯业务)                        │
│   实体(User 无 ObjectID/bson)、值对象、领域契约接口、事件     │
│   零外部 SDK 依赖                                              │
└──────────────────────────────────────────────────────────────┘
                   │
                   ▼
┌──────────────────────────────────────────────────────────────┐
│              repository/ (数据访问接口)                       │
│   输入输出 = domain 实体，零 SDK                              │
└──────────────────────────────────────────────────────────────┘
        ▲ 实现
        │
┌──────────────────────────────────────────────────────────────┐
│              infra/ (持久化实现)                              │
│  mongo: domain ↔ bson document 转换在此层                     │
│  ObjectID 生成在此层，对上不可见                              │
└──────────────────────────────────────────────────────────────┘
```

### 3.2 与 SPEC-055 现状的对比

| 对比项 | SPEC-055 现状 | SPEC-056 目标 |
|--------|--------------|---------------|
| domain/model 耦合 mongo | `primitive.ObjectID` + `bson` tag | 纯 Go 实体，ID 用 string |
| service 间依赖 | agent→chat 同层依赖 | 消除，编排上提到 logic |
| 编排层 | 无（agent.Service 兼任编排） | `logic/agent_orchestrator` |
| service 层 SDK | notification/audit/apireview 用 bson/primitive | 零 SDK |
| middleware.AuditLogger | 持 `*mongo.Collection` | 持 `repository.AuditRepository` |
| main.go 行数 | 1053 | ~300 |
| UT 覆盖率门禁 | 80% | 98% |
| gomonkey | 部分可替换未替换 | 可替换项清除（ADK/标准库保留） |

## 4. 详细设计

### 4.1 domain 层解耦 mongo-driver

**阶段化**：本 spec 完成"接口签名去 mongo 类型"；model struct 去 `bson`/`primitive` 拆为独立后续 SPEC-057（影响面大）。

**本 spec 范围**：
- `domain/model/model.go` 的 `ID` 字段类型：`primitive.ObjectID` → `string`（JSON `id` 字段不变）
- 移除所有 `bson:"..."` tag（持久化映射移到 infra 转换层）
- repository 接口签名不再出现 `primitive.ObjectID`（已合规，仅需确认 domain 类型不再带入）

**infra 转换层**（新增 `internal/infra/mongo/converter.go`）：
```go
// infra 负责 domain.User ↔ bson document 转换
func userToDoc(u *domain.User) bson.M {
    id, _ := primitive.ObjectIDFromHex(u.ID) // string → ObjectID
    return bson.M{"_id": id, "username": u.Username, /* ... */}
}
func docToUser(d bson.M) *domain.User {
    return &domain.User{ID: d["_id"].(primitive.ObjectID).Hex(), /* ... */}
}
```

> model struct 去 `bson` tag 的全量改造（涉及所有 repo 实现 + 所有 service/handler 用法）列入 SPEC-057，本 spec 仅做 ID 类型与转换层骨架，保证 repository 接口签名纯净。

### 4.2 logic 编排层：消除 service 同层依赖

**新增 `internal/logic/agent/`（编排器）**，承接原 `service/agent` 的用例编排职责：

```go
// internal/logic/agent/orchestrator.go
package agent

// 依赖 domain 契约，不依赖具体 service 包
type Orchestrator struct {
    chat    ChatPort      // domain 契约
    session SessionPort   // domain 契约
    task    TaskPort      // domain 契约
    cbReg   *security.CircuitBreakerRegistry
}

// ChatPort / SessionPort / TaskPort 定义在 domain（领域契约）
// service/chat、service/task 实现 domain 契约
```

**domain 领域契约**（新增 `internal/domain/contract/` 或各 domain 子包）：
```go
// internal/domain/chat/contract.go
package chat

type ChatService interface {
    Process(ctx context.Context, req ChatRequest) (*ChatResponse, error)
}
type SessionService interface {
    Create(userID, sessionType string) (*Session, error)
    Get(id string) (*Session, error)
    // ...
}
```

**service 层实现 domain 契约**：
- `service/chat` 的 `Manager` 实现 `domain/chat.SessionService`
- `service/chat.Service` 实现 `domain/chat.ChatService`（解耦 gin：`Process(ctx, req)` 替代 `HandleChat(c *gin.Context)`，见 4.3）
- `service/task.Service` 实现 `domain/task.TaskService`

**编排流向**：
```
handler/agent.go  →  logic/agent.Orchestrator  →  domain 契约
                            ↓ 实现
                  service/chat + service/task + service/session
                            ↓
                      repository 接口
```

**原 `service/agent`**：拆解。编排逻辑迁 `logic/agent`；纯 gin handler 包装迁 `handler/agent.go`。`service/agent` 包删除或仅留无编排的薄包装。

### 4.3 chat.Service 解耦 gin（SPEC-055 §4.6 补做）

```go
// Before:
func (s *Service) HandleChat(c *gin.Context) { ... c.JSON(...) }

// After:
func (s *Service) Process(ctx context.Context, req ChatRequest, userID string) (*ChatResponse, error)
```
- `handler/chat.go` 负责 gin → `ChatRequest` 转换 + `ChatResponse` → JSON 写入
- `domain/chat.ChatService` 契约用 `Process`，不含 `gin.Context`
- 编排器与测试均通过契约，不再需要 gomonkey mock `HandleChat`

### 4.4 service 层 SDK 泄漏清理

| 文件 | 现状 | 改为 |
|------|------|------|
| `service/notification/service.go` | `primitive.NewObjectID()` 生成 ID | ID 生成下沉到 infra repo Create（repo 返回带 ID 的实体）；或用 string ID（hex/uuid） |
| `service/audit/service.go` | `bson.Marshal/Unmarshal` 做 raw→struct | raw 用 `map[string]any` 或 domain DTO，转换在 infra |
| `service/apireview/service.go` | `bson.Marshal/Unmarshal` | 同上，service 只处理 domain/纯 Go 类型 |

### 4.5 middleware.AuditLogger 解耦

```go
// Before:
func NewAuditLogger(coll *mongo.Collection) *AuditLogger
// After:
func NewAuditLogger(repo repository.AuditRepository) *AuditLogger
```
- main.go 改为 `deps.auditLogger = middleware.NewAuditLogger(mongoinfra.NewAuditRepository(mongoClient.DB()))`
- `audit_test.go` 改用 `mocks.NewAuditRepository(t)`（mockery，已存在），删除 11 处 gomonkey

### 4.6 main.go 完成迁移

迁出项：
- `healthCheck`、`dbUnavailableHandler` → `handler/health.go`
- `makeEnhanceHandler` + `callEnhanceLLM` + `enhanceViaADK` + `recordEnhanceTokens` → `handler/enhance.go` + `service/enhance/`
- `handleMemorySearch` → `handler/memory.go`
- `hermesProxyHandler` → `handler/hermes.go`
- `getImBindHandler`/`updateImBindHandler` → 接入 `service/im` 的 IMBind 实现（补全 SPEC-055 P2 遗留）
- 路由 setup 函数 → 各 handler 包的 `RegisterXxxRoutes`

目标：main.go 仅保留 init 组装（~300 行）。

### 4.7 IMBind 补全（SPEC-055 P2 遗留）

- `infra/mongo/imbind_repository.go` 实现 `repository.IMBindRepository`
- `service/im` 增加_bind.go` 接 IMBindRepository
- main.go `getImBindHandler`/`updateImBindHandler` 接入 service，删除 stub

### 4.8 UT 覆盖率门禁恢复

`ut-workflow.yml` Coverage Gate 恢复 98%：
```yaml
if (( $(echo "$TOTAL < 98" | bc -l) )); then
  echo "ERROR: Coverage ${TOTAL}% below 98% threshold"
  exit 1
fi
```
- `go test -gcflags=all=-l -p=1`（保留，不恢复 `-race` — gomonkey 约束）
- 补齐编排层 / 新 handler 的测试达到 98%

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（契约不变，仅内部重构） |
| 是否影响现有 UI | No |
| 性能影响 | 无（interface 静态分发） |
| 是否需要新增 Skill | No |
| 风险等级 | 中 — domain ID 类型变更波及面广，建议分阶段（本 spec 做转换骨架，model 全量去 bson 列 SPEC-057） |
| gomonkey 能否全消除 | No — ADK 内部 + 标准库函数保留（既定约束） |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/domain/model/model.go` | ID 改 string、移除 bson tag（骨架） | Modify |
| `internal/domain/chat/contract.go` | 新增 ChatService/SessionService 领域契约 | **New** |
| `internal/domain/task/contract.go` | 新增 TaskService 领域契约 | **New** |
| `internal/logic/agent/orchestrator.go` | 编排器，承接原 service/agent 编排 | **New** |
| `internal/service/agent/` | 拆解：编排迁 logic，gin 包装迁 handler | **Delete/Major** |
| `internal/service/chat/session.go` | Manager 实现 domain/chat.SessionService | Modify |
| `internal/service/chat/chat_service.go` | 解耦 gin（Process 替代 HandleChat）；sessions 字段改 domain 契约 | Modify |
| `internal/service/notification/service.go` | 移除 primitive | Modify |
| `internal/service/audit/service.go` | 移除 bson | Modify |
| `internal/service/apireview/service.go` | 移除 bson | Modify |
| `internal/api/middleware/audit.go` | 改收 AuditRepository | Modify |
| `internal/infra/mongo/converter.go` | domain ↔ bson 转换层 | **New** |
| `internal/infra/mongo/imbind_repository.go` | 实现 IMBindRepository | **New** |
| `internal/service/im/bind.go` | 接 IMBindRepository | **New** |
| `internal/api/handler/health.go` `enhance.go` `memory.go` `hermes.go` `agent.go` | 迁出 inline handler | **New** |
| `internal/service/enhance/service.go` | enhance 业务逻辑 | **New** |
| `cmd/server/main.go` | 缩减至 ~300 行 | **Major reduce** |
| `.github/workflows/ut-workflow.yml` | 覆盖率 98% | Modify |
| `internal/service/agent/agent_test.go` | 删除 / 重写为编排器测试 | Modify |
| `internal/service/chat/chat_test.go` | gomonkey→mockery | Modify |
| `internal/api/middleware/audit_test.go` | gomonkey→mockery | Modify |

## 7. 测试策略

1. **Unit tests**：编排器用 mockery mock domain 契约；service 测试用 mockery mock repository（消除 service 内 gomonkey）；覆盖率 ≥98%（CI `ut-workflow.yml`）。
2. **不恢复 `-race`**：gomonkey + race 会 panic，属既定约束。
3. **gomonkey 保留范围**：仅 ADK 内部、标准库函数（hmac/io/rand/bcrypt/jwt）、package-level 函数（SystemStats/genShortID）。
4. **审计**：用 `.agent/skills/go-ut-audit` 审查。

## 8. UI Test / E2E 验收规则

> 纯架构重构，API 契约不变。

- [ ] **必须** 现有 E2E 全部通过（无 UI 变更）
- [ ] **必须** CI sonar-check + ui-tests 通过才可合并
- [ ] **严禁** 降级已有测试

## 9. Go Unit Test 验收规则

> 重构后 UT 必须全部可本地运行（无需 Docker/网络）。

### 覆盖率底线

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | domain、logic 纯函数 | **100%** |
| L2 | service（依赖 repository 接口） | **100%** |
| L3 | handler、middleware | **98%** |
| Overall | 全量 | ≥98% |

### 关键验证

- [ ] `go test ./internal/...` 无需 MongoDB 全部通过
- [ ] 覆盖率 ≥98%（恢复 SPEC-055 被降级的门禁）
- [ ] `go test -gcflags=all=-l`（不使用 `-race`，gomonkey 约束）
- [ ] service 层测试不再 mock 具体类型（`*chat.Manager`/`*chat.Service`），改用 domain 契约 + mockery
- [ ] `service/agent`、`service/chat`、`middleware/audit` 中的可替换 gomonkey 已清除
- [ ] gomonkey 仅保留在 ADK / 标准库函数 / package 函数

## 10. 验证标准

1. `go test ./internal/...` 全部通过（本地，无 Docker/网络）
2. `go test -gcflags=all=-l -coverprofile=coverage.out -coverpkg=./internal/... ./internal/...` → 覆盖率 ≥98%
3. CI `ut-workflow.yml` + `sonarqube` + `golangci-lint` + `ui-tests` 全部通过
4. `service/` 包之间无互相 import（`grep -r "internal/service/.*\"internal/service/"` 为空，编排层除外）
5. `domain/` 包零 `mongo-driver`/`qdrant`/`redis` import
6. `service/` 包零 `bson`/`primitive` import
7. `middleware.AuditLogger` 不再持 `*mongo.Collection`
8. main.go ≤ 300 行，零 inline HandlerFunc
9. IMBind 不再是 stub（接 service + repo 实现）
10. `agent.Service` 编排逻辑已迁 `logic/agent`，service 层无同层依赖

## 11. 不在本 spec 范围

- `domain/model` 所有 struct 全量移除 `bson` tag 与重构 infra 转换 → **SPEC-057**（本 spec 仅做 ID 类型与转换骨架）
- gomonkey 在 ADK / 标准库的彻底移除 → 不做（既定约束）
- `-race` 恢复 → 不做（gomonkey 约束）
