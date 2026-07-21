# 分层语义纠正（二）：logic 编排层 / chat 解耦 gin / service 扁平化 / main.go 迁移 / 覆盖率恢复 98%

> **SPEC-058** | Status: 设计中
>
> **来源**: 从 SPEC-056 拆分（2026-07-21 晓军确认）。SPEC-056 原 5 块工作中高风险部分集中到本 spec。

## 1. 目标

完成 SPEC-055/SPEC-056 分层重构的高风险部分：

1. **建立 logic 编排层**：把跨多 service 的用例编排（如 agent 编排 chat+session+task）从 `service/agent` 上提到 `internal/logic/agent/`，消除 service 层同层互相依赖。
2. **domain 领域契约**：新增 `domain/chat`、`domain/task` 契约接口，service 实现契约，编排层依赖契约。
3. **chat.Service 解耦 gin**：`HandleChat(c *gin.Context)` → `Process(ctx, req, userID) (*ChatResponse, error)`，handler 负责 gin↔DTO 转换。
4. **service 扁平化**：service 之间禁止互相 import；`service/agent` 拆解（编排迁 logic，gin 包装迁 handler）。
5. **main.go 完成迁移**：迁出残留 inline handler（health/enhance/memory/hermes/agent）与路由 setup，main.go 降至 ~300 行。
6. **恢复 UT 覆盖率 98% 门禁**。

> gomonkey 不可完全消除、`-race` 不恢复（既定工程约束，本 spec 不涉及）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-056 | 📐 设计中 | domain ID 解耦 + service SDK 清理 + middleware 解耦（本 spec 编排层基于干净的 service/domain） |
| SPEC-055 | ✅ | 分层骨架 |
| SPEC-045 | ✅ | Go UT 全覆盖规范（本 spec 恢复 98% 门禁） |

## 2. 背景

详见 SPEC-056 §2.1 (B)：`agent.Service` 持有 `*chat.Service` 与 `*chat.Manager`（同层依赖），`chat.Service.HandleChat` 接收 `*gin.Context`（service 耦合 gin），main.go 1053 行内联 health/enhance/memory/hermes/agent 等 handler。

SPEC-056 完成后 service/domain 已零 SDK 泄漏，本 spec 在干净基础上建编排层。

## 3. 架构概述

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
│   实体、值对象、领域契约接口（chat/task）、事件               │
└──────────────────────────────────────────────────────────────┘
```

## 4. 详细设计

### 4.1 logic 编排层：消除 service 同层依赖

**新增 `internal/logic/agent/`（编排器）**，承接原 `service/agent` 的用例编排职责：

```go
// internal/logic/agent/orchestrator.go
package agent

type Orchestrator struct {
    chat    ChatPort      // domain 契约
    session SessionPort   // domain 契约
    task    TaskPort      // domain 契约
    cbReg   *security.CircuitBreakerRegistry
}
```

**domain 领域契约**（新增）：
- `internal/domain/chat/contract.go` — `ChatService`、`SessionService` 接口
- `internal/domain/task/contract.go` — `TaskService` 接口

**service 层实现 domain 契约**：
- `service/chat.Manager` 实现 `domain/chat.SessionService`
- `service/chat.Service` 实现 `domain/chat.ChatService`（`Process` 替代 `HandleChat`，见 4.2）
- `service/task.Service` 实现 `domain/task.TaskService`

**原 `service/agent`**：拆解。编排逻辑迁 `logic/agent`；纯 gin handler 包装迁 `handler/agent.go`。`service/agent` 包删除或仅留无编排的薄包装。

### 4.2 chat.Service 解耦 gin（SPEC-055 §4.6 补做）

```go
// Before:
func (s *Service) HandleChat(c *gin.Context) { ... c.JSON(...) }

// After:
func (s *Service) Process(ctx context.Context, req ChatRequest, userID string) (*ChatResponse, error)
```
- `handler/chat.go` 负责 gin → `ChatRequest` 转换 + `ChatResponse` → JSON 写入
- `domain/chat.ChatService` 契约用 `Process`，不含 `gin.Context`
- 编排器与测试均通过契约，不再需要 gomonkey mock `HandleChat`

### 4.3 main.go 完成迁移

迁出项：
- `healthCheck`、`dbUnavailableHandler` → `handler/health.go`
- `makeEnhanceHandler` + `callEnhanceLLM` + `enhanceViaADK` + `recordEnhanceTokens` → `handler/enhance.go` + `service/enhance/`
- `handleMemorySearch` → `handler/memory.go`
- `hermesProxyHandler` → `handler/hermes.go`
- agent 路由 setup → `handler/agent.go`（接 logic/agent.Orchestrator）
- 路由 setup 函数 → 各 handler 包的 `RegisterXxxRoutes`

目标：main.go 仅保留 init 组装（~300 行）。

### 4.4 UT 覆盖率门禁恢复 98%

`ut-workflow.yml` Coverage Gate 恢复 98%：
```yaml
if (( $(echo "$TOTAL < 98" | bc -l) )); then
  echo "ERROR: Coverage ${TOTAL}% below 98% threshold"
  exit 1
