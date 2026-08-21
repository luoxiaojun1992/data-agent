# 用户意图识别与 LLM 输出相关性检查（Guard）

> **SPEC-067** | Status: 设计中

## 目标

在 LLM 调用链路中引入两道「把关」（Guard）：对用户输入做**意图判断**（聊天 vs 任务），对 LLM 输出做**相关性检查**（是否答非所问），把判断结果作为 `system` 角色事件写入 session 上下文供后续调用参考；相关性不达标时触发**有限次数**的 LLM 重试（Redis 计数，上限存 system config）。同时收紧 compaction 语义：消息角色改为 `system`、**仅由 user/tool 触发（整体压缩，不做部分压缩）**、只作用于 chat/feishu/agent task、且编排在意图识别之后。

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
3. **compaction 语义粗糙**：compaction 摘要的 `Content.Role = "model"`（应是 system 注入的上下文）；且内部一次性 LLM 调用（enhance/compaction 等）理论上不应触发对话 compaction。

## 架构概述

```
                     chat / feishu 请求
                           │
                           ▼
                  ┌──────────────────┐
                  │  intent check    │  ① 意图判断（仅 chat/feishu）
                  │  (UseCaseIntent) │     输入: 用户提示词(文本+图片 content)
                  └────────┬─────────┘     输出: {"is_task": bool}
                           │               ↓ 结果写 system 事件(events)
                           ▼
                  ┌──────────────────┐
                  │  compaction      │  ② 整体压缩旧事件(意图识别之后)
                  │  (由 user 触发)   │     摘要角色 = system
                  └────────┬─────────┘
                           ▼
                  ┌──────────────────┐
                  │  LLM 主调用       │  ③ 输入 = user + system(意图)
                  │  (chat/task 模型) │    (chat: Runtime.RunContent
                  └────────┬─────────┘     agent task: RunAndCollectContent)
                           │
                           ▼
                  ┌──────────────────┐
                  │  relevance check │  ④ 相关性检查（chat/feishu/agent task）
                  │  (UseCaseRelevance)│   输入: LLM 返回 vs 最近 user/tool
                  └────────┬─────────┘   输出: {"is_relevant": bool}
                           │              ↓ 结果写 system 事件(events)
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

**复用现有一次性 LLM 调用模式**（`internal/service/enhance/service.go`）：`modelcfg.BuildLLM(useCase)` → `model.LLMRequest` → `GenerateContent`。intent check / relevance check 均为内部一次性调用，**不写 `raw_events`、不触发工具、不触发 compaction**（结果仅以 system 事件写入 `events`）。

## 详细设计

### 1. 意图判断（intent check）

- **适用范围**：仅 `chat` / `feishu`（需求 6）。agent task 本身就是任务，不做意图判断。
- **触发点**：`chat.Service.prepareRun` 内，`buildUserContent` 生成 `content` 之后（`resolveSession` / `GetOrCreate` 之后、`runner.Run` 之前）。
- **输入**：`content`（用户文本 + 图片，与主 LLM 同源的 `genai.Content`）。**图片一并发给意图识别 LLM**（复用 `buildUserContent` 的 `InlineData` part）；是否多模态模型由**用户在模型配置里选择**，后端**不判断**模型能力（D4）。
- **LLM 契约**（要求返回 JSON 含布尔）：
  ```json
  {"is_task": true}
  ```
  `true` = 任务型（分析/计算/生成报告/查库等），`false` = 聊天。
- **健壮解析**：LLM 可能把 JSON 包在 ```` ```json ```` 代码块里，解析前 strip 掉代码围栏与前后空白；解析失败（非法 JSON/缺字段）按 **chat（false）** 兜底，不阻断主流程。
- **结果写 session**：以 `system` 角色事件写入 `events`（LLM 上下文），**不写 `raw_events`**（内部信号，不对用户展示）。事件文本形如 `[intent] is_task=true`。写入时机在 `runner.Run` 之前（意图识别之后、compaction 之前，见「编排」）。
- **不联动**：`is_task` 仅作为 system 提示注入，让 LLM 自知聊天/任务模式；**不**做「任务意图强制路由到 agent」等额外行为（D3）。
- **模型**：`GetModelByUseCase(UseCaseIntentCheck)`。未显式绑定时 fallback 到 legacy default 或首个 LLM（建议前端将轻量/多模态模型绑定到该 use case）。

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
- **n 的取值**：**存 system config**（`system_configs`，走 SPEC-061 的 Config service + Redis 缓存），key 形如 `guard.max_retries`，默认 `2`。guard.Service 从 config service 读取（读缓存优先，写路径热更新）；读取失败回退默认值 `2`（D2）。
- **失败兜底**：INCR/DEL 失败、LLM 重跑报错均不阻断，返回最后一次可得结果。
- **TTL**：key 设短 TTL（如 `10m`）防止会话中途崩溃导致计数残留；到达 n 主动 DEL。

### 4. compaction 语义调整

