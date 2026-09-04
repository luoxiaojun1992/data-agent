# Task 相关 skills（task_create / task_run_list / task_run_detail）

> **SPEC-087** | Status: 设计中

## 1. 目标

新增 3 个 ADK function tool，让 Agent（LLM）能自主**创建 task、查看某 task 的运行列表、查看某次运行的详细结果**，支撑「任务委派 + 结果回收」场景（如 SPEC-086 日常总结模版执行后，可创建子任务并轮询其结果）。全部复用现有 `task.Service` 能力，无后端 API 改动。

三个 skill：

1. **`task_create`**：创建 task 定义（复用 `TaskService.CreateTask`）。
2. **`task_run_list`**：按 `task_id` 列出 topN 次运行，仅返回 `run_id` 与是否完成。
3. **`task_run_detail`**：按 `run_id` 返回该次运行的详细结果与是否完成。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-063 异步/定时任务执行器 | ✅ | 复用 `TaskService.CreateTask` / `TaskRunService.ListRuns` / `GetRun` |
| SPEC-064 RBAC | ✅ | 复用 session state 注入的 `user_id` 做租户隔离 |
| SPEC-086 task 常用模版 | 📐 | 与 SPEC-086 的 `memory_list`/`kb_create_doc` 同属 tool 层扩展，无依赖冲突，可独立开发 |

> 无阻塞项，可立即开发。

## 2. 背景

现状痛点：Agent 在对话/任务上下文中**无法自主创建或查询 task**——现有 task API（`POST /api/v1/tasks`、`GET /tasks/:id/runs`、`GET /tasks/:id/runs/:run_id`）只面向 HTTP handler，LLM 没有对应的 function tool。这导致「让 Agent 派生一个子任务、再等它跑完取结果」这类编排无法实现（只有 `save_task_result` 能回写当前 run，但不能创建/查询其他 run）。

## 3. 架构概述

```
LLM（Runtime.RunAndCollect，工具集 = 全部注册 tool）
        │ 调用 task_create / task_run_list / task_run_detail
        ▼
┌─ ADK tool 层（internal/adk/tools/tools.go）──────────────┐
│ + task_create     → TaskService.CreateTask              │
│ + task_run_list   → TaskService.GetTask(归属校验)        │
│                     + TaskRunService.ListRuns            │
│ + task_run_detail → TaskRunService.GetRun(归属校验)      │
└───────────────────────────────────────────────────────────┘
        │                        │
        ▼                        ▼
   task.Service            task.Service
   (TaskService)           (TaskRunService)
        │                        │
        ▼                        ▼
  agent_task_defs        agent_task_runs
```

> `task.Service` 同时实现 `TaskService` 与 `TaskRunService`（`var _ TaskService = (*Service)(nil)` / `var _ TaskRunService = (*Service)(nil)`），wire 注入的是同一个 `*task.Service` 实例。

## 4. Skill 接口设计

### 4.1 `task_create` — 创建 task

| 字段 | 类型 | 说明 |
|------|------|------|
| `title` | string | 任务标题（必传，非空） |
| `type` | string | 任务类型：`agent_exec`（默认）/ `scheduled_exec` |
| `skill_chain` | []string | 技能链（可选，元数据） |
| `params` | map[string]any | 任务参数（可选；LLM 的指令写在 `params.message`，由 `deriveUserMessageFromParams` 提取为任务指令） |
| `cron_expr` | string | cron 表达式（仅 `type=scheduled_exec` 时生效；非空 → recurring 调度） |
| `model_id` | string | 模型 ID（可选，空 → 后端 `GetModelByID("")` 回落默认模型） |

返回：

```json
{"task_id": "task_…", "run_id": "run_…", "title": "…"}
```

- `run_id` 仅在非 scheduled 任务时有值（`CreateTask` 对 scheduled 任务不创建首个 run，返回 `run=nil`）。
- `schedule_mode` 由 tool 层按 handler 同款逻辑推导：`type==scheduled_exec && cron_expr!=""` → `recurring`；否则空（走实时路径）。
- `user_id` 从 session state 注入（LLM 不可指定），见 §5.3。

### 4.2 `task_run_list` — 列出运行

| 字段 | 类型 | 说明 |
|------|------|------|
| `task_id` | string | 任务 ID（**必传**） |
| `top_n` | int | 返回条数（默认 10，上限 50） |

返回：

```json
{
  "runs": [
    {"run_id": "run_…", "completed": true},
    {"run_id": "run_…", "completed": false}
  ],
  "count": 2
}
```

- **只返回 `run_id` 与 `completed`**（不返回 result/error 等详情）。
- 排序：`created_at` 倒序（最新在前，复用 `TaskRunRepository.List` 既有行为）。
- `status` 参数固定传空 → 全状态（不做状态过滤）。
- `completed = (status == "completed")`，见 §5.1。

