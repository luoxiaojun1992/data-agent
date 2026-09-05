# Session 生命周期管理（归档 / 删除 / 清空历史 / 永久产物隔离）

> **SPEC-090** | Status: 设计中

## 1. 目标

规范化 session 的生命周期管理，补齐三个缺失能力并修正现有软删除的语义缺陷：

1. **归档（软删除）**：仅标记 `deleted_at`，可恢复，**保留** workspace 与 chat history。
2. **删除（硬删除）**：彻底移除 session，同时删除 workspace 与 chat history（compacted `events` + raw `session_events`），但**不删** artifact / memory 等永久产物。
3. **清空聊天历史**：只清除 chat history（compacted + raw），**保留** session 文档与 workspace。
4. **永久产物隔离**：任何 session 删除操作都不得级联删除 artifact、memory（二者虽绑定 `session_id`，但已脱离 session 独立存在，是跨会话可复用的永久产物）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-053 会话存储/记忆压缩/KB 索引对齐 | ✅ | raw_events 双轨已就绪（`adk_sessions.events` compacted / `session_events` raw） |
| SPEC-069 compaction 缺陷修复 + raw_events 独立 collection | ✅ | `session_events` 独立 collection，`session_id` 索引可级联删 |
| SPEC-071 agent 调用子 agent（parent_session_id 级联删除） | ✅ | `adk_sessions.parent_session_id` + `deleteSubSessions` 已就绪，硬删除复用 |
| SPEC-064 RBAC 权限管理 | ✅ | `chat:delete` 权限已存在 |
| SPEC-005 Artifact 存储与工作区 | ✅ | `artifacts` 集合（`session_id` + `Persistent`）+ workspace 目录 |

无阻塞项，可立即开始（立项阶段不实现）。

## 2. 背景（现有实现不足）

当前 session 生命周期存在四处问题：

1. **软删除语义缺陷**：`service/chat/session.go` 的 `Manager.Delete` 先 `removeWorkspace(id)` 再 `repo.Delete`（软删 `$set deleted_at`）。归档后 workspace 被删，`Restore` 恢复的 session 丢失工作区文件，恢复不完整。
2. **无硬删除能力**：`Cleanup`（过期清理，`repo.Cleanup` → `DeleteMany expires_at < now`）只删 `sessions` 文档，不级联删 `adk_sessions` / `session_events` / workspace，产生孤儿数据。
3. **无清空聊天历史能力**：用户无法单独清除对话记录而保留 session 与 workspace。
4. **缺归属校验**：`SessionHandler` 的 `Get` / `Delete` / `Restore` 直接 `mgr.xxx(c.Param("id"))`，未校验 session 是否属于当前用户（IDOR 隐患，与 SPEC-084 的 IDOR 归属校验同源）。

## 3. 架构概述

### 3.1 存储全景

| 存储 | 集合 / 位置 | 关联字段 | 性质 |
|------|------------|---------|------|
| session 主记录 | `sessions` | `_id` | 会话元数据（含 `deleted_at` 软删标记）|
| chat history（compacted）| `adk_sessions.events` | `_id` = session_id | LLM 上下文（compaction 后：摘要 + 近 N 条）|
| chat history（raw）| `session_events` | `session_id` | 前端展示（append-only）|
| chat history（legacy raw）| `adk_sessions.raw_events` | 数组字段 | 旧版 fallback（SPEC-069 前）|
| workspace | `/tmp/data-agent-sessions/{sessionID}` | 目录 | 会话工作区文件 |
| artifact | `artifacts` + SeaweedFS | `session_id` | **永久产物**（`Persistent`）|
| memory | `memories` | `session_id` | **永久产物**（跨会话合并复用）|
| 子 session | `adk_sessions.parent_session_id` | parent | SPEC-071 子 agent |

### 3.2 生命周期语义矩阵（核心）

