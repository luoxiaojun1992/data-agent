# Task 常用模版（日常总结）+ memory 分页读取 + KB 文档创建 skill

> **SPEC-086** | Status: 设计中

## 1. 目标

在 task 创建中增加「常用模版」快捷入口（先只做「日常总结」一个模版），与现有的人工创建弹窗分开。日常总结模版的功能是**每天凌晨 1 点自动总结当天的 memory 到知识库**，最终复用现有 task API（`POST /api/v1/tasks`，`scheduled_exec`），只是前端多了个一键快捷入口。

为支撑该模版，需同时补齐两个后端 skill：

1. **`kb_create_doc`**：创建纯文本知识库 doc（限制文本长度，异步索引，不等索引完，只返回是否创建成功）。
2. **`memory_list`**：扩展 memory 读取能力，按创建时间倒序分页读取记忆（默认返回最新 n 条，兼容现有默认行为）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 知识库系统 | ✅ | 复用 `knowledge.Service` 的 doc/chunk 方法 |
| SPEC-050 adk-go-memory 迁移 | ✅ | 当前 memory 后端为 `memoryx.MongoStorage`（`memories` collection） |
| SPEC-063 异步/定时任务执行器 | ✅ | 复用 `scheduled_exec` + cron + AgentExecutor |
| SPEC-068 KB PII 脱敏 | ✅ | `kb_create_doc` 复用 `RedactText` |
| SPEC-081 KB URL 导入（长度限制） | 📐 | 复用其定义的 `MaxKBTextBytes = 5MB`（本 spec 沿用，不依赖其实现） |
| SPEC-064 RBAC | ✅ | 复用 `RequirePermission` 及 session state 注入的 `user_id` |

> 无阻塞项，可立即开发。

## 2. 背景

现状痛点：

1. 创建定时任务需要手动填 cron、描述、模型等，用户「每天总结 memory 到知识库」这种高频固定需求没有快捷入口。
2. memory 目前只有 `memory_search`（语义相似度搜索，topK=5），**没有「按时间倒序分页读取」能力**，无法支撑「读取当天记忆」的场景。
3. 知识库文档创建目前走 `UploadDoc` handler（文件上传），没有「给定纯文本直接建 doc」的复用入口，Agent 无法把总结文本写入 KB。

## 3. 架构概述

### 3.1 数据流（日常总结模版）

```
用户点击「日常总结」模版
        │
        ▼
前端调用 POST /api/v1/tasks（复用，type=scheduled_exec + cron="0 1 * * *"）
        │
        ▼
scheduler 每天 01:00 触发 → AgentExecutor.Execute(run)
        │
        ▼
LLM 执行（Runtime.RunAndCollect，工具集 = 全部注册 tool）：
  ① 调 memory_list（offset=0 → offset=n → ...）按 created_at 倒序翻页，
     直到翻到某页出现「非当天」的 created_at 即停
  ② 归纳总结当天记忆为 markdown 文本
  ③ 调 kb_create_doc 创建 KB doc（title=「YYYY-MM-DD 日常总结」）
  ④ 调 save_task_result 保存结果
```

### 3.2 模块关系

```
┌─ 前端 ──────────────────────────────────────────────┐
│ app/agent/page.tsx                                  │
│   + 常用模版入口（独立于「新建任务」弹窗）            │
│   + 日常总结确认弹窗 → POST /api/v1/tasks            │
└─────────────────────────────────────────────────────┘
        │ (复用，无后端 API 改动)
        ▼
┌─ 后端 tool 层（internal/adk/tools/tools.go）────────┐
│ + memory_list      (新) → 读 memoryx.MongoStorage   │
│ + kb_create_doc    (新) → 读 knowledge.Service      │
└─────────────────────────────────────────────────────┘
        │                    │
        ▼                    ▼
┌─ memoryx.MongoStorage ─┐  ┌─ knowledge.Service ─────┐
│ + ListRecent(created_at│  │ + CreateTextDoc(封装:    │
│   倒序 + skip/limit)    │  │   脱敏→GridFS→CreateDoc  │
│   (新方法)             │  │   →异步入队索引)         │
└────────────────────────┘  └─────────────────────────┘
```

## 4. API / Skill 接口设计

### 4.1 新增 ADK function tool：`memory_list`

