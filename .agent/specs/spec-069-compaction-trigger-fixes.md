# compaction 机制缺陷修复 + summary 语义拆分 + raw_events 存储重构

> **SPEC-069** | Status: ✅ 已实现（2026-08-31，四项修复落地 + 测试服务器验证）

## 目标

记录并修复 compaction 相关的问题与改进（SPEC-067 落地后的测试与架构审视中暴露）：

1. `estimateEventTokens` 只统计文本，漏算 tool 调用内容，导致 token 阈值失真；
2. 压缩边界可能切在 tool 链配对中间，破坏 FunctionCall/FunctionResponse 关联（已定方案 C）；
3. compaction summary 语义混淆：摘要内容（LLM 用）与前端提示（UI 用）应拆分；
4. `raw_events` 存储架构：由 session document 数组字段改为「一条 event 一个 document」，DB 层精确截取。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-066 配置存储拆分 | ✅ 已实现 | compaction `MaxTokensFn` 依赖 model config 的 `context_len` |
| SPEC-067 意图识别 + 相关性检查 | ✅ 已实现 | `shouldCompact` 收敛为 user + FunctionResponse 触发，本 spec 的两个问题由此触发 |

## 背景 / 动机

### 问题 1：`estimateEventTokens` 只统计文本，漏算 tool 调用内容

- **位置**：`internal/adk/session/mongo.go` 的 `estimateEventTokens`
- **现状**：仅累加 `p.Text` 长度 `/3`，不统计 `FunctionCall.Name/Args`（SQL 查询、搜索关键词）和 `FunctionResponse.Response`（工具返回结果）。
- **实测证据**（2026-08-23）：一个纯 tool 链 session 共 23 条 events（几乎全是 FunctionCall/Response，`text` 长度均为 0），token 估算仅约 **255**（几乎全靠最终回复文本 729 字符），而实际上下文（含多轮 SQL / 知识库 / 记忆 / 外部 API 搜索）远大于此。
- **影响**：`MaxTokens` 阈值（`context_len/2`）在 tool 密集场景形同虚设，compaction 几乎只能靠 `MaxEvents` 触发，无法在 token 维度及时压缩。

### 问题 2：压缩边界可能切在 tool 链配对中间

- **位置**：`internal/adk/session/mongo.go` 的 `maybeCompact`（`cut = len(doc.Events) - KeepRecent`）
- **现状**：压缩把前 `len-KeepRecent` 条合并成 summary，可能把某个 `FunctionCall` 压掉而保留其后的 `FunctionResponse`（或反之），导致 ADK 报 `no function call event found for function responses ids`。
- **实测证据**（2026-08-23）：`KeepRecent=1` 时稳定复现该错误；正常 `KeepRecent=20` 概率低但非零。
- **影响**：`shouldCompact` 的「tool 输出（FunctionResponse）触发 compaction」与「tool 链配对完整性」存在边界耦合——触发时机恰在 tool 链进行中，压缩可能破坏尚未完成的配对。

### 问题 3：compaction summary 语义混淆

- **位置**：`internal/adk/session/mongo.go` 的 `maybeCompact`（生成 summary 事件并同时写入 events + raw_events）
- **现状**：`maybeCompact` 生成一个**含摘要内容**的 summary 事件，同时 `$set` 进 events、`$push` 进 raw_events——「前端提示」与「上下文摘要」两种语义混在同一个事件里。
- **问题**：摘要内容（LLM 上下文）污染了 raw_events（前端展示的原始历史），前端还需靠 `IsCompactionEvent` 再转成轻提示来「遮丑」。
- **影响**：raw_events 不再是纯「原始事件流」，混入了非原始事件。

### 问题 4：raw_events 存储架构

- **位置**：`internal/adk/session/mongo.go` 的 `sessionDoc.RawEvents`（数组字段）+ `DisplayEvents`（整体读 + 内存截取）
- **现状**：raw_events 是 session document 的数组字段；`DisplayEvents` 用 `FindOne` 整体读整个 document（含全部 raw_events），再 `events[len(events)-limit:]` 内存截取最后 N 条。
- **问题**：raw_events 只增不删、持续增长 → 整体读越来越重 + MongoDB document **16MB 上限**；既非 DB 截取也非前端截取。
- **影响**：长 session 查询变慢，且存在写入超限风险。

## 详细设计

### 问题 1：token 估算补全

`estimateEventTokens` 增加对非文本 part 的估算：

- `FunctionCall`：统计 `Name` + `Args`（`json.Marshal` 后长度 `/3`）；
- `FunctionResponse`：统计 `Response`（`json.Marshal` 后长度 `/3`，或统计 `Parts` 内联数据）；
- 保持 `len/3` 的轻量估算口径，不引入 tokenizer 依赖。

