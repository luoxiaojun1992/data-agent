# Dashboard 统计重构：MongoDB 小时粒度计数 + 统一统计组件

> **SPEC-072** | Status: ✅ 已实现（2026-08-31 落地 + 测试服务器验证）

## 目标

1. 重构 Dashboard 统计：统计数据用 **MongoDB `stats_hourly` collection** 存放，**按小时维度计数（一小时一条 document）**，查询时做必要聚合（sum）；数据**只保留一年**，查询**最多支持一年**。
2. **全局统计，不区分用户**：所有指标是系统级计数，不按 user 维度拆分。
3. 统计**数据源统一通过埋点计数**：token、LLM 调用、API 调用、产出物、task run 五类指标，全部通过一个**共用统计组件**埋点累加。
4. **废弃现有代码设计**（`llmstats` 的 MongoDB `llm_usage` 明细聚合、`monitor.ComputeTrends`、现有 dashboard handler 的聚合逻辑），重新设计，不再受旧代码约束。
5. 确保**所有登录用户**都能查看 dashboard。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 基础设施（MongoDB） | ✅ 已实现 | MongoDB 已接入 |
| SPEC-051 LLM 全链路 Token 统计 | ✅ 已实现 | LLM 埋点调用点已全覆盖（含内置），本 spec 改其内部为小时计数 |
| SPEC-060 Dashboard trend 接入 | ✅ 已实现 | dashboard 路由 + 前端已就绪 |
| SPEC-064 RBAC | ✅ 已实现 | dashboard 权限检查依据 |

## 背景 / 动机（现状问题）

现有 dashboard（`internal/api/handler/dashboard.go` + `internal/service/monitor/trends.go` + `internal/infra/llmstats/llmstats.go`）未完全正确运行，且设计不合理：

| 问题 | 现状 |
|------|------|
| `Get` 只返回 `kb_docs` | 注释声明返回 `task_stats` 但实际只返回 `kb_docs` |
| 6 个趋势全空 | `GetTrends` 调 `ComputeTrends(nil, ...)`，task runs 传 nil → 仅 `token_trend` 有数据 |
| 统计实时聚合明细 | token 靠 `llmstats.AggregateByTime` 对 `llm_usage` 明细跑 aggregation，查询时算 |
| 无统一统计组件 | token/API/产出物/task 各埋点分散，无统一计数入口 |
| 无小时粒度/保留策略 | 明细无限增长，无聚合预计算、无过期清理 |
| 缺指标 | 无 LLM 调用量、API 调用量、产出物、task run 完成数、ROI |

## 架构概述

设计**统一共用的统计组件** `internal/infra/metrics`（MongoDB 后端），供各业务组件埋点计数 + dashboard 查询。**现有 `llmstats` 明细聚合 / `monitor.ComputeTrends` 废弃删除**：

```
internal/infra/metrics/            ← 共用统计组件（MongoDB stats_hourly 后端）
  ├── metrics.go                    ← Counter/Reader 接口 + Metric 定义
  └── mongo.go                      ← MongoDB 实现（upsert $inc + 聚合查询）
internal/api/middleware/            ← API 调用量埋点（gin middleware，api_calls）
LLM 调用点（原 llmstats.Record 调用处）  ← token_tokens + llm_calls 埋点（内部改走 Counter）
artifact / task 创建完成处           ← artifact_created / task_completed 埋点
internal/api/handler/dashboard.go   ← 重写：查询走 metrics.Reader
```

| 组件 | 角色 |
|------|------|
| `metrics.Counter` | 埋点计数：`Incr(ctx, metric, at, delta)`（upsert 小时 document，`$inc` 累加） |
| `metrics.Reader` | 查询：`Sum` / `Series`（对小时 document 做 sum / 分桶聚合，≤一年） |
| 业务埋点 | LLM 调用 / API middleware / artifact / task 调用 `Counter` |
| dashboard | 重写，查询走 `Reader` |

## 详细设计

