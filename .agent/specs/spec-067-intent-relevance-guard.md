# 用户意图识别与 LLM 输出相关性检查（Guard）

> **SPEC-067** | Status: 设计中

## 目标

在 LLM 调用链路中引入两道「把关」（Guard）：对用户输入做**意图判断**（聊天 vs 任务），对 LLM 输出做**相关性检查**（是否答非所问），把判断结果作为 `system` 角色事件写入 session 上下文供后续调用参考；相关性不达标时触发**有限次数**的 LLM 重试（Redis 计数）。同时收紧 compaction 语义：消息角色改为 `system`、只压缩 tool/user、只作用于 chat/feishu/agent task。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-061 配置统一缓存 | ✅ | 新 use case 走 `GetModelByUseCase`，模型配置直读 DB，无需缓存改造 |
| SPEC-062 多模型 + Session 绑定 | ✅ | `UseCase` 体系与 `GetModelByUseCase` 三级 fallback 直接支撑新 use case |
| SPEC-063 异步/定时执行器 | ✅ | agent task 相关性检查插入点基于 `runProtected` |
| SPEC-064 RBAC | ✅ | 本 spec 不新增权限/API，RBAC 无影响 |

> ⚠️ **feishu 现状**：当前 feishu 接入走 webhook，应改为 **WebSocket 长连接模式**，该改造**另行单独处理**（独立 spec），不纳入本 spec 范围。因此「feishu 的意图判断 / 相关性检查 / compaction」是**预留设计**，需等 feishu 接入 chat 链路后生效。spec 中 feishu 相关能力按「复用 chat 同一 guard 逻辑」设计，接入时零改动启用。

## 背景

1. **无意图区分**：chat 里用户可能下达「帮我分析上季度销售」这类任务型指令，也可能纯闲聊。当前统一走 chat 流程，LLM 无法感知意图差异。
2. **无相关性把关**：LLM 输出可能答非所问（跑偏、幻觉、遗漏用户意图），chat 与 agent task 都没有对输出做质量校验，坏结果直接落库/返回。
3. **compaction 语义粗糙**：compaction 摘要的 `Content.Role = "model"`（应是 system 注入的上下文）；压缩范围是全量事件（含 assistant 回复）；且内部一次性 LLM 调用（enhance/compaction 等）理论上不应参与对话 compaction。

## 架构概述

```
                     chat / feishu 请求
                           │
                           ▼
                  ┌──────────────────┐
                  │  intent check    │  ① 意图判断（仅 chat/feishu）
                  │  (UseCaseIntent) │     输入: 用户提示词
                  └────────┬─────────┘     输出: {"is_task": bool}
                           │               ↓ 结果写 system 事件
                           ▼
                  ┌──────────────────┐
                  │  LLM 主调用       │  (chat: Runtime.RunContent
                  │  (chat/task 模型) │   agent task: RunAndCollectContent)
                  └────────┬─────────┘
                           │
                           ▼
                  ┌──────────────────┐
                  │  relevance check │  ② 相关性检查（chat/feishu/agent task）
                  │  (UseCaseRelevance)│   输入: LLM 返回 vs 最近 user/tool
                  └────────┬─────────┘   输出: {"is_relevant": bool}
                           │              ↓ 结果写 system 事件
              ┌────────────┴─────────────┐
              │ 相关                     │ 不相关
              ▼                          ▼
        返回结果                   Redis INCR (key 关联 sessionID)
                                        │
                              ┌─────────┴──────────┐
                              │ 计数 < n            │ 计数 ≥ n
                              ▼                     ▼
                       原样重跑 LLM            DEL 计数，放弃重试
                       (不加 hint)             返回当前结果
                                                (等用户下一次输入)
```

**复用现有一次性 LLM 调用模式**（`internal/service/enhance/service.go`）：`modelcfg.BuildLLM(useCase)` → `model.LLMRequest` → `GenerateContent`。intent check / relevance check 均不写 session、不触发工具、不触发 compaction（内部一次性调用）。

