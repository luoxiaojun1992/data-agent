# 异步/定时 Agent 任务执行器实现

> **SPEC-063** | Status: 设计中

## 1. 目标

实现异步/定时 agent 任务的真正执行，替换当前 no-op stub `simpleExecutor`。按 RFC 架构：worker 从 Redis Stream 队列消费任务 → 调用 Agent 执行逻辑（复用实时 agent task 的 `Runtime.Run` 执行范式）→ 回写结果到 MongoDB + 通知用户。

**核心原则（晓军 2026-07-22 确认）**：严格按 RFC `docs/RFC-企业数据分析Agent-技术方案.md` §16 及 SPEC-004/009 的架构方案实现，不自行发明架构。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 | ✅ | Agent 核心引擎（定义 Agent Service + Skill 接口 + 三种执行路径） |
| SPEC-008 | ✅ | Skill 实现层（SkillContext 注入 + 4 步执行模式） |
| SPEC-009 | ✅ | 任务队列与调度（Redis Stream + Worker Pool + 状态机 + Scheduler） |
| SPEC-058 | ✅ | logic 编排层（`logic/agent.Orchestrator` 就位，执行器归 logic 层） |
| SPEC-061 | 📐 设计中 | 配置缓存（执行器读模型配置走 cache） |
| SPEC-062 | 📐 设计中 | Runtime 注册表（执行器按 task.ModelID 解析 Runtime）。**阻塞项**：executor 需 Registry 就位才能按模型选 Runtime |

> **阻塞项**：SPEC-062 的 Runtime Registry 必须先实现。executor 调 `registry.GetOrCreate(task.ModelID)` 获取 Runtime 执行。

## 2. 背景

现状（代码核对 + RFC 对照）：

### 2.1 RFC 定义的执行架构

RFC §16（lines 4207-4316）定义 `AgentWorker.processTask`：
1. `loadTask(taskID)` 从 DB 加载任务
2. `updateTaskStatus(taskID, "running")`
3. **回调 Agent Service 执行核心逻辑**（Worker 不持有 Agent Engine，完全复用 Agent Service）
4. 成功 → `saveResult` + `updateStatus("completed")` + `notifyCompletion`
5. 失败 → `updateStatus("failed")` + `notifyFailure`
6. 取消 → `updateStatus("cancelled")`

RFC lines 216-219：
> Worker 是所有异步路径的统一入口：定时任务和异步 Agent 任务都经 Redis Stream → Worker → Agent Service。Worker 是纯 Go goroutine，不涉及外部运行时。

### 2.2 SPEC-009 定义的 Worker 职责

SPEC-009 lines 45-54：
1. `XREADGROUP` 从 Redis Stream 消费
2. 解析 AgentTask → **调用 Agent Service 内部 `Execute()` 方法**
3. **写执行结果到 MongoDB `agent_tasks` 集合**
4. `XACK`

状态机（SPEC-009 line 58-63）：`pending → queued → running → completed / failed → retrying`

### 2.3 当前代码的三处缺陷

| 缺陷 | 文件:行 | 现状 | RFC 要求 |
|------|---------|------|---------|
| **执行器是 no-op** | `main.go:279-287` `simpleExecutor.Execute` | 直接 return nil，不执行任何逻辑 | 回调 Agent Service 执行 Runtime.Run |
| **不从 DB 加载 task** | `pool.go:102-145` `processWorkerMessage` | 从 QueueMessage 内存重建 `task.Task`，不调 `taskService.GetTask` | `loadTask(taskID)` 从 DB 加载完整任务 |
| **不回写结果到 DB** | `pool.go:121-138` | 更新内存 task 的 Status/Result，**不调** `UpdateTaskResult`/`UpdateStatus` | `saveResult` + `updateStatus` 持久化到 MongoDB |
| **无通知** | `pool.go` 全文 | 不调 notifSvc | `notifyCompletion`/`notifyFailure` |

### 2.4 ADK 架构下的"执行同实时 agent task"

RFC 原设计（SPEC-004/008）是外部 skill chain 编排：Agent Service 遍历 `skill_chain []string` 逐个调 `skill.Execute`。