| 操作 | session 文档 | workspace | compacted(events) | raw(session_events) | artifact | memory | 子 session |
|------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **归档（软删除）** | 标 `deleted_at` | 保留 | 保留 | 保留 | 保留 | 保留 | 保留 |
| **恢复** | `$unset deleted_at` | 保留 | 保留 | 保留 | 保留 | 保留 | 保留 |
| **删除（硬删除）** | 硬删 | 删 | 删 | 删 | 保留 | 保留 | 删（级联）|
| **清空聊天历史** | 保留 | 保留 | 清空 | 清空 | 保留 | 保留 | 不涉及 |
| **过期清理（Cleanup）** | 硬删 | 删 | 删 | 删 | 保留 | 保留 | 删（级联）|

> 红线（需求 4）：artifact / memory 在所有操作下**一律保留**。删除 session 只清理「会话本身 + 会话专属的 workspace + chat history」，永久产物不受影响。

### 3.3 与现有代码的关系

- **归档** = 修正现有 `Manager.Delete`：去掉 `removeWorkspace` 调用，纯软删。
- **删除（硬删）** = 新增 `Manager.HardDelete`：编排 `repo.HardDelete`（物理删 `sessions`）+ `removeWorkspace` + `adkSessions.Delete`（硬删 `adk_sessions` + 级联删 `session_events` + 子 session）。
- **清空历史** = 新增 `Manager.ClearHistory`：编排 `adkSessions.ClearHistory`（清空 `events` + `session_events`，保留 `adk_sessions` 文档与 `state`）。
- **过期清理** = 增强 `Manager.Cleanup`：先 `List` 出过期 session ID，逐个级联删（复用 `HardDelete` 的级联路径）。

## 4. API 设计

| Method | Path | Description | 权限 | 归属校验 |
|--------|------|-------------|------|:---:|
| DELETE | `/api/v1/sessions/:id` | 归档（软删除），保留 workspace + chat history | `chat:delete` | ✅ |
| POST | `/api/v1/sessions/:id/restore` | 恢复归档 session | `chat:view` | ✅ |
| GET | `/api/v1/sessions/deleted` | 列归档 session（恢复窗口） | `chat:view` | ✅（按 user 过滤）|
| **DELETE** | **`/api/v1/sessions/:id/history`** | **清空聊天历史**（compacted + raw），保留 session + workspace | `chat:delete` | ✅ |
| **DELETE** | **`/api/v1/sessions/:id?permanent=true`** | **硬删除**（session + workspace + chat history + 子 session），保留 artifact/memory | `chat:delete` | ✅ |

> D1 决策点：硬删除采用 query 参数 `permanent=true` 复用现有 DELETE 路由（避免新增路由 + 权限挂载复杂度），语义清晰且幂等。备选：`DELETE /api/v1/sessions/:id/forever`。默认推荐 query 参数方案。

### 4.1 响应

- 归档 / 恢复 / 清空 / 硬删：`200 {"message": "..."}`
- 归属不符：`403 {"error": "session does not belong to the current user"}`
- 不存在：`404 {"error": "session not found"}`

## 5. 详细设计

### 5.1 数据流

**归档**：
```
DELETE /sessions/:id
  → verifyOwnership(session.user_id)
  → repo.Delete(id)  // $set deleted_at（不再删 workspace）
  → 200
```

**硬删除**：
```
DELETE /sessions/:id?permanent=true
  → verifyOwnership
  → repo.HardDelete(id)          // DeleteOne sessions（物理删）
  → removeWorkspace(id)          // os.RemoveAll 工作区
  → adkSessions.Delete(sessionID) // 硬删 adk_sessions + session_events + 级联子 session
  → 200
```

**清空历史**：
```
DELETE /sessions/:id/history
  → verifyOwnership
  → adkSessions.ClearHistory(sessionID) // $set events=[] + DeleteMany session_events + 清 legacy raw_events
  → 200
```

**过期清理**（后台定时）：
```
Cleanup()
  → 列出 expires_at < now 的 session（含软删的 ListDeleted）
  → 逐 id 复用 HardDelete 级联路径（不删 artifact/memory）
```