## 详细设计

### 1. 意图判断（intent check）

- **适用范围**：仅 `chat` / `feishu`（需求 6）。agent task 本身就是任务，不做意图判断。
- **触发点**：`chat.Service.prepareRun` 内，`lastUserMessage` 拿到用户文本后、`buildUserContent` 之前。
- **输入**：用户提示词纯文本（`lastText`）。图片消息（`lastText` 为空且仅有图片）跳过意图判断（直接视为 chat）。
- **LLM 契约**（要求返回 JSON 含布尔）：
  ```json
  {"is_task": true}
  ```
  `true` = 任务型（分析/计算/生成报告/查库等），`false` = 聊天。
- **健壮解析**：LLM 可能把 JSON 包在 ```` ```json ```` 代码块里，解析前 strip 掉代码围栏与前后空白；解析失败（非法 JSON/缺字段）按 **chat（false）** 兜底，不阻断主流程。
- **结果写 session**：以 `system` 角色事件写入 `events`（LLM 上下文），**不写 `raw_events`**（内部信号，不对用户展示）。事件文本形如 `[intent] is_task=true`。
- **模型**：`GetModelByUseCase(UseCaseIntentCheck)`。未显式绑定时 fallback 到 legacy default 或首个 LLM（建议前端将轻量模型绑定到该 use case，降低延迟）。

### 2. 相关性检查（relevance check）

- **适用范围**：`chat` / `feishu` / `agent task`（需求 6）。
- **触发点**：
  - chat/feishu：`Process` 拿到 `assistantText` 后、`Stream` 流结束后（`runAndCollect` / `RunContent` 循环之后）。
  - agent task：`executor.runProtected` 拿到 `*out` 后。
- **对比基准**：「上一条最近的 user 消息 或 tool 输出」。从 session 事件中取最近一条 `Role=="user"` 或 `FunctionResponse != nil` 的事件内容；取不到（异常）则跳过相关性检查（不阻断）。
- **LLM 契约**：
  ```json
  {"is_relevant": true}
  ```
- **健壮解析**：同意图判断，strip 代码围栏；解析失败按 **相关（true）** 兜底，不阻断主流程。
- **结果写 session**：`system` 角色事件写入 `events`（不写 `raw_events`），文本形如 `[relevance] is_relevant=false`。

### 3. 相关性不达标的重试与计数（Redis）

- **计数 key**：`guard:relevance:{sessionID}`（关联 session，跨 chat/feishu/agent task 共用同一 key 前缀）。
- **流程**：
  1. 相关性检查返回 `false` → `INCR guard:relevance:{sessionID}`。
  2. 计数 `< n` → **原样重跑**上一次的 LLM 调用（**不额外加提示词**，区别于 agent executor 现有的 `save_task_result` hint 重试），重跑后**再次**做相关性检查。
  3. 计数 `≥ n` → `DEL guard:relevance:{sessionID}`，放弃重试，返回当前结果（即使仍不相关）。
  4. 用户下一次输入时计数从 0 重新开始（上一轮已 DEL）。
- **n 的取值**：可配置。建议入 `CompactionConfig` 同级的 `GuardConfig{MaxRetries: n}`（默认 `2`），可下沉到 system settings 供运行时调整。
- **失败兜底**：INCR/DEL 失败、LLM 重跑报错均不阻断，返回最后一次可得结果。
- **TTL**：key 设短 TTL（如 `10m`）防止会话中途崩溃导致计数残留；到达 n 主动 DEL。

### 4. compaction 语义调整

- **消息角色**：`maybeCompact` 生成的摘要事件 `Content.Role` 由 `"model"` 改为 `"system"`（与 intent/relevance 的 system 事件统一；`Author` 保留 `"compaction"` 或改用 `CustomMetadata["compaction"]` 标记，保证 `IsCompactionEvent` 仍能识别并过滤展示）。
- **压缩范围**：`summarizer.transcriptOf` 只纳入 **`user` 消息**与 **`tool` 输出（FunctionResponse）**；`assistant` 回复文本不进入摘要 transcript（详见「决策点 D1」）。
- **适用场景**：仅 `chat` / `feishu` / `agent task`（有 session 的对话场景）。
- **内部 LLM 调用不 compaction**：enhance（提示词增强）、compaction 自身、intent check、relevance check 均为一次性调用、不写 session events，天然不触发 `maybeCompact`（现有 `event.Author != "compaction"` 守卫继续生效）。

### 5. 新增 use case

`internal/adk/modelcfg/provider.go` 的 `UseCase` 常量新增：

```go
UseCaseIntentCheck   UseCase = "intent_check"
UseCaseRelevanceCheck UseCase = "relevance_check"
```

- `GetModelByUseCase` 三级 fallback（`is_default_for` → legacy `is_default` → first LLM）自动覆盖，未绑定也能跑（用主模型，较重）。
- 前端 `frontend/app/admin/models/page.tsx` 的 `USE_CASES` 增加两项：`intent_check`（意图判断）、`relevance_check`（相关性检查），并建议绑定轻量/低成本模型。

### 6. 新增 Guard 模块

新增 `internal/service/guard/`，职责单一、可被 chat service 与 agent executor 复用：

| 组件 | 职责 |
|------|------|
| `guard.Service` | 聚合 intent/relevance/retry，对外暴露 `CheckIntent(ctx, text) (bool, error)`、`CheckRelevance(ctx, llmOutput, base) (bool, error)`、`RecordAndShouldRetry(ctx, sessionID) (bool, error)` |
| `intent.go` | 意图判断：组装 prompt → 调 LLM → 解析 JSON → 兜底 |
| `relevance.go` | 相关性检查：同上 |
| `retry.go` | Redis 计数封装：`INCR` / 达 n `DEL` / 返回是否应重试 |

依赖注入：`guard.Service` 持有 `*modelcfg.Provider`（BuildLLM）与 `*redis.Client`（计数）。`wire.go` 构造后注入 `chat.Service` 与 `AgentExecutor`。

## 数据流

1. chat 请求 → `prepareRun` 内意图判断 → system 事件入 `events` → LLM 主调用。
2. LLM 返回 → 相关性检查（对比最近 user/tool）→ system 事件入 `events`。
3. 不相关 → Redis INCR → 未达 n 原样重跑 → 再检查；达 n → DEL + 放弃。
4. `events` 超阈值 → `maybeCompact` 只压缩 user/tool → 摘要以 `system` 角色写回。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（计数用 Redis，system 事件复用现有 session `events` 数组） |
| 是否影响现有 API | No（无新 REST 端点；system 事件不进 `raw_events`，前端无感知） |
| 性能影响 | **每次 chat 增加 1~2 次额外 LLM 调用**（意图 + 相关性）；不相关时再叠加重试次数。意图判断建议绑定轻量模型；相关性检查可与意图判断共用一个轻量模型。SSE 场景下相关性检查在流结束后执行，不阻塞首字延迟 |
| 是否需要新增 Skill | No（纯内部 Go 逻辑） |
| 是否新增 LLM use case | Yes（intent_check / relevance_check） |
| Redis 依赖 | Yes（复用 `deps.redisClient`，需补 `Incr` 方法） |
| feishu 就绪 | ❌ 当前 feishu 走 webhook，应改 WebSocket 长连接模式（另行单独处理），相关能力预留 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/guard/service.go` + `intent.go`/`relevance.go`/`retry.go` | 新增 Guard 模块 | New |
| `internal/adk/modelcfg/provider.go` | UseCase 常量 + 2 个新 use case | Small |
| `internal/service/chat/chat_service.go` | 注入 guard；`prepareRun` 意图判断；`Process`/`Stream` 相关性检查 | Medium |
| `internal/logic/agent/executor.go` | 注入 guard；`runProtected` 后相关性检查 + 重试 | Medium |
| `internal/adk/session/mongo.go` | compaction 角色 model→system；只压缩 user/tool；system 事件写入 | Medium |
| `internal/adk/session/summarizer.go` | `transcriptOf` 过滤（仅 user/tool） | Small |
| `internal/infra/redis/client.go` | 新增 `Incr` 方法 | Small |
| `cmd/server/wire.go` | 构造 guard.Service 并注入 chat/executor | Small |
| `frontend/app/admin/models/page.tsx` | `USE_CASES` 增加 intent_check/relevance_check | Small |

