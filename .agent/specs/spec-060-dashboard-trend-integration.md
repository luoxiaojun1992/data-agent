# Dashboard trend 接入 + 路径/字段修复

> **SPEC-060** | Status: ✅ 已实现
>
> **来源**: 从 SPEC-059 拆分（2026-07-21 晓军确认）。SPEC-059 聚焦 token 真数据 + 统计分析架构归置设计，本 spec 按该架构实现 dashboard 完整统计分析展示。

## 1. 目标

按 SPEC-059 归置的统计分析架构，实现 dashboard 完整展示：

1. **路径/字段修复**：`/admin/dashboard` → `/dashboard`（与前端匹配）+ `Get` 返回 `{task_stats, kb_docs}`（对齐前端/测试期望，修复 KPI 全 0）。
2. **`/dashboard/trends` 端点实现**：接入所有 trend（CallTrend/DurationDist/SuccessTrend/OutputStats/ROITrend + token_trend 真实），返回完整 `DashboardTrends`。
3. **`ComputeTrends` 改造**：`token_trend` 用 SPEC-059 的 `llmstats.AggregateByTime` 真实聚合 + 清理死参数（sessions/docCount）。
4. **前端全 trend 显示真数据** + E2E 验证。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-059 | 📐 设计中 | 统计分析架构归置 + `llmstats.AggregateByTime` 能力（本 spec 调用）+ `computeTokenTrend` 假函数已清理 |
| SPEC-051 | ✅ | `llm_usage` + `recordEnhanceTokens` |
| SPEC-033 | ✅ | UI E2E — 增强提示词 |
| — | — | SPEC-059 须先实现（提供 `AggregateByTime` + 架构） |

## 2. 背景

### 2.1 现状（SPEC-059 调研结论）

**(A) dashboard stats 路径/字段全错** ❌（SPEC-055 重构遗留）
- 路径：前端 `page.tsx` L74 调 `/dashboard`；测试调 `/dashboard/stats`（5 处）；后端注册 `/admin/dashboard` —— 全不匹配，stats 一直 404
- 字段：前端 L88 期望 `stats.task_stats`；测试期望 `stats.kb_docs`+`stats.task_stats`；后端 `Get` 返回 `{tasks, sessions, docs}` —— 字段名全不对，KPI 全 0
- 测试 UI-229/230/231 用 `if (statsRes.ok())` 容忍 404，API 验证一直跳过
- `/admin/dashboard` 中间件是 `AuthMiddleware`（登录即可，非 admin guard），路径名含 "admin" 是误导

**(B) `/dashboard/trends` 端点不存在** ❌
- 前端 `page.tsx` L74 调 `/dashboard/trends`，后端无此端点 → 404 → trends null → 所有 trend 图表空 + "Token 消耗" KPI 显示 "—"
- L77 `catch { /* ignore */ }` 静默吞 404
- monitor `ComputeTrends`（trend 计算）是死代码，未接入任何路由

**(C) 前端已消费全部 7 trend + 已有 data-testid** ✅
`frontend/app/page.tsx` 已消费 `call_trend`/`duration_dist`/`req_dist`/`success_trend`/`token_trend`/`output_stats`/`roi_trend`，testid 齐全（`chart-call-trend` 等 + `dashboard-token-kpi-0/1/2`）。后端补端点后自动显示。

### 2.2 SPEC-059 提供的能力

- `llmstats.AggregateByTime(since, bucketMs)` — token 按时间桶聚合（本 spec `token_trend` 用）
- 统计分析架构归置设计（§3 职责划分 + API 域）
- `computeTokenTrend` ×500 假函数已删除

## 3. 架构概述（引用 SPEC-059 §3）

```
GET /api/v1/dashboard          → handler/dashboard.go Get         → {task_stats, kb_docs}
GET /api/v1/dashboard/trends   → handler/dashboard.go GetTrends   → monitor.ComputeTrends(tasks, tokenBuckets)
                                    │                                    ├─ call_trend/duration_dist/... 从 task (现有)
                                    │                                    └─ token_trend 从 llmstats.AggregateByTime (真实)
                                    └─ llmstats.AggregateByTime (SPEC-059 提供)
```