### 5.2 模块设计

#### 5.2.1 `internal/adk/session`（ADK 存储层）

新增 `ClearHistory` 方法到 `Service`：

```go
// ClearHistory wipes a session's chat history but keeps the session document
// and its state (workspace binding survives). It clears:
//   - adk_sessions.events (compacted LLM context)
//   - adk_sessions.raw_events (legacy array)
//   - session_events (raw event stream)
func (s *Service) ClearHistory(ctx context.Context, sessionID string) error
```

实现要点：
- `UpdateOne({_id: sessionID}, {$set: {events: [], raw_events: [], updated_at: now}})`
- `DeleteMany(session_events, {session_id: sessionID})`
- 保留 `state`、`parent_session_id`、`user_id`、`app_name`。

#### 5.2.2 `internal/repository`（SessionRepository）

新增 `HardDelete`（物理删除）：

```go
HardDelete(ctx context.Context, id string) error // DeleteOne sessions 文档
```

`Delete` 保持软删（`$set deleted_at`）。

#### 5.2.3 `internal/service/chat`（chat.Manager）

- 修正 `Delete`：去掉 `removeWorkspace`，纯软删。
- 新增 `HardDelete(id string) error`：编排 `repo.HardDelete` + `removeWorkspace` + `adkSessions.Delete`。
- 新增 `ClearHistory(id string) error`：编排 `adkSessions.ClearHistory`。
- 增强 `Cleanup()`：级联清理过期 session 的 workspace + chat history。
- `Manager` 注入窄接口 `HistoryStore`（仅 `Delete` / `ClearHistory`），避免直接依赖 `google.golang.org/adk/session.Service`：

```go
// domain/chat/contract.go 新增（service 层依赖的窄接口，infra 由 adk session.Service 实现适配）
type SessionHistoryStore interface {
    Delete(ctx context.Context, sessionID string) error      // 硬删 adk_sessions + session_events + 子 session
    ClearHistory(ctx context.Context, sessionID string) error // 清空 events + session_events
}
```

> 分层约束：`internal/service/chat` 依赖 `internal/adk/session`（infra/ADK 层）是 service→infra 依赖，符合分层铁律；通过窄接口注入避免直接耦合 adk 具体类型。

#### 5.2.4 `internal/api/handler/session.go`

- `Delete`：支持 `permanent=true` query 参数 → 走 `HardDelete`；否则走 `Delete`（归档）。
- 新增 `ClearHistory` handler（挂 `DELETE /:id/history` 路由）。
- `Get` / `Delete` / `Restore` / `ClearHistory` 统一补归属校验 `verifyOwnership`（`session.UserID != userID && role != "system_admin"` → 403）。
- 路由注册：`rg.DELETE("/:id/history", ..., h.ClearHistory)`。

#### 5.2.5 前端 `chat/page.tsx`

- session 列表每个条目：保留现有「删除」按钮（归档）；归档入口已有恢复 banner。
- 新增「清空聊天历史」入口（hover 菜单 / 按钮）。
- 硬删除入口：归档后的恢复 banner 内提供「彻底删除」选项（`DELETE /sessions/:id?permanent=true`）。

### 5.3 数据模型（Go struct）

```go
// repository.SessionRecord 已有 deleted_at / recoverable / recovery_hours 字段，无需新增。
// domain/chat.Session 已有 DeletedAt / RecoveryUntil 字段，无需新增。
```

