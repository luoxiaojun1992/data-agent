# Task Run 删除与会话并发治理（排他锁 + 会话过滤 + 子会话访问隔离）

> **SPEC-104** | Status: 设计中（立项）
> 日期：2026-09-24 立项

## 0. 现状梳理（2026-09-24 调研实测）

| 项 | 现状 |
|---|---|
| task run 删除 | **无**。`TaskRunRepository` 只有 Create/Get/List/UpdateStatus/UpdateResult/UpdateError/UpdateSessionID/Cancel，**无 Delete**；路由只有 `PUT /task-runs/:id/cancel`（软取消），无 DELETE run 端点 |
| task 定义删除 | `DeleteTask`（`DELETE /tasks/:id`）只 `DeleteOne` agent_task_defs，**不级联 run、不删 session**（SPEC-082 §1.3 红线「删除 ≠ 取消」） |
| run ↔ session 关联 | `TaskRun.SessionID`；executor 创建 ADK session（归属 `run.UserID`）后 `UpdateRunSessionID` 回写。session 执行完**持久保留**，出现在用户 chat 会话列表 |
| session 分类字段 | `Session.IsTask` / `Session.IsFeishu`（domain）+ `SessionRecord.is_task` / `is_feishu`（bson，`omitempty`）。已有 `CreateTaskSession` / `CreateFeishuSession` |
| chat 会话列表过滤 | `ListByUser` / `ListByUserPaged` / `ListDeleted` 查询**未过滤** `is_task`/`is_feishu` → task/feishu session 会混入 chat 会话列表 |
| 子 session | ADK 层 `adk_sessions` 集合，`sessionDoc.ParentSessionID`（`omitempty`）标记，`idx_parent_session` 索引支撑级联删除；**不进业务层 `sessions` 集合**。子 agent 返回即硬删（`subagent/runner.go` cleanup） |
| 子 session 功能边界 | 见 §5.6 —— 审计/压缩在 ADK 层（具备），意图识别/相关性检查在业务入口（不经过，合理缺失），故「子 session 隔离」零改动 |
| 子 session API 泄漏点 | `GET /sessions/:id/messages` 直接查 ADK 层（`adkSessions.Get`），**未先查业务层** → 传入子 session ID 可能读到子 session 消息 |
| 并发锁现状 | **全为进程内 `sync.Mutex`**：① compaction 锁 `adk/session.Service.locks`（per-session，SPEC-092 §4.1）；② 流式批写入锁 `adk/session.Service.bufMu`（chunk 缓冲 map，SPEC-069）。**多服务实例部署时二者都会失效**，本 spec 的排他锁需走 Redis 跨进程 |
| compaction 与 tool 并发保护 | 已有**两层**（均来自 SPEC-069，锁语义 SPEC-092 补充）：① **per-session 锁互斥**——`AppendEvent` 的 `$push/$set events`（`mongo.go:370-375`）与 `maybeCompact` 全程「读 events→算 cut→summarize LLM 调用→`$set events` 整体替换」（`mongo.go:606-608`）共用同一把锁，防止整体替换覆盖并发 append；② **dangling call 边界规则（方案 C）**——`adjustCutForDanglingCalls`/`latestDanglingCallIndex`（`mongo.go:690-731`）保证压缩 cut 点不落在「FunctionCall 与其未返回的 FunctionResponse」之间，异步工具下迟到 response 不会丢配对 call。触发仅限 user message 与 FunctionResponse（`shouldCompact`，FunctionCall 本身不触发）。二者均为进程内机制，多实例失效，与 spec 新增 Redis 锁职责正交 |
| Redis | `infra/redis/client.go` + `cache_repository.go` 已有 Redis 客户端封装（缓存用），**无分布式锁组件** |
| 硬删幂等性 | `Manager.HardDelete` 级联（session record + workspace + ADK history + sub sessions + session_stats），其中 `sessionStatStore.DeleteBySession` 幂等（deletedCount=0 视为成功，SPEC-100 D6）；`repo.HardDelete`/`historyStore.Delete` 均为 DeleteOne/DeleteMany（不存在的文档不报错） |