## 4. API 设计

| Method | Path | Description | 权限 |
|--------|------|-------------|------|
| GET | `/api/v1/dashboard` | stats（KPI: task_stats/kb_docs） | 登录 |
| GET | `/api/v1/dashboard/trends` | trends（7 个 trend，token_trend 真实） | 登录 |

**`/dashboard` 响应**：`{task_stats: {total,pending,running,completed,failed}, kb_docs: N}`
**`/dashboard/trends` 响应**：`{call_trend, duration_dist, req_dist, success_trend, token_trend, output_stats, roi_trend}`

## 5. 详细设计

### 5.1 `/admin/dashboard` → `/dashboard` 路径修复

`internal/api/handler/dashboard.go` `RegisterDashboardRoutes`：
- 路由 `/api/v1/admin/dashboard` → `/api/v1/dashboard`（与前端 `page.tsx` L74 匹配）
- 中间件不变（`AuthMiddleware` 登录即可）
- 不保留 `/admin/dashboard` 别名（grep 确认无生产调用方）

### 5.2 `Get` 返回格式对齐 + `aggregateTaskStats`

`dashboard.go` `Get` 改造：
```go
func (h *DashboardHandler) Get(c *gin.Context) {
    userID := c.GetString("user_id")
    tasks, _ := h.taskService.ListAllTasks(userID)
    docs, _ := h.kbService.ListAllDocs()
    c.JSON(http.StatusOK, gin.H{
        "task_stats": aggregateTaskStats(tasks),
        "kb_docs":    len(docs),
    })
}

func aggregateTaskStats(tasks []task.Task) map[string]int {
    stats := map[string]int{"total": len(tasks), "pending": 0, "running": 0, "completed": 0, "failed": 0}
    for _, t := range tasks {
        switch t.Status {
        case task.StatusPending: stats["pending"]++
        case task.StatusRunning: stats["running"]++
        case task.StatusCompleted: stats["completed"]++
        case task.StatusFailed: stats["failed"]++
        }
    }
    return stats
}
```

> 移除 `sessions` 返回（前端不消费，grep 确认无 `stats.sessions`）。`sessionManager` 依赖从 handler 移除（或保留但 Get 不用）。

### 5.3 `/dashboard/trends` 端点实现

`dashboard.go` 扩展：
- `DashboardHandler` 增加 `llmRecorder *llmstats.Recorder` 字段
- `GetTrends(c *gin.Context)`：
```go
func (h *DashboardHandler) GetTrends(c *gin.Context) {
    userID := c.GetString("user_id")
    tasks, _ := h.taskService.ListAllTasks(userID)
    since := time.Now().Add(-24 * time.Hour)
    tokenBuckets, _ := h.llmRecorder.AggregateByTime(c.Request.Context(), since, int64((4*time.Hour).Milliseconds()))
    trends := monitor.ComputeTrends(tasks, tokenBuckets)
    c.JSON(http.StatusOK, trends)
}
```
- 路由：`router.GET("/api/v1/dashboard/trends", midd, h.GetTrends)`

### 5.4 `ComputeTrends` 改造（token_trend 真实 + 死参数清理）

`internal/service/monitor/trends.go`：

**(a) 清理死参数**：`ComputeTrends` 现签名 `(tasks, sessions, docCount)` 中 `sessions`/`docCount` 完全未用。新签名：
```go
func ComputeTrends(tasks []task.Task, tokenBuckets []llmstats.TimeBucketResult) *DashboardTrends
```

**(b) token_trend 真实**：SPEC-059 已删 `computeTokenTrend`。新增 `mapTokenBuckets`：
```go
func mapTokenBuckets(buckets []llmstats.TimeBucketResult, now time.Time) []TrendPoint {
    // 24h 内每 4h 一桶，与 call_trend label 对齐（0时/4时/.../20时）
    // 每桶 sum(total_tokens)
}
```
`t.TokenTrend = mapTokenBuckets(tokenBuckets, now)`

