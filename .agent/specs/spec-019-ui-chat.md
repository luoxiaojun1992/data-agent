# UI E2E 测试设计 — Chat 模式（轻量工作区）(CHAT)

> **SPEC-019** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent Chat 模式（轻量工作区）的 E2E UI 测试用例规范。覆盖在线状态、新对话、快捷提示词、消息气泡渲染（用户/AI）、SQL 代码块、数据表格、工具调用卡片、数据图表、进度动画、会话历史管理和提示词弹窗等全部交互功能。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §6
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-018 ~ UI-038（共 21 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（Session 管理） |
| SPEC-004 | ✅ | Agent 核心引擎（Chat 模式依赖） |
| SPEC-005 | ✅ | Artifact 存储（文件下载功能） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH（登录后访问） |
| SPEC-018 | ✅ | LAYOUT（导航到轻量工作区） |
| SPEC-043 | 📐 | Mock Model Service（**P8 前置依赖**） |

### Mock Model Service 策略

> **硬性约束**: 所有 Chat 模式的 AI 回复（消息内容、SQL 代码块、数据表格、工具调用、图表、KPI 卡片）均来自 mock model service，**严禁使用真实 LLM API**。

| 测试场景 | Mock 方式 | mock 返回内容 |
|----------|----------|------|
| AI 文本回复 | `page.route('**/api/v1/chat/**')` → mock SSE 流 | 预定义 Markdown 文本 |
| SQL 代码块 | mock 模型返回含 SQL 的回复 | `SELECT product_name, SUM(revenue) AS total FROM sales GROUP BY product_name` |
| 数据表格 | mock 模型返回含表格的回复 | 3 行 × 4 列 mock 数据 |
| 工具调用卡片 | mock 模型返回 tool_call 结构 | 模拟 `sql_executor` 调用（输入参数 + mock 输出） |
| 数据图表 | mock 模型返回含图表配置的回复 | ECharts/Chart.js 的 mock option JSON |
| 流式 SSE | mock 返回 `text/event-stream` | 分片发送 mock 文本 |

> 所有 mock 响应在 `tests/ui/fixtures/chat.fixture.ts` 中集中管理，不硬编码在测试文件中。

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 在线状态 Badge | UI-018 | P1 |
| 新对话按钮 | UI-019 | P1 |
| 快捷提示词渲染 | UI-020 | P0 |
| 点击快捷提示词触发查询 | UI-021 | P0 |
| 文本输入与发送 | UI-022 | P0 |
| 用户消息气泡渲染 | UI-023 | P0 |
| AI 消息气泡渲染 | UI-024 | P0 |
| SQL 代码块渲染 | UI-025 | P0 |
| 数据表格渲染 | UI-026 | P0 |
| 工具调用卡片（折叠式） | UI-027 | P0 |
| 数据图表内嵌渲染 | UI-028 | P1 |
| 进度动画 | UI-029 | P1 |
| 会话历史侧边栏 | UI-030 | P0 |
| 会话历史项渲染 | UI-031 | P0 |
| 点击历史会话恢复对话 | UI-032 | P0 |
| 搜索历史会话 | UI-033 | P1 |
| 删除历史会话 | UI-034 | P1 |
| 提示词弹窗（Frosted Glass） | UI-035 | P1 |
| 从提示词弹窗选择填入 | UI-036 | P1 |
| 用户自定义快捷提示词保存 | UI-037 | P2 |
| 消息内嵌 KPI 卡片 | UI-038 | P2 |

## 4. 测试用例

### UI-018: Chat — 在线状态 Badge

- **优先级**: P1
- **前置条件**: 导航到轻量工作区
- **步骤**:
  1. 检查页面 Header 区域
- **预期结果**:
  - Header 显示「轻量工作区」标题
  - 右侧显示在线状态 Badge：绿色脉冲圆点 (6x6px `#34D399`) + 文字「在线」
  - 呼吸灯动画：每 2s 脉冲一次（opacity 1↔0.3）
  - `data-testid`: `chat-header`, `chat-online-badge`, `chat-online-dot`
- **相关设计**: UI原型设计文档 §1.6

### UI-019: Chat — 「新对话」按钮

- **优先级**: P1
- **前置条件**: 已登录，当前有活跃会话
- **步骤**:
  1. 点击 Header 区域的「新对话」按钮
- **预期结果**:
  - 当前会话上下文被清除
  - 输入框清空
  - 消息区清空
  - 快捷提示词行保持显示
  - `data-testid`: `chat-new-session-btn`

### UI-020: Chat — 快捷提示词渲染

- **优先级**: P0
- **前置条件**: 导航到轻量工作区，无活跃对话
- **步骤**:
  1. 检查输入区域上方
