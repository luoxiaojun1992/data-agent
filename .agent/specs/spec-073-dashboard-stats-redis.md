# Dashboard 统计重构：Redis 多维计数 + 统一统计组件

> **SPEC-073** | Status: 详细设计（后续需进一步深化）

## 目标

1. 重构 Dashboard 统计：统计数据用 **Redis 计数器**存放，支持**日/周/月/年**四维分开存储并设 TTL；查看统计时**直接从 Redis 计数读取**，不再对原始数据（MongoDB）做聚合查询。
2. 统一统计 6 类指标：**token 消耗、LLM API 调用量、后端 API 调用量、产出物数、task run 完成数、ROI**（含内置 LLM 调用），全部通过一个**共用统计组件**埋点计数 + 查询。
3. 修复现有 dashboard「task_stats 缺失 / 6 个趋势全空」问题，并**清理历史遗漏代码**（MongoDB 聚合等）。
4. 确保**所有登录用户**都能查看 dashboard。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 基础设施（Redis） | ✅ 已实现 | Redis 已接入（缓存 + Stream） |
| SPEC-051 LLM 全链路 Token 统计与 Redis 缓存 | ✅ 已实现 | `llmstats.Recorder` 已覆盖所有 LLM 调用点（含内置） |
| SPEC-059 统计分析 Token 统计真数据 | ✅ 已实现 | `AggregateByTime`（MongoDB 聚合）现供 dashboard 用，本 spec 改为 Redis |
| SPEC-060 Dashboard trend 接入 | ✅ 已实现 | dashboard 路由 + 前端已就绪，但 task 趋势未接（现问题） |
| SPEC-064 RBAC | ✅ 已实现 | dashboard 权限检查依据 |

## 背景 / 动机（现状问题）

现有 dashboard（`internal/api/handler/dashboard.go` + `internal/service/monitor/trends.go`）未完全正确运行：

| 问题 | 现状 |
|------|------|
| `Get` 只返回 `kb_docs` | 注释声明返回 `task_stats` 但实际只返回 `kb_docs`（`dashboard.go:55-57`） |
| 6 个趋势全空 | `GetTrends` 调 `ComputeTrends(nil, tokenBuckets)`，task runs 传 nil → `call_trend`/`req_dist`/`success_trend`/`duration_dist`/`output_stats`/`roi_trend` 全空，仅 `token_trend` 有数据（`dashboard.go:72-77`） |
| 统计实时聚合 MongoDB | token 统计靠 `llmstats.AggregateByTime` 对 `llm_usage` collection 跑 aggregation pipeline，查询时算，非预聚合计数 |
| 无统一统计组件 | token/API/产出物/task 各埋点分散，无统一计数入口 |
| 无多维 + TTL | 仅 24h 单一时窗，无日/周/月/年维度、无 TTL |
| 缺指标 | 无 LLM 调用量、后端 API 调用量、产出物、task run 完成数、ROI 统计 |

## 架构概述

设计**统一共用的统计组件** `internal/infra/metrics`（与 `llmstats`/`qdrant` 同层），供各业务组件埋点计数 + dashboard 查询：

```
internal/infra/metrics/            ← 共用统计组件（Redis 后端）
  ├── metrics.go                    ← Counter/Reader 接口 + Metric/Period 定义
  └── redis.go                      ← Redis 实现（INCRBY + EXPIRE + MGET）
internal/api/middleware/            ← API 调用量埋点（gin middleware，api_calls 计数）
internal/service/...                ← 各业务组件埋点（llmstats.Record 内 / artifact / task）
internal/api/handler/dashboard.go   ← 查询走 metrics.Reader（不再聚合 MongoDB）
internal/service/monitor/trends.go  ← 清理或改造（去掉 MongoDB 聚合依赖）
```

| 组件 | 角色 |
|------|------|
| `metrics.Counter` | 埋点计数接口：`Incr(ctx, metric, at, delta)`（写 Redis，多 period 同时累加 + 设 TTL） |
| `metrics.Reader` | 查询接口：`Sum(ctx, metric, period, since, until)` / `Series(ctx, metric, period, buckets)` |
| 业务埋点 | LLM 调用 / API middleware / artifact / task 调用 `Counter` |
| dashboard | 查询走 `Reader`，不再查 MongoDB 聚合 |

## 详细设计

### 指标定义（6 类 metric）

| Metric | 含义 | 埋点位置 | 备注 |
|--------|------|---------|------|
| `token_tokens` | 总 token 消耗（prompt+completion） | `llmstats.Record` 内 | 含内置 LLM（enhance/compaction/memory/kb 等所有 call point） |
| `llm_calls` | LLM API 调用次数 | `llmstats.Record` 内 | 含内置 LLM |
| `api_calls` | data-agent 后端 HTTP API 调用次数 | gin middleware | 所有 `/api/v1/*` 请求 |
| `artifact_created` | 产出物生成数量 | artifact 创建处 | **只增不减**，不管后续删除 |
| `task_completed` | task run 完成数量 | task executor 完成处 | **只增不减**，不管后续删除 |

