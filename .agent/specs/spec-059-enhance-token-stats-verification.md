# 增强调用 Token 统计验证 + Dashboard Token 真数据展示

> **SPEC-059** | Status: 设计中

## 1. 目标

1. **纠正 UI-160**：案例名"增强调用不计入 Token 统计"错误（后端已计入），改为"计入"并真正验证 token 统计增加。
2. **Dashboard 显示 Token 真数据**：前端 dashboard 首页的"Token 消耗"KPI + 趋势图当前显示 "—"（前端调 `/dashboard/trends` 但后端无此端点；monitor 的 `computeTokenTrend` 是 ×500 假数据死代码）。新增 `/api/v1/dashboard/trends` 端点，`token_trend` 从 `llm_usage` 真实聚合。
3. **清理 monitor 死代码**：删除 `computeTokenTrend` ×500 假函数。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-051 | ✅ | LLM Token 统计（`llm_usage` + `recordEnhanceTokens` 已实现） |
| SPEC-043 | ✅ | Mock Model Service（UI 测试用 mockllm） |
| SPEC-033 | ✅ | UI E2E — 增强提示词（UI-156~160） |
| — | — | 无阻塞前置依赖 |

## 2. 背景

### 2.1 现状调研（2026-07-21）

**(A) 后端 enhance 已计入 token** ✅
`main.go` `recordEnhanceTokens` (L713) 写 `llm_usage`（CallPoint=`enhance`），`makeEnhanceHandler` L753 调用（cache miss 路径）。历史：SPEC-051 PR#63 曾错误回退，晓军 2026-07-18 纠正后恢复。

**(B) token 查询能力缺失** ❌
- 无 LLM token 查询端点
- SPEC-051 §4.3 "monitor token trend 从 llm_usage 聚合" **未落地**

**(C) monitor `computeTokenTrend` 是 ×500 假数据 + 死代码** ❌
`trends.go` L127：`Value: p.Value * 500`（CallTrend ×500 当 token）。`ComputeTrends` 生产代码**无人调用**（只测试用），`/api/v1/system/stats`（monitor.Handler）只返回 SystemStats（runtime 内存/CPU），不返回 trends。

**(D) 前端 dashboard token 显示 "—"** ❌
`frontend/app/page.tsx`：
- L74: `apiFetch('/dashboard/trends')` → `/api/v1/dashboard/trends`（后端**无此端点**，404）
- L77: `catch { /* ignore */ }` 静默吞 404 → `trends` 为 null
- L177: "Token 消耗" KPI = `trends?.token_trend ? ... : '—'` → 显示 "—"
- L191: token 趋势图 = `(trends?.token_trend || [])` → 空
- L98 注释: "Time-series charts placeholder — needs /api/v1/dashboard/trends endpoint"

**(E) UI-160 错误** ❌
`tests/ui/prompt.spec.ts` L104 名"增强调用不计入 Token 统计"（实际计入），测试逻辑只验证 UI 流程（L121 注释"Token stats tracked server-side"），未验证 token。

### 2.2 问题根因

token 统计"写入但读不出"——`llm_usage` 有真实数据，但：
1. 无查询 API（UI-160 无法验证 enhance 计入）
2. `/dashboard/trends` 端点不存在（dashboard 显示空）
3. monitor `computeTokenTrend` 是死代码假数据（即使接入也是假的）

## 3. 架构概述

```
写入 (已实现 ✅):
  enhance/chat/embedding/compaction → llmstats.Recorder.Record → llm_usage

读取 (本 spec 新增):
  ┌─────────────────────────────────────────────────────┐
  │ GET /api/v1/stats/llm?call_point=enhance            │ ← UI-160 验证用
  │   → handler/stats.go → llmstats.Aggregate           │   (by call_point)
  │   → {call_point, count, prompt_tokens, ...}         │
  └─────────────────────────────────────────────────────┘
  ┌─────────────────────────────────────────────────────┐
  │ GET /api/v1/dashboard/trends                        │ ← dashboard 首页用
  │   → handler/dashboard.go GetTrends                  │
  │   → monitor.ComputeTrends (改造)                    │
  │     · call_trend/duration_dist/... 从 task (现有)   │
  │     · token_trend 从 llm_usage 聚合 (新增, 真实)    │
  │   → {call_trend, token_trend, ...}                  │
  └─────────────────────────────────────────────────────┘

前端:
  page.tsx /dashboard/trends → 真实 token_trend → KPI + 趋势图显示真数据
  UI-160 → /stats/llm 验证 enhance count+1
```