- **预期结果**:
  - 显示 4 个快捷提示词 Pill：
    - 「今日数据概览」(蓝底高亮 primary)
    - 「本月销售趋势」(secondary)
    - 「同比环比分析」(secondary)
    - 「TOP10 产品」(secondary)
  - Primary 芯片：`rgba(177,226,255,0.12)` 背景 + `#B1E2FF` 蓝点呼吸灯
  - Secondary 芯片：`rgba(255,255,255,0.04)` 背景
  - `data-testid`: `chat-prompt-row`, `chat-prompt-chip-0`, `chat-prompt-chip-1`, `chat-prompt-chip-2`, `chat-prompt-chip-3`
- **相关设计**: UI原型设计文档 §3.2，PRD §F-04

### UI-021: Chat — 点击快捷提示词触发查询

- **优先级**: P0
- **前置条件**: 导航到轻量工作区
- **步骤**:
  1. 点击「今日数据概览」Pill
- **预期结果**:
  - 提示词文本自动填入输入框
  - 自动发送消息
  - 消息区显示用户消息（蓝紫渐变气泡，右对齐）
  - AI 开始回复（流式或 loading 状态）
  - `data-testid`: `chat-input`, `chat-messages`

### UI-022: Chat — 文本输入与发送

- **优先级**: P0
- **前置条件**: 导航到轻量工作区
- **步骤**:
  1. 在输入框中输入「查询华东区上月销售额」
  2. 按 Enter 键发送
  3. 或点击发送按钮
- **预期结果**:
  - 输入框接受文本输入，placeholder 为半透明白色
  - 按 Enter 发送消息
  - 发送按钮为 44x44px 蓝紫渐变圆角方（内含箭头 SVG）
  - 消息发送后输入框清空
  - `data-testid`: `chat-input`, `chat-send-btn`

### UI-023: Chat — 用户消息气泡渲染

- **优先级**: P0
- **前置条件**: 已发送一条消息
- **步骤**:
  1. 检查消息区中用户消息的外观
- **预期结果**:
  - 右对齐（`justify-content: flex-end`）
  - 蓝紫渐变背景
  - 黑色文字（`#000`），500 字重
  - 底部显示时间戳（10px 半透明白色）
  - 最大宽度 560px
  - `data-testid`: `chat-msg-user-{index}`
- **相关设计**: UI原型设计文档 §3.2

### UI-024: Chat — AI 消息气泡渲染

- **优先级**: P0
- **前置条件**: AI 已回复一条消息
- **步骤**:
  1. 检查消息区中 AI 消息的外观
- **预期结果**:
  - 左对齐
  - 左侧显示 AI 头像（32px 绿色圆形，显示 "DA"）
  - 气泡为玻璃卡片风格（`rgba(255,255,255,0.05)` + 玻璃边框）
  - 白色文字，行高 1.7
  - 支持 Markdown 基础语法（加粗/斜体/列表/链接）
  - `data-testid`: `chat-msg-ai-{index}`, `chat-msg-avatar`
- **相关设计**: UI原型设计文档 §3.2，PRD §F-04

### UI-025: Chat — SQL 代码块渲染

- **优先级**: P0
- **前置条件**: AI 回复中包含 SQL 代码
- **步骤**:
  1. 发送需要 SQL 查询的问题
  2. 检查 AI 回复中的 SQL 代码块
- **预期结果**:
  - SQL 代码块背景 `rgba(0,0,0,0.3)` + `1px rgba(52,211,153,0.2)` 绿色边框
  - Header 显示「SQL」绿色标签 + 绿色圆点 + 「复制」按钮
  - SQL 关键字 (`SELECT`, `FROM`, `WHERE` 等) 为 `#B1E2FF` (蓝色)
  - 字符串为 `#FB7185` (粉色)
  - 数字为 `#FBBF24` (琥珀色)
  - `IBM Plex Mono` 等宽字体
  - 点击「复制」按钮：SQL 复制到剪贴板，显示「已复制」反馈
  - `data-testid`: `chat-sql-block`, `chat-sql-copy-btn`, `chat-sql-code`
- **相关设计**: PRD §F-04 消息美化渲染

### UI-026: Chat — 数据表格渲染

- **优先级**: P0
- **前置条件**: AI 回复中包含数据表格
- **步骤**:
  1. 检查 AI 回复中的数据表格
- **预期结果**:
  - 斑马纹行背景（交替明暗）
  - 表头 11px Bold `#666` + 大写字母
  - 数据行 13px `#7A7A7A`
  - 支持列排序（点击列头切换升序/降序）
  - 列头显示排序指示器（↑/↓）
  - 支持导出按钮
  - `data-testid`: `chat-table`, `chat-table-header-{col}`, `chat-table-export-btn`
