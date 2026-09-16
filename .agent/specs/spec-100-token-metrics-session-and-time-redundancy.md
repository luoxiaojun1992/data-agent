# SPEC-100 优化 Token 用量埋点（sessionID 维度 + 时间冗余字段 + 索引 + 趋势聚合优化 + session token 展示）

> **SPEC-100** | Status: 设计中（立项，暂不展开调研深化）
> 日期：2026-09-16

## 1. 目标

1. 在 `stats_hourly` 埋点记录中引入 **sessionID 维度**，使 token 用量从「全局聚合」细化到「按 session 聚合」，支撑 chat / task 页面展示单会话 token 消耗。
2. 为埋点记录补充**时间冗余字段**（小时、星期几、几号、几月）并建索引，让不同时间维度的趋势统计（日/周/月/年 + 周内/月内/时段分布）聚合直接走 MongoDB 索引，减少 Go 层分桶与全量文档读取。
3. 优化 dashboard 趋势统计逻辑以应用新字段，并在 chat / task 页面新增 session token 用量展示（用 sessionID 查询聚合）。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-072 Dashboard 统计重构（stats_hourly + 统一 Counter/Reader） | ✅ | stats_hourly 集合、HourlyStat 结构、{metric,hour} 唯一索引、Sum/Series 已就绪，本 spec 在其上扩展 |
| SPEC-051 LLM 全链路 Token 统计 | ✅ | llmstats.Record 已含 SessionID 字段（见 §2），埋点链路完整 |
| SPEC-059 Token 统计真数据 | ✅ | 埋点已接入真实 usage |
| — | — | 无新增前置，可立即开始 |

## 2. 背景与动机

SPEC-072 落地了 `stats_hourly` 小时粒度计数（`internal/infra/metrics`），但存在两个能力缺口：

### 2.1 埋点丢失 session 维度

`internal/infra/llmstats/llmstats.go` 的 `Record` 结构体**已包含 `SessionID` / `UserID` 字段**，但 `Recorder.Record()` 调用 `Counter.Incr()` 落库时**丢弃了 session 维度**——因为 `Counter.Incr(ctx, m, at, delta)` 接口本身没有 session 参数，`HourlyStat` 结构也没有 `session_id` 字段。

**后果**：token 用量只能全局聚合，无法回答「这个 chat 会话 / 这个 task run 消耗了多少 token」，chat/task 页面无法展示单会话用量。

### 2.2 趋势统计聚合未充分利用 MongoDB

当前 `MongoReader.Series` 的做法是：`Find` 拉回区间内**全部 hour 文档**，再在 Go 层 `bucketHours` 内存分桶（`metrics.go` 的 `bucketStart/bucketAdvance/bucketHours`）。对日/周/月/年连续时间轴尚可（一年 ≤8760 文档/metric），但：

- 无法高效做**周期性分布**统计（如「每周二 vs 每周五」「每月 1 号 vs 15 号」「一天中哪几个小时用量高」）——这类查询需要按「日历位置」分组，现有方案只能全量拉回后 Go 层遍历，且无对应索引。
- 缺少按 session 聚合的索引路径。

**方向**：在写入时为每条记录冗余计算「日历位置」字段（小时、星期几、几号、几月），查询时用 `$match` + `$group` 直接对这些字段分桶并命中索引，把聚合下沉到 MongoDB。

## 3. 架构概述（方向）

```
埋点调用点（llmstats.Recorder.Record 等）
        │  rec.SessionID + CreatedAt
        ▼
Counter.Incr(ctx, metric, at, delta, sessionID)   ← 接口扩展
        │  计算时间冗余字段（由 CreatedAt 派生）
        ▼
stats_hourly 文档 { _id, metric, hour, session_id, hour_of_day,
                    day_of_week, day_of_month, month, value, updated_at }
        │
        ├── 全局趋势：{metric,hour} 仍可 $group 重聚合（见 §5.2 关键决策）
        └── session 聚合：{metric,session_id} 索引直查
```

与现有模块的关系：**不改** Metric 枚举、Granularity 枚举、ROI 派生逻辑、TTL 一年上限；**改** `Counter` 接口、`HourlyStat` 结构、`upsertHourly`、`Series`/`Sum` 聚合路径、索引集合。

## 5. 详细设计（方向性，暂不展开）

### 5.1 数据模型（方向）

`HourlyStat` 新增字段（均 omitempty，向后兼容存量文档）：

| 字段 | 类型 | 说明 |
|------|------|------|
| `session_id` | string | 会话 ID（chat session / task run 所属 session）；全局聚合类指标可为空 |
| `hour_of_day` | int | 0–23，一天中的第几小时 |
| `day_of_week` | int | 1–7，星期几（ISO，周一=1） |
| `day_of_month` | int | 1–31，几号 |
| `month` | int | 1–12，几月 |