**ROI**（派生指标，不单独存储，查询时计算）：

```
ROI = (artifact_created + task_completed) / token_tokens
```

### Redis key 设计 + 多维 + TTL

**Key 格式**（string 计数器，每 bucket 独立 key，便于独立 TTL）：

```
stats:{metric}:{period}:{bucket}
```

| period | bucket 格式 | TTL |
|--------|------------|-----|
| `day` | `2026-08-30`（YYYY-MM-DD） | 31 天 |
| `week` | `2026-W35`（ISO week） | 12 周 |
| `month` | `2026-08`（YYYY-MM） | 12 月 |
| `year` | `2026` | 2 年 |

- **写入**：一次 `Incr` 同时累加 day/week/month/year 四个 key（`INCRBY`），并对每个 key `EXPIRE` 设 TTL（刷新）。
- **读取**：`Sum` 用 `MGET` 读一批 bucket key 求和；`Series` 读连续 bucket 序列（如最近 7 天、12 周、12 月、N 年）。
- 计数器只增不减（artifact/task 不管删除；token/call 自然只增），无回退。

### 统一统计组件接口

```go
// internal/infra/metrics/metrics.go
package metrics

type Metric string

const (
    MetricTokenTokens   Metric = "token_tokens"
    MetricLLMCalls      Metric = "llm_calls"
    MetricAPICalls      Metric = "api_calls"
    MetricArtifact      Metric = "artifact_created"
    MetricTaskCompleted Metric = "task_completed"
)

type Period string

const (
    PeriodDay   Period = "day"
    PeriodWeek  Period = "week"
    PeriodMonth Period = "month"
    PeriodYear  Period = "year"
)

// Counter 埋点计数（各业务组件注入）
type Counter interface {
    // Incr 对指定 metric 在 at 时刻累加 delta（同时写 day/week/month/year 四 key + 刷新 TTL）
    Incr(ctx context.Context, m Metric, at time.Time, delta int64) error
}

// Reader 查询（dashboard handler 注入）
type Reader interface {
    // Sum 汇总 [since, until] 区间内某 metric 的计数值（按 period 的 bucket 求和）
    Sum(ctx context.Context, m Metric, p Period, since, until time.Time) (int64, error)
    // Series 返回区间内按 period 分桶的时间序列（bucket → 计数）
    Series(ctx context.Context, m Metric, p Period, since, until time.Time) ([]Bucket, error)
}

type Bucket struct {
    Time  time.Time
    Value int64
}
```

### 埋点位置

| 埋点 | 位置 | 说明 |
|------|------|------|
| token + LLM 调用 | `llmstats.Recorder.Record` 内部（注入 `metrics.Counter`） | 每次 `Record` 时 `Incr(token_tokens, billed_tokens)` + `Incr(llm_calls, 1)`；MongoDB `llm_usage` 记录保留（审计用途），但 dashboard 不再聚合它 |
| API 调用量 | 新增 gin middleware（`api_calls`） | 全局 `/api/v1/*` 请求计数，注入 `metrics.Counter` |
| 产出物 | artifact service 创建成功后 | `Incr(artifact_created, 1)`（只增，不管删除） |
| task run | task executor 完成（成功）后 | `Incr(task_completed, 1)`（只增） |

