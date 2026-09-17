# SPEC-100 优化 Token 用量埋点（session 独立统计表 + session 删除级联 + session token 展示）

> **SPEC-100** | Status: ✅ 设计定稿（D1~D7 全定稿；暂不实现）
> 日期：2026-09-16 立项 → 2026-09-17 设计定稿

## 0. 决策记录（2026-09-16 晓军拍板）

| # | 决策 | 结论 |
|---|------|------|
| 1 | hourly record 是否关联 session | ❌ **不关联**。`stats_hourly` 保持 `{metric,hour}` 全局计数**完全不动**，不新增 `session_id` 字段 |
| 2 | session token 统计方式 | ✅ **独立建表**统计；删 session 时**关联删除**（早于 session 主记录删除，幂等删除） |
| 3 | 是否加时间冗余字段 | ❌ **不加**。现有 UTC `time.Time` 存储天然兼容时区（见 §2.2），一年数据量 ≤8760/指标，实时聚合足够 |
| 4 | session 删除语义 | 级联删除**仅指硬删除（`HardDelete`）**；软删除（归档 `Delete` 设 `deleted_at`）**不级联**——session 主记录保留、可恢复，token 统计同步保留，恢复后累计继续 |
| 5 | 日历维度分桶时区 | 按小时/星期几/几号/几月的聚合展示**按实际时区 Asia/Shanghai（UTC+8）计算**，非 UTC（见 §2.2-B；当前存在 UTC 口径偏差，本 spec 一并修正） |
| 6 | 分桶时区机制 | 分桶与展示**统一按浏览器时区**：前端传 `timezone` 参数（浏览器 `Intl.DateTimeFormat().resolvedOptions().timeZone`），后端校验后按前端时区确定时间区间 → 转 UTC 区间查询 DB → **按前端传参时区分桶聚合（分桶/聚合 100% 后端执行，严禁原始数据回前端聚合）**；横轴标签用浏览器时区 `toLocaleString` 显示，口径天然一致。缺省回退 Asia/Shanghai，无效时区 400（见 §2.2-B） |
| 7 | 子 session token 归属 | 子 session **使用父 sessionID 计数**，`session_stats` 统一主（父）session 维度；删除子 session **不级联**删父 session token 统计（本身不关联，见 §4/§5.2） |
| 8 | stats_hourly 是否已有 session_id | ✅ **已确认没有**：代码 `HourlyStat` 无该字段 + 线上 197 条文档字段仅 `_id/metric/hour/updated_at/value`（2026-09-16 实测），无需删除 |
| 9 | get_current_time 时区口径 | 附带修正（归属 SPEC-080 工具）：改为**按 UTC 返回** + 输出显式标注时区（`timezone: "UTC"`），让 LLM 与系统 UTC 时间戳对齐（见 §2.3，无业务时区参考字段） |

## 1. 目标

1. **新增独立的 session token 统计表**（不复用 `stats_hourly`），让 token 用量从「全局聚合」细化到「按 session 聚合」，支撑 chat / task 页面展示单会话 token 消耗。
2. **删 session 时级联删除**该 session 的 token 统计，且删除发生在 session 主记录删除**之前**、**幂等**，杜绝孤儿数据。
3. **优化 dashboard 趋势统计逻辑**——不做下沉 MongoDB，而是基于「数据量小」的事实，保持/精简现有 Go 层实时聚合，仅在需要处优化。
4. **chat / task 页面展示 session 消耗的 token 数量**，用新增的 session 统计表按 sessionID 点查（单文档 O(1)，无需聚合）。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-072 Dashboard 统计重构（stats_hourly + 统一 Counter/Reader） | ✅ | `stats_hourly` 集合、`HourlyStat`、`{metric,hour}` 唯一索引、`Sum/Series` 已就绪。本 spec **不修改**其 schema（决策 1） |
| SPEC-051 LLM 全链路 Token 统计 | ✅ | `llmstats.Record` 已含 `SessionID` 字段（见 §3），埋点链路完整 |
| SPEC-059 Token 统计真数据 | ✅ | 埋点已接入真实 usage |
| SPEC-090 Session 生命周期（软删/硬删/Cleanup） | ✅ | `Manager.HardDelete` 是唯一物理删除路径（见 §4），级联落点明确 |
| — | — | 无新增前置，可立即开始 |