### 4.3 `task_run_detail` — 查看运行详情

| 字段 | 类型 | 说明 |
|------|------|------|
| `run_id` | string | 运行 ID（**必传**） |

返回：

```json
{
  "run_id": "run_…",
  "task_id": "task_…",
  "status": "completed",
  "completed": true,
  "result": {…},
  "error": ""
}
```

- `result` 为该 run 的详细结果（`save_task_result` 写回的 map）。
- `error` 非空表示失败原因（`UpdateRunError` 写入）。
- `completed = (status == "completed")`。

## 5. 详细设计

### 5.1 「是否完成」语义（⚠️ 关键，须与实现一致）

`TaskRun.Status` 可能值为：`pending / queued / running / completed / failed / retrying / cancelled`。

**本 spec 定义 `completed` 布尔 = `status == "completed"`（成功完成才为 true）**：

| status | completed | 说明 |
|--------|:---:|------|
| completed | ✅ true | 成功完成 |
| failed / cancelled / running / pending / queued / retrying | ❌ false | 未（成功）完成 |

- `failed` 的 run 其 `completed=false`；LLM 若要区分失败原因，用 `task_run_detail` 查看 `error` 与 `status` 字段。
- 实现时在 tool 层加辅助函数 `isCompleted(status)`，`task_run_list` 与 `task_run_detail` 共用，避免两处判断漂移。

### 5.2 Deps 变更

`internal/adk/tools/tools.go` 的 `Deps` 新增一个字段：

```go
// TaskDefs backs the task_create tool and task_run_list ownership check.
// If nil, task_create is not registered.
TaskDefs domaintask.TaskService
```

- 现有 `Tasks domaintask.TaskRunService`（用于 `save_task_result` / `task_run_list` / `task_run_detail`）。
- `TaskDefs` 与 `Tasks` 在 wire 中注入**同一个 `*task.Service` 实例**（见 §5.4）。

### 5.3 租户隔离与归属校验（红线，防 IDOR）

`TaskRunRepository.List` 的 filter 仅 `{task_id: taskID}`、`Get` 仅按 `run_id`，**均无 `user_id` 过滤**。若 LLM 可任意传 `task_id`/`run_id`，将越权读取他人 task。因此三个 tool 必须做归属校验：

| tool | 校验 |
|------|------|
| `task_create` | `user_id` 直接取 session state 的 `user_id`（LLM 无 user_id 参数，天然隔离） |
| `task_run_list` | 先 `TaskDefs.GetTask(task_id)`，校验 `task.UserID == stateUserID`，不匹配则报错拒绝 |
| `task_run_detail` | `GetRun(run_id)` 得 run，校验 `run.UserID == stateUserID`，不匹配则报错拒绝 |

- **不引入 system_admin 豁免**：LLM tool 只操作/查询**本人**资源（最小权限），管理他人资源走 admin UI，不暴露给 LLM。与 `save_task_result`（`run_id` 从 state 注入、LLM 不能操作任意 run）的设计哲学一致。
- `stateUserID = stateString(tc, "user_id")`；为空时拒绝（与 `save_task_result` 的「无 task 上下文」报错同款）。

### 5.4 wire.go 注入

`cmd/server/wire.go` 的 `toolDeps` 与 `adktools.Names` 两处 `Deps{}` 补一行：

```go
TaskDefs: deps.taskService,
```

> `deps.taskService` 是 `*task.Service`（`initTaskService` 创建），同时满足 `TaskService` 与 `TaskRunService`。`TaskDefs` 赋值必须在 `toolDeps` 构建（消费点）之前（沿用既有「赋值在消费之前」的铁律）。

### 5.5 tool 注册条件

在 `specs(deps)` 中：

- `deps.TaskDefs != nil` → 注册 `task_create`。
- `deps.Tasks != nil` → 注册 `task_run_list`、`task_run_detail`（与 `save_task_result` 同组，可一并放 `if deps.Tasks != nil` 分支）。

### 5.6 skill 注册三步同步 + 原始 seed 数据同步（红线）

与 SPEC-086 §5.6/§5.7 完全一致，新增 3 个 skill 需三处同步：

1. `internal/adk/tools/tools.go` `specs()`：注册 function tool。
2. `internal/service/skill/config.go` `predefinedSkills()`：补 3 个 SkillConfig（name/display_name/description）。
3. `SeedSkills` 幂等自动补齐（启动时）。

**原始 seed 数据同步**：`predefinedSkills()` 是 skill 的原始 seed 数据，3 个新 skill 必须补进其中，保证**全新部署（空 DB）自动插入、存量部署增量补齐，重新部署不遗漏**。语义红线：`SeedSkills` 是「存在即跳过、不覆盖」，新 skill 名字全新必 seed。

### 5.7 task_create 参数到 CreateTask 的映射