**实际 ADK 实现**（SPEC-048 迁移后）：ADK `llmagent` + `runner` 使用**内部 ReAct loop**，LLM 自主决定调哪些工具。`skill_chain` 字段变为元数据（非外部编排）。执行即 `Runtime.Run(ctx, userID, sessionID, message, runCfg)`——与实时 chat 的 `chat.Service.runAndCollect`（`chat_service.go:232-256`）完全相同。

> **结论**："执行同实时 agent task" = executor 调 `Runtime.Run` 迭代事件流收集结果，复用 chat 路径的执行范式。不构建外部 skill-chain 迭代器。

### 2.5 定时任务的 RFC 两阶段（已由 scheduler 完成）

RFC lines 203-213 定义定时任务两阶段：Scheduler(Cron) → 创建 ScheduledTask → XADD → Worker 转换为 AgentTask → re-enqueue → Worker 执行。

**当前实现**（`scheduler.go:146-161` `executeJob`）：scheduler 已完成"ScheduledTask → Task(type=scheduled)"转换并入队。executor 收到的都是 `Task`（type 为 `agent` 或 `scheduled`），**统一执行**即可，无需 executor 内再做两阶段转换。

## 3. 架构概述

### 模块关系图

```
┌─────────────────────────────────────────────────────────────────────────┐
│                     Redis Stream (agent:task:queue)                      │
└──────────────┬────────────────────────────────────────────────────────────┘
               │ XREADGROUP
               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Worker Pool (internal/worker/pool.go) — 消费层 (修复)                     │
│  processWorkerMessage:                                                    │
│    1. 解析 QueueMessage                                                   │
│    2. taskService.GetTask(qm.TaskID) ← 从 DB 加载完整 task (修复缺陷2)    │
│    3. executor.Execute(ctx, task) ← 调执行器                              │
│    4. 据 err 处理: 成功 ACK / 失败重试/DLQ                                │
│    5. XACK                                                               │
└──────────────┬────────────────────────────────────────────────────────────┘
               │ executor.Execute
               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  AgentExecutor (internal/logic/agent/executor.go) — 执行层 (新增)          │
│  实现 worker.TaskExecutor 接口                                            │
│  注入: Registry + adkSessions + taskService + notifSvc + cbReg            │
│  Execute(ctx, *task.Task) error:                                          │
│    1. taskService.UpdateStatus(running)                                  │
│    2. adkSessions.Create(state: user_id/session_id/role)                 │
│    3. registry.GetOrCreate(task.ModelID) → Runtime (SPEC-062)            │
│    4. runtime.RunAndCollect(ctx, userID, sessionID, message, runCfg)      │
│    5. taskService.UpdateTaskResult(result) + UpdateStatus(completed)      │
│    6. notifSvc.Send(完成通知)  (修复缺陷4)                                 │
│    失败: UpdateStatus(failed) + UpdateError + notifSvc.Send(失败通知)      │
└──────────────┬────────────────────────────────────────────────────────────┘
               │ Runtime.RunAndCollect (复用实时 agent 执行范式)
               ▼
┌─────────────────────────────────────────────────────────────────────────┐
│  Runtime (internal/adk/runtime/runtime.go) — ADK 执行引擎                 │
│  RunAndCollect: 封装 Run 迭代 + 收集最终文本 (从 chat.Service 抽取)        │
│  内部 ReAct loop 自主调工具 (非外部 skill chain 编排)                      │
└─────────────────────────────────────────────────────────────────────────┘
```

### 与现有模式对比

| 维度 | 现状 | SPEC-063 后 |
|------|------|------------|
| 异步执行 | no-op stub（不执行） | AgentExecutor 调 Runtime.Run 真正执行 |
| task 加载 | 内存重建（不读 DB） | `taskService.GetTask` 从 DB 加载完整任务 |
| 结果回写 | 不回写（内存更新丢失） | `UpdateTaskResult` + `UpdateStatus` 持久化 |
| 通知 | 无 | `notifSvc.Send` 完成失败通知 |
| 模型选择 | 无（单例 Runtime） | `registry.GetOrCreate(task.ModelID)` 按 task 绑定模型（SPEC-062） |
| 执行范式 | 无 | 复用 `Runtime.RunAndCollect`（同实时 chat） |

## 4. API 设计

无新增 HTTP API。本 spec 是后端内部执行器实现。