### 存储模型（MongoDB `stats_hourly`）

**每小时一条 document（per metric）**，全局不区分用户：

```go
// collection: stats_hourly
type HourlyStat struct {
    ID        string    `bson:"_id"`      // 纯 UUID（应用层生成，不承载业务语义）
    Metric    string    `bson:"metric"`    // metric 名（业务字段）
    Hour      time.Time `bson:"hour"`      // 小时桶起点（时间维度，单独字段，truncate to hour，UTC）
    Value     int64     `bson:"value"`     // 该小时累计计数（只增不减）
    UpdatedAt time.Time `bson:"updated_at"`
}
```

- **主键约定（铁律）**：`_id` 用**纯 UUID**（应用层生成），不承载业务语义。`metric` + `hour` 是业务字段，靠**唯一索引** `{metric, hour}` 保证同一 metric+hour 只有一条 document。
- **索引**：`{metric: 1, hour: 1}`（唯一，用于 upsert 定位 + 范围查询）；`{hour: 1}`（TTL）。
- **TTL 保留一年**：`hour` 字段建 TTL index `expireAfterSeconds: 31536000`（365 天），MongoDB 自动删除一年前 document。数据最多保留一年。
- **写入（upsert by 唯一索引，非 `_id`）**：`Incr` 用 `filter{metric, hour}` 定位 + `$inc value` + `$setOnInsert {_id: newUUID()}`：

```go
filter := bson.M{"metric": m, "hour": hourBucket}          // 靠唯一索引定位同小时
update := bson.M{
    "$inc":         bson.M{"value": delta},
    "$set":         bson.M{"updated_at": now},
    "$setOnInsert": bson.M{"_id": newUUID()},              // 仅首次插入时生成 UUID
}
coll.UpdateOne(ctx, filter, update, options.Update().SetUpsert(true))
```

  一小时内多次埋点：首次插入（生成 UUID），后续命中唯一索引累加 `value`，无需按 user 拆分。

### Seed 初始化（幂等，无迁移/无历史数据回填）

新表 `stats_hourly` 通过 **seed 机制**幂等初始化索引（对齐现有 `migration.SeedRBAC` / Qdrant `EnsureCollection` 模式），**不做数据迁移、不回填历史数据**：

```go
// cmd/server/migration/stats_seed.go
package migration

// SeedStats 幂等初始化 stats_hourly collection 的索引。
// 无业务数据预置：计数从 0 开始，全部由埋点动态累加。
func SeedStats(ctx context.Context, db *mongo.Database) error {
    coll := db.Collection("stats_hourly")
    // 唯一索引 {metric, hour}：用于 upsert（filter 按 metric+hour 定位同小时 document）
    if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
        Keys:    bson.D{{Key: "metric", Value: 1}, {Key: "hour", Value: 1}},
        Options: options.Index().SetUnique(true).SetName("uniq_metric_hour"),
    }); err != nil {
        return err
    }
    // TTL index {hour}：365 天自动清理
    if _, err := coll.Indexes().CreateOne(ctx, mongo.IndexModel{
        Keys:    bson.D{{Key: "hour", Value: 1}},
        Options: options.Index().SetExpireAfterSeconds(31536000).SetName("ttl_hour"),
    }); err != nil {
        return err
    }
    return nil
}
```

- 调用点：`cmd/server/main.go` 启动时（与 `migration.SeedRBAC` 同处）`migration.SeedStats(ctx, mongoClient.DB())`，失败 `WARN` 不阻断启动。
- **幂等**：MongoDB 索引创建幂等（同 spec 索引重复创建不报错，重复启动安全）。
- **无 seed 业务数据**：5 个 metric 是代码常量（非 DB 配置），无需预置；计数从 0 靠埋点累加。
- **不迁移历史数据**：旧 `llm_usage` 等历史数据不回填进 `stats_hourly`。

### 指标定义（5 类计数 + 1 派生，全局）