## 2. 背景与动机

### 2.1 埋点丢失 session 维度

`internal/infra/llmstats/llmstats.go` 的 `Record` 结构体**已包含 `SessionID` / `UserID` 字段**，但 `Recorder.Record()` 调用 `Counter.Incr()` 落库时**丢弃了 session 维度**——`Counter.Incr(ctx, m, at, delta)` 接口没有 session 参数，落库目标是全局 `stats_hourly`。

**后果**：token 用量只能全局聚合，chat/task 页面无法展示单会话用量。

### 2.2 时区结论（决策 3 依据 + 澄清 #5）

**A. 存储层天然兼容时区（决策 3 依据）**：

- `stats_hourly.hour` 是 Go `time.Time`，落库前 `HourBucket()` 强制 `.UTC().Truncate(time.Hour)`。
- MongoDB BSON Date 是**绝对时间戳**（int64 毫秒 since epoch），**不携带时区**，任何客户端/语言读取都还原为同一绝对时刻。
- 因此「星期几 / 几号 / 几月」只是绝对时刻的日历投影，**无需冗余存储**，可在查询/展示层按需实时计算。一年 ≤8760 文档/指标，数据量小，Go 层实时聚合足够——时间维度索引下沉 MongoDB 无收益。

**B. 日历分桶时区口径（澄清 #5 + 决策 #6 v2，⚠️ 当前存在偏差）**：

- **现状（偏差）**：`metrics.go` 的 `bucketStart`/`bucketHours` 全 `.UTC()` 分桶，`dashboard.go` 的 `now`/默认窗口也取 UTC → 按「小时/星期几/几号/几月」的**日历归属按 UTC 计算**，对 Asia/Shanghai 用户整体偏 8 小时（例：UTC 14 点被归为「14 点高峰」，实际是北京 22 点；「今天」从 UTC 00:00 起，对应北京 08:00，跨两个北京自然日）。
- **目标口径（决策 #6 v2）**：分桶与展示**统一按浏览器时区**——与全站其他时间字段的 `toLocaleString`（浏览器时区）一致。落库 `HourBucket` **保持 UTC**（存储层绝对时间戳不变，仅展示/分桶层变）。

**时区参数流**：

```
前端 dashboard 页面
  tz = Intl.DateTimeFormat().resolvedOptions().timeZone   // 浏览器时区，如 "Asia/Shanghai"
  GET /api/v1/dashboard?granularity=day&timezone=Asia/Shanghai
  GET /api/v1/dashboard/trends?granularity=day&timezone=Asia/Shanghai
        ↓
后端 handler
  1. 校验 timezone：time.LoadLocation(tz) → 失败返回 400
     缺省 → Asia/Shanghai（后端常用时区兜底）
  2. 确定时间区间（按统计维度 + 前端时区）：
     since = bucketStart(now.In(loc), 对应粒度, loc)  // 前端时区的自然窗口起点
     until = now                                      // 当下时刻
     日视图 → 今日 00:00（loc）起；周 → 本周一 00:00；月 → 本月 1 号；年 → 本年 1 月 1 日
  3. 查询 DB：since/until 本就是绝对时刻，直接 $match hour ∈ [since, until)
     —— DB 存 UTC 绝对时间戳，区间无时区，无需额外转换
  4. 分桶：bucketStart/bucketHours 用 loc 做自然日/周/月边界（.In(loc) 后 Truncate）
  5. 返回 Bucket.Time = 分桶边界绝对时刻（UTC 表示）
        ↓
前端
  横轴标签 new Date(t).toLocaleString()（浏览器时区）→ 显示即分桶边界，口径天然一致
```

