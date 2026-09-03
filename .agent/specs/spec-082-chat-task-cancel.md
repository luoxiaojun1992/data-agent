# Chat 与 Agent Task 支持取消

> **SPEC-082** | Status: 设计中

## 1. 目标

1. **Chat 取消**：前端发送中显示「停止」按钮，点击后中断与后端的 SSE 连接；后端 runtime 调用**继承上层 ctx**（现状已透传，本次只补兜底），客户端断开 → 请求 ctx 取消 → 底层 ADK 运行随之终止（**不侵入、不修改底层 ADK 的 ctx 用法**）。

2. **Task 启停统一为开关字段**：**删除 task 级「取消」（现为 DeleteOne 物理删除）**，非定时 task 与定时 task 统一使用同一开关字段 `ScheduledEnabled`（`scheduled_enabled`）。**关闭开关 → 不再创建 run**；开关语义从「仅定时任务」泛化为「所有 task 的启停开关」。

3. **Task Run 取消**：取消状态只落在 **run 上**（`StatusCancelled` 已存在）。已创建的 run 分两条路径取消：
   - **未执行**（pending/queued）：执行前判断 DB，直接跳过；
   - **执行中**（running）：executor 内**轮询 DB** 对应 run 的状态（`for select case`），发现已取消 → `cancel()` 子 ctx → 继承的子 ctx 使底层运行终止（**不侵入底层 ADK**）。

4. **完成临界点**：取消是**尽力而为**。一旦 run 走到「执行完成」——即使只差把结果写回 DB（`save_task_result` 已置 `completed`）——就**当作已完成**，不再响应取消。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 / SPEC-048 / SPEC-062 | ✅ | chat/agent task 的 ADK Runtime 运行链路已就绪 |
| SPEC-063 | ✅ | worker pool（ants）+ AgentExecutor 已上线，Execute 入口已有 cancelled 判断与事后检查 |
| SPEC-069 | ✅ | 事件落库不受取消影响（cancel 后 partial 内容按现有流式机制落库） |
| — | — | 无阻塞项 |

## 2. 背景（现状与缺口）

| 现状 | 缺口 |
|------|------|
| `ScheduledEnabled` 开关字段已存在，前端 agent 页已有 ON/OFF 切换 UI（`PATCH /admin/tasks/:id/scheduled-enabled`）；调度器 `ListScheduled` 已按 `scheduled_enabled != false` 过滤 | **语义只覆盖定时 task**：非定时 task 无启停概念；`CreateRun` 未检查开关，关闭后手动触发仍会建 run |
| `CancelTask`（`PUT /tasks/:task_id/cancel`）= `TaskDefRepository.Cancel` = **DeleteOne 物理删除** | **task 级「取消」是物理删除**，与「开关」语义重复且不可恢复——应删除，统一改用开关 |
| `PauseTask`/`ResumeTask` 存在但语义混乱（PauseTask 误把 task_id 当 run_id 调 `CancelRun`），前端未调用 | 应随 task 级取消一并清理 |
| `StatusCancelled` + `service.CancelRun` + `TaskRunRepository.Cancel` 已存在 | **run 级取消无 HTTP 路由**；`Cancel` 条件仅 `[queued, pending]`，**不支持执行中取消** |
| `AgentExecutor.Execute` 入口检查 `StatusCancelled`（执行前判断 ✅）；事后 `wasRunCancelled` 保护 ✅ | **执行中无法响应取消**：`RunAndCollectContent` 阻塞期间取消不生效 |
| chat ctx 链路已透传：`c.Request.Context() → Stream → streamOnce → RunContent` ✅ | **前端无取消按钮/AbortController**；`streamOnce` 无 `ctx.Done` 兜底；取消后可能误入 relevance 重试计数 |
| 前端 agent 任务详情页展示运行状态 | **前端运行列表无取消按钮** |

## 3. 架构概述