### 问题 2：压缩边界与 tool 链配对（已定：方案 C）

> 2026-08-24 晓军拍板：选 **方案 C**。方案 A 有缺陷（见下），故否决。

- ~~方案 A：压缩边界对齐配对~~ —— **已否决**：只保护「已有 response 的 call」，无法识别「response 尚未返回的悬空 call」。
- ~~方案 B：触发时机避开 tool 链~~ —— 未选（需额外维护「tool 链是否结束」状态）。
- **方案 C：KeepRecent 动态下限（选）** —— 保证保留范围覆盖「进行中 tool 链」，含悬空 call。

#### 方案 A 为什么被否决（晓军指出）

异步 tool 调用下，`FunctionCall` 落库后其 `FunctionResponse` 可能**延迟返回**（不同 goroutine）。方案 A 的算法是「从保留范围内的 response 反查对应 call」，但悬空 call（response 尚未返回）根本不在反查范围里，无法被识别——期间触发 compaction 会把悬空 call 压掉，后续 response 返回即丢失配对 call。

#### 方案 C 详细设计

**核心**：compaction 触发时，若 events 末尾存在「悬空 call」（FunctionCall 已落库但对应 FunctionResponse 未出现），则动态扩大保留范围，保证 `cut` 不落在悬空 call 之前。

**实现**（`maybeCompact` 内，计算 `cut` 后做边界修正）：

1. 一次遍历 events，建立 `callID → 是否已有 response` 的映射（收集所有 FunctionCall / FunctionResponse 的 ID）。
2. 从 events 末尾往前，定位「最晚的悬空 call 事件」（含未配对 FunctionCall 的事件）。
3. 若该事件 `index < cut`，则 `cut = index`（前移，把悬空 call 纳入保留范围）。

这样：已完成的 call/response 配对可被整体压缩（无孤儿）；悬空 call 及其后续事件被保留，等 response 返回后配对完整。

#### 并发安全（方案 C 的前置约束，需一并修复）

- **现状**：`maybeCompact` 有 `s.mu.Lock()`（`mongo.go:380`），但 `AppendEvent` 的 events 落库 `s.coll.UpdateOne`（`mongo.go:208`，`$push events`）**无锁**。
- **风险**：异步 tool response 的 AppendEvent 与 compaction 并发时，`maybeCompact` 的 `$set events`（整体替换）会覆盖 `AppendEvent` 的 `$push`，丢失刚 append 的事件（竞态：读快照 → 他人 $push → 本函数 $set 覆盖）。
- **处理**：方案 C 实现时统一 events 读写的锁粒度——AppendEvent 的 events 落库与 maybeCompact 共用同一把锁，保证「读 events → 算 cut → $set events」原子。

#### 入库删除确认（无需额外删除）

- 现状 `maybeCompact` 用 `$set {events: newEvents}`（`mongo.go:425-428`）**整体替换** events 数组，已等价于「删除旧范围 + 写入 summary + 保留最近 KeepRecent 条」，**无需额外的 `$pull`/删除旧范围操作**。
- `raw_events` 只 `$push` compactionEvent（`mongo.go:429-431`），只增不删，符合「raw_events 永远不动」约定。

#### 配对粒度：以「call 事件」为锚点

ADK 配对是**事件粒度**的，不是单个 call 粒度：一个 call 事件可含多个 `FunctionCall`，其对应的多个 `FunctionResponse` 可能分散在多个事件（异步返回顺序不定）。`rearrangeEventsForFunctionResponsesInHistory`（`contents_processor.go:331`）遍历时，response 事件本身被 `continue` 跳过，必须靠它的 call 事件「认领」合并回来。

因此压缩边界规则：

> **cut 不能落在「某个 call 事件」和「它的任何一个 response 事件」之间。**

对齐锚点是那个 call 事件，而非「紧邻的单个 call/response 对」。

#### 隐藏风险：response 后还有 msg 时是「静默丢失」而非报错

- 若「最后一个事件是 response」且 call 被压掉 → `rearrangeEventsForLatestFunctionResponse` 报 `no function call event found`（显式报错）。
- 若「response 后面还有 msg」（如 `[call1,call2][resp2][resp1][msg][msg][msg]`）→ `rearrangeEventsForFunctionResponsesInHistory` 把孤儿 response 静默 `continue` 丢弃，**不报错但 tool 结果凭空消失**，比报错更难察觉。