- **相关设计**: PRD §F-04 消息美化渲染

### UI-027: Chat — 工具调用卡片（折叠式）

- **优先级**: P0
- **前置条件**: AI 回复中包含工具调用
- **步骤**:
  1. 检查工具调用卡片
  2. 点击卡片 Header 展开
  3. 再次点击收起
- **预期结果**:
  - 卡片背景 `rgba(255,255,255,0.03)` + 浅白边框
  - Header 显示：工具图标 (32px 圆角方) + 工具名称 (13px Bold) + 执行耗时 (Mono 字体)
  - 右侧 Chevron 箭头（收起时向下，展开时旋转 180°）
  - 展开后显示「输入参数」和「输出结果」两个区域
  - `data-testid`: `chat-tool-call-card-{index}`, `chat-tool-call-header`, `chat-tool-call-body`
- **相关设计**: PRD §F-04 消息美化渲染，UI原型设计文档 §3.2

### UI-028: Chat — 数据图表内嵌渲染

- **优先级**: P1
- **前置条件**: AI 回复中包含图表
- **步骤**:
  1. 检查图表消息
  2. 点击放大按钮
  3. 点击下载按钮
- **预期结果**:
  - 图表内嵌在消息中渲染（非独立图片链接）
  - 工具栏显示：放大 🔍 / 下载 📥 / 引用 🔗 按钮
  - 点击放大：弹出全屏预览
  - 点击下载：触发文件下载
  - 点击引用：生成分享链接
  - `data-testid`: `chat-chart`, `chat-chart-zoom-btn`, `chat-chart-download-btn`, `chat-chart-cite-btn`
- **相关设计**: PRD §F-04 消息美化渲染

### UI-029: Chat — 进度动画

- **优先级**: P1
- **前置条件**: AI 正在处理中
- **步骤**:
  1. 发送需要计算的问题
  2. 观察等待状态
- **预期结果**:
  - 显示旋转动画 + 状态文本
  - 可能的状态文本：「查询中…」/「计算中…」/「索引中…」
  - `data-testid`: `chat-loading-indicator`, `chat-loading-text`
- **相关设计**: UI原型设计文档 §3.2

### UI-030: Chat — 会话历史侧边栏

- **优先级**: P0
- **前置条件**: 至少有一个历史会话
- **步骤**:
  1. 检查会话历史侧边栏（主侧边栏右侧 280px 面板）
- **预期结果**:
  - 面板宽度 280px，右侧有 1px 玻璃边框
  - 标题「📋 历史会话」
  - 搜索框 (placeholder: 搜索历史会话)
  - 会话列表按时间倒序排列
  - 每个会话项显示：标题 + 消息数 + 时间戳
  - `data-testid`: `session-sidebar`, `session-search`, `session-list`
- **相关设计**: UI原型设计文档 §3.2，PRD §F-04 会话历史

### UI-031: Chat — 会话历史项渲染

- **优先级**: P0
- **前置条件**: 有会话历史数据
- **步骤**:
  1. 检查单个会话项的外观
- **预期结果**:
  - 标题 13px Bold
  - 元数据 11px `#666`（如「8 条消息 · 今天 10:32」）
  - Hover 时有半透明背景
  - 当前激活会话有蓝色高亮背景（`rgba(177,226,255,0.10)`）+ 蓝色边框
  - `data-testid`: `session-item-{id}`, `session-item-title`, `session-item-meta`
- **相关设计**: UI原型设计文档 §3.2

### UI-032: Chat — 点击历史会话恢复对话

- **优先级**: P0
- **前置条件**: 有多个历史会话
- **步骤**:
  1. 当前在一个活跃会话中
  2. 点击另一个历史会话
- **预期结果**:
  - 消息区切换到所选会话的历史消息
  - 可继续在该会话中追问
  - 会话项显示激活态
  - 先前的会话保持原有状态
  - `data-testid`: `session-item-{id}`

### UI-033: Chat — 搜索历史会话

- **优先级**: P1
- **前置条件**: 有多个历史会话
- **步骤**:
  1. 在搜索框中输入关键词「销售」
- **预期结果**:
  - 会话列表实时过滤，只显示标题包含「销售」的会话
  - 无匹配时显示「暂无搜索结果」
  - 清空搜索框恢复完整列表
  - `data-testid`: `session-search`, `session-list`

### UI-034: Chat — 删除历史会话

- **优先级**: P1
- **前置条件**: 有至少 2 个历史会话
- **步骤**:
  1. Hover 某个会话项，点击出现的删除图标
  2. 确认删除