## 4. API 设计

| Method | Path | Description | 权限 |
|--------|------|-------------|------|
| GET | `/api/v1/stats/llm` | LLM token 按 call_point 聚合（UI-160 验证用） | admin |
| GET | `/api/v1/dashboard/trends` | Dashboard 趋势数据（token_trend 真实） | 登录用户 |

**`/api/v1/stats/llm` query**：`call_point`(可选)、`since`(可选, ISO8601)
**响应**：`{stats: [{call_point, count, prompt_tokens, completion_tokens, total_tokens}]}`

**`/api/v1/dashboard/trends` 响应**：
```json
{
  "call_trend": [...], "duration_dist": [...], "req_dist": [...],
  "success_trend": [...], "token_trend": [{"label":"0时","value":1234}, ...],
  "output_stats": [...], "roi_trend": [...]
}
```
`token_trend` 按时间桶（4小时桶，24h，与 call_trend 对齐）聚合 `llm_usage` 的 `prompt_tokens + completion_tokens`。

## 5. 详细设计

### 5.1 llmstats 查询方法（新增）

`internal/infra/llmstats/llmstats.go` 现有 `Recorder.Record`（写）。新增：

```go
// Aggregate 按 call_point 聚合 token 用量。callPoint 空则全部，since 零值则不过滤时间。
func (r *Recorder) Aggregate(ctx context.Context, callPoint string, since time.Time) ([]AggregateResult, error)

type AggregateResult struct {
    CallPoint        string `bson:"_id" json:"call_point"`
    Count            int64  `bson:"count" json:"count"`
    PromptTokens     int64  `bson:"prompt_tokens" json:"prompt_tokens"`
    CompletionTokens int64  `bson:"completion_tokens" json:"completion_tokens"`
}

// AggregateByTime 按时间桶聚合 token 用量（用于趋势图）。
// buckets 为桶起始时间列表（如 24h 内每 4h 一桶），返回每桶的 token 总量。
func (r *Recorder) AggregateByTime(ctx context.Context, since time.Time, bucketMs int64) ([]TimeBucketResult, error)

type TimeBucketResult struct {
    BucketStart time.Time `bson:"_id" json:"bucket_start"`
    TotalTokens int64     `bson:"total_tokens" json:"total_tokens"`
}
```

MongoDB aggregation pipeline：
- `Aggregate`: `$match`(call_point+since) → `$group` by call_point
- `AggregateByTime`: `$match`(since) → `$group` by `$bucket` of `created_at` → sum(prompt+completion)

### 5.2 `/api/v1/stats/llm` 端点（UI-160 验证用）

`internal/api/handler/stats.go`（新文件）：
- `LLMStatsHandler` 持 `*llmstats.Recorder`
- `GetLLMStats(c *gin.Context)`：解析 query → `Recorder.Aggregate` → JSON
- 路由：`router.GET("/api/v1/stats/llm", jwtManager.AuthMiddleware(), adminGuard, statsHandler.GetLLMStats)`
- main.go 注入 `deps.llmRecorder`

### 5.4 `/api/v1/dashboard/trends` 端点（dashboard 真数据，全 trend 接入）

`internal/api/handler/dashboard.go` 扩展：
- `DashboardHandler` 增加 `llmRecorder *llmstats.Recorder` 字段
- `GetTrends(c *gin.Context)`：调 `monitor.ComputeTrends`（改造后）→ JSON
- 路由：`router.GET("/api/v1/dashboard/trends", midd, h.GetTrends)`