## 1. 目标

1. **新增 task run 删除**：`DELETE /task-runs/:id` 硬删除 run，删除顺序「先删关联 session（幂等，已删不报错）→ 最后删 run 记录（兜底，失败 abort 可重试）」，归属校验防 IDOR。
2. **run 排他锁**：run 运行期间加排他锁，不允许并发删除；锁 24h 超时自动释放；run 运行成功（含失败/取消）主动释放锁。
3. **chat 会话记录排除 task/feishu**：会话列表（含归档列表）不再展示 task/feishu 的 session。
4. **chat session 并发归档排他锁 + 子 session 访问隔离**：chat 在「用户输入 → LLM 输出」循环期间加排他锁，禁止并发归档（软删）；锁 24h 超时自动释放，session 不被占用后主动释放；子 session 内部继续直接硬删，API 层禁止用户访问子 session。
5. **悬空引用容错**：删除异常出现悬空引用（如 session 已删、run 未删）时，只记录错误日志，保证系统不崩溃。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-082 chat/task 取消（run 级取消 + DeleteTask 正名） | ✅ | `TaskRunRepository`、`TaskRunService`、`CancelRun` 就绪；本 spec 在其上新增 Delete |
| SPEC-090 Session 生命周期（软删/硬删/级联/归属校验） | ✅ | `Manager.HardDelete`（级联 session+workspace+ADK history+子 session，幂等）、`verifyOwnership` 就绪 |
| SPEC-100 Token 埋点 session 统计（DeleteBySession 幂等级联） | ✅ | `SessionStatStore.DeleteBySession` 幂等；`GetRun` 读 token 已容错 |
| SPEC-092 会话并发治理（per-session 锁） | ✅ | 现有进程内锁仅序列化同 session 事件写；本 spec 新增**跨进程（Redis）带 TTL 的排他锁** |
| SPEC-071 子 agent（parent_session_id + 返回即硬删） | ✅ | `ParentSessionID` 字段 + `idx_parent_session` 索引就绪，供 API 层隔离判断 |
| SPEC-003 基础设施（Redis） | ✅ | `infra/redis/client.go` 客户端就绪，新增分布式锁组件复用 |

## 2. 背景与动机

当前 task run **只能取消、不能删除**（SPEC-082 红线「取消 ≠ 删除」只定义了「删 task 定义」和「取消 run」两件事）。后果：

- run 历史只能软取消（状态置 cancelled），无法物理清除；长期堆积的 run 记录 + 其关联 session 无删除出口。
- run 关联的 session 会混入用户 chat 会话列表（`is_task` 未过滤），污染「真实对话」列表；飞书 session 同理。
- 用户可在 chat 列表里误删一个「正在执行中」的 task session（或正在对话的 session），造成 executor 仍在写、session 已删的竞态；反过来删 run 也可能撞上正在运行的 run。
- `GET /sessions/:id/messages` 直查 ADK 层，子 session（`parent_session_id` 非空）可能被越权读到。

**方向**：补齐「run 删除」能力，并用**带 TTL 的排他锁**保护「运行中不删除 / 对话中不归档」两类竞态；会话列表做来源过滤；子 session 在 API 层隔离。

## 3. 架构概述

```
                     Redis 排他锁（SET NX EX 86400，Lua 校验释放）
          ┌──────────────────────────┬──────────────────────────┐
          │  lock:run:{runID}        │  lock:session:{sessionID} │
          └──────────────────────────┴──────────────────────────┘
                ▲ executor acquire            ▲ chat 流处理 acquire
                │  delete 前 try-acquire       │  归档前 try-acquire
                ▼                              ▼
   executor（run 执行）              chat handler（用户输入→LLM 输出）
        │                                   │
        ├─ acquire lock:run 成功后置 running  ├─ acquire lock:session 成功后处理
        ├─ 执行 LLM / 工具链                 ├─ 流式输出
        └─ 完成/失败/取消 → release lock      └─ 流结束 → release lock

   DeleteRun（新）                      Delete（归档，软删）
        │                                   │
        ├─ try-acquire lock:run：失败→409     ├─ try-acquire lock:session：失败→409
        ├─ 先删 session（幂等 HardDelete）     └─ 归档（$set deleted_at）
        └─ 再删 run 记录（DeleteOne 兜底）
```