| 字段 | 类型 | 说明 |
|------|------|------|
| `limit` | int | 每页条数，默认 5（兼容 memory_search 默认），最大 50 |
| `offset` | int | 偏移量（分页游标），默认 0 |

返回：

```json
{
  "memories": [
    {"id": "…", "content": "…", "created_at": "2026-09-04T…"}
  ],
  "count": 3,
  "has_more": true
}
```

- 排序：**`created_at` 倒序**（最新在前）。
- `created_at` 必须回传，LLM 用它判断「是否当天」来决定何时停止翻页。
- `has_more`：是否还有更早的记忆（`offset+count < total`）。

### 4.2 新增 ADK function tool：`kb_create_doc`

| 字段 | 类型 | 说明 |
|------|------|------|
| `title` | string | 文档标题（必填） |
| `content` | string | 纯文本内容（必填，≤ `MaxKBTextBytes`） |

返回：

```json
{"doc_id": "kbdoc_…", "status": "created"}
```

- 只返回是否创建成功（不等索引完），索引走现有 `kb_index` 队列异步完成。
- 超限：`content` 字节数 > `MaxKBTextBytes` → 返回错误「文本超过 5MB 上限」。

### 4.3 复用 task API（无改动）

`POST /api/v1/tasks`，模版预置参数见 §5.3。

## 5. 详细设计

### 5.1 memory 创建时间字段调查结论（⚠️ 关键）

当前 memory 后端为 `internal/adk/memoryx/mongo_storage.go` 的 `MongoStorage`（`memories` collection）：

```go
type mongoDoc struct {
    ...
    CreatedAt time.Time `bson:"created_at"`   // ← 创建时间，本 spec 分页排序字段
    UpdatedAt time.Time `bson:"updated_at"`   // ← 更新时间（合并/upsert 时刷新）
}
```

**结论与坑：**

| 现有方法 | 排序字段 | 能否复用 |
|---------|---------|:---:|
| `List`（memory handler `/memory/list` 在用） | **`updated_at` 倒序** | ❌ 不能用——记忆被相似度合并（upsert）时 `updated_at` 会刷新，导致「当天记忆」排序错乱 |
| `QueryRecent` | `created_at` 倒序 | ⚠️ 只有 `limit`，无 `offset`/`skip` 分页 |
| `Search` | `created_at` 倒序 + 时间衰减 | ❌ 语义搜索，非分页读取 |

因此**必须新增** `ListRecent(ctx, userID, limit, offset)` 方法，明确用 **`created_at` 倒序** + `skip/limit`。**严禁**复用 `List`（其 `updated_at` 排序是坑）。

### 5.2 memory 分页扩展

**① `memoryx.Storage` 接口新增：**

```go
ListRecent(ctx context.Context, userID string, limit, offset int) ([]adapter.Observation, int64, error)
```

**② `MongoStorage` 实现：**

- filter：`{app_name: s.appName, user_id: userID}`（userID 为空则全量，与现有 `List` 对齐；实际由 session state 注入的 `user_id` 保证租户隔离）
- sort：`created_at` 倒序
- `skip(offset)` + `limit(limit)`
- 返回 total（用于 `has_more` 计算）

**③ `tools.Deps` 新增字段：**

```go
MemoryLister interface {
    ListRecent(ctx context.Context, userID string, limit, offset int) ([]adapter.Observation, int64, error)
}
```

`wire.go` 注入 `deps.memoryKit.Storage()`（已实现 `memoryx.Storage`）。若 nil 则 `memory_list` tool 不注册（与现有 `save_task_result` 的 nil 降级一致）。

**④ `memory_list` tool 实现要点：**

- `limit` 默认 5、上限 50；`offset` 默认 0、下限 0。
- 每条返回 `id`/`content`/`created_at`，`has_more = offset+len(memories) < total`。
- 时间倒序 → 默认行为「返回最新 n 条」天然成立，兼容现有 memory_search 的默认 5 条心智。

### 5.3 kb_create_doc 复用重构

**现状**：`UploadDoc` handler 把「脱敏 → GridFS → CreateDoc → 异步入队索引」串在 handler 层（`queueRepo` 经 `SetQueueRepo` 注入），service 层拿不到 queue，tool 无法复用。

**重构方案**：把全流程下沉到 `knowledge.Service`：