**关键实现注意**：`time.LoadLocation` 依赖 IANA 时区数据库——后端容器（debian:bookworm-slim）**无 tzdata 包**时 LoadLocation 会失败。必须 `import _ "time/tzdata"`（Go 内嵌时区库，~450KB）或容器安装 tzdata；不可用 `FixedZone` 兜底（只能兜 +08:00 一种，无法支持任意 IANA 时区）。

**⚠️ 红线（分桶/聚合必须后端执行）**：严禁把 `stats_hourly` 原始 hourly 文档返回前端聚合——数据量（一年 ≤8760 文档/指标）会使 API payload 膨胀、前端计算重复且无法加索引优化。**分桶/聚合 100% 在后端完成**（`Series` 返回已聚合的 `[]Bucket`，每 bucket 一个点），前端只消费聚合结果并渲染。

**C. 系统时区口径全景（2026-09-16 实测确认，回答「其他数据是否按服务器时区转换返回前端」→ 否）**：

| 层 | 现状口径 | 证据 |
|---|---|---|
| 存储（MongoDB） | UTC 绝对时间戳（BSON Date，int64 ms since epoch，无时区） | mongo 容器 `date` = UTC +0000；BSON Date 规范 |
| 后端内存/落库 | `time.Now()` = UTC 的 `time.Time` | 后端容器 `date` = UTC +0000、`TZ=[]`（compose/Dockerfile 均无 TZ 设置）；唯一显式时区是 `get_current_time` 工具（`LoadLocation("Asia/Shanghai")`，注释明言「时区显式指定，从不依赖服务器默认 TZ」） |
| 后端返回前端 | **UTC RFC3339（`...Z`），无「服务器时区转换」这一步** | gin 序列化 UTC `time.Time` 直接输出 `...Z` |
| 前端展示 | **浏览器本地时区**渲染 | 全站统一 `new Date(x).toLocaleString()/toLocaleDateString()` |
| 聚合分桶（stats_hourly） | ⚠️ UTC 自然日边界（偏差，§2.2-B 修正） | `bucketStart`/`bucketHours` 全 `.UTC()` |

**关键区分（两层不冲突）**：
- **时间字段展示**（session/chat/task/kb/artifact 的 `created_at`/`updated_at` 等）：存 UTC → 返回 UTC RFC3339 → 前端浏览器时区渲染，**已正确，无需改**。
- **聚合分桶**（趋势统计日历维度）：**与展示统一按浏览器时区**——前端传 `timezone`、后端按该时区确定区间并分桶（决策 #6 v2），横轴标签用同一浏览器时区显示，口径天然一致。不同时区用户各自看到正确归属的视图（dashboard 是用户视角的展示语义，按用户时区个性化即正确行为）。

## 2.3 附带修正：get_current_time 时区口径（决策 #9，归属 SPEC-080 工具）

现状（`internal/adk/tools/tools.go` 的 `currentTime()`）：返回 **Asia/Shanghai 本地时间**（`t.In(loc)` 后 RFC3339，`Timezone: "Asia/Shanghai"`）——与系统内所有 UTC 时间戳口径**不一致**，LLM 拿到「now」后若与系统 UTC 时间戳（如 session 的 `created_at`、task 的 `scheduled_at`）做比较/运算会差 8 小时。

修正方向：

- `time`/`date`/`weekday` 按 **UTC** 返回（RFC3339 带 `Z`），与系统时间戳口径对齐。
- 输出**显式标注时区**：`timezone: "UTC"`，让 LLM 明确知道返回的是 UTC 时刻，与系统 UTC 时间戳可直接对齐比较。
- `unix` 保持不变（绝对时刻，无时区）。

> 注：不提供业务时区参考字段；LLM 需要的「now」以 UTC 单一口径返回。

## 3. 架构概述