> 顺带修复：`service/chat/session.go` 的 `recordToSession` 未回填 `Status` / `DeletedAt` / `RecoveryUntil`（Get/List 返回的 session 丢失归档状态）。本 spec 一并修复。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `sessions` / `adk_sessions` / `session_events`）|
| 是否影响现有 API | Yes（`DELETE /sessions/:id` 语义修正 + 新增 2 个端点 + 权限/归属校验补强）|
| 性能影响 | 低（单 session 级联删除，索引 `session_id` / `parent_session_id` 已就绪）|
| 是否需要新增 Skill | No（纯 session 管理，无 LLM 工具）|
| 是否需要新增权限 | No（复用 `chat:delete` / `chat:view`）|
| 是否涉及永久产物误删风险 | 需谨慎：artifact/memory 一律不删，明确红线 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/session/mongo.go` | 新增 `ClearHistory`（清 events + session_events）| Medium |
| `internal/adk/session/mongo_test.go` | `ClearHistory` 单测 | Medium |
| `internal/repository/session.go` | 接口新增 `HardDelete` | Small |
| `internal/infra/mongo/session_repository.go` | 实现 `HardDelete`（物理删）| Small |
| `internal/service/chat/session.go` | 修正 `Delete`、新增 `HardDelete`/`ClearHistory`、增强 `Cleanup`、修复 `recordToSession` | Large |
| `internal/domain/chat/contract.go` | 新增 `SessionHistoryStore` 窄接口 + `SessionService` 接口扩展 | Medium |
| `internal/api/handler/session.go` | `Delete` 支持 permanent、新增 `ClearHistory`、补归属校验 | Medium |
| `internal/api/handler/session_test.go` | 新增 handler 单测 | Medium |
| `cmd/server/wire.go` / `main.go` | 注入 `SessionHistoryStore`（adk session.Service 适配）| Small |
| `frontend/app/chat/page.tsx` | 清空历史 + 硬删除入口 | Medium |

## 8. 测试策略

1. **Unit tests**（Go）：覆盖率基线 SPEC-045。L1 纯逻辑 100%，L3 完整链路 98%。重点：`ClearHistory` 清空三处存储且保留 state；`HardDelete` 级联删 workspace + adk + events 且不删 artifact/memory；归属校验 403。
2. **Integration tests**：条件使用 Docker Compose（`go test -tags=integration`）。
3. **E2E tests**：用例编号 `UI-XXX`，覆盖前端清空历史 / 硬删除交互。
4. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互（清空历史 / 硬删除入口）时同步编写 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `logic/*`, `config` |
| L2 | 依赖接口，可 mock | **100%** | service interfaces |
| L3 | 依赖 MongoDB/HTTP | **98%** | `service/chat`, `api/handler`, `adk/session` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** `HardDelete` 测试验证「session 删除 + workspace 删除 + adk/events 删除」三个副作用
- [ ] **必须** `ClearHistory` 测试验证「events 清空 + raw 清空 + state/session 保留」
- [ ] **必须** 归属校验测试：跨用户访问返回 403，system_admin 豁免返回 200
- [ ] **必须** 验证 artifact/memory 在删除操作后仍存在（红线回归测试）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证副作用

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%
- [ ] `go vet` 无警告

## 10. 验证标准

- [ ] 归档后 session 不出现在列表，`GET /sessions/deleted` 可见，恢复后 workspace 文件仍在（当前 bug 已修复）
- [ ] 硬删除后 `sessions` / `adk_sessions` / `session_events` / workspace 四者全部消失，`artifacts` / `memories` 记录仍在
- [ ] 清空历史后 `adk_sessions.events` 与 `session_events` 为空，session 与 workspace 保留，下一轮对话从空白上下文开始
- [ ] 跨用户删除/清空/恢复返回 403，system_admin 豁免 200
- [ ] 过期清理不再产生孤儿 workspace / adk_sessions / session_events

## 待拍板决策点（D1-D4）

| 决策点 | 内容 | 推荐值 |
|--------|------|--------|
| D1 | 硬删除 API 形态 | `DELETE /sessions/:id?permanent=true`（复用路由）|
| D2 | 清空历史后是否保留 compaction summary | 不保留（events 整体清空，从空白上下文重来）|
| D3 | 归档是否保留 workspace | 保留（软删可完整恢复；修正现有删 workspace 的 bug）|
| D4 | 过期清理（Cleanup）是否纳入本次级联修复 | 纳入（一并修复孤儿数据）|