- **消息角色**：`maybeCompact` 生成的摘要事件 `Content.Role` 由 `"model"` 改为 `"system"`（与 intent/relevance 的 system 事件统一；`Author` 保留 `"compaction"` 或改用 `CustomMetadata["compaction"]` 标记，保证 `IsCompactionEvent` 仍能识别并过滤展示）。
- **压缩方式**：**整体压缩**（保持现有 `maybeCompact` 语义：把 `events` 前段（超出 `KeepRecent` 的部分）整体替换为一个摘要），**不引入「只压缩部分事件」的过滤逻辑**（D1）。`transcriptOf` 仍按全量事件渲染（user/assistant/tool 均纳入）。
- **触发条件**：仅由 `user` 消息与 `tool` 输出触发（自然边界）；`system` 事件（intent/relevance/compaction 摘要本身）**不触发** compaction。
- **适用场景**：仅 `chat` / `feishu` / `agent task`（有 session 的对话场景）。
- **编排**：chat 场景 compaction 在**意图识别之后**触发（先写 user + system 意图事件，再压缩），见「编排」。
- **内部 LLM 调用不 compaction**：enhance（提示词增强）、compaction 自身、intent check、relevance check 均为一次性调用、结果仅写 system 事件，不触发 `maybeCompact`（现有 `event.Author != "compaction"` 守卫继续生效）。

### 5. 编排：意图识别 → compaction → 主 LLM（chat）

结合 ADK 现有链路（`vendor_adk_v1.5.0/runner/runner.go` 的 `Run` → `appendMessageToSession`(写 user 并触发 maybeCompact) → `agentToRun.Run`(主 LLM)），chat 单次请求编排如下：

```
chat.Service.prepareRun
  1. lastUserMessage → lastText, images
  2. buildUserContent → content (user)
  3. resolveSession → sessionID / modelID
  4. registry.GetOrCreate → rt
  5. SetTitle
  6. buildState + adkSessions.Create (确保 ADK session 存在)
  7. ① 意图判断: guard.CheckIntent(content) → is_task   (内部 LLM，一次性，不触发 compaction)
  8. 写 system 意图事件 → adkSessions.AppendEvent          (不写 raw_events；作为最新事件落 events)
  9. ② compaction: 由步骤 8 的 AppendEvent 触发 maybeCompact (整体压缩「上一轮」旧事件，
     此时 user 尚未写、system 意图已写且处于 KeepRecent 内被保留)
     —— 语义上「意图识别之后 compaction」
chat.Service.Process / Stream
 10. ③ rt.RunContent(content) → runner.Run → appendMessageToSession 写 user 事件
     → agentToRun.Run 主 LLM (上下文 = [摘要 + system 意图 + user])
 11. ④ 相关性检查: guard.CheckRelevance(assistantText, 最近 user/tool) → is_relevant
     → 写 system 相关性事件；不相关则 INCR + 有限重试
```

> 关键点：**意图识别在 `runner.Run` 之前完成**（不侵入 vendor 的 runner 内部）；意图 system 事件先于 user 事件落库，compaction 由「意图 system 事件的 AppendEvent」触发，从而保证「意图识别后 compaction」。agent task 无意图判断，直接 `RunAndCollectContent`（user 由 executor 构造），compaction 由 user/tool 事件自然触发。

### 6. 新增 use case

`internal/adk/modelcfg/provider.go` 的 `UseCase` 常量新增：

```go
UseCaseIntentCheck   UseCase = "intent_check"
UseCaseRelevanceCheck UseCase = "relevance_check"
```

- `GetModelByUseCase` 三级 fallback（`is_default_for` → legacy `is_default` → first LLM）自动覆盖，未绑定也能跑（用主模型，较重）。
- 前端 `frontend/app/admin/models/page.tsx` 的 `USE_CASES` 增加两项：`intent_check`（意图判断）、`relevance_check`（相关性检查），并建议绑定轻量/低成本模型。

### 7. 新增 Guard 模块

新增 `internal/service/guard/`，职责单一、可被 chat service 与 agent executor 复用：

| 组件 | 职责 |
|------|------|
| `guard.Service` | 聚合 intent/relevance/retry，对外暴露 `CheckIntent(ctx, content *genai.Content) (bool, error)`、`CheckRelevance(ctx, llmOutput, base string) (bool, error)`、`RecordAndShouldRetry(ctx, sessionID string) (bool, error)` |
| `intent.go` | 意图判断：组装 prompt（含图片 part）→ 调 LLM → 解析 JSON → 兜底 |
| `relevance.go` | 相关性检查：同上 |
| `retry.go` | Redis 计数封装：`INCR` / 达 n `DEL` / 返回是否应重试 |

依赖注入：`guard.Service` 持有 `*modelcfg.Provider`（BuildLLM）、`*redis.Client`（计数）与 config service（读 `guard.max_retries`）。`wire.go` 构造后注入 `chat.Service` 与 `AgentExecutor`。

## 数据流

