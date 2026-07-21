# 增强调用 Token 统计验证（UI-160 案例名纠正 + LLM Token 查询 API）

> **SPEC-059** | Status: 设计中

## 1. 目标

纠正 UI-160 案例名与验证逻辑错误：增强调用**已经计入** Token 统计（SPEC-051 `recordEnhanceTokens` 写 `llm_usage`，CallPoint=`enhance`），但 UI-160 案例名为"增强调用不计入 Token 统计"，名实不符，且测试逻辑只验证 UI 流程（输入框替换），未真正验证 token 统计。

为让 UI-160 能真正验证"增强调用计入 Token 统计"，新增 LLM Token 查询 API（从 `llm_usage` 聚合），UI-160 改为：enhance 前查 enhance token → enhance → 查 → 验证增加。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-051 | ✅ | LLM 全链路 Token 统计（`llm_usage` 集合 + `recordEnhanceTokens` 已实现） |
| SPEC-043 | ✅ | Mock Model Service（UI 测试用 mockllm，enhance handler 真实执行） |
| SPEC-033 | ✅ | UI E2E — 增强提示词（UI-156~160 所在 spec） |
| — | — | 无阻塞前置依赖 |

## 2. 背景

### 2.1 现状调研（2026-07-21）

**(A) 后端：enhance 已计入 token 统计** ✅

`cmd/server/main.go`：
- `recordEnhanceTokens` (L713) 调用 `deps.llmRecorder.Record(ctx, llmstats.Record{CallPoint: "enhance", ...})`
- `makeEnhanceHandler` (L727) 在 L753 调用 `recordEnhanceTokens`（cache miss 路径）
- `llmstats.Recorder.Record` 写 MongoDB `llm_usage` 集合