- **预期结果**:
  - 弹出删除确认弹窗：「确定要删除此会话吗？此操作不可撤销。」
  - 确认后该会话从列表中移除
  - 如果删除的是当前活跃会话，自动切换到最近一个会话
  - `data-testid`: `session-item-delete-{id}`, `session-delete-confirm-modal`

### UI-035: Chat — 提示词弹窗 (Frosted Glass)

- **优先级**: P1
- **前置条件**: 导航到轻量工作区
- **步骤**:
  1. 点击输入框左侧的「📋 提示词」按钮
- **预期结果**:
  - 弹出磨砂玻璃弹窗（低透明度，不被背景干扰）
  - 背景 `rgba(15,15,25,0.85)` + `backdrop-filter: blur(40px)`
  - 标题「提示词」
  - 显示「系统预设」分组：4 个预置提示词
  - 显示「我的常用」分组：用户自定义提示词（最多 5 条）
  - 点击弹窗外部或关闭按钮关闭弹窗
  - `data-testid`: `prompt-btn`, `prompt-modal`, `prompt-modal-close`, `prompt-modal-chip-{index}`
- **相关设计**: UI原型设计文档 §3.2，PRD §F-04 快捷提示词设计

### UI-036: Chat — 从提示词弹窗选择填入

- **优先级**: P1
- **前置条件**: 提示词弹窗已打开
- **步骤**:
  1. 点击「系统预设」下的「本月销售趋势」芯片
- **预期结果**:
  - 弹窗关闭
  - 输入框自动填入所选提示词文本
  - 不会自动发送（仅填入）
  - `data-testid`: `prompt-modal-chip-1`, `chat-input`

### UI-037: Chat — 用户自定义快捷提示词保存

- **优先级**: P2
- **前置条件**: 提示词弹窗已打开
- **步骤**:
  1. 在弹窗底部的自定义输入框输入「查询上月客户留存率」
  2. 点击「保存到常用」
- **预期结果**:
  - 提示词添加到「我的常用」分组
  - 不超过 5 条限制时正常添加
  - 超过 5 条时显示提示「最多保存 5 个常用提示词」
  - `data-testid`: `prompt-modal-custom-input`, `prompt-modal-save-btn`

### UI-038: Chat — 消息内嵌 KPI 卡片

- **优先级**: P2
- **前置条件**: AI 回复包含统计指标
- **步骤**:
  1. 检查 AI 回复中的内嵌 KPI 元素
- **预期结果**:
  - KPI 区域有独立背景 `rgba(255,255,255,0.06)` + 圆角
  - 每个 KPI 项包含数值（18px IBM Plex Mono Bold）和标签（11px #7A7A7A）
  - 水平排列（`display: flex`）
  - `data-testid`: `chat-inline-kpi`, `chat-inline-kpi-val`, `chat-inline-kpi-lbl`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — Chat 消息发送/接收、Session 管理 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 预置会话历史数据、含 SQL/表格/图表的 AI 回复 mock |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | **High** — 需 mock 模型 SSE 流式响应、多类型消息渲染（SQL/表格/图表/tool_call） |
| 是否有无法实现的用例 | 无 — 所有 AI 回复通过 mock model service 返回可控的预定义内容 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/chat.spec.ts` | CHAT E2E 测试实现 | New |
| `tests/ui/fixtures/chat.fixture.ts` | Chat mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-020 ~ UI-027, UI-030 ~ UI-032）必须通过
- [ ] 所有 P1 用例（UI-018, UI-019, UI-028, UI-029, UI-033 ~ UI-036）必须通过
- [ ] 所有 P2 用例（UI-037, UI-038）建议通过
- [ ] Chat UI 100% 符合 UI 原型设计文档 §3.2 和 PRD §F-04
- [ ] SSE 流式响应测试通过

## 8. UI Test / E2E 验收规则

- [ ] **必须** 使用 mock model service（`page.route()` 拦截 `/api/v1/chat/**`），返回预定义的 AI 响应
- [ ] **必须** mock SSE 流式响应（返回 `text/event-stream`，分片发送 mock 文本）
- [ ] **必须** mock 模型返回包含 SQL、表格、tool_call、图表配置的多样化响应
- [ ] **必须** 验证消息气泡的 CSS 样式（颜色、对齐、字体）
- [ ] **必须** 验证 SQL 代码块的复制功能
- [ ] **必须** 测试工具调用卡片展开/收起动画
- [ ] **严禁** 使用真实 LLM/模型 API
- [ ] **严禁** 跳过任何消息渲染类型的验证

参考: `.agent/memory/E2E_TESTING.md`