| Metric | 含义 | 埋点位置 | 备注 |
|--------|------|---------|------|
| `token_tokens` | 总 token 消耗（prompt+completion） | LLM 调用点 | 含内置 LLM（enhance/compaction/memory/kb 等所有 call point） |
| `llm_calls` | LLM API 调用次数 | LLM 调用点 | 含内置 LLM |
| `api_calls` | data-agent 后端 HTTP API 调用次数 | gin middleware | 所有 `/api/v1/*` 请求 |
| `artifact_created` | 产出物生成数量 | artifact 创建处 | **只增不减**，不管后续删除 |
| `task_completed` | task run 完成数量 | task executor 完成处 | **只增不减**，不管后续删除 |

**ROI**（派生指标，不单独存储，查询时计算）：

```
ROI = (artifact_created + task_completed) / token_tokens
```

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

// Counter 埋点计数（各业务组件注入）
type Counter interface {
    // Incr 对指定 metric 在 at 时刻累加 delta（upsert 到小时 document，$inc）
    Incr(ctx context.Context, m Metric, at time.Time, delta int64) error
}

// Reader 查询（dashboard handler 注入）
type Reader interface {
    // Sum 汇总 [since, until] 区间内某 metric 的计数值（对小时 document sum）
    Sum(ctx context.Context, m Metric, since, until time.Time) (int64, error)
    // Series 返回区间内按指定粒度分桶的时间序列（日/周/月/年）
    Series(ctx context.Context, m Metric, since, until time.Time, gran Granularity) ([]Bucket, error)
}

type Granularity string

const (
    GranularityDay   Granularity = "day"
    GranularityWeek  Granularity = "week"
    GranularityMonth Granularity = "month"
    GranularityYear  Granularity = "year"
)