## 测试策略

1. **Unit tests**（Go）：guard 包 L1/L2 全绿——意图/相关性 JSON 解析（含代码围栏、非法 JSON 兜底）、重试计数状态机（<n 重试 / ≥n 停止 + DEL）、LLM 失败兜底。chat/executor 用 gomonkey 注入 mock guard 验证插入点与 system 事件写入。
2. **Integration tests**：条件使用 Docker Compose（真实 MongoDB session + Redis INCR/DEL）。
3. **E2E tests**（前端涉及时）：模型管理页 use case chip 展示 intent_check/relevance_check（`UI-XXX`）。
4. **审计**：`.agent/skills/go-ut-audit`。

## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 模型管理页新增 use case chip 时同步 E2E 用例（`tests/ui/`，`UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid`
- [ ] **必须** CI Pipeline sonar-check + ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

## Go Unit Test 验收规则

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/解析/状态机 | **100%** | guard 的 JSON 解析、重试计数状态机 |
| L2 | 依赖接口可 mock | **100%** | guard.Service（mock Provider + Redis） |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | chat service、executor、session |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少 **2 个行为验证断言**
- [ ] **必须** 验证重试计数状态机：INCR 次数、达 n 后 DEL、返回「是否重试」布尔
- [ ] **必须** 验证 system 事件写入 `events` 且**不写入** `raw_events`
- [ ] **严禁** `t.Skip()` 绕过；**严禁** Success 只验 `err == nil`