```go
// Service 增加 queue 注入（可选，nil = 不异步入队，仅同步建 doc）
func (s *Service) WithQueue(q repository.QueueRepository) *Service

// CreateTextDoc 创建纯文本 KB doc：脱敏 → GridFS → CreateDoc → 异步入队索引
// 不等索引完成，返回 doc（Status=uploaded）。
func (s *Service) CreateTextDoc(ctx context.Context, userID, title, text string) (*knowledge.KnowledgeDoc, error)
```

内部步骤（复用现有方法，不新增存储逻辑）：

1. 长度校验：`len(text) > MaxKBTextBytes` → error。
2. `RedactText(ctx, text)`（SPEC-068 PII 脱敏）。
3. `UploadFile(title+".txt", "text/plain", bytes.NewReader(redacted))` → gridFSFileID。
4. `CreateDoc(userID, title, title+".txt", knowledge.FileTypeTxt, int64(len(redacted)), gridFSFileID)`。
5. `queue.EnqueueRaw(ctx, "kb_index", task.KBIndexPayload{DocID, GridFSFileID})`（queue 为 nil 时跳过，log 降级）。
6. 返回 doc。

**⑤ `kb_create_doc` tool 实现要点：**

- 走 `deps.KBService.CreateTextDoc`，`user_id` 从 session state 注入（与 `knowledge_search` 一致，LLM 不传身份）。
- 返回 `{doc_id, status:"created"}`；错误原样上抛（含超限/脱敏/建 doc 失败）。

### 5.4 文本长度限制

沿用 SPEC-081 定义的 `MaxKBTextBytes = 5 MB (5×1024×1024)`，与 KB 上传/URL 导入统一。日常总结场景文本远小于 5MB，此上限仅作安全兜底。常量建议定义在 `internal/domain/knowledge`（供 service + handler + tool 共用）。

### 5.5 前端「常用模版」UI

在 `app/agent/page.tsx` 顶部（「+ 新建任务」按钮旁）新增「⚡ 常用模版」入口，与「新建任务」弹窗**分开**：

- 点击展开模版卡片列表（当前仅「日常总结」一个）：
  - 名称：「日常总结」
  - 说明：「每天 01:00 自动总结当天记忆，写入知识库」
- 点击「日常总结」→ 弹出**独立确认弹窗**（`.glass` + blur20，遵循 SPEC-085 弹窗基准）：
  - 标题（可改，默认「日常总结」）
  - 调度（只读展示「每天 01:00」）
  - 说明文字
  - 按钮：「创建模版任务」/「取消」
- 确认后前端调 `POST /api/v1/tasks`（body 见下），成功后关闭弹窗 + toast + 刷新任务列表。

**模版预置 task 参数：**

```json
{
  "title": "日常总结",
  "type": "scheduled_exec",
  "schedule_mode": "recurring",
  "cron_expr": "0 1 * * *",
  "skill_chain": ["memory_list", "kb_create_doc"],
  "params": {
    "message": "你是日常总结助手。请执行：1) 用 memory_list 按创建时间倒序分页读取今天的记忆（offset 从 0 开始，每页 limit=20，翻页直到某页返回的 created_at 早于今天为止）；2) 将今天的记忆归纳为结构化 markdown 总结；3) 用 kb_create_doc 创建文档，title 用「YYYY-MM-DD 日常总结」；4) 用 save_task_result 保存结果。"
  }
}
```

> `skill_chain` 为元数据（执行时工具集为全部注册 tool，实际行为由 `params.message` 提示词驱动，与现有 scheduled_exec 语义一致）。`params.message` 会被 `deriveUserMessageFromParams` 提取为任务指令。

### 5.6 skill 注册三步同步（红线）

新增 `memory_list`、`kb_create_doc` 两个 tool 需三处同步：