> 任务创建 API（`POST /api/v1/tasks`、`POST /api/v1/agent/tasks`）已存在，本 spec 只实现消费端的执行逻辑。

## 5. 详细设计

### 5.1 Runtime.RunAndCollect 抽取（共享执行逻辑）

从 `chat.Service.runAndCollect`（`chat_service.go:232-256`）抽取为 `adkruntime.Runtime` 的方法，供 chat.Service 和 executor 共用：

`internal/adk/runtime/runtime.go` 新增：

```go
// RunAndCollect 执行一次 ADK turn 并返回最终助手文本。
// 封装 Run 的迭代事件流收集逻辑，供 chat.Service 和异步 executor 共用。
func (rt *Runtime) RunAndCollect(ctx context.Context, userID, sessionID, message string, rc RunConfig) (string, error) {
    var finalText strings.Builder
    for evt, err := range rt.Run(ctx, userID, sessionID, message, rc); {
        if err != nil { return finalText.String(), err }
        if evt == nil || evt.Content == nil || !evt.IsFinalResponse() { continue }
        for _, p := range evt.Content.Parts {
            if p != nil && p.Text != "" { finalText.WriteString(p.Text) }
        }
    }
    return finalText.String(), nil
}
```

`chat.Service.runAndCollect` 改为调 `rt.RunAndCollect(...)`（消除重复）。

### 5.2 AgentExecutor（执行层）

新增 `internal/logic/agent/executor.go`，实现 `worker.TaskExecutor` 接口：

```go
package agent

// AgentExecutor 实现 worker.TaskExecutor，按 RFC §16 执行异步 agent 任务。
// 复用 Runtime.RunAndCollect（同实时 agent 执行范式），不构建外部 skill-chain 迭代器。
type AgentExecutor struct {
    registry    *adkruntime.Registry   // SPEC-062: 按 task.ModelID 解析 Runtime
    adkSessions session.Service         // 创建 ADK session + 注入 identity state
    tasks       task.TaskService        // 加载/回写 task（修复缺陷2/3）
    notif       notifsvc.Service        // 完成失败通知（修复缺陷4）
    cbReg       *security.CircuitBreakerRegistry
}

func NewAgentExecutor(registry *adkruntime.Registry, adkSessions session.Service,
    tasks task.TaskService, notif notifsvc.Service, cbReg *security.CircuitBreakerRegistry) *AgentExecutor {
    return &AgentExecutor{registry: registry, adkSessions: adkSessions, tasks: tasks, notif: notif, cbReg: cbReg}
}

// Execute 实现 worker.TaskExecutor。RFC processTask 映射。
func (e *AgentExecutor) Execute(ctx context.Context, t *task.Task) error {
    // 1. 更新状态为 running
    _ = e.tasks.UpdateStatus(t.ID, task.StatusRunning)

    // 2. 构建 ADK session + identity state（同 chat.Service.prepareRun）
    userID := t.UserID
    sessionID := t.SessionID
    state := map[string]any{
        "user_id":    userID,
        "session_id": sessionID,
        "task_id":    t.ID,
    }
    if kbID, ok := t.Params["kb_id"].(string); ok { state["kb_id"] = kbID }
    if _, cerr := e.adkSessions.Create(ctx, &session.CreateRequest{
        AppName: e.registry.AppName(), UserID: userID, SessionID: sessionID, State: state,
    }); cerr != nil {
        e.failTask(t, fmt.Errorf("adk session init: %w", cerr))
        return cerr
    }

    // 3. 解析 Runtime（按 task.ModelID，SPEC-062）
    rt, rErr := e.registry.GetOrCreate(ctx, t.ModelID)
    if rErr != nil {
        e.failTask(t, fmt.Errorf("resolve runtime: %w", rErr))
        return rErr
    }

    // 4. 派生 user message（从 Task.Params）
    message := deriveUserMessage(t)

    // 5. 执行（复用实时 agent 范式，熔断器保护）
    runCfg := adkruntime.RunConfig{StateDelta: state}
    var content string
    cb := e.cbReg.GetOrCreate("agent")
    execErr := cb.Call(func() error {
        text, err := rt.RunAndCollect(ctx, userID, sessionID, message, runCfg)
        content = text
        return err
    })

    // 6. 结果回写 + 通知
    if execErr != nil {
        e.failTask(t, execErr)
        return execErr
    }
    e.completeTask(t, content)
    return nil
}

// completeTask 回写成功结果 + 状态 + 通知。
func (e *AgentExecutor) completeTask(t *task.Task, content string) {
    result := map[string]interface{}{"content": content, "status": "success"}
    _ = e.tasks.UpdateTaskResult(t.ID, result)
    _ = e.tasks.UpdateStatus(t.ID, task.StatusCompleted)
    e.notif.Send("任务完成", fmt.Sprintf("任务 %q 已完成", t.ID), "task", []string{t.UserID})
}

// failTask 回写失败状态 + 错误 + 通知。
func (e *AgentExecutor) failTask(t *task.Task, err error) {
    _ = e.tasks.UpdateStatus(t.ID, task.StatusFailed)
    // Task.Error 字段回写（若 repo 支持 UpdateError，否则写入 Result）
    e.notif.Send("任务失败", fmt.Sprintf("任务 %q 失败: %v", t.ID, err), "task", []string{t.UserID})
}
```

