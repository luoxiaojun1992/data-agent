# UI E2E 测试设计 — Agent 模式（专业工作区）(AGENT)

> **SPEC-020** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent Agent 模式（专业工作区）的 E2E UI 测试用例规范。覆盖任务列表页渲染、新建分析任务（同步/异步/定时）、任务列表筛选、状态 Pill 渲染、任务详情（内联展开、进度条、步骤指示器、执行日志、Artifact 列表）、Artifact 批量下载、取消/重试任务等全部功能。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §7
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-039 ~ UI-056（共 18 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 | ✅ | Agent 核心引擎（任务创建、同步/异步执行） |
| SPEC-005 | ✅ | Artifact 存储（文件下载） |
| SPEC-009 | ✅ | 任务队列与调度（任务生命周期、定时任务） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-018 | ✅ | LAYOUT（导航到专业工作区） |
| SPEC-043 | 📐 | Mock Model Service（**P8 前置依赖**） |

### Mock Model Service 策略

> **硬性约束**: Agent 任务执行中的分析逻辑（SQL 生成、统计计算、报告写作）均来自 mock model service，**严禁使用真实 LLM API**。

| 测试场景 | Mock 方式 | 说明 |
|----------|----------|------|
| 同步任务执行 | `page.route('**/api/v1/agent/**')` → mock 任务结果 | 返回预定义的分析结果（含 SQL、表格、图表） |
| 异步任务生命周期 | mock 任务状态轮询（queued → running → done） | 逐步更新 task status 和 progress |
| 定时任务 | mock 创建成功 + 调度配置 | 不需要真实 cron 调度 |
| 任务日志 | mock SSE 日志流 | 分片发送预定义日志行 |
| Artifact 下载 | `page.route('**/api/v1/artifacts/**')` | mock 文件列表 + 下载响应 |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 任务列表页渲染 | UI-039 | P0 |
| 新建分析任务（模态窗口） | UI-040 | P0 |
| 创建同步任务 | UI-041 | P0 |
| 创建异步任务 | UI-042 | P0 |
| 任务列表筛选 | UI-043 | P0 |
| 任务列表状态 Pill 渲染 | UI-044 | P0 |
| 任务详情 — 内联展开 | UI-045 | P0 |
| 任务详情 — 标题与操作按钮 | UI-046 | P0 |
| 任务详情 — 进度条 | UI-047 | P0 |
| 任务详情 — 步骤指示器 | UI-048 | P0 |
| 任务详情 — 执行日志 | UI-049 | P0 |
| 任务详情 — Artifact 列表 | UI-050 | P0 |
| 任务详情 — 批量下载 ZIP | UI-051 | P0 |
| 取消正在执行的任务 | UI-052 | P0 |
| 重试失败任务 | UI-053 | P1 |
| 定时任务创建 | UI-054 | P1 |
| 暂停/恢复定时任务 | UI-055 | P1 |
| 任务列表分页 | UI-056 | P1 |

## 4. 测试用例

### UI-039: Agent — 任务列表页渲染

- **优先级**: P0
- **前置条件**: 以分析师或管理员身份登录，点击「专业工作区」
- **步骤**:
  1. 检查页面渲染
- **预期结果**:
  - Header:「专业工作区 · 任务管理」
  - 右上角蓝紫渐变「+ 新建分析任务」按钮
  - 筛选标签行：全部 / 执行中 / 已完成 / 定时任务
  - 默认选中「全部」
  - 任务表格显示已有任务
  - `data-testid`: `agent-page-header`, `agent-create-btn`, `agent-filter-tabs`, `agent-task-table`
- **相关设计**: UI原型设计文档 §3.3

### UI-040: Agent — 新建分析任务（模态窗口）

- **优先级**: P0
- **前置条件**: 在专业工作区页面
- **步骤**:
  1. 点击「+ 新建分析任务」按钮