### 查询 API 设计

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/dashboard` | KPI 汇总：`kb_docs`、`task_stats`（count by status）、`token_tokens`、`llm_calls`、`api_calls`、`artifact_created`、`task_completed`、`roi` |
| GET | `/api/v1/dashboard/trends?period=day\|week\|month\|year` | 按 period 返回各指标时间序列（token/llm_calls/api_calls/artifact/task_completed/roi） |

- 所有统计从 `metrics.Reader`（Redis）读，**不再调用 MongoDB 聚合**。
- `kb_docs` / `task_stats` 仍从 Mongo 查（非计数型快照，数据量小，见下）。
- ROI 由 `(artifact_created + task_completed) / token_tokens` 实时计算（token 为 0 时 ROI=0）。

### 权限检查（所有登录用户可看）

- 现有 dashboard 路由 `RegisterDashboardRoutes(router, deps.JWTManager.AuthMiddleware(), ...)` 仅 JWT 认证，**无 RBAC 权限限制** → 所有登录用户已可访问（需确认前端 Sidebar 未按角色隐藏 dashboard 入口）。
- 结论：权限满足，无需改；若发现前端按角色隐藏，需改为所有角色可见。

### 历史遗漏代码清理

| 清理项 | 说明 |
|--------|------|
| `monitor.ComputeTrends` 的 task-based 计算 | 改为从 `metrics.Reader` 读，去掉对 `[]task.TaskRun` 的依赖 |
| `llmstats.Aggregate` / `AggregateByTime` | dashboard 改走 Redis 后不再使用；若 `/admin/stats/llm`（`stats.go GetLLMStats`）仍用，则一并改造或明确保留 |
| `DashboardHandler.Get` 的 `task_stats` 缺失 | 补上（从 task service 查 count by status，或后续深化决定是否也走计数） |
| `dashboard.go` 的 `TODO: add TaskRunService` | 按新设计落实 |

### 不处理历史数据

- 新 Redis 计数器从 0 开始，**不回填**历史 `llm_usage` / task / artifact 数据。
- 旧 MongoDB 聚合代码清理后，历史数据不再参与 dashboard 统计（测试环境，可接受）。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No — 用 Redis 计数器（现有 Redis 实例，无新基础设施） |
| 是否影响现有 API | Yes — dashboard 查询语义改变（Redis 计数替代 MongoDB 聚合）；新增 metrics 埋点 |
| 性能影响 | 查询从 MongoDB 聚合 → Redis O(1)/O(bucket) 读取，大幅降低；写入多 4 次 INCRBY（每指标） |
| 是否需要新增 Skill | No |
| 内存/存储 | 每 metric 每 period 每 bucket 一个 string key，量级小（6 metric × 4 period × bucket 数） |
| 复用现有能力 | 复用 Redis client、`llmstats.Record` 埋点、gin middleware |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/metrics/metrics.go` + `redis.go` | 统一统计组件（Counter/Reader + Redis 实现） | New |
| `internal/infra/llmstats/llmstats.go` | `Record` 内注入 `metrics.Counter`，token + llm_calls 埋点 | Modify |
| `internal/api/middleware/metrics.go` | API 调用量埋点 middleware（api_calls） | New |
| `internal/service/artifact/*` | 产出物创建埋点（artifact_created） | Modify |
| `internal/worker/pool.go`（或 task executor） | task run 完成埋点（task_completed） | Modify |
| `internal/api/handler/dashboard.go` | 查询走 `metrics.Reader`，补 task_stats，加 period 参数 | Modify |
| `internal/service/monitor/trends.go` | 清理/改造（去 MongoDB 聚合依赖） | Modify |
| `internal/api/handler/stats.go` | 若保留 `/admin/stats/llm`，明确数据源 | Modify |
| `cmd/server/wire.go` | 初始化 metrics 组件 + 注入各埋点 | Modify |
| `frontend/app/page.tsx` | dashboard 前端适配新 API（summary + period 趋势） | Modify |

## 测试策略

1. **Unit tests（Go）**：L2 `metrics.Counter`/`Reader` 接口 mock；L3 Redis 实现走真实 Redis（`go test -tags=integration`）或 miniredis。覆盖率 gate 见 SPEC-045。
2. **Integration**：验证 Redis key 生成、TTL 设置、INCRBY 累加、MGET 求和、period 分桶边界。
3. **E2E**：dashboard 页面加载 summary + 各 period 趋势（用例编号 `UI-XXX`）。
4. **ROI 边界**：token=0 时 ROI=0，不 panic。

## UI Test / E2E 验收规则

> 开发完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** dashboard 页面前端交互变更时同步编写 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

参考: `.agent/memory/E2E_TESTING.md`

## Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | 纯函数（ROI 计算、bucket/period key 生成） | 100% |
| L2 | `Counter`/`Reader` 接口 mock | 100% |
| L3 | Redis 实现 / dashboard handler / middleware / 埋点 | 98% |
| Overall | 全量 | ≥98% |

- [ ] 每个 Success 测试 ≥2 个行为验证断言
- [ ] `Incr` 验证同时写 4 个 period key + TTL 设置
- [ ] `Sum`/`Series` 验证分桶边界（day/week/month/year）与求和正确
- [ ] ROI 验证 token=0 边界
- [ ] **严禁** `t.Skip()` 绕过不可测场景

## 验证标准

- [ ] 埋点后 Redis 出现 4 个 period 的 `stats:*` key，TTL 正确
- [ ] `GET /api/v1/dashboard` 返回完整 KPI（含 task_stats、token、llm_calls、api_calls、artifact、task_completed、roi）
- [ ] `GET /api/v1/dashboard/trends?period=day|week|month|year` 各返回对应分桶序列
- [ ] 查询不再触发 MongoDB `llm_usage` 聚合（可加日志/断点验证）
- [ ] 所有登录用户（user/admin/system_admin）均可访问 dashboard
- [ ] 历史遗漏代码（ComputeTrends nil 依赖、AggregateByTime 旧调用）已清理
- [ ] ROI = (artifact + task_completed) / token_tokens，token=0 时 ROI=0

## 待深化项（后续）

1. `task_stats`（count by status）是走 Mongo 快照查询还是也进 Redis 计数（与 task_completed 累计数的关系）。
2. 前端 dashboard 图表具体布局（summary 卡片 + period 切换器 + 各指标趋势图）。
3. API 调用量是否区分「内置/用户」「成功/失败」子维度。
4. token 统计是否按 model / call_point 细分（现有 `llmstats.Aggregate` 的 call_point 维度是否保留）。
5. 指标 key 的 Redis namespace 前缀是否与现有缓存隔离（避免与 Cache-Aside 混用）。