返回完整 `DashboardTrends`（前端 `page.tsx` 消费全部 7 个 trend 字段）：
`call_trend` / `duration_dist` / `req_dist` / `success_trend` / `token_trend` / `output_stats` / `roi_trend`

```go
func (h *DashboardHandler) GetTrends(c *gin.Context) {
    userID := c.GetString("user_id")
    tasks, _ := h.taskService.ListAllTasks(userID)
    since := time.Now().Add(-24 * time.Hour)
    tokenBuckets, _ := h.llmRecorder.AggregateByTime(c.Request.Context(), since, int64((4*time.Hour).Milliseconds()))
    trends := monitor.ComputeTrends(tasks, tokenBuckets) // 清理死参数后
    c.JSON(http.StatusOK, trends)
}
```

### 5.5 `ComputeTrends` 改造（token_trend 真实 + 清理死参数 + 全 trend 接入）

`internal/service/monitor/trends.go`：

**(a) 清理死参数**：`ComputeTrends` 现签名 `(tasks, sessions, docCount)` 中 `sessions`/`docCount` **完全未用**（grep 确认函数体内无引用）。新签名：
```go
func ComputeTrends(tasks []task.Task, tokenBuckets []llmstats.TimeBucketResult) *DashboardTrends
```

**(b) token_trend 真实**：删除 `computeTokenTrend`（×500 假函数），`t.TokenTrend` 改为从 `tokenBuckets` 映射（按 4h 桶 label 对齐 0时/4时/.../20时）。

**(c) 其他 trend 保持现有逻辑**（从 task 派生，合理）：
- `call_trend` = `computeCallTrend(tasks)` — 24h 每 4h 桶 task 数 ✅
- `duration_dist` = `computeDurationDist(tasks)` — 耗时分布 ✅
- `success_trend` = `computeSuccessTrend(tasks)` — 7 天成功率 ✅
- `output_stats` = `computeOutputStats(tasks)` — skill_chain 类型分布 ✅
- `roi_trend` = `computeROITrend(tasks)` — 4 周完成数 ✅

**(d) `req_dist` 问题**：现状 `t.ReqDist = t.CallTrend`（L33，与 call_trend 完全重复）。本 spec 保留但明确语义为"24h agent 调用分布"（与 call_trend 同数据不同视图标题），不真实化 API 请求量（超出范围）。

### 5.6 前端 dashboard 全 trend 显示（testid 已存在，无需新增）

`frontend/app/page.tsx` 已消费全部 7 个 trend 字段并**已有 data-testid**（调研确认，无需新增）：

| Trend 字段 | 前端 testid | 显示位置 |
|-----------|------------|---------|
| `call_trend` | `chart-call-trend` | L150 趋势图 |
| `duration_dist` | `chart-duration-dist` | L160 趋势图 |
| `req_dist` | `chart-req-dist` | L163 趋势图 |
| `success_trend` | `chart-success-trend` | L170 趋势图 |
| `token_trend` | `dashboard-token-kpi-0` + `dashboard-token-value-0`（KPI）, `chart-token-trend`（趋势图） | L177/L191 |
| `output_stats` | `dashboard-token-kpi-1` + `dashboard-token-value-1`（KPI）, `chart-output-stats`（趋势图） | L178/L194 |
| `roi_trend` | `dashboard-token-kpi-2` + `dashboard-token-value-2`（KPI）, `chart-roi-dual`（趋势图） | L179/L197 |

后端补 `/api/v1/dashboard/trends` 端点后，前端自动获取真数据，所有 trend 显示真实值（非空/非 "—"）。

> **清理**：`page.tsx` L98-103 `emptyChart` 函数未使用（死代码，L149+ 用 `Chart` 组件），一并删除。

### 5.6 UI-160 改造

`tests/ui/prompt.spec.ts` L104：