- **预期结果**:
  - 弹出新建任务模态窗口
  - 包含以下字段：
    - 任务名称（必填，文本输入）
    - 分析类型下拉（回归分析/聚类分析/PCA/时间序列/财务分析/聚合分析）
    - 执行模式（同步/异步 单选）
    - 描述/分析需求（文本域）
    - 数据源选择
  - 「取消」和「创建任务」按钮
  - `data-testid`: `agent-create-modal`, `agent-create-name`, `agent-create-type`, `agent-create-mode`, `agent-create-submit`
- **相关 Spec**: SPEC-004, SPEC-009

### UI-041: Agent — 创建同步任务

- **优先级**: P0
- **前置条件**: 新建任务模态窗口已打开
- **步骤**:
  1. 填写任务名称「Q3 销售预测分析」
  2. 选择「回归分析」
  3. 选择「同步模式」
  4. 点击「创建任务」
- **预期结果**:
  - 模态窗口关闭
  - 页面进入任务详情（内联展开或独立页面）
  - 实时显示执行进度
  - 执行完成后显示结果
  - 任务出现在任务列表中，状态为「已完成」
  - `data-testid`: `agent-task-detail`
- **相关 PRD**: PRD §F-05

### UI-042: Agent — 创建异步任务

- **优先级**: P0
- **前置条件**: 新建任务模态窗口已打开
- **步骤**:
  1. 填写任务名称「全国客户聚类分析」
  2. 选择「聚类分析」
  3. 选择「异步模式」
  4. 点击「创建任务」
- **预期结果**:
  - 模态窗口关闭
  - 显示成功提示：「任务已创建，任务ID: xxx」
  - 任务列表中出现新任务，状态为「排队中」或「执行中」
  - 不需要等待任务完成即可继续操作
  - `data-testid`: `agent-task-created-toast`
- **相关 PRD**: PRD §F-05

### UI-043: Agent — 任务列表筛选

- **优先级**: P0
- **前置条件**: 任务列表中有不同状态的任务
- **步骤**:
  1. 点击「执行中」筛选标签
  2. 点击「已完成」筛选标签
  3. 点击「定时任务」筛选标签
  4. 点击「全部」恢复
- **预期结果**:
  - 「执行中」：只显示状态为 running/queued 的任务
  - 「已完成」：只显示状态为 completed 的任务
  - 「定时任务」：只显示 type=scheduled 的任务
  - 「全部」：显示所有任务
  - 筛选标签高亮切换正确
  - `data-testid`: `agent-filter-tab-all`, `agent-filter-tab-running`, `agent-filter-tab-completed`, `agent-filter-tab-scheduled`

### UI-044: Agent — 任务列表状态 Pill 渲染

- **优先级**: P0
- **前置条件**: 有不同状态的任务
- **步骤**:
  1. 检查每个任务行的状态列
- **预期结果**:
  - 🟢 执行中：`pill-blue` (`rgba(177,226,255,0.15)` + `#B1E2FF`)
  - 🟡 排队中：`pill-amber` (`rgba(251,191,36,0.15)` + `#FBBF24`)
  - 🟢 已完成：`pill-green` (`rgba(52,211,153,0.15)` + `#34D399`)
  - 🔴 失败：`pill-pink` (`rgba(251,113,133,0.15)` + `#FB7185`)
  - Pill 样式：12px Bold，圆角 12px，内边距 5px 12px
  - `data-testid`: `agent-task-status-{taskId}`

### UI-045: Agent — 任务详情 — 内联展开

- **优先级**: P0
- **前置条件**: 任务列表中有任务
- **步骤**:
  1. 点击某个任务的「查看」按钮
- **预期结果**:
  - 在当前页面内展示详情面板（不跳转到独立页面 — F-21 要求）
  - 面板顶部显示「← 返回任务列表」按钮
  - 面板内容：进度条 + 步骤指示器 + 执行日志 + Artifact 列表
  - `data-testid`: `agent-task-detail-panel`, `agent-task-detail-back-btn`
- **相关 PRD**: PRD §F-21

### UI-046: Agent — 任务详情 — 标题与操作按钮

- **优先级**: P0
- **前置条件**: 已打开任务详情面板
- **步骤**:
  1. 检查面板 Header