> 历史背景：SPEC-051 (PR #63) 曾因 UI-158 失败错误回退 enhance token recording，晓军 2026-07-18 纠正后已恢复（红线：禁止降级功能）。当前 main 上 `recordEnhanceTokens` 已恢复执行。

**(B) token 查询 API 缺失** ❌

SPEC-051 §4.3 设计了"monitor token trend 改为从 `llm_usage` 聚合（替换现有基于 CallTrend 的估算）"，但**未落地**：
- `internal/service/monitor/trends.go` L35: `TokenTrend = computeTokenTrend(t.CallTrend)` —— 仍从 task 调用趋势派生，**不含 enhance**（enhance 不创建 task）
- 无独立的 LLM token 查询端点（`grep` handler/ 无 `llm_usage` 查询）
- 前端 `frontend/src/app/dashboard` 无 token 统计组件

**(C) UI-160 案例名与逻辑错误** ❌

`tests/ui/prompt.spec.ts` L104:
```ts
test('[UI-160] Prompt — 增强调用不计入 Token 统计', async ({ page, request }) => {
  // ... mock enhance，点击增强按钮，验证输入框内容替换
  // (Token stats are tracked server-side; we verify enhance UI flow works)  ← L121 注释
});
```
- 案例名"不计入"与后端实际行为相反
- 测试逻辑只验证 UI 流程（输入框替换 + dashboard 加载），**未验证 token 统计**

### 2.2 问题影响

- UI-160 名实不符，误导后续维护者认为 enhance 不计 token
- enhance token 统计"写入但读不出"，无法在前端/API 验证
- SPEC-051 §4.3 的 monitor 聚合设计悬空

## 3. 架构概述

```
enhance 请求 → makeEnhanceHandler → recordEnhanceTokens → llm_usage (CallPoint=enhance)
                                        (已实现 ✅)

查询 API (新增):
GET /api/v1/stats/llm?call_point=enhance
  → handler/stats.go LLMStatsHandler
  → service/llmstats 查询方法 (新增 Aggregate)
  → llm_usage 聚合 → {call_point, prompt_tokens, completion_tokens, count}

UI-160 (改造):
enhance 前查 /api/v1/stats/llm?call_point=enhance → 记录 count
→ 点击增强按钮 (mockllm mock, 后端真实 recordEnhanceTokens)
→ enhance 后查 /api/v1/stats/llm?call_point=enhance → 验证 count 增加
```

## 4. API 设计

| Method | Path | Description | 权限 |
|--------|------|-------------|------|
| GET | `/api/v1/stats/llm` | LLM token 统计聚合查询 | admin |

**Query 参数**：
- `call_point` (可选): 过滤 call_point（`enhance`/`chat`/`embedding`/`compaction`），不传则返回全部
- `since` (可选): ISO 8601 时间，只统计该时间之后的记录

**响应**：
```json
{
  "stats": [
    {
      "call_point": "enhance",
      "count": 5,
      "prompt_tokens": 120,
      "completion_tokens": 340,
      "total_tokens": 460
    }
  ]
}
```

## 5. 详细设计

### 5.1 llmstats 查询方法（新增）

`internal/infra/llmstats/llmstats.go` 现有 `Recorder.Record`（写）。新增查询方法：

```go
// Aggregate returns token usage aggregated by call_point.
// If callPoint is empty, all call points are included. If since is zero, no time filter.
func (r *Recorder) Aggregate(ctx context.Context, callPoint string, since time.Time) ([]AggregateResult, error)

type AggregateResult struct {
    CallPoint        string `bson:"_id" json:"call_point"`
    Count            int64  `bson:"count" json:"count"`
    PromptTokens     int64  `bson:"prompt_tokens" json:"prompt_tokens"`
    CompletionTokens int64  `bson:"completion_tokens" json:"completion_tokens"`
}
```

MongoDB aggregation pipeline：`$match` (call_point + since) → `$group` by call_point。

### 5.2 LLMStatsHandler（新增）

`internal/api/handler/stats.go`（新文件）：
- `LLMStatsHandler` 持 `*llmstats.Recorder`
- `GetLLMStats(c *gin.Context)`：解析 query → 调 `Recorder.Aggregate` → 返回 JSON
- 路由注册：`router.GET("/api/v1/stats/llm", jwtManager.AuthMiddleware(), adminGuard, statsHandler.GetLLMStats)`

### 5.3 UI-160 改造

`tests/ui/prompt.spec.ts` L104：

```ts
// ═══ UI-160: 增强调用计入 Token 统计 ═══
test('[UI-160] Prompt — 增强调用计入 Token 统计', async ({ page, request }) => {
  await clearMocks(request);
  await seedMock(request, 'test token', '增强后的测试内容');

  // 1. enhance 前查 enhance token count
  const before = await getLLMStats(request, token, 'enhance');
  const beforeCount = before.stats[0]?.count ?? 0;

  // 2. 触发 enhance (mockllm mock, 后端真实 recordEnhanceTokens)
  const input = page.locator('[data-testid="chat-input"]');
  await input.fill('test token');
  await page.locator('[data-testid="chat-enhance-btn"]').click();
  await expect(input).toHaveValue('增强后的测试内容', { timeout: 5000 });

  // 3. enhance 后查 enhance token count，验证增加
  //    (轮询：recordEnhanceTokens 异步写 llm_usage)
  await expect.poll(async () => {
    const after = await getLLMStats(request, token, 'enhance');
    return after.stats[0]?.count ?? 0;
  }, { timeout: 5000 }).toBe(beforeCount + 1);
});
```

辅助函数 `getLLMStats(request, token, callPoint)` 调 `GET /api/v1/stats/llm?call_point=enhance`。

> **注意 cache miss**：`makeEnhanceHandler` L742-747 cache hit 时不调 `recordEnhanceTokens`。UI-160 用唯一 prompt（`'test token-' + uid`）保证 cache miss。

### 5.4 monitor TokenTrend 死代码清理（纳入范围）

调研发现 `internal/service/monitor/trends.go` 的 `computeTokenTrend` 是 **×500 硬编码假数据**：

```go
// L127 — 把 CallTrend（task 调用次数）×500 当 token 趋势，毫无逻辑
func computeTokenTrend(callTrend []TrendPoint) []TrendPoint {
    for _, p := range callTrend {
        trend = append(trend, TrendPoint{Label: p.Label, Value: p.Value * 500})
    }
}
```

更严重：`ComputeTrends` 整个函数生产代码**没人调用**（只 `monitor_test.go` 测试用），`/api/v1/system/stats`（`monitor.Handler`）只返回 `SystemStats`（runtime 内存/CPU/goroutines），**不返回任何 trend**；前端 `frontend/src` 也不消费 `call_trend`/`token_trend` 等字段。所以整个 `DashboardTrends`（含 TokenTrend/CallTrend/DurationDist/SuccessTrend/OutputStats/ROITrend）是**死代码**。

SPEC-051 §4.3 写的"monitor token trend 改为从 llm_usage 聚合"无从落地——monitor 根本没暴露 trends。

**本 spec 处理**（聚焦 token，不清理整个 trends.go）：
- 删除 `computeTokenTrend` 函数（×500 假数据）
- `DashboardTrends.TokenTrend` 字段移除（死代码 + 假数据，无消费方）
- `ComputeTrends` L35 删除 `t.TokenTrend = computeTokenTrend(t.CallTrend)`
- `monitor_test.go` 删除 `TestComputeTrends_TokenTrend`（L385）及相关断言
- **不删**其他 trend 函数（CallTrend/DurationDist 等逻辑合理，虽未接入但可能未来 dashboard 用，超出本 spec 范围）

> token 统计的真实查询入口统一走本 spec 新增的 `/api/v1/stats/llm`（从 llm_usage 聚合），不再走 monitor 的假 TokenTrend。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `llm_usage`） |
| 是否影响现有 API | No（新增端点，向后兼容） |
| 是否影响现有 UI | No（UI-160 改造，无新 UI 组件） |
| 性能影响 | 低（aggregation 查询，可加索引） |
| 是否需要新增 Skill | No |
| 风险等级 | 低 — 新增查询端点 + 改一个 E2E case |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/infra/llmstats/llmstats.go` | 新增 `Aggregate` 查询方法 + `AggregateResult` struct | Modify |
| `internal/infra/llmstats/llmstats_test.go` | 新增 Aggregate 测试 | Modify |
| `internal/api/handler/stats.go` | 新增 `LLMStatsHandler` + `GetLLMStats` | **New** |
| `internal/api/handler/stats_test.go` | handler 测试 | **New** |
| `cmd/server/main.go` | 注册 `/api/v1/stats/llm` 路由 + 注入 recorder | Modify |
| `internal/service/monitor/trends.go` | 删除 `computeTokenTrend` ×500 假函数 + `DashboardTrends.TokenTrend` 字段 | Modify |
| `internal/service/monitor/monitor_test.go` | 删除 `TestComputeTrends_TokenTrend` + TokenTrend 断言 | Modify |
| `tests/ui/prompt.spec.ts` | UI-160 改名 + 改验证逻辑 + `getLLMStats` 辅助函数 | Modify |
| `.agent/memory/E2E_TESTING.md` | UI-160 描述更新 | Modify |

## 8. 测试策略

1. **Unit tests**（Go）:
   - `llmstats.Aggregate`：mock mongo collection，验证 aggregation pipeline（call_point 过滤、since 过滤、group 聚合）
   - `LLMStatsHandler.GetLLMStats`：mock Recorder，验证 query 解析 + 响应格式 + 权限
   - 覆盖率 ≥98%（CI `ut-workflow.yml`）
2. **E2E tests**（UI）:
   - UI-160 改造：enhance 前后 token count 验证
   - CI: `ui-tests.yml` + mockllm（enhance handler 真实执行 recordEnhanceTokens）
3. **审计**: `.agent/skills/go-ut-audit`

## 9. UI Test / E2E 验收规则

- [ ] **必须** UI-160 改名为"增强调用计入 Token 统计"
- [ ] **必须** UI-160 验证 enhance 后 `/api/v1/stats/llm?call_point=enhance` 的 count 增加
- [ ] **必须** CI sonar-check + ui-tests 通过
- [ ] **严禁** 降级 UI-160 断言（如只验证 UI 流程不验证 token）

## 9.5. Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | llmstats.Aggregate 纯聚合逻辑 | **100%** |
| L3 | handler/stats GetLLMStats | **98%** |

- [ ] `Aggregate` 测试覆盖：call_point 过滤、since 过滤、空结果、多 call_point 聚合
- [ ] `GetLLMStats` 测试覆盖：正常查询、无 call_point 参数、权限拒绝、Recorder 错误
- [ ] Success 测试 ≥2 个行为验证断言

## 10. 验证标准

1. `GET /api/v1/stats/llm?call_point=enhance` 返回 enhance token 聚合（count/prompt_tokens/completion_tokens）
2. enhance 调用后（cache miss），该端点返回的 count 增加
3. UI-160 案例名为"增强调用计入 Token 统计"
4. UI-160 测试逻辑验证 enhance 前后 token count 增加（非仅 UI 流程）
5. `go test ./internal/...` 全通过，覆盖率 ≥98%
6. CI sonar-check + ui-tests 通过
7. `/api/v1/stats/llm` 端点需 admin 权限（非 admin 返回 403）
8. `computeTokenTrend` ×500 假函数已删除，`DashboardTrends.TokenTrend` 字段已移除
9. `grep -rn "computeTokenTrend\|TokenTrend" internal/` 为空

## 11. 不在本 spec 范围

- monitor 其他 trend 函数（CallTrend/DurationDist/SuccessTrend/OutputStats/ROITrend）的激活或清理 —— 它们逻辑合理但生产未接入，超出本 spec 范围
- 前端 dashboard 显示 LLM token 统计组件 → 另开 spec
- enhance cache hit 时 token 统计补偿（cache hit 不计 token 是合理行为，本 spec 不改）