```
埋点调用点（llmstats.Recorder.Record）
        │  rec.SessionID + billed_tokens + CreatedAt
        ├────────────────────────────────────────────┐
        ▼ 全局（现状，不动）                            ▼ session（新增）
Counter.Incr(metric, at, delta)                    SessionStatStore.Upsert(sessionID, delta)
        │  {metric,hour} 唯一索引 $inc                   │  _id = session_id，$inc 幂等累加
        ▼                                              ▼
stats_hourly（全局趋势/看板）                          session_stats（单会话 token 用量）
        │                                                │
        └── 查询：Sum/Series（不变）                       ├── 查询：chat/task 页按 session_id 点查
                                                         └── 删除：Manager.HardDelete 级联删除（先于主记录，幂等）
```

与现有模块的关系：**不改** `stats_hourly` schema、`Metric`/`Granularity` 枚举、`Counter` 接口签名、`Sum/Series` 聚合、ROI 派生、TTL 一年上限；**新增** `SessionStatStore`（session 维度累计）、`session_stats` collection、`Recorder` 双写、`Manager.HardDelete` 级联。

## 4. 删除级联落点（决策 2 + 澄清 #4）

> **软删除（归档）不级联**：软删除走 `Manager.Delete` → `repo.Delete`（仅设 `deleted_at`），**不触发** `session_stats` 删除——session 主记录保留、可恢复，token 统计同步保留，`Restore` 后累计继续。级联**仅发生在硬删除**。

`internal/service/chat/session.go` 的 `Manager.HardDelete` 是**唯一物理删除路径**（`Cleanup` 与显式硬删都走它）：

```go
func (m *Manager) HardDelete(id string) error {
    // 新增：先删 session token 统计（早于主记录删除；不存在=成功继续，
    // 仅 err != nil 失败即中止，主记录不删 → 无孤儿）
    if m.sessionStatStore != nil {
        if err := m.sessionStatStore.DeleteBySession(ctx, id); err != nil {
            return err
        }
    }
    if err := m.repo.HardDelete(context.Background(), id); err != nil {
        return err
    }
    removeWorkspace(id)
    if m.historyStore != nil {
        if err := m.historyStore.Delete(context.Background(), id); err != nil {
            return err
        }
    }
    return nil
}
```

**顺序语义（「早于主记录删除」的理由）**：token 统计先删成功、主记录删除失败（session 仍在）时，后续埋点会重新 `upsert` 累计，**不丢数据**；反之若主记录先删、token 统计删除失败，则永久残留孤儿。

**幂等**：`DeleteBySession` 用 `DeleteOne({_id: session_id})`——删除**不存在的文档返回 `deletedCount=0` 且 `err == nil`，视为成功（幂等，绝不因「已不存在」报错）**；仅 `err != nil`（真实 DB 错误）才失败中止。

**子 session 边界（决策 #7，已定）**：子 session 的 token **归父 session 计数**（`session_stats._id` 统一为顶层父 session ID），子 session **不产生独立统计**。因此删除子 session（`adk/session/mongo.go` 的 `DeleteByID`/`deleteSubSessions`、`subagent/runner.go` 的 `cleanup`）**不级联**删任何 `session_stats` 文档——统计挂在父 session 上，与子 session 无关联。父 session 删除时由其自身 `HardDelete` 级联一次覆盖。

## 5. 详细设计（方向）

### 5.1 session_stats 数据模型

新建 collection `session_stats`，以 `session_id` 为主键（`_id`），单文档累计：

| 字段 | 类型 | 说明 |
|------|------|------|
| `_id` | string | = `session_id`（`sess_` 前缀），点查 O(1) |
| `session_id` | string | 冗余（与 `_id` 同），便于语义查询 |
| `billed_tokens` | int64 | 累计计费 token（`(prompt+completion) × multiplier`） |
| `prompt_tokens` | int64 | 累计 prompt token（原始，未乘 multiplier） |
| `completion_tokens` | int64 | 累计 completion token（原始） |
| `llm_calls` | int64 | LLM 调用次数 |
| `created_at` | time.Time | 首条写入（UTC 绝对时间） |
| `updated_at` | time.Time | 最近一次 `$inc`（UTC 绝对时间） |

写入（`Incr`，幂等累加；实现为 `$inc` + upsert）：