## 验证标准

1. chat 发任务型指令 → session `events` 出现 `system` 意图事件 `is_task=true`；闲聊 → `is_task=false`。
2. 构造答非所问（mock LLM 返回无关内容）→ 相关性检查 `false` → Redis `guard:relevance:{sid}` 递增 → 未达 n 原样重跑 → 达 n 后 key 被 DEL、返回结果。
3. agent task 输出跑偏 → 同样触发相关性重试，达 n 放弃。
4. `events` 超阈值 → compaction 摘要 `Content.Role=="system"`，且摘要 transcript 仅含 user/tool 内容。
5. 前端模型管理页可见 `intent_check` / `relevance_check` 两个 use case 并可为模型设默认。

## 决策点（待晓军拍板）

- **D1 — compaction「只针对 tool 和 user」的确切语义**：
  - 理解 A（推荐）：`transcriptOf` 只把 user 消息 + tool 输出送进 summarizer，assistant 回复**不进入摘要输入**；压缩后旧 user/tool 事件替换为摘要，assistant 事件**保留原样**（用户需回顾历史答案）。
  - 理解 B：只过滤 transcript 输入，但压缩后仍整体替换旧事件（assistant 内容随之丢失）。
  - 默认按 A 实现。
- **D2 — 重试上限 n 的取值与配置位置**：默认 `2`，放 `GuardConfig`（可下沉 system settings）。
- **D3 — 意图判断结果是否影响后续行为**：本 spec 仅作 `system` 信息注入（让 LLM 自知聊天/任务模式），**不**做「任务意图强制路由到 agent」等额外行为。若需联动，另立 spec。
- **D4 — 图片消息的意图判断**：仅图片无文本时跳过意图判断、直接视为 chat（本 spec 默认）。

## 提交约定

```bash
git add .agent/specs/spec-067-intent-relevance-guard.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-067 intent & relevance guard"
```