type Bucket struct {
    Time  time.Time
    Value int64
}
```

### 查询聚合（对小时 document 做 sum / 分桶）

- **Sum**：`match {metric, hour ∈ [since, until)}` + `$group {_id: null, total: $sum value}`。
- **Series**：按 `Granularity` 对 `hour` 分桶后 `$sum value`（如 day 分桶 = 按天聚合小时计数）。可在 MongoDB 用 `$dateTrunc`/`$dateToString` 分桶，或拉取小时 document（≤8760 条/年，量小）后在 Go 层分桶——**优先 Go 层分桶**（简单可控，避免 MongoDB 版本依赖）。
- **范围限制**：`since` 不得早于 `now - 365 天`（最多查一年），超出则 clamp 或报 400。
- **全局统计**：无 user 维度，所有用户看到同一份全局统计。

### 埋点位置（统一走 Counter）

| 埋点 | 位置 | 说明 |
|------|------|------|
| token + LLM 调用 | LLM 调用点（原 `llmstats.Record` 调用处，内部改走 `Counter`） | 每次调用 `Incr(token_tokens, billed_tokens)` + `Incr(llm_calls, 1)`；**不再写 `llm_usage` 明细** |
| API 调用量 | 新增 gin middleware | 全局 `/api/v1/*` 请求 `Incr(api_calls, 1)` |
| 产出物 | artifact 创建成功后 | `Incr(artifact_created, 1)` |
| task run | task executor 完成（成功）后 | `Incr(task_completed, 1)` |

- **token 不细分**：全局 5 个 metric，**不按 model / call_point 拆分**（废弃原 `llmstats.Aggregate` 的 call_point 维度，保持简单）。若未来需要细分，另立 spec 扩展 metric 维度。

### 埋点批量合并（内存缓冲 + 定时刷盘，并发安全）

API 调用量埋点频率高（每请求一次），为降低 MongoDB 写压力，`Counter` 实现**内存缓冲 + 定时刷盘**，且**必须并发安全**（多个业务 goroutine 同时埋点）：

```go
// Counter 内部：按 (metric, hourBucket) 聚合 delta 到内存 map，定时 flush 批量 upsert
type mongoCounter struct {
    mu     sync.Mutex
    buffer map[bucketKey]int64   // key = {metric}:{hour}，仅锁内读写
    client *mongo.Collection
    flushInterval time.Duration  // 默认 5s
    stop   chan struct{}
}

// Incr 并发安全：锁内仅做一次 map 累加（O(1)，无 IO），不阻塞其他埋点
func (c *mongoCounter) Incr(ctx context.Context, m Metric, at time.Time, delta int64) error {
    key := bucketKey{m, hourBucket(at)}
    c.mu.Lock()
    c.buffer[key] += delta
    c.mu.Unlock()
    return nil
}

// flush 用 swap 模式：锁内换出旧 buffer（O(1)），批量 upsert 在锁外执行，不阻塞埋点
func (c *mongoCounter) flush(ctx context.Context) {
    c.mu.Lock()
    old := c.buffer
    c.buffer = make(map[bucketKey]int64)   // 换入新空 map
    c.mu.Unlock()
    for key, delta := range old {          // 锁外批量 upsert
        upsertHourly(ctx, c.client, key, delta)
    }
}
```

**并发安全要点（必须遵守）**：

| 要点 | 说明 |
|------|------|
| `Incr` 锁内 O(1) | 互斥锁仅保护 map 累加（纳秒级），无 IO/网络调用在锁内 → 高频埋点不构成锁竞争瓶颈 |
| **swap 模式 flush** | flush 时锁内只做「换出旧 buffer + 换入新空 map」（O(1)），批量 upsert 在**锁外**执行 → flush 期间埋点不阻塞、不丢失 |
| 禁止锁内 IO | ⛔ 严禁在持锁状态下执行 MongoDB 操作（会阻塞所有埋点 goroutine） |
| 竞态防护 | buffer map 的所有读写都必须在 `mu` 保护下（`go test -race` 语义验证，UT 覆盖并发 `Incr` + `flush` 交错） |
| flush 失败处理 | 单条 upsert 失败 `log.Printf` + 丢弃该条增量（统计口径，容忍 ≤5s 增量丢失），不重试不阻塞 |
| 退出 flush | `Stop()` 持锁换出最后 buffer 后锁外 flush 完再退出，避免丢失最后几秒计数 |

- 埋点 `Incr` 只写内存 buffer（O(1)，无 IO）；后台 goroutine 每 `flushInterval`（默认 5s）flush 一次。
- 低频埋点（artifact/task/token）走同一 buffer，无需特殊处理。
- 计数是统计口径，**容忍进程崩溃丢失 ≤5s 的增量**（可接受）。

### 查询 API 设计

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/dashboard` | KPI 汇总（近一年累计）：`kb_docs`、`token_tokens`、`llm_calls`、`api_calls`、`artifact_created`、`task_completed`、`roi` |
| GET | `/api/v1/dashboard/trends?granularity=day\|week\|month\|year&since=&until=` | 按粒度返回各指标时间序列（默认近一年，最多一年） |

- 所有计数指标从 `metrics.Reader`（`stats_hourly`）读，sum/分桶聚合。
- `kb_docs` 是**非计数型快照**，从 Mongo 业务表查（数据量小，一次 count）。
- **task_stats 口径简化**：dashboard **不做** task 状态分布（pending/running/failed 快照，废弃原 `TaskStats` 结构）；task 维度仅展示 `task_completed`（埋点累计数）。若后续需要状态分布，另立 spec。
- ROI 由 `(artifact_created + task_completed) / token_tokens` 实时计算（token=0 时 ROI=0）。
- **时区口径**：`hour` 桶统一 **UTC** 截断存储；查询参数 `since`/`until` 由前端转 UTC 传入，返回的 bucket 时间为 UTC，**前端展示时转本地时区**。

### 权限检查（所有登录用户可看）

- 现有 dashboard 路由 `RegisterDashboardRoutes(router, deps.JWTManager.AuthMiddleware(), ...)` 仅 JWT 认证，**无 RBAC 权限限制** → 所有登录用户已可访问（需确认前端 Sidebar 未按角色隐藏 dashboard 入口）。
- 统计全局不区分用户，所有角色看到同一份全局统计。

### 前端 Dashboard 布局（`frontend/app/page.tsx`）

| 区域 | 内容 |
|------|------|
| KPI 汇总卡片（8 个） | `kb_docs`、`token_tokens`、`llm_calls`、`api_calls`、`artifact_created`、`task_completed`、`roi`（7 个指标卡片，近一年累计） |
| 粒度切换器 | `day / week / month / year` 单选（默认 day），切换后趋势图按该粒度重查 |
| 趋势图（6 条） | token_tokens、llm_calls、api_calls、artifact_created、task_completed、roi 各一条时间序列曲线（复用现有图表组件，废弃旧 `token_trend`/`call_trend` 等 7 系列结构） |

- 前端调 `GET /api/v1/dashboard` 渲染 KPI 卡片，调 `GET /api/v1/dashboard/trends?granularity=...` 渲染趋势图。
- bucket 时间为 UTC，前端展示时转本地时区。
- data-testid 沿用现有 dashboard 相关 testid 或按新组件命名（实现时定）。

### 废弃代码清单（现有设计删除）

| 废弃项 | 处理 |
|--------|------|
| `internal/infra/llmstats/llmstats.go` 的 `Aggregate` / `AggregateByTime` | **删除**（MongoDB 明细聚合逻辑） |
| `internal/infra/llmstats` 的 `Record` 写 `llm_usage` 明细 | **改为** 调 `metrics.Counter.Incr`，不再写明细 collection |
| `internal/service/monitor/trends.go` 的 `ComputeTrends` 系列 | **删除**（基于 `[]task.TaskRun` 的实时计算） |
| `internal/api/handler/dashboard.go` 现有 `Get`/`GetTrends` | **重写**（走 `metrics.Reader`） |
| `internal/api/handler/stats.go` 的 `GetLLMStats`（MongoDB 聚合） | **删除或改造**（若保留管理端明细，需明确数据源；统计主链路不再用） |
| 旧 `llm_usage` collection | **不再写入**；历史数据不迁移（见下） |

### 不处理历史数据

- 新 `stats_hourly` 从 0 开始，**不回填**历史 `llm_usage` / task / artifact 数据。
- 旧 `llm_usage` collection 停止写入后遗留，可后续手动清理（测试环境可接受）。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes — MongoDB `stats_hourly`（小时粒度计数） |
| 是否影响现有 API | Yes — dashboard 查询语义改变；废弃 llmstats 明细聚合 |
| 性能影响 | 写入：每埋点一次 upsert $inc（可合并批量）；查询：对 ≤8760 条/年 document sum，极快 |
| 存储量 | 5 metric × 8760 小时/年 = ~43800 document/年，量级小；TTL 自动清理一年前 |
| 是否需要新增 Skill | No |
| 复用现有能力 | 复用 MongoDB client、LLM 埋点调用点、gin middleware |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/metrics/metrics.go` + `mongo.go` | 统一统计组件（Counter/Reader + MongoDB stats_hourly 实现） | New |
| `internal/infra/llmstats/llmstats.go` | `Record` 内部改走 `Counter`，删除 Aggregate/AggregateByTime | Modify |
| `internal/api/middleware/metrics.go` | API 调用量埋点 middleware（api_calls） | New |
| `internal/service/artifact/*` | 产出物创建埋点（artifact_created） | Modify |
| `internal/worker/pool.go`（或 task executor） | task run 完成埋点（task_completed） | Modify |
| `internal/api/handler/dashboard.go` | 重写：查询走 `metrics.Reader`，补 KPI 指标 + granularity | Rewrite |
| `internal/service/monitor/trends.go` | 删除（ComputeTrends 废弃） | Delete |
| `internal/api/handler/stats.go` | 删除或改造（GetLLMStats 明细聚合） | Modify |
| `cmd/server/wire.go` | 初始化 metrics 组件 + 注入各埋点 | Modify |
| `cmd/server/migration/stats_seed.go` | **新增** `SeedStats`（幂等初始化索引：唯一 {metric,hour} + TTL {hour}） | New |
| `cmd/server/main.go` | 启动时调用 `migration.SeedStats` | Modify |
| `frontend/app/page.tsx` | dashboard 前端适配新 API（summary + granularity 趋势） | Modify |

## 测试策略

1. **Unit tests（Go）**：L2 `Counter`/`Reader` 接口 mock；L3 MongoDB 实现走真实 Mongo（`go test -tags=integration`）。覆盖率 gate 见 SPEC-045。
2. **Integration**：验证小时 document upsert `$inc` 累加、TTL index、sum/分桶聚合、粒度边界、一年范围限制。
3. **E2E**：dashboard 页面加载 summary + 各 granularity 趋势（用例编号 `UI-XXX`）。
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
| L1 | 纯函数（ROI、小时桶生成、粒度分桶） | 100% |
| L2 | `Counter`/`Reader` 接口 mock | 100% |
| L3 | MongoDB 实现 / dashboard handler / middleware / 埋点 | 98% |
| Overall | 全量 | ≥98% |

- [ ] 每个 Success 测试 ≥2 个行为验证断言
- [ ] `Incr` 验证 upsert 到同小时 document（多次累加同一条）+ `$inc` 正确
- [ ] **并发安全**：多 goroutine 并发 `Incr` + `flush` 交错，验证无 data race（`-race` 语义）、计数不丢不重
- [ ] `Sum`/`Series` 验证小时聚合、粒度分桶（day/week/month/year）、一年范围限制
- [ ] ROI 验证 token=0 边界
- [ ] **严禁** `t.Skip()` 绕过不可测场景

## 验证标准

- [ ] 启动后 `SeedStats` 幂等执行，`stats_hourly` 唯一索引 `{metric,hour}` + TTL index `{hour}` 存在
- [ ] `SeedStats` 重复调用不报错（幂等），无 seed 业务数据预置
- [ ] 埋点后 `stats_hourly` 出现对应小时 document，同一小时多次埋点累加进同一条
- [ ] `stats_hourly` TTL index（365 天）生效，一年前数据自动删除
- [ ] `GET /api/v1/dashboard` 返回完整 KPI（kb_docs、token、llm_calls、api_calls、artifact、task_completed、roi 七指标）
- [ ] `GET /api/v1/dashboard/trends?granularity=day|week|month|year` 各返回正确分桶序列
- [ ] 查询不触发旧 `llm_usage` 明细聚合（旧聚合代码已删除）
- [ ] 查询范围超过一年被拒绝或 clamp
- [ ] 所有登录用户（user/admin/system_admin）均可访问 dashboard
- [ ] 旧代码（ComputeTrends、AggregateByTime、llm_usage 明细写入）已废弃删除
- [ ] 无历史数据迁移/回填（`stats_hourly` 仅含上线后埋点数据）
- [ ] ROI = (artifact + task_completed) / token_tokens，token=0 时 ROI=0

## 深化记录（2026-08-30）

| # | 深化点 | 结论 |
|---|--------|------|
| 1 | task_stats 口径 | **简化为 `task_completed` 埋点累计数**，不做状态分布（废弃 `TaskStats` 的 pending/running/failed 快照） |
| 2 | 前端布局 | KPI 卡片（7 指标）+ granularity 切换器 + 6 条趋势曲线（见「前端 Dashboard 布局」章节） |
| 3 | token 细分 | **不按 model / call_point 细分**（保持 5 个全局 metric 简单，未来另立 spec） |
| 4 | 埋点批量 | **内存缓冲 + 定时刷盘（默认 5s）**，进程退出强制 flush，容忍 ≤5s 增量丢失 |
| 5 | 时区口径 | `hour` 桶统一 **UTC** 存储；前端查询传 UTC、展示转本地时区 |