### 5.3 派生 user message

`Task.Params` 是 `map[string]interface{}`，user message 从中派生：

```go
// deriveUserMessage 从 Task.Params 提取用户输入。
func deriveUserMessage(t *task.Task) string {
    // 优先级：query > message > prompt > description > title
    for _, key := range []string{"query", "message", "prompt", "description"} {
        if v, ok := t.Params[key].(string); ok && v != "" { return v }
    }
    return t.Params["title"].(string) // 兜底
}
```

> Orchestrator `CreateAgentTask` 当前丢弃 `req.Messages`（`orchestrator.go:53-83` 不传 messages 到 task）。SPEC-062 修复 orchestrator 传 modelId 时，应同时将 `req.Messages`/`req.Title` 存入 `Task.Params` 供 executor 派生 message。本 spec 的 `deriveUserMessage` 按约定 key 提取。

### 5.4 Worker Pool 修复

`internal/worker/pool.go` `processWorkerMessage`（当前 lines 102-145）改造：

**修复缺陷 2（从 DB 加载 task）：**

```go
func (p *Pool) processWorkerMessage(ctx context.Context, msg redis.XMessage) {
    data, ok := msg.Values["data"]
    if !ok { return }
    var qm task.QueueMessage
    if err := json.Unmarshal([]byte(data.(string)), &qm); err != nil {
        log.Printf("Failed to parse queue message: %v", err)
        return
    }
    // 修复：从 DB 加载完整 task（而非内存重建）
    t, err := p.taskSvc.GetTask(qm.TaskID)   // ← 新增：注入 taskService
    if err != nil || t == nil {
        log.Printf("Failed to load task %s: %v", qm.TaskID, err)
        _ = p.queue.Ack(ctx, msg.ID)
        return
    }
    start := time.Now()
    execErr := p.executor.Execute(ctx, t)    // 执行（回写在 executor 内完成）
    duration := time.Since(start).Milliseconds()
    t.DurationMs = duration
    now := time.Now()
    t.CompletedAt = &now
    // 重试/DLQ 逻辑（现有保留）
    if execErr != nil {
        t.RetryCount++
        if t.RetryCount >= t.MaxRetries {
            _ = p.queue.MoveToDLQ(ctx, msg.ID, []byte(data.(string)))
        }
        // 失败状态已由 executor.failTask 回写
    }
    _ = p.queue.Ack(ctx, msg.ID)
}
```

**变更点：**
1. `Pool` 新增 `taskSvc` 字段（注入 `task.TaskService`），用于 `GetTask`
2. 从 DB 加载完整 task（修复缺陷 2）
3. **移除** pool 内的内存状态更新（`t.Status = StatusCompleted` 等），改为由 executor 内部回写（修复缺陷 3）
4. 保留重试/DLQ 逻辑

### 5.5 定时任务统一执行

`scheduler.executeJob`（`scheduler.go:146-161`）已创建 `Task(type=scheduled)` 入队。executor 收到 scheduled 类型 task 时，**执行逻辑与 agent 类型完全相同**（统一调 Runtime.RunAndCollect）。无需 executor 内做两阶段转换——RFC 的"ScheduledTask → AgentTask 转换"已由 scheduler 完成。