```
┌─ Chat 取消 ─────────────────────────────────────────────┐
│ 前端「停止」按钮 → AbortController.abort() → fetch SSE 中断 │
│   │                                                      │
│   ▼ 客户端断开 → c.Request.Context() Done                 │
│ 后端 Stream → streamOnce → RunContent(ctx)（透传，现状）    │
│   + streamOnce 加 ctx.Done 兜底（select 提前退出，跳过      │
│     relevance 重试，避免取消被计入 retry 计数）             │
└──────────────────────────────────────────────────────────┘

┌─ Task 启停（开关，替代原 task 级取消）─────────────────────┐
│ PATCH /tasks/:task_id/enabled {enabled:false} → 关闭       │
│   → CreateRun / 调度器 检查开关 → 不创建 run                │
│ （删除 PUT /tasks/:task_id/cancel 的 DeleteOne 语义）       │
└──────────────────────────────────────────────────────────┘

┌─ Task Run 取消 ─────────────────────────────────────────┐
│ 前端运行列表「取消」→ PUT /task-runs/:run_id/cancel        │
│   → CancelRun 置 StatusCancelled（DB）                    │
│   │                                                      │
│   ├─ 未执行：worker 派发前 / Execute 入口复核 DB → 跳过     │
│   └─ 执行中：Execute 内子 ctx + 轮询 goroutine             │
│        for select case:                                   │
│          ticker(2s) → GetRun → cancelled? → cancel()      │
│          completed/failed? → 退出（完成临界点，不再取消）    │
│          runCtx.Done() → 退出轮询                          │
│        RunAndCollectContent(runCtx) → 底层随取消终止        │
│   （底层 ADK 的 ctx 用法不修改、不侵入）                      │
└──────────────────────────────────────────────────────────┘
```

## 4. API 设计

### 4.1 变更：Task 启停开关（删除 task 级取消）

| Method | Path | 变更 | Description |
|--------|------|------|-------------|
| PATCH | `/api/v1/tasks/:task_id/enabled` | **新增** | 统一启停开关（JWT 认证；body `{"enabled": bool}`；非定时/定时 task 通用） |
| PUT | `/api/v1/tasks/:task_id/cancel` | **删除** | 原 DeleteOne 物理删除，废弃 |
| PUT | `/api/v1/tasks/:task_id/pause` | **删除** | 语义混乱（误把 task_id 当 run_id），废弃 |
| PUT | `/api/v1/tasks/:task_id/resume` | **删除** | 与开关重复，废弃 |

> 现有 admin 侧 `PATCH /admin/tasks/:id/scheduled-enabled` 由新用户侧接口替代；前端 agent 页的 ON/OFF 切换改调 `/tasks/:task_id/enabled`。字段名 `ScheduledEnabled`（bson `scheduled_enabled`）保留（前端已按此渲染），仅语义泛化为「所有 task 启停开关，默认开启」。
>
> 关闭开关的语义：**不删除、不置终态，仅停止后续创建 run**。已有 run 不受影响（继续跑完或按 §4.2 单独取消）。

### 4.2 新增：run 级取消（Task）

| Method | Path | Description |
|--------|------|-------------|
| PUT | `/api/v1/task-runs/:run_id/cancel` | 取消指定 run（JWT 认证；校验 run 归属当前用户；仅 pending/queued/running 可取消） |

响应 200：

```json
{ "status": "cancelled", "run_id": "uuid" }
```

错误语义：
- 404：run 不存在或不属于当前用户
- 409：run 已处于终态（completed/failed/cancelled），不可取消

### 4.3 Chat 无新 API

chat 取消由「客户端断开 SSE」表达，无需新端点（取消语义 = 请求上下文终止）。

## 5. 详细设计

### 5.1 Chat 前端：停止按钮 + SSE 中断

- 发送中（`streaming` 状态）时，发送按钮位置切换为「停止」按钮（`data-testid="chat-stop-btn"`）。
- 点击：`AbortController.abort()` 中断 fetch SSE；本地状态复位（streaming=false，输入框恢复）；已流出的部分内容保留在消息区（现状行为）；toast/轻提示「已停止生成」。
- 多轮/相关性重试期间按钮保持可见（整个 Stream 生命周期内都可点）。

### 5.2 Chat 后端：ctx 透传兜底（不侵入 ADK）