- **预期结果**:
  - 标题：「任务详情 · {任务名称}」
  - 操作按钮：「取消任务」(ghost 样式) + 「下载结果」(gradient 样式，仅完成后显示)
  - `data-testid`: `task-detail-title`, `task-detail-cancel-btn`, `task-detail-download-btn`

### UI-047: Agent — 任务详情 — 进度条

- **优先级**: P0
- **前置条件**: 任务执行中（如 65%）
- **步骤**:
  1. 检查进度条区域
- **预期结果**:
  - 进度条高度 10px，背景 `rgba(255,255,255,0.10)`
  - 填充区域蓝紫渐变（`linear-gradient(135deg, #B1E2FF, #9381FF)`）
  - 百分比显示「65%」（IBM Plex Mono 24px Bold, #B1E2FF）
  - 动画过渡时间 1s ease
  - `data-testid`: `task-detail-progress-bar`, `task-detail-progress-fill`, `task-detail-progress-text`

### UI-048: Agent — 任务详情 — 步骤指示器

- **优先级**: P0
- **前置条件**: 任务详情已加载
- **步骤**:
  1. 检查步骤指示器
- **预期结果**:
  - 4 个步骤：SQL生成 / 数据提取 / 回归计算(或对应分析类型) / 生成报告
  - 已完成步骤：绿色圆点 (`#34D399`) + 绿色标签
  - 当前步骤：蓝色圆点 (`#B1E2FF` + pulse 动画) + 蓝色标签
  - 未开始步骤：灰色圆点 (`rgba(255,255,255,0.15)`) + `#666` 标签
  - 步骤标签 11px
  - `data-testid`: `task-detail-steps`, `task-detail-step-{0-3}`, `task-detail-step-dot-{0-3}`
- **相关设计**: UI原型设计文档 §3.4

### UI-049: Agent — 任务详情 — 执行日志

- **优先级**: P0
- **前置条件**: 任务详情已加载
- **步骤**:
  1. 检查执行日志区域
- **预期结果**:
  - 标题：「执行日志」
  - IBM Plex Mono 等宽字体 12px
  - 每条日志带时间戳 `[HH:MM:SS]`
  - 成功日志：绿色 (`#34D399`)
  - 进行中日志：蓝色 (`#B1E2FF`)
  - 失败日志：粉色 (`#FB7185`)
  - `data-testid`: `task-detail-log`, `task-detail-log-line-{index}`
- **相关设计**: UI原型设计文档 §3.4

### UI-050: Agent — 任务详情 — Artifact 列表

- **优先级**: P0
- **前置条件**: 任务已有产出物
- **步骤**:
  1. 检查右侧 Artifact 面板
- **预期结果**:
  - 标题：「产出物 (Artifacts)」
  - 每个 Artifact 项显示：
    - 文件图标 + 文件名 + 文件大小
    - 类型标签（chart/export/report/interim/screenshot）
    - Checkbox 用于多选
  - 单个文件可点击下载
  - `data-testid`: `task-detail-artifacts`, `task-detail-artifact-{id}`, `task-detail-artifact-checkbox-{id}`
- **相关设计**: UI原型设计文档 §3.4，PRD §F-17

### UI-051: Agent — 任务详情 — 批量下载 ZIP

- **优先级**: P0
- **前置条件**: 任务有多个 Artifact
- **步骤**:
  1. 勾选 2 个 Artifact 的 checkbox
  2. 点击「📦 打包下载 ZIP」按钮
- **预期结果**:
  - 按钮显示文件统计信息（如「打包下载 ZIP (2 个文件, 1.5 MB)」）
  - 触发 ZIP 下载
  - 下载文件名格式：`task_{task_id}_artifacts_{date}.zip`
  - `data-testid`: `task-detail-zip-download-btn`
- **相关 PRD**: PRD §F-21

### UI-052: Agent — 取消正在执行的任务

- **优先级**: P0
- **前置条件**: 有 status=running 的任务
- **步骤**:
  1. 在任务列表中找到执行中的任务
  2. 点击该任务的「取消」操作
  3. 确认取消
- **预期结果**:
  - 弹出确认弹窗：「确定要取消此任务吗？正在进行的计算将丢失。」
  - 确认后任务状态变为「已取消」
  - 任务列表自动刷新
  - `data-testid`: `agent-task-cancel-btn-{taskId}`, `task-cancel-confirm-modal`

