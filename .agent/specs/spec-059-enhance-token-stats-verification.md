# 统计分析架构归置 + Token 统计真数据

> **SPEC-059** | Status: 设计中
>
> **范围说明**: 原 SPEC-059 过大，经 2026-07-21 晓军确认拆分：
> - **SPEC-059（本 spec）**: 统计分析架构归置（trend 归属 stats 不割裂）+ Token 统计真数据（查询能力 + UI-160 验证 + 假数据清理）
> - **SPEC-060**: Dashboard trend 接入（CallTrend/DurationDist 等）+ 路径/字段修复 + 前端全 trend 显示（依赖本 spec 架构 + AggregateByTime 能力）

## 1. 目标

1. **系统整理归置统计分析架构**：当前统计分析分散在 `monitor`（trend 计算死代码）/ `dashboard handler`（stats API）/ `llmstats`（token 写入），且 trend 与 stats 割裂。本 spec 统一归置：明确职责划分 + 统一 API 域（**trend 属于 stats，不割裂**）+ 前端展示规范。
2. **Token 统计真数据**：enhance 已计入 `llm_usage` 但读不出。本 spec 建设 token 查询能力（`llmstats.Aggregate`/`AggregateByTime` + `/api/v1/stats/llm` 端点）+ UI-160 验证 + 清理 monitor ×500 假数据死代码。

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
- `llmstats` 只有 `Record`（写），无查询方法

**(C) monitor `computeTokenTrend` 是 ×500 假数据 + 死代码** ❌
`trends.go` L127：`Value: p.Value * 500`（CallTrend ×500 当 token）。`ComputeTrends` 生产代码无人调用（只测试用）。

**(D) 统计分析分散割裂** ❌
- `monitor`（trend 计算）— 死代码，未接入 API
- `dashboard handler`（stats API）— `/admin/dashboard` 返回 `{tasks,sessions,docs}`
- `llmstats`（token 写入）— 无查询
- trend 与 stats 割裂：前端调 `/dashboard`（stats）+ `/dashboard/trends`（trend）两个端点，后端 trend 端点不存在

**(E) UI-160 错误** ❌
`tests/ui/prompt.spec.ts` L104 名"增强调用不计入 Token 统计"（实际计入），测试逻辑只验证 UI 流程，未验证 token。

### 2.2 问题根因

统计分析"写入但读不出"+ "逻辑分散未归置"——`llm_usage` 有真实 token 数据，但无查询 API；monitor trend 计算是死代码假数据；stats/trend 割裂在不同位置。

## 3. 架构概述（统计分析统一归置）

### 3.1 职责划分（归置）

| 模块 | 职责 | 本 spec 改动 |
|------|------|-------------|
| `llmstats`（infra） | token 数据源：写入 `llm_usage`（已实现）+ **查询聚合**（新增） | 新增 `Aggregate`/`AggregateByTime` |
| `monitor`（service） | trend 计算：`ComputeTrends` 从 task + llmstats 数据计算趋势 | 清理 `computeTokenTrend` ×500 假函数（trend 计算改造在 SPEC-060） |
| `dashboard handler`（api） | API 编排：`/dashboard`（stats）+ `/dashboard/trends`（trends） | 本 spec 实现 `/stats/llm`；`/dashboard`+`/dashboard/trends` 在 SPEC-060 |
| 前端 | dashboard 首页统一展示 stats KPI + trend 图表 | token 展示规范定义；全 trend 显示在 SPEC-060 |

### 3.2 统一 API 域（trend 属于 stats，不割裂）

```
/api/v1/dashboard          — stats（KPI: task_stats/kb_docs）           [SPEC-060 修复]
/api/v1/dashboard/trends   — trends（含 token_trend + 其他 trend）       [SPEC-060 实现]
/api/v1/stats/llm          — LLM token 按 call_point 聚合（UI-160 验证） [本 spec 实现]
```