- ctx 链路现状已符合「继承上层 ctx」要求，**不改** `RunContent` 的调用方式与 ADK 底层。
- `streamOnce` 的事件循环加 `ctx.Done` 兜底：循环外 `select`（或循环内检查 `ctx.Err()`）→ 客户端断开时提前 return，不再向已断开的连接写 SSE。
- `Stream` 的 relevance 重试段：进入前检查 `ctx.Err() != nil` 则跳过（**取消不触发 `RecordAndShouldRetry` 计数**，避免取消被误判为「不相关」消耗重试额度）。
- 取消产生的 partial 内容落库：按现有流式事件机制处理，**不新增特殊分支**。

### 5.3 Task 启停开关（替代 task 级取消）

- **删除**：`TaskHandler.CancelTask`、`TaskHandler.PauseTask`、`TaskHandler.ResumeTask` + 对应路由；`service.CancelTask`；`TaskDefRepository.Cancel`（DeleteOne 语义）。
- **泛化** `ScheduledEnabled`：语义从「仅定时任务开关」改为「所有 task 启停开关」，`CreateTask` 已默认 `ScheduledEnabled = true`（保留）；非定时 task 同样可切换。
- **新增** `service.SetEnabled`（复用现有 `SetScheduledEnabled` 逻辑，语义不变）+ `PATCH /api/v1/tasks/:task_id/enabled`（用户侧，替代 admin `scheduled-enabled`）。
- **建 run 前置检查**（关键）：`CreateRun` 加载 task 后先判 `!task.ScheduledEnabled → 返回错误（409，语义「task 已停用」），不创建 run、不入队`。调度器 `ListScheduled` 已过滤 `scheduled_enabled != false`（保留），关闭的定时 task 不再被派发。
- `CreateTask` 对非定时 task 仍立即创建首个 run（现状，默认开启）；若未来支持「创建时即停用」，再扩展入参。

### 5.4 Task Run 取消：数据层支持执行中取消

`TaskRunRepository.Cancel` 的过滤条件从 `[queued, pending]` 扩展为 `[queued, pending, running]`：

```go
bson.M{"_id": id, "status": bson.M{"$in": []string{
    string(task.StatusQueued), string(task.StatusPending), string(task.StatusRunning),
}}}
```

- 这样 run 处于 `running` 时也能被置为 `cancelled`（供执行中轮询检测）。
- **完成临界点在数据层的天然保证**：条件**不含** `completed/failed/cancelled`，run 一旦进入终态，Cancel 的 `UpdateOne` 匹配数为 0、不生效 → 完成后的 run 不会被取消。

### 5.5 Task 后端：执行中轮询取消（不侵入 ADK）

`AgentExecutor.Execute` 改造（红线：只在 executor 层加监督逻辑，`RunAndCollectContent` 调用方式不变）：

```go
runCtx, cancel := context.WithCancel(ctx)
defer cancel()

// 轮询监督：执行中检测 DB 取消状态；检测到终态（完成临界点）即退出
done := make(chan struct{})
defer close(done)
go func() {
    t := time.NewTicker(2 * time.Second)
    defer t.Stop()
    for {
        select {
        case <-runCtx.Done():
            return
        case <-t.C:
            latest, err := e.runs.GetRun(run.ID)
            if err != nil || latest == nil {
                continue // 静默继续，DB 抖动不能误杀正常任务
            }
            switch latest.Status {
            case domaintask.StatusCancelled:
                cancel()
                return
            case domaintask.StatusCompleted, domaintask.StatusFailed:
                return // 完成临界点：已结束，无需再取消
            }
        }
    }
}()

// 后续 RunAndCollectContent / relevanceLoop / retry 全部改用 runCtx
```

- 轮询覆盖整个 Execute（首次运行 + relevance 重试 + save_task_result 重试）。
- `cancel()` 后底层 LLM 调用随 `runCtx.Done()` 终止（ADK 内部对 ctx 的响应行为不改、不侵入）。
- 取消后状态保持 `cancelled` 不被覆盖（现有 `wasRunCancelled` 检查已保证，保留）。
- 轮询失败（GetRun 出错）静默继续下一轮，不中断执行（取消是尽力而为）。

### 5.6 完成临界点语义（显式约束）