1. chat 请求 → `prepareRun` 内意图判断（含图片）→ system 意图事件入 `events` → 触发 compaction（整体压缩旧事件）→ LLM 主调用（上下文含 system 意图 + user）。
2. LLM 返回 → 相关性检查（对比最近 user/tool）→ system 事件入 `events`。
3. 不相关 → Redis INCR → 未达 n 原样重跑 → 再检查；达 n → DEL + 放弃。
4. `events` 超阈值 → `maybeCompact` 整体压缩（角色 system，仅由 user/tool 触发）。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（计数用 Redis，system 事件复用现有 session `events` 数组；`n` 存 system config 现有集合） |
| 是否影响现有 API | No（无新 REST 端点；system 事件不进 `raw_events`，前端无感知） |
| 性能影响 | **每次 chat 增加 1~2 次额外 LLM 调用**（意图 + 相关性）；不相关时再叠加重试次数。意图判断建议绑定轻量/多模态模型；相关性检查可与意图判断共用一个轻量模型。SSE 场景下相关性检查在流结束后执行，不阻塞首字延迟 |
| 是否需要新增 Skill | No（纯内部 Go 逻辑） |
| 是否新增 LLM use case | Yes（intent_check / relevance_check） |
| Redis 依赖 | Yes（复用 `deps.redisClient`，需补 `Incr` 方法） |
| feishu 就绪 | ❌ 当前 feishu 走 webhook，应改 WebSocket 长连接模式（另行单独处理），相关能力预留 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/guard/service.go` + `intent.go`/`relevance.go`/`retry.go` | 新增 Guard 模块 | New |
| `internal/adk/modelcfg/provider.go` | UseCase 常量 + 2 个新 use case | Small |
| `internal/service/chat/chat_service.go` | 注入 guard；`prepareRun` 意图判断 + system 事件；`Process`/`Stream` 相关性检查 | Medium |
| `internal/logic/agent/executor.go` | 注入 guard；`runProtected` 后相关性检查 + 重试 | Medium |
| `internal/adk/session/mongo.go` | compaction 角色 model→system；system 事件写入（仅 events 不写 raw_events）；compaction 触发条件收敛为 user/tool | Medium |
| `internal/adk/session/summarizer.go` | 无改动（整体压缩语义保持） | — |
| `internal/infra/redis/client.go` | 新增 `Incr` 方法 | Small |
| `internal/service/config/` + handler | `guard.max_retries` 读 system config（默认 2） | Small |
| `cmd/server/wire.go` | 构造 guard.Service（注入 provider/redis/config）并注入 chat/executor | Small |
| `frontend/app/admin/models/page.tsx` | `USE_CASES` 增加 intent_check/relevance_check | Small |

## 测试策略

1. **Unit tests**（Go）：guard 包 L1/L2 全绿——意图/相关性 JSON 解析（含代码围栏、非法 JSON 兜底）、重试计数状态机（<n 重试 / ≥n 停止 + DEL）、LLM 失败兜底、图片 part 随意图识别 content 透传。chat/executor 用 gomonkey 注入 mock guard 验证插入点与 system 事件写入（含「意图识别后 compaction」时序断言）。
2. **Integration tests**：条件使用 Docker Compose（真实 MongoDB session + Redis INCR/DEL + system config `guard.max_retries` 读取）。
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
4. `events` 超阈值 → compaction 摘要 `Content.Role=="system"`；chat 场景 compaction 发生在意图 system 事件写入**之后**（意图事件处于 `KeepRecent` 内被保留、不被压缩掉）。
5. 图片消息（文本 + 图片）→ 图片随 content 发给意图识别 LLM，意图 system 事件正常落库。
6. 前端模型管理页可见 `intent_check` / `relevance_check` 两个 use case 并可为模型设默认。

## 设计结论（已拍板）

- **D1 — compaction 方式**：**整体压缩**，保持现有 `maybeCompact` 全量压缩语义，**不做**「只压缩部分事件」的过滤。需求 5 的「只针对 tool 和 user 提示词」解读为**触发条件**——compaction 仅由 `user` 消息与 `tool` 输出触发，`system` 事件（intent/relevance/compaction 摘要）不触发。
- **D2 — 重试上限 n**：默认 `2`，**存 system config**（key `guard.max_retries`），走 SPEC-061 Config service + Redis 缓存；读取失败回退默认值。
- **D3 — 意图判断是否联动**：**不联动**，仅作 system 提示注入（让 LLM 自知聊天/任务模式）。编排上 `user + system(意图)` 作为主 LLM 输入；`user` 仍触发 compaction，但 compaction 编排在**意图识别之后**（见「5. 编排」）。
- **D4 — 图片消息的意图判断**：**图片也发给 LLM 做意图识别**（复用 `buildUserContent` 的 content，含 `InlineData` part）；是否多模态模型由**用户在模型配置里选择**，后端**不判断**模型能力。

## 提交约定

```bash
git add .agent/specs/spec-067-intent-relevance-guard.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-067 intent & relevance guard"
```