> **trend 是 stats 的趋势维度**，归属 `/dashboard/*` 同一 API 域，不割裂。`/stats/llm` 是 LLM token 专用查询（按 call_point 聚合，供 UI-160 验证 enhance 计入）。

## 4. API 设计

| Method | Path | Description | 权限 | 实现 Spec |
|--------|------|-------------|------|-----------|
| GET | `/api/v1/stats/llm` | LLM token 按 call_point 聚合 | admin | SPEC-059（本 spec） |
| GET | `/api/v1/dashboard` | stats（KPI） | 登录 | SPEC-060 |
| GET | `/api/v1/dashboard/trends` | trends（含 token_trend） | 登录 | SPEC-060 |

**`/api/v1/stats/llm` query**：`call_point`(可选)、`since`(可选, ISO8601)
**响应**：`{stats: [{call_point, count, prompt_tokens, completion_tokens, total_tokens}]}`

## 5. 详细设计

### 5.1 统计分析架构归置（设计）

定义统计分析的统一架构（§3.1 职责划分 + §3.2 API 域）。后续 SPEC-060 按此架构实现 `/dashboard` + `/dashboard/trends`。

**归置原则**：
- trend 属于 stats 的趋势维度，同一 API 域（`/dashboard/*`）
- token 数据源统一在 `llmstats`（写+读），monitor/handler 不直接操作 `llm_usage`
- monitor 负责纯计算（trend 聚合），不持有数据源依赖
- handler 负责编排（调 monitor + llmstats），不写计算逻辑

### 5.2 llmstats 查询方法（新增）

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

// AggregateByTime 按时间桶聚合 token 用量（供 SPEC-060 token_trend 趋势图用）。
// buckets 为桶大小（毫秒），返回每桶的 token 总量。
func (r *Recorder) AggregateByTime(ctx context.Context, since time.Time, bucketMs int64) ([]TimeBucketResult, error)

type TimeBucketResult struct {
    BucketStart time.Time `bson:"_id" json:"bucket_start"`
    TotalTokens int64     `bson:"total_tokens" json:"total_tokens"`
}
```

MongoDB aggregation pipeline：
- `Aggregate`: `$match`(call_point+since) → `$group` by call_point
- `AggregateByTime`: `$match`(since) → `$bucket` of `created_at` → sum(prompt+completion)

> `AggregateByTime` 本 spec 实现（token 查询能力），SPEC-060 的 `/dashboard/trends` 端点调用它生成 `token_trend`。

### 5.3 `GET /api/v1/stats/llm` 端点（UI-160 验证用）

`internal/api/handler/stats.go`（新文件）：
- `LLMStatsHandler` 持 `*llmstats.Recorder`
- `GetLLMStats(c *gin.Context)`：解析 query → `Recorder.Aggregate` → JSON
- 路由：`router.GET("/api/v1/stats/llm", jwtManager.AuthMiddleware(), adminGuard, statsHandler.GetLLMStats)`
- main.go 注入 `deps.llmRecorder`

### 5.4 UI-160 改造

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

辅助函数 `getLLMStats(request, token, callPoint)` 调 `GET /api/v1/stats/llm?call_point=enhance`。

### 5.5 monitor `computeTokenTrend` ×500 假数据清理

`internal/service/monitor/trends.go`：
- 删除 `computeTokenTrend` 函数（L127-133，×500 硬编码假数据）
- `ComputeTrends` L35 `t.TokenTrend = computeTokenTrend(t.CallTrend)` 删除（`TokenTrend` 字段暂留空，SPEC-060 的 ComputeTrends 改造会用 `AggregateByTime` 填充真实数据）
- `monitor_test.go` 删除 `TestComputeTrends_TokenTrend`（L385）及相关断言

> `ComputeTrends` 签名改造（加 `tokenBuckets` 参数）+ `token_trend` 真实填充 + 死参数清理（sessions/docCount）→ SPEC-060。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `llm_usage`） |
| 是否影响现有 API | No（新增 `/stats/llm`，向后兼容） |
| 是否影响现有 UI | No（UI-160 改造，无新 UI 组件） |
| 性能影响 | 低（aggregation 查询，`llm_usage` 已有索引） |
| 风险等级 | 低 — 新增查询端点 + 查询方法 + 改一个 E2E case + 删死代码 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/infra/llmstats/llmstats.go` | 新增 `Aggregate` + `AggregateByTime` + 结果 struct | Modify |
| `internal/infra/llmstats/llmstats_test.go` | 新增查询方法测试 | Modify |
| `internal/api/handler/stats.go` | 新增 `LLMStatsHandler` + `GetLLMStats` | **New** |
| `internal/api/handler/stats_test.go` | handler 测试 | **New** |
| `cmd/server/main.go` | 注册 `/api/v1/stats/llm` 路由 + 注入 recorder | Modify |
| `internal/service/monitor/trends.go` | 删除 `computeTokenTrend` ×500 假函数 + L35 TokenTrend 赋值 | Modify |
| `internal/service/monitor/monitor_test.go` | 删除 `TestComputeTrends_TokenTrend` | Modify |
| `tests/ui/prompt.spec.ts` | UI-160 改名 + 改验证逻辑 + `getLLMStats` 辅助 | Modify |
| `.agent/memory/E2E_TESTING.md` | UI-160 描述更新 | Modify |