fi
```
- `go test -gcflags=all=-l -p=1`（保留，不恢复 `-race`）
- 补齐编排层 / 新 handler / enhance service 的测试达到 98%

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | No（契约不变，仅内部重构） |
| 是否影响现有 UI | No |
| 性能影响 | 无（interface 静态分发） |
| 是否需要新增 Skill | No |
| 风险等级 | 高 — chat 解耦 gin 波及 handler/chat_test + chat_test；main.go 迁移波及路由注册；编排层新建波及 agent_test 全量重写 |
| gomonkey 能否全消除 | No — ADK 内部 + 标准库函数保留 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/domain/chat/contract.go` | 新增 ChatService/SessionService 领域契约 | **New** |
| `internal/domain/task/contract.go` | 新增 TaskService 领域契约 | **New** |
| `internal/logic/agent/orchestrator.go` | 编排器，承接原 service/agent 编排 | **New** |
| `internal/service/agent/` | 拆解：编排迁 logic，gin 包装迁 handler | **Delete/Major** |
| `internal/service/chat/session.go` | Manager 实现 domain/chat.SessionService | Modify |
| `internal/service/chat/chat_service.go` | 解耦 gin（Process 替代 HandleChat） | Modify |
| `internal/api/handler/health.go` `enhance.go` `memory.go` `hermes.go` `agent.go` | 迁出 inline handler | **New** |
| `internal/service/enhance/service.go` | enhance 业务逻辑 | **New** |
| `cmd/server/main.go` | 缩减至 ~300 行 | **Major reduce** |
| `.github/workflows/ut-workflow.yml` | 覆盖率 98% | Modify |
| `internal/service/agent/agent_test.go` | 删除 / 重写为编排器测试 | Modify |
| `internal/service/chat/chat_test.go` | gomonkey→mockery | Modify |

## 7. 测试策略

1. **Unit tests**：编排器用 mockery mock domain 契约；service 测试用 mockery mock repository；覆盖率 ≥98%（CI `ut-workflow.yml`）。
2. **不恢复 `-race`**：gomonkey + race 会 panic，属既定约束。
3. **gomonkey 保留范围**：仅 ADK 内部、标准库函数、package-level 函数。

## 8. UI Test / E2E 验收规则

> 纯架构重构，API 契约不变。

- [ ] **必须** 现有 E2E 全部通过（无 UI 变更）
- [ ] **必须** CI sonar-check + ui-tests 通过才可合并
- [ ] **严禁** 降级已有测试

## 9. Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | domain、logic 纯函数 | **100%** |
| L2 | service（依赖 repository 接口） | **100%** |
| L3 | handler、middleware | **98%** |
| Overall | 全量 | ≥98% |

## 10. 验证标准

1. `go test ./internal/...` 全部通过（本地，无 Docker/网络）
2. `go test -gcflags=all=-l -coverprofile=coverage.out -coverpkg=./internal/... ./internal/...` → 覆盖率 ≥98%
3. CI `ut-workflow.yml`（98% 门禁）+ `sonarqube` + `golangci-lint` + `ui-tests` 全部通过
4. `service/` 包之间无互相 import（`grep -r "internal/service/.*\"internal/service/"` 为空，编排层除外）
5. `agent.Service` 编排逻辑已迁 `logic/agent`，service 层无同层依赖
6. `chat.Service` 无 `*gin.Context` 参数（`grep "gin.Context" internal/service/chat/` 为空）
7. main.go ≤ 300 行，零 inline HandlerFunc
8. `logic/agent.Orchestrator` 存在且被 handler/agent.go 调用

## 11. 不在本 spec 范围

- `domain/model` 所有 struct 全量移除 `bson` tag → **SPEC-057**
- gomonkey 在 ADK / 标准库的彻底移除 → 不做（既定约束）
- `-race` 恢复 → 不做（gomonkey 约束）