### UI-053: Agent — 重试失败任务

- **优先级**: P1
- **前置条件**: 有 status=failed 的任务
- **步骤**:
  1. 在任务列表中找到失败的任务
  2. 点击「重试」按钮
- **预期结果**:
  - 任务被重新加入队列
  - 状态变为「排队中」
  - 保留原有任务参数
  - `data-testid`: `agent-task-retry-btn-{taskId}`

### UI-054: Agent — 定时任务创建

- **优先级**: P1
- **前置条件**: 在专业工作区，点击新建任务
- **步骤**:
  1. 在新建任务模态窗口中
  2. 填写任务信息
  3. 勾选「设为定时任务」
  4. 配置调度规则（每日 8:00 / 每周一 / 每月1号）
  5. 点击创建
- **预期结果**:
  - 调度配置区域出现：频率选择 + 时间选择
  - MVP 阶段支持：每日/每周/每月（下拉选择）
  - 创建成功后任务列表中显示「定时」标签
  - 定时任务在筛选时单独分组
  - `data-testid`: `agent-schedule-toggle`, `agent-schedule-frequency`, `agent-schedule-time`
- **相关 PRD**: PRD §F-06

### UI-055: Agent — 暂停/恢复定时任务

- **优先级**: P1
- **前置条件**: 有 status=active 的定时任务
- **步骤**:
  1. 点击定时任务的「暂停」按钮
  2. 确认暂停
  3. 点击「恢复」
- **预期结果**:
  - 暂停后状态显示为「已暂停」
  - 任务不再按调度自动执行
  - 恢复后重新激活调度
  - `data-testid`: `agent-scheduled-pause-btn-{taskId}`, `agent-scheduled-resume-btn-{taskId}`
- **相关 PRD**: PRD §F-06

### UI-056: Agent — 任务列表分页

- **优先级**: P1
- **前置条件**: 任务总数 > 20
- **步骤**:
  1. 检查任务列表底部
  2. 点击「下一页」
  3. 切换每页条数到 50
- **预期结果**:
  - 底部显示分页控件：共 N 条 / 页码导航 / 每页条数切换
  - 默认每页 20 条
  - 支持 10/20/50/100 条切换
  - `data-testid`: `agent-task-pagination`, `agent-task-page-size-select`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 任务 CRUD、任务执行状态、Artifact 列表 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 多种状态的任务 mock 数据（running/completed/failed/queued/scheduled） |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | **High** — 需要 mock 完整的任务生命周期（创建→队列→执行→完成），含进度条更新、日志流、Artifact 生成 |
| 是否有无法实现的用例 | 无 — 所有 Agent 任务通过 mock model service 返回可控的预定义结果，任务生命周期通过状态轮询 mock 逐阶段推进 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/agent.spec.ts` | AGENT E2E 测试实现 | New |
| `tests/ui/fixtures/agent.fixture.ts` | Agent mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-039 ~ UI-052）必须通过
- [ ] 所有 P1 用例（UI-053 ~ UI-056）必须通过
- [ ] 任务全生命周期 UI 100% 符合 UI 原型设计文档 §3.3 ~ §3.4 和 PRD §F-05, §F-06
- [ ] 进度条、步骤指示器、日志实时更新验证通过

## 8. UI Test / E2E 验收规则

- [ ] **必须** 使用 mock model service（`page.route()` 拦截 `/api/v1/agent/**`, `/api/v1/tasks/**`），返回预定义的分析结果
- [ ] **必须** mock 模型返回包含 SQL、表格、图表的分析结果
- [ ] **必须** mock 任务状态变更（通过 API 响应模拟状态轮询：queued → running → done）
- [ ] **必须** 验证状态 Pill 的颜色和文本
- [ ] **必须** 验证模态窗口的字段完整性和交互
- [ ] **严禁** 使用真实的 Agent 引擎执行任务
- [ ] **严禁** 跳过步骤指示器动画验证

参考: `.agent/memory/E2E_TESTING.md`