```ts
// ═══ UI-160: 增强调用计入 Token 统计 ═══
test('[UI-160] Prompt — 增强调用计入 Token 统计', async ({ page, request }) => {
  await clearMocks(request);
  await seedMock(request, 'test token ' + uid, '增强后的测试内容'); // 唯一 prompt 保证 cache miss

  const before = await getLLMStats(request, adminToken, 'enhance');
  const beforeCount = before.stats[0]?.count ?? 0;

  const input = page.locator('[data-testid="chat-input"]');
  await input.fill('test token ' + uid);
  await page.locator('[data-testid="chat-enhance-btn"]').click();
  await expect(input).toHaveValue('增强后的测试内容', { timeout: 5000 });

  // recordEnhanceTokens 异步写 llm_usage，轮询验证 count+1
  await expect.poll(async () => {
    const after = await getLLMStats(request, adminToken, 'enhance');
    return after.stats[0]?.count ?? 0;
  }, { timeout: 5000 }).toBe(beforeCount + 1);
});
```

### 5.7 新增 E2E：Dashboard 全 trend 真数据显示

`tests/ui/dashboard.spec.ts` 新增（或扩展现有 dashboard 测试）`UI-2XX: Dashboard — 全 trend 真数据显示`：

1. 触发一次 enhance（mockllm mock，后端真实写 llm_usage）+ 确保有 task 数据
2. 导航 dashboard 首页 `/`
3. 验证所有 trend 显示真数据（非空/非 "—"）：
   - `chart-call-trend` 有数据点
   - `chart-duration-dist` 有数据点
   - `chart-success-trend` 有数据点
   - `chart-token-trend` 有数据点（token_trend 真实）
   - `chart-output-stats` 有数据点
   - `chart-roi-dual` 有数据点
   - `dashboard-token-value-0`（Token 消耗）非 "—" 且为数字
   - `dashboard-token-value-1`（AI 产出）非 "—"
   - `dashboard-token-value-2`（ROI）非 "—"

> 注：需 admin 账号。mockllm mock LLM，后端 recordEnhanceTokens 真实写 llm_usage，task 数据来自 task service。`req_dist` = `call_trend` 同数据，验证 `chart-req-dist` 即可。