```
filter = {_id: session_id}
update = {
  $inc: { billed_tokens, prompt_tokens, completion_tokens, llm_calls },
  $set: { updated_at: now },
  $setOnInsert: { _id: session_id, session_id, created_at: now },
}
```

> **不存 `user_id`**（晓军确认 2026-09-17）：查询全走 session_id（点查 `_id`/级联删除/批量点查），RBAC 归属校验在上游 sessions 主表完成，user_id 无任何过滤/聚合用途，冗余不存。

索引（D2 定稿）：
- 仅 `_id`（session_id）主键——点查/删除唯一命中。
- 不建 `user_id`、不建 TTL（见 §11 D2）。

### 5.2 埋点双写

`llmstats.Recorder` 新增依赖 `SessionStatStore`（接口），`Record()` 中：

```go
if r.counter != nil {
    _ = r.counter.Incr(ctx, metrics.MetricTokenTokens, at, int64(billed))
    _ = r.counter.Incr(ctx, metrics.MetricLLMCalls, at, 1)
}
// 新增：session 维度累计（仅当 rec.SessionID 非空）
if r.sessionStats != nil && rec.SessionID != "" {
    _ = r.sessionStats.Incr(ctx, rec.SessionID, rec.PromptTokens, rec.CompletionTokens, billed, at)
}
```

- `SessionID` 为空（理论上不应发生，LLM 调用必有会话上下文）时仅走全局埋点，不写 `session_stats`，防御性处理。
- 不修改 `Counter` 接口签名，避免全量调用点改动。
- **子 session 归父（决策 #7，D4 已定稿）**：`RecordingLLM` 经 `agent.InvocationContext` 取 `state["session_id"]`——主 chat/task = 自身、子 agent = 父 session ID（`subagent/tool.go:59`），三场景天然统一，无需专门解析 `parent_session_id`。解析失败降级仅全局埋点，不阻塞主流程。

### 5.3 API 方向（D1 定稿）