- **定义**：run 一旦「执行完成」——含 `save_task_result` 已把 `status` 置为 `completed`（可能只差结果写回 DB）——即视为已完成，**不再接受取消**。
- **数据层保证**：§5.4 的 Cancel 条件不含终态，完成后的 run 无法被置 cancelled（`matched=0`，API 返回 409）。
- **executor 保证**：§5.5 轮询检测到 `completed/failed` 即退出监督 goroutine，不再尝试取消；正常完成路径（`notifyRun`）不会被轮询打断。
- **边界说明**：若取消信号恰在「LLM 已返回、尚未写 completed」的极小窗口内到达，以「先落库者为准」——`UpdateRunResult` 写 completed 与 `Cancel` 写 cancelled 的竞态由 Mongo 单文档原子更新裁决，最终状态必为二者之一，无脏状态。

### 5.7 Task 未执行取消（已有 + 兜底复核）

- 现状：`Execute` 入口 `if run.Status == StatusCancelled { return nil }`（执行前判断 ✅，保留）。
- 兜底：`pool.dispatch` 在 `Execute` 前加一道 DB 状态复核（拉最新 run，cancelled 则跳过执行并 ack 消息）——覆盖「入队后被取消、派发时内存对象过期」的窗口。

### 5.8 前端 agent 任务取消按钮

- agent 任务详情页（`agent/tasks/[taskId]`）运行列表：状态为 `pending/queued/running` 的行显示「取消」按钮（`data-testid="run-cancel-{runId}"`）。
- 点击 → 二次确认 → PUT `/task-runs/:run_id/cancel` → 刷新列表（状态变 cancelled）。
- 终态（completed/failed/cancelled）行不显示按钮。
- 前端 agent 任务列表 ON/OFF 开关改调 `/tasks/:task_id/enabled`（替代 `/admin/tasks/:id/scheduled-enabled`）；移除 `cancelTask`（调 `/tasks/:task_id/cancel`）的入口。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 task 的 `scheduled_enabled` 与 task_runs 的 `status`） |
| 是否影响现有 API | Yes（删除 task 级 cancel/pause/resume；新增 `/tasks/:id/enabled` 与 `/task-runs/:id/cancel`；chat 无 API 变更） |
| 数据迁移 | 无 schema 变更（`scheduled_enabled` 已存在且默认 true；`TaskDefRepository.Cancel` 删除后旧「已删除」数据自然停止使用） |
| 性能影响 | 极低（每个执行中 run 多一个 2s ticker goroutine，run 结束或终态即回收） |
| 是否需要新增 Skill | No |
| 是否需要改 ADK 底层 | **No**（红线：不修改、不侵入 vendor_adk 的 ctx 处理） |
| 风险 | ① LLM 取消的即时性取决于底层对 ctx 的响应速度——可能延迟到当前 token 生成结束，可接受；② 取消后 partial 内容落库与正常中断一致，前端已展示内容不回滚；③ task 级取消改为开关后，历史前端若仍调 `/tasks/:task_id/cancel` 会 404——需同步前端（本次一并改） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/task/model.go` | `ScheduledEnabled` 注释语义泛化（字段名保留） | Tiny |
| `internal/infra/mongo/task_def_repository.go` | 删除 `TaskDefRepository.Cancel`（DeleteOne）；`TaskRunRepository.Cancel` 条件扩展含 running | Small |
| `internal/repository/` 接口 + mocks | 删 `TaskRepository.Cancel`；`TaskRunRepository.Cancel` 签名不变 | Small |
| `internal/service/task/service.go` | 删 `CancelTask`；`CreateRun` 加 `ScheduledEnabled` 前置检查 | Small |
| `internal/logic/agent/executor.go` | 子 ctx + 轮询监督 goroutine（含完成临界点退出）；runCtx 贯穿全流程 | Medium |
| `internal/worker/pool.go` | dispatch 前 DB 状态复核（未执行取消兜底） | Small |
| `internal/api/handler/task.go` + `routes.go` | 删 cancel/pause/resume；新增 `PATCH /tasks/:id/enabled` + `PUT /task-runs/:id/cancel` | Small |
| `internal/service/chat/chat_service.go` | `streamOnce` ctx.Done 兜底；relevance 段取消短路 | Small |
| `frontend/app/agent/page.tsx` | ON/OFF 改调 `/tasks/:id/enabled`；移除 `cancelTask` | Small |
| `frontend/app/agent/tasks/[taskId]/page.tsx` | 运行列表「取消」按钮 + 二次确认 + 刷新 | Medium |
| `frontend/app/chat/page.tsx` | 「停止」按钮 + AbortController + 状态复位 | Medium |
| `internal/logic/agent/executor_test.go` 等 | 轮询取消（ticker 触发 cancel）/ 完成临界点退出 / ctx 传导单测 | Medium |
| `tests/ui/chat.spec.ts` / `agent*.spec.ts` | E2E：停止按钮中断 / run 取消 / 开关停用后不建 run | Medium |

## 8. 测试策略

1. **Unit tests**（Go）：
   - executor：运行中调用 `CancelRun`（mock 状态置 cancelled）→ 轮询触发 `cancel()` → 底层调用收到 ctx 取消（可注入可取消的 fake runtime 验证 ctx.Done 传导）；**完成临界点**：run 置 completed/failed 后轮询退出、不再 cancel；轮询 goroutine 在 runCtx.Done 后退出（无泄漏，goroutine 计数断言）；
   - 未执行取消：Execute 入口 cancelled 直接 return；dispatch 复核跳过；
   - 开关：`CreateRun` 在 `ScheduledEnabled=false` 时返回错误且不建 run、不入队；`CancelRun` 对 completed/failed/cancelled run 不生效（matched=0）；
   - chat：`streamOnce` 在 ctx 取消时提前退出且不 panic；relevance 段 ctx 取消时短路、`RecordAndShouldRetry` 未被调用；
   - handler：`PATCH /tasks/:id/enabled`、run 级取消路由（404/409/200）+ 归属校验；删除的 cancel/pause/resume 路由 404。
2. **E2E tests**（`tests/ui/`，编号 UI-XXX）：
   - chat 发送中点击「停止」→ SSE 中断、按钮复位、消息区保留已流出内容；
   - agent 任务运行中点击「取消」→ 状态变 cancelled、按钮消失；
   - agent 任务关闭 ON/OFF 开关后手动触发 run → 被拒（不建 run）。
3. **审计**：`.agent/skills/go-ut-audit`。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
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
| L1 | 纯函数/纯结构体 | **100%** | 取消状态判定辅助函数 |
| L2 | 依赖接口，可 mock | **100%** | executor 轮询监督、chat 取消短路、开关前置检查 |
| L3 | 依赖 MongoDB/HTTP | **98%** | handler、pool dispatch |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc` 验证参数传递
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证实际结果

