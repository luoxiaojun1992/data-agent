# SPEC-100 优化 Token 用量埋点（session 独立统计表 + session 删除级联 + session token 展示）

> **SPEC-100** | Status: 设计中（立项，方向已定）
> 日期：2026-09-16

## 0. 决策记录（2026-09-16 晓军拍板）

| # | 决策 | 结论 |
|---|------|------|
| 1 | hourly record 是否关联 session | ❌ **不关联**。`stats_hourly` 保持 `{metric,hour}` 全局计数**完全不动**，不新增 `session_id` 字段 |
| 2 | session token 统计方式 | ✅ **独立建表**统计；删 session 时**关联删除**（早于 session 主记录删除，幂等删除） |
| 3 | 是否加时间冗余字段 | ❌ **不加**。现有 UTC `time.Time` 存储天然兼容时区（见 §2.2），一年数据量 ≤8760/指标，实时聚合足够 |
| 4 | session 删除语义 | 级联删除**仅指硬删除（`HardDelete`）**；软删除（归档 `Delete` 设 `deleted_at`）**不级联**——session 主记录保留、可恢复，token 统计同步保留，恢复后累计继续 |
| 5 | 日历维度分桶时区 | 按小时/星期几/几号/几月的聚合展示**按实际时区 Asia/Shanghai（UTC+8）计算**，非 UTC（见 §2.2-B；当前存在 UTC 口径偏差，本 spec 一并修正） |
| 6 | 分桶时区机制 | 确认分桶由**后端处理**（Go `Series`/`bucketHours`）→ **后端直接按 Asia/Shanghai（后端常用时区）分桶聚合**，不改 API、不传时区参数；前端 dashboard 横轴标签同步按 Asia/Shanghai 格式化，保证「分桶口径 = 展示口径」（见 §2.2-B） |
| 7 | 子 session token 归属 | 子 session **使用父 sessionID 计数**，`session_stats` 统一主（父）session 维度；删除子 session **不级联**删父 session token 统计（本身不关联，见 §4/§5.2） |
| 8 | stats_hourly 是否已有 session_id | ✅ **已确认没有**：代码 `HourlyStat` 无该字段 + 线上 197 条文档字段仅 `_id/metric/hour/updated_at/value`（2026-09-16 实测），无需删除 |
| 9 | get_current_time 时区口径 | 附带修正（归属 SPEC-080 工具）：改为**按 UTC 返回** + 输出体现时区（UTC 标注 + 业务时区参考），让 LLM 与系统 UTC 时间戳对齐（见 §2.3） |

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

**B. 日历分桶时区口径（澄清 #5，⚠️ 当前存在偏差）**：

- **现状**：`metrics.go` 的 `bucketStart`/`bucketHours` 全 `.UTC()` 分桶，`dashboard.go` 的 `now`/默认窗口也取 UTC → 按「小时/星期几/几号/几月」的**日历归属按 UTC 计算**，对 Asia/Shanghai 用户整体偏 8 小时（例：UTC 14 点被归为「14 点高峰」，实际是北京 22 点；「今天」从 UTC 00:00 起，对应北京 08:00，跨两个北京自然日）。
- **目标口径**：日历维度分桶**按实际时区 Asia/Shanghai（UTC+8）** 计算——`bucketStart`/`bucketHours` 改用 `time.LoadLocation("Asia/Shanghai")` 做自然日/周/月边界；`dashboard.go` 默认窗口的「今天」= 北京自然日。落库 `HourBucket` **保持 UTC**（存储层绝对时间戳不变，仅展示/分桶层变）。
- **分桶机制（决策 #6，已确认现状）**：趋势分桶由**后端处理**（Go 层 `Series`/`bucketHours`），因此采用**后端直接按 `Asia/Shanghai`（后端常用时区）分桶聚合**——不改 API 签名、不引入时区参数（「前端传时区字段」仅适用于前端分桶架构，此处不需要）。前端 dashboard 横轴标签用 `Intl.DateTimeFormat` 指定 `timeZone: 'Asia/Shanghai'` 格式化，保证「分桶口径 = 展示口径」，不依赖浏览器时区，与中国用户浏览器显示（其他时间字段的 `toLocaleString`）一致。

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
- **聚合分桶**（趋势统计日历维度）：是**服务端聚合语义**，必须**服务端固定时区**——不能依赖浏览器时区，否则同一份数据不同用户分桶边界不同、聚合结果跨用户不一致。故本 spec 固定 `Asia/Shanghai` 分桶，与「展示用浏览器时区」正确互补。

## 2.3 附带修正：get_current_time 时区口径（决策 #9，归属 SPEC-080 工具）

现状（`internal/adk/tools/tools.go` 的 `currentTime()`）：返回 **Asia/Shanghai 本地时间**（`t.In(loc)` 后 RFC3339，`Timezone: "Asia/Shanghai"`）——与系统内所有 UTC 时间戳口径**不一致**，LLM 拿到「now」后若与系统 UTC 时间戳（如 session 的 `created_at`、task 的 `scheduled_at`）做比较/运算会差 8 小时。