## 8. 测试策略

1. **Unit tests**（Go）:
   - `llmstats.Aggregate`/`AggregateByTime`：mock mongo collection，验证 pipeline（过滤/分组/桶聚合）
   - `LLMStatsHandler.GetLLMStats`：query 解析 + 响应 + 权限
   - 覆盖率 ≥98%
2. **E2E tests**（UI）:
   - UI-160：enhance 前后 `/stats/llm` count+1
   - CI: `ui-tests.yml` + mockllm

## 9. UI Test / E2E 验收规则

- [ ] **必须** UI-160 改名"增强调用计入 Token 统计" + 验证 `/stats/llm` count+1
- [ ] **必须** CI sonar-check + ui-tests 通过
- [ ] **严禁** 降级断言

## 9.5. Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | llmstats 聚合逻辑 | **100%** |
| L3 | handler/stats GetLLMStats | **98%** |

- [ ] `Aggregate`/`AggregateByTime` 覆盖：过滤、空结果、多 call_point、桶对齐
- [ ] `GetLLMStats` 覆盖：正常、无 call_point 参数、权限拒绝、Recorder 错误
- [ ] Success 测试 ≥2 个行为验证断言

## 10. 验证标准

1. `GET /api/v1/stats/llm?call_point=enhance` 返回 enhance token 聚合
2. enhance 调用后（cache miss），该端点 count 增加
3. UI-160 案例名"增强调用计入 Token 统计" + 验证 count+1
4. `llmstats.Aggregate` + `AggregateByTime` 可用（SPEC-060 将调用 `AggregateByTime`）
5. `computeTokenTrend` ×500 假函数已删除，`grep -rn "computeTokenTrend" internal/` 为空
6. `go test ./internal/...` 全通过，覆盖率 ≥98%
7. CI sonar-check + ui-tests 通过
8. `/api/v1/stats/llm` 需 admin 权限

## 11. 不在本 spec 范围（→ SPEC-060）

- `/admin/dashboard` → `/dashboard` 路径修复 + `Get` 返回 `{task_stats, kb_docs}` 字段对齐
- `GET /api/v1/dashboard/trends` 端点实现（所有 trend 接入：CallTrend/DurationDist/SuccessTrend/OutputStats/ROITrend + token_trend 用本 spec `AggregateByTime`）
- `ComputeTrends` 改造（加 `tokenBuckets` 参数 + `token_trend` 真实填充 + 死参数 sessions/docCount 清理）
- 测试 `/dashboard/stats` → `/dashboard` 路径对齐（5 处）
- 前端全 trend 显示真数据 + E2E（UI-2XX）
- `page.tsx` `emptyChart` 死代码清理