- **session token 点查**：新增 `GET /api/v1/sessions/:id/token-usage`（挂 sessions 路由组，`PermChatView` + 复用 `verifyOwnership` 归属校验，system_admin 豁免）→ `{"token_tokens": N}`（按 `_id=session_id` 点查，无记录 0）。
- **task run 详情内嵌**：`GET /api/v1/tasks/:task_id/runs/:run_id` 响应内嵌 `token_tokens`（`run.SessionID` 查，无=0）。
- **列表接口不加字段**（sessions 列表 / task 列表均不动）。
- **前端展示**：chat 页在消息历史最下方、输入框（含增强/脱敏/语音按钮行）上方显示「本会话消耗 X tokens」（`data-testid="chat-token-usage"`），进入会话时点查 + 每次流结束刷新；task run 详情页在 run 详情区显示 token。
- **dashboard 趋势统计**：`Sum/Series` 无 schema 变更（`Series` 签名加 `loc *time.Location`）、不引入冗余字段；**修正日历分桶时区口径**——新增 `timezone` 查询参数（校验 + 缺省回退 + 400），默认窗口起点与分桶边界均按前端传入时区（见 §2.2-B，决策 #6 v2）；**分桶/聚合仅在后端执行，API 只返回聚合后的 `[]Bucket`，严禁原始 hourly 文档回前端**；前端横轴标签保持浏览器时区 `toLocaleString`（与分桶口径天然一致）。
- **get_current_time 附带修正**：按 UTC 返回 + 时区标注（见 §2.3）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | ✅ Yes：新增 `session_stats`（`stats_hourly` 不动） |
| 是否影响现有 API | `Counter` 接口**不改**；`stats_hourly` 查询**不回归**；新增 session token 查询接口 |
| 性能影响 | 正向：单会话 token 点查 O(1)；`stats_hourly` 无写放大（决策 1 不加字段） |
| 是否需要新增 Skill | No |
| 数据迁移 | 无需迁移存量（`session_stats` 全新建表，`stats_hourly` 不改） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/llmstats/llmstats.go` | `Recorder` 新增 `SessionStatStore` 依赖 + `Record` 双写 | Medium |
| `internal/infra/llmstats/session_stats.go`（新） | `SessionStatStore` Mongo 实现（Upsert/DeleteBySession/GetBySession） | Medium |
| `internal/service/chat/session.go` | `Manager` 注入 store；`HardDelete` 级联（先于主记录、幂等） | Medium |
| `cmd/server/wire.go` / `main.go` | DI 注入 `SessionStatStore`（赋值顺序：消费前）+ `import _ "time/tzdata"` | Low |
| `internal/api/handler/session.go` | 新增 `GET /:id/token-usage` 点查接口 + verifyOwnership 归属校验 | Low |
| `internal/api/handler/task.go` | `GetRun` 响应内嵌 `token_tokens`（run.SessionID 查） | Low |
| `internal/infra/metrics/metrics.go` | `Series`/`bucketStart`/`bucketHours` 加 `loc *time.Location` 参数，按传入时区做日历分桶（决策 #6 v2） | Medium |
| `internal/api/handler/dashboard.go` | 新增 `timezone` 参数解析+校验（400/缺省回退 Asia/Shanghai）、默认窗口起点按传入时区自然日（决策 #6 v2） | Medium |
| `internal/adk/tools/tools.go` | `get_current_time` 改 UTC 输出 + 时区标注（附带修正 §2.3，归属 SPEC-080） | Low |
| 埋点出口（buildBackends `RecordingLLM`） | 经 InvocationContext 取 state["session_id"] 归父（决策 #7，D4） | Medium |
| 前端 chat 页面 | 消息历史下方/输入框按钮行上方显示 token（`chat-token-usage`，进入+流结束刷新） | Medium |
| 前端 task run 详情页 | run 详情区显示 token | Low |
| 前端 dashboard 页面 | 传 `timezone` 参数（`Intl.DateTimeFormat().resolvedOptions().timeZone`），横轴保持浏览器时区 `toLocaleString` | Low |

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能（chat/task 页 session token 展示）时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
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
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | token 累计/派生纯函数 |
| L2 | 依赖接口，可 mock | **100%** | `SessionStatStore` 接口、`Recorder` 双写逻辑 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `session_stats` repo、`Manager.HardDelete` 级联、session token handler |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** 验证 `Incr` 幂等：同 session 多次 `$inc` 累计正确，`$setOnInsert` 首写字段不覆盖
- [ ] **必须** 验证 `HardDelete` 级联顺序与幂等：token 统计先删、主记录后删；**删除不存在的 session（deletedCount=0）视为成功并继续删主数据**；仅真实 DB 错误（err != nil）才中止且主记录不删
- [ ] **必须** 验证 `Recorder` 双写：`SessionID` 非空写 `session_stats`，空则仅全局埋点，互不干扰
- [ ] **必须** 验证 `timezone` 参数链路（决策 #6 v2）：无效时区 400；缺省回退 Asia/Shanghai；`Series` 按传入时区分桶——UTC 00:00 的样本归入北京「前一天」、北京自然日边界 00:00（= UTC 前一日 16:00）分桶正确；`bucketStart` 对 hour/day/week/month/year 的边界样本断言正确（覆盖非整点偏移时区如 `Asia/Kathmandu` UTC+5:45）
- [ ] **必须** 验证软删除（归档 `Delete`）**不**级联删 `session_stats`，仅 `HardDelete` 级联
- [ ] **必须** 验证子 session 归父（决策 #7）：`state["session_id"]` = 父 session 的上下文埋点写**父 session** 文档；删子 session 后父 session 统计保留、无任何 `session_stats` 被级联
- [ ] **必须** 验证 `token-usage` 点查接口：归属校验（他人 session 403、system_admin 豁免）、无记录返回 0、有记录返回累计值；`GetRun` 响应内嵌 `token_tokens` 正确（run.SessionID 查、无=0）
- [ ] **必须** 验证 `GetBySession` 单点查：无记录 `(0, nil)`、有记录返回 billed_tokens
- [ ] **必须** 验证 `get_current_time` 按 UTC 返回且显式标注时区（`time` 带 `Z`、`timezone=UTC`、`weekday`/`date` 按 UTC 计算正确；不包含业务时区参考字段）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%；`go vet` 无警告

## 10. 验证标准

1. `session_stats` 写入正确：埋点后同一 session 多次 LLM 调用累计到单文档，`billed_tokens`/`llm_calls` 单调递增，`_id == session_id`。
2. `stats_hourly` 完全不受影响：`Sum/Series` 结果与改造前一致（无回归）。
3. `HardDelete` 级联正确：删除 session 后 `session_stats` 对应文档消失；**删除不存在的 session 幂等成功（deletedCount=0 继续删主数据）**；仅真实 DB 错误才中止且主记录不删（无孤儿）。
4. chat 页在消息历史下方、输入框按钮行上方正确显示「本会话消耗 X tokens」，数值与 `session_stats` 文档一致（进入会话 + 流结束刷新）；task run 详情正确显示该 run 的 token 消耗。
5. 趋势统计（日/周/月/年）结果正确，`stats_hourly` 查询路径无性能回退。
6. 日历维度（小时/星期几/几号/几月）聚合按**前端传入时区**归属正确：前端传 `Asia/Shanghai` 时跨 8 小时时差的边界样本归入北京自然日/自然周/自然月；无效时区返回 400；缺省回退 Asia/Shanghai；软删除（归档）后 `session_stats` 保留，硬删除后消失。
7. 子 session token 归父正确：子 agent 运行产生的 token 计入父 session 文档（`_id == 父 session ID`）；删除子 session 后父 session 统计不受影响。
8. `get_current_time` 按 UTC 返回并显式标注时区（`time` 带 `Z`、`timezone=UTC`），与系统 UTC 时间戳口径一致，无业务时区参考字段。

## 11. 设计定稿记录（2026-09-17，D1~D7 全定稿）

> 立项方向（决策①~⑨）不变；本章将立项遗留的「实现阶段定」项逐一定稿。

### D1 — session token 展示：点查接口 + run 详情内嵌（列表不加字段，晓军拍板 2026-09-17）

- **新增点查接口** `GET /api/v1/sessions/:id/token-usage`（挂 sessions 路由组，`PermChatView` + 复用 `verifyOwnership` 归属校验，system_admin 豁免）→ `{"token_tokens": N}`（无记录 0）。
- **task run 详情内嵌**：`GET /api/v1/tasks/:task_id/runs/:run_id`（`GetRun`）响应内嵌 `token_tokens`（`run.SessionID` 查 session_stats，无=0）。
- **列表接口一律不加字段**（sessions 列表 / task 列表不动）。
- **前端展示位置**：chat 页在消息历史最下方、输入框（含增强/脱敏/语音按钮行）**上方**显示「本会话消耗 X tokens」（`data-testid="chat-token-usage"`），进入会话时点查一次 + 每次流结束刷新；task run 详情页在 run 详情区显示。
- RBAC：无新增权限（复用接口现有权限与归属校验）。

### D2 — session_stats 索引：仅主键

- 仅 `_id`（session_id，Mongo 自动主键）。
- **不建** `user_id` 索引（无按用户聚合的查询需求，YAGNI）。
- **不建** `updated_at` TTL（孤儿不可能产生：`HardDelete` 先删统计、失败即中止主记录删除；软删除（归档）保留统计与 SPEC-090「归档无 TTL」语义一致）。

### D3 — SessionStatStore 接口（放 `internal/infra/llmstats` 同包）

> 命名定稿（晓军 2026-09-17）：埋点是**增量累加语义**，方法名用 `Incr`（对齐 `metrics.Counter.Incr`），不用 `Upsert`（那是实现细节——内部用 `$inc` + upsert 实现幂等累加）。

```go
type SessionStatStore interface {
    // Incr 幂等累加一次 LLM 调用的 token 到该 session 的累计值（不存 user_id，见 §5.1 注）。
    Incr(ctx context.Context, sessionID string, promptTokens, completionTokens int, billedTokens int64, at time.Time) error
    // DeleteBySession 幂等删除（DeleteOne；不存在 = deletedCount 0 + err nil = 成功）。
    DeleteBySession(ctx context.Context, sessionID string) error
    // GetBySession 单点查 billed_tokens（点查接口 + run 详情内嵌用）；无记录返回 (0, nil)。
    GetBySession(ctx context.Context, sessionID string) (int64, error)
}
```

Mongo 实现 `internal/infra/llmstats/session_stats.go`：collection `session_stats`；`Incr` = `$inc` + `$setOnInsert`（`_id/session_id/created_at`）+ `$set updated_at`；文档结构同 §5.1（无 `user_id`）。

### D4 — RecordingLLM session 解析（子 session 归父零成本实现，决策⑦落地）

- `llmagent` 内部调用 model 的 `ctx` 是 `agent.InvocationContext`（vendor `agent/context.go:62`，继承 `context.Context`，含 `Session()`）。
- 解析顺序（`recording.go` 内新增 helper，仅需 sessionID）：
  1. `ic.Session().State().Get("session_id")` → string 且非空则用之；
  2. fallback `ic.Session().ID()`。
- **三场景 `state["session_id"]` 语义已天然统一（实测代码）**：主 chat = 自身（`chat_service.go:329` buildState）；task = 自身（`executor.go:155`）；**子 agent = 父 session ID**（`subagent/tool.go:59`）→ 子 session token 自动归父，无需专门解析 `parent_session_id`。
- 解析失败（ctx 断言失败/state 缺键）→ 仅全局埋点，不写 `session_stats`，不阻塞主流程。

### D5 — Recorder 双写落点

- `Recorder` 加 `sessionStats SessionStatStore` 字段（nil 安全 no-op）。
- `recording.go` 两处 `Record` 调用补 `SessionID`（经 D4 helper 从 ctx 解析；不取 UserID——session_stats 不存 user_id）。
- `Recorder.Record()` 内 billed 计算后：`sessionStats != nil && SessionID != ""` → `sessionStats.Incr(...)`。不修改 `Counter` 接口。

### D6 — HardDelete 级联落点（幂等语义定稿）

- `Manager` 注入 `SessionStatStore`（nil 安全，测试可省略）。
- `HardDelete` **首行** `DeleteBySession`（在 `repo.HardDelete` 主数据删除**之前**执行）：
  - **文档已不存在（deletedCount=0、err==nil）→ 视为成功，继续删除主数据**（幂等，绝不因「已删过」而中止）；
  - 仅 `err != nil`（真实 DB 错误）→ 失败即中止，主数据不删 → 无孤儿。
- 软删除不级联（决策④）。

### D7 — 时区口径与 get_current_time 细节（决策⑤⑥⑨的签名级定稿）

- `metrics.Reader.Series` 签名改为 `Series(ctx, m Metric, since, until time.Time, gran Granularity, loc *time.Location)`；`bucketStart`/`bucketHours` 加 `loc` 参数（`t.In(loc)` 后 Truncate/自然日边界）。
- `dashboard.go`：`timezone` query 参数 → `time.LoadLocation` 失败返回 400、缺省 `Asia/Shanghai`；默认窗口 `since = bucketStart(now.In(loc), gran, loc)`、`until = now`；`Sum` 无需 loc（只依赖绝对区间）。
- `main.go`（或 wire）`import _ "time/tzdata"` 内嵌时区库（容器无 tzdata）。
- `get_current_time`：`CurrentTimeResult` 保持 5 字段（time/date/weekday/timezone/unix），值全 UTC（`time` RFC3339 带 `Z`、`date` UTC、`weekday` 按 UTC、`timezone="UTC"`、`unix` 不变）；删除 `time.LoadLocation` 分支。
- 前端 dashboard：请求带 `timezone`（`resolvedOptions().timeZone`）；横轴 `toLocaleString()`（浏览器时区）显示，与分桶口径天然一致。