**(c) 其他 trend 保持现有逻辑**（从 task 派生，合理）：
- `call_trend` = `computeCallTrend(tasks)` ✅
- `duration_dist` = `computeDurationDist(tasks)` ✅
- `success_trend` = `computeSuccessTrend(tasks)` ✅
- `output_stats` = `computeOutputStats(tasks)` ✅
- `roi_trend` = `computeROITrend(tasks)` ✅
- `req_dist` = `call_trend`（L33，保留，语义"24h agent 调用分布"）

### 5.5 测试路径对齐

- `tests/ui/dashboard-integration.spec.ts`：`/dashboard/stats` → `/dashboard`（4 处：L138/159/176/238）
- `tests/ui/utils/mongo-query.ts`：`/dashboard/stats` → `/dashboard`（1 处：L103）
- UI-229/230/231 的 `if (statsRes.ok())` 容忍逻辑移除（端点不再 404，改为真实断言）

### 5.6 前端全 trend 显示 + E2E

**前端**（`frontend/app/page.tsx`）：
- 路径已匹配（`/dashboard` + `/dashboard/trends`），后端补端点后自动获取真数据
- 删除 `emptyChart` 死代码（L98-103 未使用）
- data-testid 已存在（无需新增）：
  - `chart-call-trend` / `chart-duration-dist` / `chart-req-dist` / `chart-success-trend` / `chart-token-trend` / `chart-output-stats` / `chart-roi-dual`
  - `dashboard-stat-0~3`（KPI 卡片）+ `dashboard-token-kpi-0/1/2` + `dashboard-token-value-0/1/2`

**E2E**（`tests/ui/dashboard.spec.ts` 新增 `UI-2XX: Dashboard — 全 trend 真数据显示`）：
1. 触发一次 enhance（mockllm mock，后端真实写 llm_usage）+ 确保有 task 数据
2. 导航 dashboard 首页 `/`
3. 验证所有 trend 显示真数据（非空/非 "—"）：
   - `chart-call-trend`/`chart-duration-dist`/`chart-success-trend`/`chart-token-trend`/`chart-output-stats`/`chart-roi-dual` 有数据点
   - `dashboard-token-value-0`（Token 消耗）非 "—" 且为数字
   - `dashboard-stat-0~3`（KPI）显示真实 task_stats（非全 0）
4. API 验证：`/dashboard` 返回 `task_stats`/`kb_docs`；`/dashboard/trends` 返回 7 个 trend