- **锁实现（选型：Redis 分布式锁）**：新增 `internal/infra/redis/lock.go`——`Acquire(ctx, key, value, ttl)`（`SET key value NX EX ttl`）+ `Release(ctx, key, value)`（Lua 脚本校验 value 再 DEL，防误删他人锁）。TTL 统一 **24h（86400s）**。**选 Redis 而非进程内锁的理由**：现有锁（compaction `locks`、流式批写入 `bufMu`）都是进程内 `sync.Mutex`，多服务实例部署时各实例各持一把锁、互不感知，无法实现「跨实例排他」。本 spec 的 run 删除 / 会话归档排他锁要求**适应未来多实例**，故必须走 Redis（`SET NX EX` 原子抢占 + TTL 兜底），复用现成的 `infra/redis` 客户端。
- **run 锁语义**：executor 执行前 acquire（value=runID 唯一 token），执行结束（成功/失败/取消）主动 release；24h 超时兜底自动释放。DeleteRun 前置 try-acquire：失败说明 run 仍在运行 → 409。
- **session 锁语义**：chat 消息处理（用户输入→LLM 输出）期间 acquire，流结束（含异常）release；24h 超时兜底。归档（Delete 软删）前置 try-acquire：失败说明会话正在对话 → 409。
- **与现有进程内锁的关系**：`adk/session.Service.locks`（SPEC-092）与 `bufMu`（SPEC-069）仍是进程内锁，只管「同 session 事件写入串行 / 流式缓冲」，与本 spec 的「跨操作 Redis 排他锁」职责正交，二者并存、互不替换。现有进程内锁在单实例下继续工作；本 spec 不把它们改造成分布式锁（超出范围）。

## 4. API 设计

| Method | Path | Description | 权限 |
|--------|------|-------------|------|
| DELETE | `/api/v1/task-runs/:run_id` | 硬删除 run（先删关联 session 幂等，再删 run 记录兜底） | `PermAgentEdit` |

会话列表/详情接口**不改签名**，仅内部过滤/隔离：

- `GET /api/v1/sessions`（List）、`GET /api/v1/sessions/deleted`（ListDeleted）→ 结果排除 `is_task`/`is_feishu` session。
- `GET /api/v1/sessions/:id/messages` → 增加业务层 `mgr.Get` 前置 ownership 检查（子 session 不在业务层 → 404）。

## 5. 详细设计

### 5.1 决策 D1 —— task run 删除（DeleteRun）

删除顺序（不可颠倒）：

1. `GetRun(runID, userID, isSystemAdmin)` 校验归属（非本人且非 system_admin → 404，防 IDOR，复用现有 `ErrNotFound`）。
2. **排他锁前置**（见 D2）：try-acquire `lock:run:{runID}`，失败 → 409 `run is running`。
3. **先删关联 session**：`run.SessionID != ""` 时调用 `chat.Manager.HardDelete(run.SessionID)`——**幂等**（session 不存在时各子删返回 deletedCount=0 但 err=nil，视为成功，不报错）。此步失败 → abort（不删 run），记错误日志，返回 500。
4. **最后删 run 记录（兜底）**：`runRepo.Delete(ctx, runID)`（新增 `TaskRunRepository.Delete`，`DeleteOne` agent_task_runs）。失败 → abort（session 已删、run 保留），记错误日志，返回 500，**可重试删除**（重试时步骤 3 幂等跳过）。
5. release 锁，返回 `{"status":"deleted","run_id":runID}`。

### 5.2 决策 D2 —— run 排他锁