```go
// tool 层伪代码
scheduleMode := ""
if args.Type == domaintask.TaskTypeScheduledExec && strings.TrimSpace(args.CronExpr) != "" {
    scheduleMode = domaintask.ScheduleModeRecurring
}
t, run, err := deps.TaskDefs.CreateTask(
    stateUserID, args.Type, args.SkillChain, args.Params,
    args.ModelID, scheduleMode, args.CronExpr, nil /* scheduledAt */)
```

- `params` 为 nil 时初始化空 map（与 handler 一致）。
- 建议 tool 层把 `title` 也写入 `params["title"]`（与 handler 行为对齐，`NewTask` 已从 `params["title"]` 取标题）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `agent_task_defs` / `agent_task_runs`） |
| 是否影响现有 API | No（复用 `task.Service`，无 handler 改动；`Deps` 仅新增一个字段） |
| 性能影响 | 低（`task_run_list` 单次 topN≤50；`agent_task_runs` 建议加 `{task_id, created_at}` 复合索引，量小可后置） |
| 是否需要新增 Skill | Yes（`task_create` / `task_run_list` / `task_run_detail` 三个 function tool + 三个 skill 配置） |
| 是否引入新依赖 | No（全复用现有组件） |
| License | 无新增第三方库 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/tools/tools.go` | 新增 3 个 tool + `Deps.TaskDefs` 字段 + 注册 | Medium |
| `internal/service/skill/config.go` | `predefinedSkills()` 补 3 个 skill（seed 数据） | Small |
| `cmd/server/wire.go` | `Deps{}` 两处注入 `TaskDefs` | Small |
| `internal/adk/tools/tools_test.go`（或新文件） | 3 个 tool 的 UT（mock TaskService/TaskRunService） | Medium |

## 8. 测试策略

1. **Unit tests（Go）**：见 §9.5 底线。重点：
   - `task_create`：参数校验（title 必填）、schedule_mode 推导、`CreateTask` 收到的 userID/type/skillChain/params/modelID/cronExpr 正确、返回 task_id/run_id。
   - `task_run_list`：task_id 必填、top_n 默认/上限、归属校验（`task.UserID != stateUserID` → 拒绝）、`completed` 计算、返回仅含 run_id+completed。
   - `task_run_detail`：run_id 必填、归属校验（`run.UserID != stateUserID` → 拒绝）、返回 result/error/status/completed。
   - nil 依赖降级不 panic（`TaskDefs`/`Tasks` 为 nil 时报清晰错误）。
2. **Integration tests**：条件使用 Docker Compose（`go test -tags=integration`）。
3. **E2E tests**：无独立前端 UI，不新增 E2E 用例（LLM 侧通过任务执行间接覆盖）。
4. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量。

## 9. UI Test / E2E 验收规则

> 本 spec 无前端 UI 变更（纯后端 tool 层），不新增 E2E 用例；但仍须保证 CI（sonar-check + ui-tests）通过、UT（ut-workflow）通过。

- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体 | **100%** | `isCompleted` |
| L2 | 依赖接口，可 mock | **100%** | tool 层（注入 mock TaskService/TaskRunService） |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `task.Service`（本 spec 不改动，维持既有覆盖） |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（验证实际值/状态/副作用，非仅 `err == nil`）
- [ ] **必须** `task_create` 测试验证 `CreateTask` 收到的 userID/type/skillChain/params/cronExpr 具体值
- [ ] **必须** `task_run_list` 测试验证归属校验拒绝路径 + `completed` 布尔计算正确
- [ ] **必须** `task_run_detail` 测试验证返回的 result/error/status 与 mock 一致
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil`

### 测试模式

- tool 层：注入 mock 的 `TaskService` / `TaskRunService`（mockery 生成，已有 `internal/service/task/mocks` / `internal/domain/task/mocks`）

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%
- [ ] `go vet` 无警告

参考: `.agent/specs/spec-045-go-service-ut.md`、`.agent/skills/go-ut-audit/SKILL.md`

## 10. 验证标准

1. **task_create**：LLM 传入 title/type/skill_chain/params/cron_expr → 成功创建 task，返回 task_id；scheduled_exec + cron 时 schedule_mode=recurring 且 run_id 为空；agent_exec 时 run_id 非空。user_id 恒等于 session user_id（LLM 无法指定他人）。
2. **task_run_list**：`task_id` 必传（空则报错）；返回 `[{run_id, completed}]` 且无多余字段；top_n 默认 10、上限 50；归属不符 → 拒绝。
3. **task_run_detail**：`run_id` 必传；返回 result/error/status/completed；归属不符 → 拒绝。
4. **seed 同步**：全新空 DB 启动后 `SeedSkills` 自动补齐 3 个 skill（`/skills` 可见、`skill_search` 可搜到）；存量 DB 增量部署后同样补齐，不遗漏。
5. **回归**：`save_task_result`、task HTTP API（创建/列表/详情）行为不变。