> **E2E 全局约束**：dashboard 趋势依赖 task + llm_usage 真实数据。测试用 admin 账号创建 task（或预置），enhance 触发 token 写入。不 mock `/dashboard` + `/dashboard/trends` 端点（走真实后端链路）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `llm_usage` + task） |
| 是否影响现有 API | Yes（`/admin/dashboard` → `/dashboard`，路径变更；返回格式变更。但前端/测试早已期望新路径+格式，是修复） |
| 是否影响现有 UI | Yes（dashboard 从全空变真数据，是修复） |
| 性能影响 | 低（aggregation + task 查询） |
| 风险等级 | 中 — 改 ComputeTrends 签名（影响 monitor_test）+ 路径/格式变更（影响测试） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/api/handler/dashboard.go` | 路径 `/admin/dashboard`→`/dashboard` + `Get` 返回 `{task_stats, kb_docs}` + `aggregateTaskStats` + `GetTrends` + `llmRecorder` 字段 + 路由 | Modify |
| `internal/api/handler/dashboard_test.go` | Get（新格式）+ GetTrends 测试 | Modify |
| `internal/service/monitor/trends.go` | `ComputeTrends` 新签名 `(tasks, tokenBuckets)` + `mapTokenBuckets` + 删死参数 | Modify |
| `internal/service/monitor/monitor_test.go` | 适配 ComputeTrends 新签名 | Modify |
| `cmd/server/main.go` | 注册 `/dashboard/trends` 路由 + dashboard handler 注入 `llmRecorder` | Modify |
| `frontend/app/page.tsx` | 删除 `emptyChart` 死代码（testid 已存在） | Modify |
| `tests/ui/dashboard-integration.spec.ts` | `/dashboard/stats`→`/dashboard`（4 处）+ UI-229/230/231 移除 if(ok()) 改真实断言 | Modify |
| `tests/ui/utils/mongo-query.ts` | `/dashboard/stats`→`/dashboard`（1 处） | Modify |
| `tests/ui/dashboard.spec.ts` | 新增 UI-2XX 全 trend 真数据验证 | Modify |
| `.agent/memory/E2E_TESTING.md` | UI-2XX 描述 | Modify |

## 8. 测试策略

1. **Unit tests**（Go）:
   - `DashboardHandler.Get`：返回 `{task_stats, kb_docs}` + `aggregateTaskStats` 按 Status 聚合
   - `DashboardHandler.GetTrends`：mock taskSvc/llmRecorder，验证 trends 结构 + token_trend 真实
   - `monitor.ComputeTrends`：新签名，token_trend 从 tokenBuckets 映射
   - 覆盖率 ≥98%
2. **E2E tests**（UI）:
   - UI-2XX：dashboard 全 trend 真数据（7 chart + 3 KPI 非空）
   - UI-229/230/231：API 验证不再跳过（`/dashboard` 不 404）

## 9. UI Test / E2E 验收规则

- [ ] **必须** 新增 UI-2XX dashboard 全 trend 真数据显示（7 chart + 3 KPI 非空/非 "—"）
- [ ] **必须** UI-229/230/231 API 验证不再跳过（移除 `if(ok())`，真实断言）
- [ ] **必须** CI sonar-check + ui-tests 通过
- [ ] **严禁** 降级断言

## 9.5. Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | monitor `mapTokenBuckets` + `aggregateTaskStats` | **100%** |
| L3 | handler/dashboard `Get` + `GetTrends` | **98%** |

- [ ] `aggregateTaskStats` 覆盖：各 Status 计数、空 task
- [ ] `GetTrends` 覆盖：正常、llmRecorder 错误、空 task
- [ ] Success 测试 ≥2 个行为验证断言

## 10. 验证标准

1. `/api/v1/dashboard`（stats）不再 404，返回 `{task_stats, kb_docs}`（字段与前端/测试期望对齐）
2. `/admin/dashboard` 路径已改为 `/dashboard`，`grep -rn "admin/dashboard" internal/ cmd/` 为空
3. `/api/v1/dashboard/trends` 返回完整 7 个 trend 字段
4. `token_trend` 从 `llm_usage` 真实聚合（`AggregateByTime`，非 ×500 假数据）
5. `ComputeTrends` 签名 `(tasks, tokenBuckets)`，死参数 sessions/docCount 已移除
6. dashboard 所有 trend 显示真数据：`chart-*` 有数据点，`dashboard-token-value-0` 非 "—"
7. `dashboard-stat-0~3` KPI 显示真实 task_stats（非全 0）
8. 测试 `/dashboard/stats` → `/dashboard`（5 处），UI-229/230/231 不再跳过 API 验证
9. `page.tsx` `emptyChart` 死代码已删除
10. `go test ./internal/...` 全通过，覆盖率 ≥98%
11. CI sonar-check + ui-tests 通过

## 11. 不在本 spec 范围

- dashboard `timeFilter`（today/week/month）联动 trends 时间范围 → 后续优化
- `req_dist` 真实化（现状 = call_trend 重复，语义"24h agent 调用分布"）
- `roi_trend` 语义优化（现状 4 周完成数，前端算倍数，逻辑可改进但现有合理）
- enhance cache hit 时 token 统计补偿（cache hit 不计 token 是合理行为）