- **加锁时机**：executor 将 run 状态置 `running` 之前 `Acquire("lock:run:"+runID, token, 24h)`。acquire 失败（已有锁）说明并发冲突，记错误日志并跳过执行（不覆盖）。
- **释放时机**：run 到达终态（completed/failed/cancelled）时 `Release`，用 `defer` 保证异常路径也释放。
- **删除侧**：`DeleteRun` 前置 try-acquire，失败 → 409（run 仍在运行）；成功则**持有锁直至删除完成**再 release（挡住删除过程中 run 被重新启动的竞态），删除顺序见 D1。
- **超时兜底**：TTL 24h 自动过期释放，防止 run 卡死导致锁永久占用；过期后删除放行（此时 run 大概率已异常，符合「兜底」语义）。

### 5.3 决策 D3 —— chat 会话排除 task/feishu

- `SessionRepository.ListByUser` / `ListByUserPaged` / `ListDeleted` 的 Mongo 查询统一追加过滤：`is_task: {$ne: true}, is_feishu: {$ne: true}`。
- `$ne: true` 语义：匹配「字段不存在」或「字段 ≠ true」的历史存量文档（`omitempty` 未写字段的老数据正确纳入 chat 列表）。
- 仅过滤**展示列表**，不影响 `Get`/`Delete`/`Restore`/`HardDelete` 等按 ID 精确操作（task session 仍可被精确访问/删除，只是不进列表）。

### 5.4 决策 D4 —— chat session 并发归档排他锁 + 子 session 访问隔离

**并发归档锁**：

- 加锁时机：chat 消息处理入口（用户输入提交 → LLM 流式输出全程）`Acquire("lock:session:"+sessionID, token, 24h)`；流结束（含 panic/异常）`defer Release`。
- 归档侧：`Delete`（软删归档）前置 try-acquire，失败 → 409 `session is busy`（会话正在对话，禁止归档）；成功则持有锁直至归档（`$set deleted_at`）完成再 release，挡住归档过程中新对话重新占用该 session 的竞态。
- 硬删 `HardDelete`（`permanent=true`）**不加锁**。理由（晓军拍板）：一旦走到硬删，说明该 session 已经**不能被操作**——业务层 `sessions` 记录即将/已经删除，API 层 `mgr.Get` 前置校验会让后续对该 session 的访问（Get/Messages/Delete）一律 404/403，因此不可能再有「对话进行中」与硬删并发。硬删本身级联清理 ADK 历史，是显式不可逆操作，无需锁保护。

**子 session 访问隔离**：

- 子 session 内部继续走 `subagent/runner.go` cleanup 直接硬删，**不做额外处理**（本 spec 不改子 session 生命周期）。
- API 层隔离：`Messages` handler 在查 ADK 层前先 `mgr.Get(id)` 做业务层存在性 + `verifyOwnership` 校验——子 session 只存在于 ADK 层、不在业务层 `sessions` 集合，因此天然 404，同时统一了 IDOR 防护（与 Get/Delete 一致）。

### 5.5 决策 D5 —— 悬空引用容错

- 删除链路每步失败都 `log.Printf` 记错误日志（含 runID/sessionID/err），**不 panic、不把半删除状态扩散到主流程**。
- 悬空引用形态及处置：
  - 「session 已删、run 未删」：`run.SessionID` 悬空。`GetRun` 内嵌 token 读 `sessionStats.GetBySession` 已容错（err 忽略返回 0，SPEC-100）；前端 run 详情跳 session 时 session 404，由前端容错展示。下次重试 DeleteRun 时步骤 3 幂等跳过、继续删 run。
  - 「run 已删、session 残留」：理论上不产生（删除顺序先 session 后 run），若因异常残留，属孤儿 session，由现有 `Cleanup`（过期硬删）兜底。
- 兜底原则：**日志可追溯、系统不崩溃、操作可重试**。

### 5.6 决策 D6 —— 子 session 功能边界（澄清「隔离零改动」的依据）

子 agent 的执行链路是 `subagent.Runner.Run → rt.RunAndCollect → adkruntime.Runtime.RunContent → ADK runner`，**绕过** chat/task 业务入口。据此逐项核对「子 session 是否具备基本功能」：