> 定时任务的 `userID="system"`（`scheduler.go:155`），executor 内 `notif.Send` 对 system user 可跳过通知（或记日志）。

### 5.6 注入与布线

`cmd/server/wire.go` `initTaskQueue`（当前 lines 227-267）改造：

```go
func initTaskQueue(...) {
    // ... Redis + Stream 初始化（现有）...
    
    // 替换 simpleExecutor 为 AgentExecutor（依赖 Registry 就位，SPEC-062）
    executor := agentlogic.NewAgentExecutor(
        deps.registry,           // SPEC-062 Registry
        deps.adkSessions,
        deps.taskService,
        deps.notifSvc,            // 新增注入（现有 notifSvc 在 wire.go:223 创建）
        deps.cbRegistry,
    )
    
    workerPool := worker.NewPool(taskStream, redisClient.Client(), 4, executor, deps.taskService)
    // ... worker start（现有）...
}
```

`worker.NewPool` 签名扩展：新增 `taskSvc task.TaskService` 参数（用于 `processWorkerMessage` 内 `GetTask`）。

移除 `main.go:279-287` 的 `simpleExecutor` stub。

> **布线顺序依赖**：`deps.registry`（SPEC-062）必须在 `initTaskQueue` 前就位。当前 `initServices`（含 Registry 构造）在 `initTaskQueue` 前调用（`main.go` 启动顺序），满足。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `tasks` 集合，Task 已有 Result/Status 字段） |
| 是否影响现有 API | No（内部执行器实现，无新 HTTP 端点） |
| 性能影响 | 正向：异步任务真正执行（原 stub 不执行）。并发 4 worker，每个任务一次 Runtime.Run |
| 是否需要新增 Skill | No |
| ADK 执行可行性 | Yes：复用 Runtime.RunAndCollect（从 chat.Service 抽取），ADK ReAct loop 自主调工具 |
| 模型选择 | 依赖 SPEC-062 Registry（task.ModelID → GetOrCreate → Runtime） |
| 通知可行性 | Yes：notifSvc 已存在（`wire.go:223`），当前未接入 worker，本 spec 接入 |
| 熔断器 | Yes：cbReg 已存在，executor 用 `GetOrCreate("agent")` 保护 |
| 定时任务 | scheduler 已转 Task 入队，executor 统一执行，无需两阶段 |
| 任务消息派生 | Task.Params 约定 key（query/message/prompt），orchestrator 需存入（SPEC-062 配合） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/runtime/runtime.go` | 新增 `RunAndCollect` 方法（从 chat.Service 抽取） | Small |
| `internal/adk/runtime/runtime_test.go` | `RunAndCollect` UT | Small |
| `internal/logic/agent/executor.go` | **新增** AgentExecutor（实现 worker.TaskExecutor） | New |
| `internal/logic/agent/executor_test.go` | **新增** executor UT | New |
| `internal/worker/pool.go` | `processWorkerMessage` 改 DB 加载 + 移除内存回写；`NewPool` 加 taskSvc 参数 | Medium |
| `internal/worker/pool_test.go` | 适配 | Medium |
| `cmd/server/wire.go` | `initTaskQueue` 注入 AgentExecutor + notifSvc + taskSvc 到 pool | Small |
| `cmd/server/main.go` | 删除 `simpleExecutor` stub（lines 279-287） | Small |
| `internal/logic/agent/orchestrator.go` | `CreateAgentTask` 存 messages/title 到 Task.Params（配合 deriveUserMessage） | Small |
| 各 mock（`worker/mocks`、`logic/agent/mocks`） | 重新生成 | Auto |

## 8. 测试策略

1. **Unit tests（Go）**：覆盖率基线见 SPEC-045。L1 100%，L3 98%。CI: `ut-workflow.yml`
   - `logic/agent/executor_test.go`（L2，mock Registry/adkSessions/taskService/notifSvc）：
     - 成功路径：UpdateStatus(running) → adkSessions.Create → registry.GetOrCreate → RunAndCollect → UpdateTaskResult + UpdateStatus(completed) + notif.Send
     - 失败路径：Runtime 错误 → UpdateStatus(failed) + notif.Send(失败)
     - ADK session 创建失败 → failTask
     - Runtime 解析失败（模型不存在）→ failTask
     - 熔断器触发 → 返回错误
     - deriveUserMessage 各 key 优先级
   - `worker/pool_test.go`（L2，mock executor + taskService + queue）：
     - GetTask 成功 → executor.Execute → Ack
     - GetTask 失败（task 不存在）→ Ack（丢弃）
     - Execute 失败 → 重试计数 / DLQ
   - `runtime/runtime_test.go`：RunAndCollect 事件流收集
2. **Integration tests**：Docker Compose（Redis+Mongo），验证端到端：入队 → worker 消费 → 执行 → DB 结果可见 + 通知
3. **E2E tests**：异步任务执行后前端可见状态变更（若前端有任务列表轮询）
4. **审计**：`.agent/skills/go-ut-audit`

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 若前端任务列表有异步状态展示，编写 E2E 验证任务从 queued → running → completed 流转
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试
- [ ] **后端为主**：本 spec 主要是后端执行器实现，E2E 聚焦任务状态流转可见性；核心逻辑用 Go UT + 集成测试覆盖

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体 | **100%** | `deriveUserMessage` |
| L2 | 依赖接口，可 mock | **100%** | `AgentExecutor`、`Pool.processWorkerMessage` |
| L3 | 依赖 MongoDB/Redis | **98%** | （本 spec 无直接 L3，Runtime 已有测试） |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少 2 个行为验证断言（如 executor 成功测试验证 UpdateTaskResult 被调 + notif.Send 被调）
- [ ] **必须** 验证 executor 内 UpdateStatus(running) 先于 UpdateStatus(completed) 调用（mock 期望顺序）
- [ ] **必须** 失败路径验证 UpdateStatus(failed) + notif.Send(失败通知) 均被调用
- [ ] **必须** pool 测试验证 GetTask 被调（修复缺陷2）+ Ack 被调
- [ ] **必须** RunAndCollect 测试验证多事件流收集最终文本
- [ ] **严禁** `t.Skip()` 绕过
- [ ] **严禁** Success 测试只验证 `err == nil`

### 测试模式

- Executor：mock Registry（返回 fake Runtime）、mock adkSessions、mock taskService（验证调用顺序与参数）、mock notifSvc
- Pool：mock executor（返回 nil/error）、mock taskService（GetTask 返回/nil）、mock queue（Ack/MoveToDLQ）
- RunAndCollect：用 fake model.LLM 返回预设事件流

### CI 门禁

- [ ] `go test -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考:
- `.agent/specs/spec-045-go-service-ut.md`
- `.agent/skills/go-ut-audit/SKILL.md`
- `.github/workflows/ut-workflow.yml`

