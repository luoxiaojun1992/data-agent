# UI E2E 测试设计 — 数据看板 (DASH)

> **SPEC-022** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 数据看板（Dashboard）的 E2E UI 测试用例规范。覆盖问候语、时间筛选、KPI 卡片行、Agent 调用量趋势图、任务状态分布、任务耗时分布柱状图、24h 请求量分布、成功率趋势面积图、Token/产出/ROI 指标卡片、Token 消耗趋势堆叠柱状图、产出统计分组柱状图、AI Agent ROI 双轴图表和实时更新 Badge。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §9
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-062 ~ UI-074（共 13 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-010 | ✅ | 系统统计监控（Dashboard 数据来源） |
| SPEC-013 | ✅ | 管理后台（Dashboard 页面定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-018 | ✅ | LAYOUT |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 问候语与日期 | UI-062 | P0 |
| 时间筛选 | UI-063 | P0 |
| KPI 卡片行 | UI-064 | P0 |
| Agent 调用量趋势图（Chart.js 折线图） | UI-065 | P0 |
| 任务状态分布面板 | UI-066 | P0 |
| 任务耗时分布柱状图 | UI-067 | P1 |
| 24h 请求量分布柱状图 | UI-068 | P1 |
| 成功率趋势面积图 | UI-069 | P1 |
| Token/产出/ROI 指标卡片行 | UI-070 | P0 |
| Token 消耗趋势堆叠柱状图 | UI-071 | P1 |
| 产出统计分组柱状图 | UI-072 | P1 |
| AI Agent ROI 双轴图表 | UI-073 | P1 |
| 图表实时更新 Badge | UI-074 | P2 |

## 4. 测试用例

### UI-062: Dashboard — 问候语与日期

- **优先级**: P0
- **前置条件**: 以管理员身份登录，点击数据看板
- **步骤**:
  1. 检查页面顶部