> **时区口径待定**：现有 `hour` 为 UTC 截断。冗余字段基于 UTC 还是 `Asia/Shanghai` 需调研确认（涉及「星期几/几号」的业务语义）。

### 5.2 关键设计决策（待调研深化，立项阶段仅列出）

1. **文档粒度**：加 `session_id` 后，`{metric,hour}` 唯一索引不再唯一（同一小时可有多 session）。需决策：
   - 方案 A：唯一索引改为 `{metric,hour,session_id}`，全局 `Sum/Series` 改为 `$group` 重聚合（语义变化，读取成本上升）。
   - 方案 B：**双轨**——全局桶保留 `{metric,hour}`（session_id 空）继续 `$inc`；另起 session 维度记录。token 指标双写，其余指标维持全局。
   - 方案 C：`session_id` 仅 token 类指标落库，非 token 指标保持现状。
2. **接口扩展方式**：`Counter.Incr` 改签名 vs 新增 `IncrWithSession`（避免全量调用点改签名）。建议后者（现有 4 处调用点仅 token 类需带 session）。
3. **索引组合与膨胀控制**：`{metric,hour}`、`{metric,session_id}`、各时间冗余字段组合索引的数量需评估（防止写放大 + 内存占用）。
4. **时间冗余字段作用域**：是否所有 5 个 metric 都写冗余字段，还是仅 token 类。

### 5.3 API 方向（待调研）

- dashboard 趋势统计：优化现有 `Series/Sum` 聚合逻辑，无新增对外 API（或复用 `/api/v1/stats/*`）。
- session token 展示：新增查询接口（形如 `GET /api/v1/sessions/:id/token-usage` 或 chat/task 列表内嵌聚合），用 `{metric,session_id}` 索引直查。具体路径待调研。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `stats_hourly`，新增字段 + 索引） |
| 是否影响现有 API | 影响：`Counter` 接口 + `Series/Sum` 聚合语义（见 §5.2）；可能新增 session token 查询接口 |
| 性能影响 | 正向：趋势聚合下沉 MongoDB + 索引命中；代价：写路径多算 4 个冗余字段 + 潜在索引写放大（需评估） |
| 是否需要新增 Skill | No |
| 数据迁移 | 存量文档无冗余字段/无 session_id（omitempty 兼容）；如需回填历史冗余字段需一次性脚本（待调研） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/metrics/metrics.go` | Counter/Reader 接口、Metric/Granularity/Bucket、bucketHours | Medium |
| `internal/infra/metrics/mongo.go` | HourlyStat 结构、upsertHourly、Sum/Series 聚合 | High |
| `internal/infra/llmstats/llmstats.go` | Recorder.Record 传 SessionID 给 Counter | Low |
| `internal/api/handler/dashboard.go` | 趋势统计逻辑应用冗余字段 | Medium |
| `cmd/server/migration/stats_seed.go` | 新增索引 | Low |
| `internal/api/middleware/metrics.go` / `internal/service/artifact/storage.go` / `internal/adk/tools/tools.go` | 其余埋点调用点（是否传 session 视 §5.2 决策） | Low |
| 前端 chat / task / dashboard 页面 | session token 展示 + 趋势图 | Medium |

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
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | 时间冗余字段派生函数、bucketHours 类纯函数 |
| L2 | 依赖接口，可 mock | **100%** | Counter/Reader 接口实现 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `metrics/mongo.go`、`llmstats.go`、`handler/dashboard.go` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** 验证时间冗余字段派生正确性（跨月/跨年/跨周边界：周日→周一、月末→次月 1 号、12 月→1 月）
- [ ] **必须** 验证带 session 埋点后 `Sum/Series` 的全局重聚合语义不回归
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%；`go vet` 无警告

## 10. 验证标准

1. `stats_hourly` 新增字段写入正确：埋点后新文档含 `session_id` + 4 个时间冗余字段，值符合 UTC（或确定时区）日历语义。
2. 唯一索引调整后，同一小时多 session 文档可正常落库，`Sum/Series` 全局聚合结果与改造前一致（无回归）。
3. 新增索引生效：`explain()` 显示按时间冗余字段分桶的聚合命中索引（`IXSCAN`），非全表扫描。
4. chat 页 / task 页正确显示单会话 token 消耗，数值与 session 埋点聚合一致。
5. 趋势统计（日/周/月/年 + 周内/月内/时段分布）结果正确，性能较改造前提升（量化对比待调研阶段给出基线）。