1. `internal/adk/tools/tools.go`：`specs()` 注册 function tool（`deps.KBService` 非 nil 才注册 `kb_create_doc`；`deps.MemoryLister` 非 nil 才注册 `memory_list`）。
2. `internal/service/skill/config.go` `predefinedSkills()`：补两个 SkillConfig（name/display_name/description，供 skill 管理页 + search 发现）。
3. `SeedSkills` 幂等自动补齐（启动时）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `memories` / `kb_docs` / `kb_chunks` / GridFS / 现有 queue） |
| 是否影响现有 API | No（task API 复用；`memoryx.Storage` 接口新增方法，现有实现不受影响） |
| 性能影响 | 低（`created_at` 无索引，量大时建议加 `{app_name,user_id,created_at}` 复合索引；日常总结每天 1 次、单用户记忆量小） |
| 是否需要新增 Skill | Yes（`memory_list`、`kb_create_doc` 两个 function tool + 两个 skill 配置） |
| 是否引入新依赖 | No（全复用现有组件） |
| License | 无新增第三方库 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/memoryx/storage.go` | Storage 接口加 `ListRecent` | Small |
| `internal/adk/memoryx/mongo_storage.go` | 实现 `ListRecent`（created_at 倒序 + 分页） | Small |
| `internal/adk/tools/tools.go` | 注册 `memory_list` / `kb_create_doc` + Deps 加 `MemoryLister` | Medium |
| `internal/service/knowledge/service.go` | 加 `WithQueue` + `CreateTextDoc` | Medium |
| `internal/service/knowledge/interface.go` | `KnowledgeService` 接口补 `CreateTextDoc` | Small |
| `internal/domain/knowledge/model.go`（或新文件） | `MaxKBTextBytes` 常量 | Small |
| `internal/service/skill/config.go` | `predefinedSkills()` 补 2 个 skill | Small |
| `cmd/server/wire.go` | 注入 `MemoryLister` / queue 到 knowledge.Service | Small |
| `frontend/app/agent/page.tsx` | 「常用模版」入口 + 日常总结确认弹窗 | Medium |

## 8. 测试策略

1. **Unit tests（Go）**：覆盖率底线见 SPEC-045。重点：
   - `MongoStorage.ListRecent`：created_at 倒序、limit/offset 边界、total 正确、user_id 隔离。
   - `knowledge.Service.CreateTextDoc`：长度超限报错、脱敏调用、CreateDoc 参数正确、queue 入队（gomonkey mock EnqueueRaw）、queue 为 nil 时降级。
   - `memory_list` / `kb_create_doc` tool：参数校验（limit 上限、offset 下限、content 必填）、session state 注入 user_id、nil 依赖降级不 panic。
2. **Integration tests**：条件使用 Docker Compose（`go test -tags=integration`）。
3. **E2E tests**：用例编号 `UI-XXX`（前端模版入口 → 创建 scheduled_exec 任务，断言列表出现「日常总结」+ cron 正确）。
4. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端「常用模版」交互时同步编写 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性（如 `agent-template-btn`、`agent-template-daily-summary`、`agent-template-confirm`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体 | **100%** | — |
| L2 | 依赖接口，可 mock | **100%** | tool 层（注入 mock service） |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `MongoStorage`、`knowledge.Service` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（验证实际值/状态/副作用，非仅 `err == nil`）
- [ ] **必须** `CreateTextDoc` 测试验证 `CreateDoc` 收到的 title/fileName/fileType/sizeBytes/gridFSFileID 具体值
- [ ] **必须** `ListRecent` 测试验证排序方向（created_at 倒序）与分页切片正确性
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil`

### 测试模式

- tool 层：注入 mock 的 `KBService` / `MemoryLister`（mockery 生成）
- Service：直接注入 mock repository / gomonkey 模拟 MongoDB collection
- Logic（L1）：纯 table-driven test

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%
- [ ] `go vet` 无警告

参考: `.agent/specs/spec-045-go-service-ut.md`、`.agent/skills/go-ut-audit/SKILL.md`

## 10. 验证标准

1. **memory_list 分页**：按 `created_at` 倒序返回；`offset` 翻页无重漏；`has_more` 正确；默认 `limit=5` 返回最新 5 条；`created_at` 早于今天的记忆出现在后续页。
2. **kb_create_doc**：`content` 超 5MB 报错；脱敏生效（PII 不落库）；doc 创建成功返回 `{doc_id, created}` 且不等索引；queue 入队 `kb_index`；索引异步完成后 doc status=ready。
3. **日常总结模版**：前端点模版 → 创建 `type=scheduled_exec` + `cron_expr="0 1 * * *"` 的 task；任务列表可见；scheduler 触发后 Agent 完成「读当天 memory → 总结 → 建 KB doc → save_task_result」全链路。
4. **UI 分离**：「常用模版」入口与「新建任务」弹窗相互独立，不共用弹窗。
5. **回归**：`memory_search`、`/memory/list`（`updated_at` 排序）行为不变。