- **预期结果**:
  - 左侧：「下午好，管理员 👋」(24px Bold)
  - 下方：「实时数据概览 · 2026-07-09」(13px #7A7A7A)
  - （根据当前时间动态显示「上午好/下午好/晚上好」和角色名）
  - `data-testid`: `dashboard-greeting`, `dashboard-subtitle`

### UI-063: Dashboard — 时间筛选

- **优先级**: P0
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查时间筛选标签
  2. 点击「本周」
  3. 点击「本月」
  4. 点击「今日」
- **预期结果**:
  - 3 个标签：今日 / 本周 / 本月
  - 默认选中「今日」
  - 选中标签高亮（10% 白色背景 + `#B1E2FF` #B1E2FF 文字 + 700 字重）
  - 切换后 KPI 卡片数据和图表数据相应更新
  - `data-testid`: `dashboard-time-today`, `dashboard-time-week`, `dashboard-time-month`

### UI-064: Dashboard — KPI 卡片行

- **优先级**: P0
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查 4 个 KPI 卡片
- **预期结果**:
  - 4 列等宽网格布局（`grid-template-columns: repeat(4, 1fr)`）
  - 每个卡片：玻璃背景 + 28px 圆角 + 32px 内边距
  - KPI 1：Chat 查询 — 标签紫色系 + 数值 1,247 (42px IBM Plex Mono SemiBold) + ↑ 12.5% vs 昨日
  - KPI 2：Agent 任务 — 标签蓝色系 + 数值 38 + ↑ 8.3% vs 昨日
  - KPI 3：成功率 — 标签绿色系 + 数值 96.8% + ↑ 2.1% vs 上周
  - KPI 4：在线用户 — 标签琥珀色系 + 数值 24 + ↓ 2 vs 昨日
  - 卡片间距 20px
  - `data-testid`: `dashboard-kpi-row`, `dashboard-kpi-{0-3}`, `dashboard-kpi-{0-3}-value`, `dashboard-kpi-{0-3}-change`
- **相关设计**: UI原型设计文档 §3.5

### UI-065: Dashboard — Agent 调用量趋势图（Chart.js 折线图）

- **优先级**: P0
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查第一个大图表
- **预期结果**:
  - 标题：「Agent 调用量」+ 「实时」绿色 Badge（呼吸灯动画）
  - 折线图：带渐变填充面积
  - X 轴：时间（如 00:00 ~ 24:00）
  - Y 轴：调用次数
  - Chart.js 渲染，Canvas 元素存在
  - `data-testid`: `dashboard-chart-agent-calls`, `dashboard-live-badge`

### UI-066: Dashboard — 任务状态分布面板

- **优先级**: P0
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查 Agent 调用量图右侧面板
- **预期结果**:
  - 标题：「任务状态分布」
  - 4 行状态统计：
    - ✅ 已完成 24 (绿色)
    - 🔵 执行中 8 (蓝色)
    - 🟡 排队中 4 (琥珀色)
    - ❌ 失败 2 (粉色)
  - 或为 Donut 图表形式展示
  - `data-testid`: `dashboard-task-status-panel`, `dashboard-task-status-donut`

### UI-067: Dashboard — 任务耗时分布柱状图

- **优先级**: P1
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查图表行 2 的第一个图表
- **预期结果**:
  - 标题：「各类型任务耗时分布 (P50/P95)」
  - 柱状图：4 种分析类型的 P50 和 P95 对比
  - SQL（蓝色渐变柱）· 回归（紫色渐变柱）· 聚类（绿色渐变柱）· 财务（琥珀渐变柱）
  - `data-testid`: `dashboard-chart-task-duration`

### UI-068: Dashboard — 24h 请求量分布柱状图

- **优先级**: P1
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查图表行 2 的第二个图表
- **预期结果**:
  - 标题：「24h 请求量分布」
  - 绿色渐变柱状图
  - X 轴：小时（0-23）
  - Y 轴：请求数
  - `data-testid`: `dashboard-chart-24h-requests`

### UI-069: Dashboard — 成功率趋势面积图

- **优先级**: P1
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查图表行 2 的第三个图表
- **预期结果**:
  - 标题：「成功率趋势 (7天)」
  - 绿色面积图
  - X 轴：日期（近 7 天）
  - Y 轴：成功率百分比
  - `data-testid`: `dashboard-chart-success-rate`

### UI-070: Dashboard — Token/产出/ROI 指标卡片行

- **优先级**: P0
- **前置条件**: 在数据看板页面，向下滚动
- **步骤**:
  1. 检查第二个 KPI 卡片行
- **预期结果**:
  - 4 个 KPI 卡片：
    - Token 消耗 (今日)：「1,482K」+ 成本 ¥14.82 (青色)
    - 本月累计 Token：「38,450K」+ 成本 ¥384.50 (绿色)
    - 产出统计：「1,047」+ 报告127·图表843·导出77 (紫色)
    - AI Agent ROI：「2,600%」+ 节省 847 人时 (琥珀色)
  - `data-testid`: `dashboard-kpi-row-2`, `dashboard-kpi-token-today`, `dashboard-kpi-token-month`, `dashboard-kpi-output`, `dashboard-kpi-roi`

### UI-071: Dashboard — Token 消耗趋势堆叠柱状图

- **优先级**: P1
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查图表行 3 的第一个图表
- **预期结果**:
  - 标题：「Token 消耗趋势（按模型）」
  - 堆叠柱状图：按 GPT-4o / GPT-4o-mini / Claude 3.5 分色
  - `data-testid`: `dashboard-chart-token-trend`

### UI-072: Dashboard — 产出统计分组柱状图

- **优先级**: P1
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查图表行 3 的第二个图表
- **预期结果**:
  - 标题：「产出统计（本周）」
  - 分组柱状图：报告 / 图表 / 数据导出 三类对比
  - `data-testid`: `dashboard-chart-output-stats`

### UI-073: Dashboard — AI Agent ROI 双轴图表

- **优先级**: P1
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查图表行 3 的第三个图表
- **预期结果**:
  - 标题：「AI Agent ROI 分析」
  - 双轴图表：左轴 AI 成本 (¥) / 右轴 等效人时节省
  - `data-testid`: `dashboard-chart-roi`

### UI-074: Dashboard — 图表实时更新 Badge

- **优先级**: P2
- **前置条件**: 在数据看板页面
- **步骤**:
  1. 检查带「实时」Badge 的图表
- **预期结果**:
  - Badge 内有绿色脉冲圆点动画（2s 周期）
  - 「实时」文字 11px 绿色 Bold
  - `data-testid`: `dashboard-live-badge`, `dashboard-live-dot`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — Dashboard 统计数据 API、图表数据 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 完整的 KPI 数据、图表时间序列数据 mock |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | **High** — 需要 mock 7+ 个不同图表的 Chart.js/Canvas 数据；需要 mock 时间筛选后的数据更新 |
| 是否有无法实现的用例 | 无 — Chart.js Canvas 图表可通过 `page.evaluate()` 获取实例数据验证；DOM 元素（KPI 卡片、指标行）可直接读取；`UI-074` 实时 Badge CSS animation 可通过 `getComputedStyle` 验证 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/dashboard.spec.ts` | DASH E2E 测试实现 | New |
| `tests/ui/fixtures/dashboard.fixture.ts` | Dashboard mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-062 ~ UI-066, UI-070）必须通过
- [ ] 所有 P1 用例（UI-067 ~ UI-069, UI-071 ~ UI-073）必须通过
- [ ] 所有 P2 用例（UI-074）建议通过
- [ ] Dashboard UI 100% 符合 UI 原型设计文档 §3.5
- [ ] 时间筛选切换后数据正确更新

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock Dashboard API（`page.route()` 拦截 `/api/v1/dashboard/**`, `/api/v1/stats/**`）
- [ ] **必须** 验证 KPI 卡片数值的正确渲染（非 Canvas，可直接读取 DOM）
- [ ] **必须** 对于 Canvas 图表，通过 `page.evaluate()` 获取 Chart.js 实例数据并验证
- [ ] **必须** 测试时间筛选标签的切换和数据更新
- [ ] **严禁** 依赖真实后端统计数据
- [ ] **严禁** 使用截图对比作为唯一验证手段（Canvas 除外）

参考: `.agent/memory/E2E_TESTING.md`