> **E2E 全局约束**：dashboard 趋势依赖 task + llm_usage 真实数据。测试用 admin 账号创建 task（或预置），enhance 触发 token 写入。不 mock `/dashboard/trends` 端点（走真实后端链路）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `llm_usage`） |
| 是否影响现有 API | No（新增 2 端点，向后兼容） |
| 是否影响现有 UI | Yes（dashboard token 从 "—" 变真数据，是修复） |
| 性能影响 | 低（aggregation 查询，`llm_usage` 已有 created_at/call_point 索引） |
| 风险等级 | 低-中 — 新增端点 + 改 ComputeTrends 签名（影响 monitor_test） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/infra/llmstats/llmstats.go` | 新增 `Aggregate` + `AggregateByTime` + 结果 struct | Modify |
| `internal/infra/llmstats/llmstats_test.go` | 新增查询方法测试 | Modify |
| `internal/api/handler/stats.go` | 新增 `LLMStatsHandler` + `GetLLMStats` | **New** |
| `internal/api/handler/stats_test.go` | handler 测试 | **New** |
| `internal/api/handler/dashboard.go` | 增加 `llmRecorder` 字段 + `GetTrends` + 路由 | Modify |
| `internal/api/handler/dashboard_test.go` | GetTrends 测试 | Modify |
| `internal/service/monitor/trends.go` | `ComputeTrends` 加 tokenBuckets 参数 + 删 `computeTokenTrend` + `mapTokenBuckets` | Modify |
| `internal/service/monitor/monitor_test.go` | 适配 ComputeTrends 新签名 + 删 TestComputeTrends_TokenTrend | Modify |
| `cmd/server/main.go` | 注册 2 路由 + dashboard handler 注入 recorder | Modify |
| `frontend/app/page.tsx` | 删除 `emptyChart` 死代码（data-testid 已存在无需新增） | Modify |
| `tests/ui/prompt.spec.ts` | UI-160 改名 + 改验证逻辑 + `getLLMStats` 辅助 | Modify |
| `tests/ui/dashboard.spec.ts` | 新增 UI-2XX dashboard token 真数据验证 | Modify |
| `.agent/memory/E2E_TESTING.md` | UI-160 + UI-2XX 描述更新 | Modify |

## 8. 测试策略

1. **Unit tests**（Go）:
   - `llmstats.Aggregate` / `AggregateByTime`：mock mongo collection，验证 pipeline（过滤/分组/桶聚合）
   - `LLMStatsHandler.GetLLMStats`：query 解析 + 响应 + 权限
   - `DashboardHandler.GetTrends`：mock taskSvc/sessionMgr/kbSvc/llmRecorder，验证 trends 结构 + token_trend 真实
   - `monitor.ComputeTrends`：适配新签名，token_trend 从 tokenBuckets 映射
   - 覆盖率 ≥98%
2. **E2E tests**（UI）:
   - UI-160：enhance 前后 `/stats/llm` count+1
   - UI-2XX：dashboard token KPI + 趋势图显示真数据（非 "—"）
   - CI: `ui-tests.yml` + mockllm

## 9. UI Test / E2E 验收规则

- [ ] **必须** UI-160 改名"增强调用计入 Token 统计" + 验证 `/stats/llm` count+1
- [ ] **必须** 新增 UI-2XX dashboard 全 trend 真数据显示（7 个 chart + 3 个 KPI 非空/非 "—"）
- [ ] **必须** CI sonar-check + ui-tests 通过
- [ ] **严禁** 降级断言（如只验证 UI 流程不验证 token，或只验证部分 trend）

## 9.5. Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | llmstats 聚合逻辑 + monitor mapTokenBuckets | **100%** |
| L3 | handler/stats + handler/dashboard GetTrends | **98%** |

- [ ] `Aggregate`/`AggregateByTime` 覆盖：过滤、空结果、多 call_point、桶对齐
- [ ] `GetLLMStats`/`GetTrends` 覆盖：正常、权限拒绝、Recorder 错误
- [ ] Success 测试 ≥2 个行为验证断言

## 10. 验证标准

1. `GET /api/v1/stats/llm?call_point=enhance` 返回 enhance token 聚合
2. enhance 调用后（cache miss），该端点 count 增加
3. `GET /api/v1/dashboard/trends` 返回完整 7 个 trend 字段（call_trend/duration_dist/req_dist/success_trend/token_trend/output_stats/roi_trend）
4. `token_trend` 从 `llm_usage` 真实聚合（非 ×500 假数据）
5. UI-160 案例名"增强调用计入 Token 统计" + 验证 count+1
6. dashboard 所有 trend 显示真数据：`chart-call-trend`/`chart-duration-dist`/`chart-success-trend`/`chart-token-trend`/`chart-output-stats`/`chart-roi-dual` 有数据点
7. `dashboard-token-value-0`（Token 消耗）显示真实数字（非 "—"）
8. `go test ./internal/...` 全通过，覆盖率 ≥98%
9. CI sonar-check + ui-tests 通过
10. `/api/v1/stats/llm` 需 admin 权限；`/api/v1/dashboard/trends` 需登录
11. `computeTokenTrend` ×500 假函数已删除，`grep -rn "computeTokenTrend" internal/` 为空
12. `ComputeTrends` 签名清理死参数（移除未用的 sessions/docCount），新签名 `(tasks, tokenBuckets)`
13. `page.tsx` `emptyChart` 死代码已删除

## 11. 不在本 spec 范围

- dashboard `timeFilter`（today/week/month）联动 trends 时间范围 → 后续优化（前端 L121-130 有 filter UI，但后端 ComputeTrends 硬编码 24h/7d）
- `req_dist` 真实化（现状 = call_trend 重复数据，语义为"24h agent 调用分布"；真实 API 请求量统计超出范围）
- `roi_trend` 语义优化（现状是 4 周 completed task 数，前端 L179 计算倍数，逻辑可改进但现有合理）
- `/api/v1/admin/dashboard`（stats 端点）路径与前端 `/dashboard` 不匹配问题 → 附带发现，本 spec 不修（聚焦 token + trends）
- enhance cache hit 时 token 统计补偿（cache hit 不计 token 是合理行为）