| 功能 | 挂载层 | 子 session 是否具备 | 说明 |
|---|---|---|---|
| 安全审计（脱敏） | ADK runtime callback（`buildRuntime` 注入 `Auditor`） | ✅ 具备 | `GetOrCreateSubAgent` 与父 agent 走同一 `buildRuntime`，输入/输出/工具调用脱敏均生效 |
| compaction 上下文压缩 | ADK session service（`AppendEvent → maybeCompact`） | ✅ 具备 | 子 session 复用同一 `*adksession.Service`；但子 session 生命周期极短（一次委派即 `cleanup` 硬删），通常达不到压缩阈值 |
| 意图识别 `CheckIntent` | chat/task 业务入口（`chat_service.go:162`、executor、feishu） | ❌ 不具备 | 子 agent 走 `RunAndCollect` 直接进 runtime，不经过意图识别；**合理**——子任务由父 agent 明确委派，无需再判「是不是任务」 |
| 相关性检查 `CheckRelevance` | chat/task 业务入口（`chat_service.go:419/615`、`executor.go:261`） | ❌ 不具备 | 同上，绕过；**合理**——子 agent 输出仅作 tool result 返回父 agent，由父 agent 下一轮把关、或父 agent 最终输出才过相关性检查 |

**结论**：子 session 不在业务层 `sessions` 集合（只存在于 ADK 层），且其「缺失」的意图识别/相关性检查是**设计使然**（父 agent 是最终把关者），不存在需要补齐的功能缺口。因此「子 session 隔离」本 spec 确实**零改动**——只需 `Messages` 加 `mgr.Get` 前置（见 D4），把本就正确的「子 session 不进业务层」转成 API 层的 404 语义。



### 5.7 代码变更清单（概要）