参考:
- `.agent/specs/spec-045-go-service-ut.md`
- `.agent/skills/go-ut-audit/SKILL.md`

## 10. 验证标准

1. chat 发送中点击「停止」：SSE 立即中断（浏览器网络层 fetch aborted）、按钮复位、「已停止生成」提示、已流出内容保留；后端日志无错误栈、relevance 重试计数未增加。
2. 客户端强断（关标签页/断网）与点按钮等价：后端 `streamOnce` 通过 `ctx.Done` 提前退出，不再写 SSE、不再调 LLM。
3. task 关闭开关后：`CreateRun` 返回 409 且不建 run、不入队；调度器不再派发该定时 task；已有 run 不受影响。
4. 删除的 task 级接口 `PUT /tasks/:id/cancel`、`/pause`、`/resume` 返回 404；`PATCH /tasks/:id/enabled` 正常切换且前端 ON/OFF 同步。
5. task run 未执行时取消：worker 派发后立即跳过（Execute 返回 nil，状态保持 cancelled），无 LLM 调用。
6. task run 执行中取消：2s 内（一个轮询周期）底层 LLM 调用收到 ctx 取消并终止；run 最终状态 = cancelled（不被 completed/failed 覆盖）。
7. **完成临界点**：run 已 `completed`/`failed` 时，`PUT /task-runs/:id/cancel` 返回 409 且状态不变；执行中轮询检测到终态即退出、不 cancel。
8. 轮询 goroutine 不泄漏：run 结束后 goroutine 随 `defer cancel()` + `ctx.Done`（或终态）退出。
9. run 级取消 API：他人 run 404；终态 run 409；pending/queued/running 200。
10. `go test ./internal/...` 全绿；覆盖率 ≥98%；E2E 通过；**vendor_adk 零改动**（git diff 校验）。