## 10. 验证标准

### 功能验证

1. **异步任务执行**：`POST /api/v1/tasks` 创建异步任务 → worker 消费 → Runtime.Run 执行 → Task 状态 `queued → running → completed`
2. **结果回写**：任务完成后 `GET /api/v1/tasks/:id` 返回 `result.content` 非空（修复缺陷3）
3. **DB 加载**：worker 从 DB 加载完整 task（非内存重建）—— 修改 task 后异步执行用最新数据（修复缺陷2）
4. **通知**：任务完成/失败后 `notifSvc.Send` 被调，用户通知列表可见（修复缺陷4）
5. **模型选择**：异步任务带 `model_id` → executor 用 `registry.GetOrCreate(task.ModelID)` 选对应模型 Runtime（SPEC-062 联动）
6. **定时任务执行**：scheduler 触发 → Task(type=scheduled) 入队 → worker 消费 → 执行（同 agent 类型）
7. **失败处理**：Runtime 执行失败 → Task 状态 `failed` + 通知 + 重试/DLQ
8. **熔断器**：连续失败触发熔断 → 快速返回错误不堆积请求
9. **执行同实时**：异步执行的 Runtime.Run 逻辑与实时 chat 的 runAndCollect 一致（RunAndCollect 共用）

### 反向验证

- [ ] `simpleExecutor` stub 已删除（grep 无 `simpleExecutor`）
- [ ] worker pool 不再内存重建 task（调 `taskService.GetTask`）
- [ ] executor 内完成 DB 回写（pool 不做内存状态更新）
- [ ] 异步任务真正执行（非 return nil）