| 层 | 变更 |
|---|---|
| domain/task/contract.go | `TaskRunService` 加 `DeleteRun(id, userID string, isSystemAdmin bool) error` |
| domain/task/mocks | mockery 重新生成 `TaskRunService` |
| repository/task.go | `TaskRunRepository` 加 `Delete(ctx, id) error`；mocks 同步 |
| infra/mongo/task_def_repository.go | `TaskRunRepository.Delete`（`DeleteOne` agent_task_runs） |
| service/task/service.go | `DeleteRun`（归属校验 + 锁前置 + 先删 session 幂等 + 再删 run 兜底）；注入 session 删除依赖 + lock |
| logic/agent/executor.go | run 执行前 acquire `lock:run`、终态 defer release |
| service/chat/session.go | 列表查询不在此改（改 repo）；`Delete` 前置 lock；chat 流入口 acquire/release lock |
| infra/redis/lock.go | **新增** 分布式锁组件 `Acquire`/`Release`（SET NX EX + Lua 校验释放） |
| infra/mongo/session_repository.go | `ListByUser`/`ListByUserPaged`/`ListDeleted` 加 `is_task`/`is_feishu` 过滤 |
| api/handler/task.go | `DeleteRun` handler（`DELETE /task-runs/:id`） |
| api/handler/session.go | `Messages` 加 `mgr.Get` + `verifyOwnership` 前置 |
| api/handler/routes.go | 注册 `taskRunRoutes.DELETE("/:run_id", h.DeleteRun)` |
| wire.go | 注入 lock 组件 + task service 的 session 删除依赖 |

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | **No**。锁走 Redis（不进 Mongo）；run/session 复用现有集合 |
| 是否影响现有 API | **Yes（低风险）**。新增 `DELETE /task-runs/:id`；`GET /sessions`、`GET /sessions/deleted` 结果集收窄（排除 task/feishu）；`GET /sessions/:id/messages` 增加前置 ownership 校验 |
| 性能影响 | 低。列表查询多两个 `$ne` 条件（已有索引可覆盖 user_id 过滤）；锁为一次 SET/GET+DEL 的 O(1) Redis 操作 |
| 是否需要新增 Skill | **No** |
| 是否新增外部依赖 | **No**。复用现有 Redis 客户端，锁组件为内部封装 |
| 锁的跨进程正确性 | Redis 锁天然跨进程（worker goroutine 与 HTTP handler 同进程，但 Redis 锁同样兼容未来多实例） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/redis/lock.go` | **新增** Redis 分布式锁组件（Acquire/Release） | New |
| `internal/domain/task/contract.go` | TaskRunService 加 DeleteRun | Small |
| `internal/repository/task.go` | TaskRunRepository 加 Delete | Small |
| `internal/infra/mongo/task_def_repository.go` | TaskRunRepository.Delete 实现 | Small |
| `internal/service/task/service.go` | DeleteRun 编排（锁 + 先 session 后 run） | Medium |
| `internal/logic/agent/executor.go` | run 执行加/释放排他锁 | Medium |
| `internal/service/chat/session.go` | Delete 加锁前置；chat 流入口加/释放锁 | Medium |
| `internal/infra/mongo/session_repository.go` | 列表查询加 is_task/is_feishu 过滤 | Small |
| `internal/api/handler/task.go` | DeleteRun handler | Small |
| `internal/api/handler/session.go` | Messages 前置 ownership 校验 | Small |
| `internal/api/handler/routes.go` | 注册 DELETE run 路由 | Small |
| `internal/wire.go` | 注入 lock + task service 依赖 | Small |

## 8. 测试策略

1. **Unit tests**（Go）: 覆盖率底线见 SPEC-045。`infra/redis/lock`（L2，mock redis 客户端）、`service/task.DeleteRun`（L2，mock repo + mock session 删除 + mock lock）、`service/chat` 归档锁（L2）、`session_repository` 过滤（L3，gomonkey mock Mongo）、handler DeleteRun/Messages（L3）。
2. **Integration tests**: 条件使用 Docker Compose（`go test -tags=integration`）验证真实 Redis 锁 TTL 与 Lua 释放。
3. **E2E tests**（条件）: 前端 task run 删除入口若新增则补 `UI-XXX`；`GET /sessions` 列表排除 task/feishu 断言。
4. **审计**: 使用 `.agent/skills/go-ut-audit` 审查 UT 质量。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
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
| L2 | 依赖接口，可 mock | **100%** | `infra/redis/lock`、`service/task.DeleteRun` |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `service/chat`、`api/handler/*`、`infra/mongo/*` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc`（非 `ApplyMethodReturn`）验证 handler→service 参数传递正确性
- [ ] **必须** Service 测试的写操作（`DeleteOne`, `UpdateOne` 等）验证写入内容的字段和值
- [ ] **必须** 锁组件测试覆盖：acquire 成功/冲突、TTL 透传、release 校验 token（Lua 原子性）、release 非本人 token 拒绝
- [ ] **必须** DeleteRun 测试覆盖：先删 session 再删 run 的顺序断言、session 已删（幂等）不报错、删 session 失败 abort 不删 run、删 run 失败保留可重试、锁占用 409
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

## 10. 验证标准

1. `DELETE /api/v1/task-runs/:run_id`：删除成功后 run 记录消失、关联 session 消失（幂等）；非本人 run 返回 404；running 中的 run 返回 409。
2. run 运行期间并发调用删除 → 409；run 完成后删除 → 200 且 session 级联清除。
3. `GET /api/v1/sessions` 与 `GET /api/v1/sessions/deleted` 不再返回 `is_task`/`is_feishu` 为 true 的 session；存量无字段 session 正常展示。
4. chat 会话正在对话（用户输入→LLM 输出）期间调用归档 → 409；会话空闲后归档 → 200。
5. `GET /api/v1/sessions/:id/messages` 传入子 session ID → 404（业务层不存在）；传入他人 session → 403（IDOR）。
6. 模拟「session 已删、run 未删」的悬空状态 → `GetRun` 正常返回（token 为 0）、系统无 panic、错误日志可见、重试删除成功。
7. Go UT 覆盖率 ≥98%，`go vet` 无警告，CI（ut-workflow + sonar-check + ui-tests）全部通过。