修正方向：

- `time`/`date`/`weekday` 按 **UTC** 返回（RFC3339 带 `Z`），与系统时间戳口径对齐。
- 输出**显式体现时区**：`timezone: "UTC"` + 新增业务时区参考字段（如 `biz_time` / `biz_timezone: "Asia/Shanghai"`），让 LLM 既知道 UTC 绝对时刻、也知道业务时区当前时间（「今天星期几/几号」等日历问答用 biz 字段）。
- `unix` 保持不变（绝对时刻，无时区）。

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
    // 新增：先删 session token 统计（早于主记录删除，幂等）
    if m.sessionStatStore != nil {
        if err := m.sessionStatStore.DeleteBySession(ctx, id); err != nil {
            return err   // 失败即中止，主记录不删 → 无孤儿
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

**幂等**：`DeleteBySession` 用 `DeleteOne({_id: session_id})`，删除不存在的文档返回 `deletedCount=0` 且**不报错**，天然幂等。

**子 session 边界（决策 #7，已定）**：子 session 的 token **归父 session 计数**（`session_stats._id` 统一为顶层父 session ID），子 session **不产生独立统计**。因此删除子 session（`adk/session/mongo.go` 的 `DeleteByID`/`deleteSubSessions`、`subagent/runner.go` 的 `cleanup`）**不级联**删任何 `session_stats` 文档——统计挂在父 session 上，与子 session 无关联。父 session 删除时由其自身 `HardDelete` 级联一次覆盖。

## 5. 详细设计（方向）

### 5.1 session_stats 数据模型

新建 collection `session_stats`，以 `session_id` 为主键（`_id`），单文档累计：

| 字段 | 类型 | 说明 |
|------|------|------|
| `_id` | string | = `session_id`（`sess_` 前缀），点查 O(1) |
| `session_id` | string | 冗余（与 `_id` 同），便于语义查询 |
| `user_id` | string | 归属，审计/数据隔离/清理 |
| `billed_tokens` | int64 | 累计计费 token（`(prompt+completion) × multiplier`） |
| `prompt_tokens` | int64 | 累计 prompt token（原始，未乘 multiplier） |
| `completion_tokens` | int64 | 累计 completion token（原始） |
| `llm_calls` | int64 | LLM 调用次数 |
| `created_at` | time.Time | 首条写入（UTC 绝对时间） |
| `updated_at` | time.Time | 最近一次 `$inc`（UTC 绝对时间） |

写入（`Upsert`，幂等）：

```
filter = {_id: session_id}
update = {
  $inc: { billed_tokens, prompt_tokens, completion_tokens, llm_calls },
  $set: { updated_at: now },
  $setOnInsert: { _id: session_id, session_id, user_id, created_at: now },
}
```

索引：
- `_id`（session_id）——主键，点查唯一命中（会话删除/查询都靠它）。
- `user_id` —— 按用户维度聚合/审计（可选，实现阶段评估是否需要）。
- `updated_at` TTL —— 兜底清理（可选；主路径是 `HardDelete` 级联，TTL 仅防意外泄漏，实现阶段评估）。

### 5.2 埋点双写

`llmstats.Recorder` 新增依赖 `SessionStatStore`（接口），`Record()` 中：

```go
if r.counter != nil {
    _ = r.counter.Incr(ctx, metrics.MetricTokenTokens, at, int64(billed))
    _ = r.counter.Incr(ctx, metrics.MetricLLMCalls, at, 1)
}
// 新增：session 维度累计（仅当 rec.SessionID 非空）
if r.sessionStats != nil && rec.SessionID != "" {
    _ = r.sessionStats.Upsert(ctx, rec.SessionID, rec.UserID, rec.PromptTokens, rec.CompletionTokens, billed, at)
}
```

- `SessionID` 为空（理论上不应发生，LLM 调用必有会话上下文）时仅走全局埋点，不写 `session_stats`，防御性处理。
- 不修改 `Counter` 接口签名，避免全量调用点改动。
- **子 session 归父（决策 #7）**：子 agent 运行时的 LLM 调用，埋点用**父 session ID**（非 subID）。实现要点：`RecordingLLM`（buildBackends 统一出口）从运行上下文解析——当前 session 为子 session 时取 `parent_session_id`（`adk/subagent/runner.go` 创建子 session 时已写入，StateDelta 携带），否则取自身 ID；解析失败则降级仅全局埋点（不写 `session_stats`），不阻塞主流程。

### 5.3 API 方向

- **session token 查询**：新增接口（形如 `GET /api/v1/sessions/:id/token-usage`，或 chat/task 列表接口内嵌 `billed_tokens`），直接按 `_id=session_id` 点查 `session_stats`，单文档返回，无需聚合。具体路径实现阶段定。
- **dashboard 趋势统计**：`Sum/Series` 无 schema 变更、不引入冗余字段；**修正日历分桶时区口径**——`bucketStart`/`bucketHours`/默认窗口由 UTC 改为 Asia/Shanghai（见 §2.2-B，决策 #6 后端处理方案）；前端横轴标签同步按 Asia/Shanghai 格式化（`Intl.DateTimeFormat` + `timeZone: 'Asia/Shanghai'`）。
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
| `internal/infra/metrics/mongo.go`（或独立 `session_stats.go`） | 新增 `SessionStatStore`（`Upsert`/`DeleteBySession`）实现 | Medium |
| `internal/service/chat/session.go` | `Manager` 注入 store；`HardDelete` 级联（先于主记录、幂等） | Medium |
| `cmd/server/wire.go` / `main.go` | DI 注入 `SessionStatStore`（赋值顺序：消费前） | Low |
| `cmd/server/migration/*` / `internal/infra/mongo/client.go` | `session_stats` 索引（`_id` 主键 + 可选 `user_id`/`updated_at`） | Low |
| `internal/api/handler/session.go`（或 chat/task handler） | 新增 session token 查询接口 + RBAC 归属校验（防 IDOR） | Medium |
| `internal/infra/metrics/metrics.go` | 日历分桶 `bucketStart`/`bucketHours` 由 UTC 改 Asia/Shanghai（澄清 #5/决策 #6） | Medium |
| `internal/api/handler/dashboard.go` | 默认窗口/`now` 改用 Asia/Shanghai 自然日（澄清 #5/决策 #6） | Low |
| `internal/adk/tools/tools.go` | `get_current_time` 改 UTC 输出 + 时区标注（附带修正 §2.3，归属 SPEC-080） | Low |
| 埋点出口（buildBackends `RecordingLLM`） | 子 session 归父解析（`parent_session_id` → 父 session ID 计数，决策 #7） | Medium |
| 前端 chat / task 页面 | session token 展示（回填单会话 `billed_tokens`） | Medium |
| 前端 dashboard 页面 | 趋势图横轴标签按 Asia/Shanghai 格式化（决策 #6） | Low |

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
- [ ] **必须** 验证 `Upsert` 幂等：同 session 多次 `$inc` 累计正确，`$setOnInsert` 首写字段不覆盖
- [ ] **必须** 验证 `HardDelete` 级联顺序：token 统计先删、主记录后删；删除不存在的 session 幂等不报错
- [ ] **必须** 验证 `Recorder` 双写：`SessionID` 非空写 `session_stats`，空则仅全局埋点，互不干扰
- [ ] **必须** 验证日历分桶时区口径（Asia/Shanghai）：UTC 00:00 的样本应归入北京「前一天」；北京自然日边界 00:00（= UTC 前一日 16:00）分桶正确；`bucketStart` 对 hour/day/week/month/year 的边界样本断言正确
- [ ] **必须** 验证软删除（归档 `Delete`）**不**级联删 `session_stats`，仅 `HardDelete` 级联
- [ ] **必须** 验证子 session 归父（决策 #7）：带 `parent_session_id` 上下文的埋点写**父 session** 文档；删子 session 后父 session 统计保留、无任何 `session_stats` 被级联
- [ ] **必须** 验证 `get_current_time` 按 UTC 返回且显式体现时区（`time` 带 `Z`、`timezone=UTC`、`biz_time`/`biz_timezone` 业务时区参考正确，北京 00:00-08:00 边界处 UTC 日期/星期与北京不一致的场景断言正确）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%；`go vet` 无警告

## 10. 验证标准

1. `session_stats` 写入正确：埋点后同一 session 多次 LLM 调用累计到单文档，`billed_tokens`/`llm_calls` 单调递增，`_id == session_id`。
2. `stats_hourly` 完全不受影响：`Sum/Series` 结果与改造前一致（无回归）。
3. `HardDelete` 级联正确：删除 session 后 `session_stats` 对应文档消失；删除不存在的 session 幂等；主记录删除失败时 token 统计不残留（或已先删）。
4. chat 页 / task 页正确显示单会话 token 消耗，数值与 `session_stats` 文档一致。
5. 趋势统计（日/周/月/年）结果正确，`stats_hourly` 查询路径无性能回退。
6. 日历维度（小时/星期几/几号/几月）聚合按 Asia/Shanghai 归属正确：跨 8 小时时差的边界样本归入北京自然日/自然周/自然月；软删除（归档）后 `session_stats` 保留，硬删除后消失。
7. 子 session token 归父正确：子 agent 运行产生的 token 计入父 session 文档（`_id == 父 session ID`）；删除子 session 后父 session 统计不受影响。
8. `get_current_time` 按 UTC 返回并体现时区（`time` 带 `Z`、`timezone=UTC`、含业务时区参考），与系统 UTC 时间戳口径一致。