例：`[call1,call2] [tool_response2] [tool_response1] [msg] [msg] [msg]`，cut 切在 call 事件与 resp2 之间 → 压缩后 `[summary][msg][msg][msg]`，resp2/resp1 被静默丢弃。

### 问题 3：summary 语义拆分（已定：拆分两件事）

`maybeCompact` 生成**两个**独立产物，分流到不同存储：

| 产物 | 内容 | 写入 | 消费者 |
|------|------|------|--------|
| summary 事件 | 含摘要内容（`[conversation summary] ` + summary），`Author=compaction` | 仅 `events` | LLM 下一轮上下文 |
| 压缩提示 | 轻量 `[compaction] 上下文已自动压缩`，`Author=compaction`，**无摘要内容** | 仅 `raw_events` | 前端展示 |

- `maybeCompact`：`$set events` 写入 summary 事件（现状已有）；`$push raw_events` 改为写入**轻量提示事件**（替代现状 push 的 summary 事件）。
- `Messages handler`：读 raw_events 时，压缩提示事件直接作为 system 消息展示，**不再需要** `IsCompactionEvent` 转轻提示（fallback 老 session 的 events 路径仍需跳过 summary 事件）。

### 问题 4：raw_events 存储重构（已定：一条 event 一个 document）

将 raw_events 从 session document 数组字段拆为独立 collection，实现 append-only + DB 层精确截取。

- **新 collection**：`session_events`（或 `adk_session_events`）
  - 字段：`session_id`、`app_name`、`user_id`、`seq`（递增序号）、`event`（序列化的 session.Event）、`created_at`
  - 索引：`{session_id: 1, seq: 1}`（唯一，排序 + 去重）
- **写入**：`AppendEvent` 时 raw_events 事件改为 `insertOne` 到独立 collection（`seq` 自增）。
- **查询**：`DisplayEvents` 改为 `find({session_id, app_name, user_id}).sort({seq: -1}).limit(N)` 精确截取，再倒序还原。
- **events 保留在 session document**（会被 compaction 整体 `$set` 重写，大小有界，数组字段合适）。
- **迁移**：老 session 的 `raw_events` 数组一次性迁移到独立 collection（幂等 seed/脚本）。
- **影响面**：`sessionDoc` / `AppendEvent` / `DisplayEvents` / `syncSnapshot` / `maybeCompact` 中 raw_events 的读写统一改为独立 collection 操作。

> 详细字段 / 索引 / 迁移脚本待实现时展开。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | **Yes**（问题 4：`session_events` 独立 collection） |
| 是否影响现有 API | No（`DisplayEvents` 返回结构不变） |
| 性能影响 | 问题 1：`estimateEventTokens` 增加 JSON 序列化开销，可接受；问题 4：消除整体读 + 16MB 上限 |
| 是否需要新增 Skill | No |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/session/mongo.go` | `estimateEventTokens` / `maybeCompact` / `AppendEvent`（锁粒度统一 + summary 拆分 + raw_events 独立 collection） | Modify |
| `internal/api/handler/session.go` | `Messages`（压缩提示直接展示，去掉 IsCompactionEvent 转换） | Modify |

## 测试策略

1. **Unit tests**（Go）: `estimateEventTokens` 补全后的行为断言（含 FunctionCall/Response 的 token 估算）；`maybeCompact` 方案 C 的「悬空 call 保护」断言（含悬空 call 不压缩、已配对可压缩、并发锁粒度）；summary 拆分（summary 进 events、提示进 raw_events）；raw_events 独立 collection 的 append/查询。
2. **Integration / E2E**：临时参数（`MaxEvents`/`KeepRecent` + model `context_len`）复现 tool 链场景，验证不再出现 `no function call event found`；raw_events 迁移后老 session 兼容。

## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 若影响前端展示（compaction 摘要/消息），同步更新对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

## Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

- [ ] `estimateEventTokens` / `maybeCompact` 相关新增/修改逻辑的 UT 覆盖率达标（L1 纯逻辑 100%）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因）

## 验证标准

- [ ] `estimateEventTokens` 覆盖 `FunctionCall.Args` / `FunctionResponse.Response`
- [ ] tool 密集场景 token 估算接近真实（单测断言）
- [ ] `KeepRecent` 极小值下不复现 `no function call event found`
- [ ] 现有 compaction 端到端测试仍通过
- [ ] summary 事件只进 events，压缩提示只进 raw_events，两者解耦
- [ ] raw_events 独立 collection：`DisplayEvents` DB 层截取最新 N 条，老 session 迁移后兼容

## 提交约定

```bash
git add .agent/specs/spec-069-compaction-trigger-fixes.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-069 compaction trigger fixes (token 估算 + tool 链配对)"
```
