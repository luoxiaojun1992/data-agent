# UI E2E 测试设计 — Hermes 自由探索模式 (HERMES)

> **SPEC-021** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent Hermes 自由探索模式的 E2E UI 测试用例规范。覆盖模式切换渲染、探索模式下发送消息、工具调用隔离和离线状态提示。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §8
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-057 ~ UI-061（共 5 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-012 | ✅ | Hermes 自由探索（服务配置、端点定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-019 | ✅ | CHAT（模式切换在轻量工作区页面） |
| SPEC-043 | 📐 | Mock Model Service（**P8 前置依赖**） |

### Mock Model Service 策略

> **策略**: Hermes 正常配置模型（OpenAI 兼容格式），直接接入 Mock Model Service (SPEC-043)。Mock Service 提供 OpenAI-compatible `/v1/chat/completions` 端点，Hermes 通过 `LLM_BASE_URL` 配置指向即可。

| 测试场景 | 方式 | 说明 |
|----------|------|------|
| 探索模式消息回复 | Hermes → Mock Service `/v1/chat/completions` | Hermes 正常 LLM 配置指向 mock |
| 工具调用隔离 | mock 响应不包含 tool_call 结构 | 验证回复中无 Data Agent Tool Card 元素 |
| 离线状态 | mock `/api/v1/hermes/status` → `offline` | 验证离线提示渲染 |

> **兼容性说明**: 如果 Hermes 实现时使用了非 OpenAI 兼容的模型 API（如 Claude Messages API），无法接入 Mock Service，则 UI-059/060 标记为 👤人工测试，仅自动测试 UI-057/058/061。

## 3. 测试范围

| 子功能 | 用例 | 优先级 | 测试方式 |
|--------|------|:------:|------|
| Mode Toggle 渲染 | UI-057 | P1 | 🤖 E2E |
| 切换到探索模式 | UI-058 | P1 | 🤖 E2E |
| 探索模式下发送消息 | UI-059 | P1 | 🤖 E2E（若 Hermes 兼容 OpenAI）/ 👤 人工（否则） |
| 探索模式下无法调用 Data Agent 工具 | UI-060 | P1 | 🤖 E2E（若 Hermes 兼容 OpenAI）/ 👤 人工（否则） |
| 离线状态提示 | UI-061 | P1 | 🤖 E2E |

## 4. 测试用例

### UI-057: Hermes — Mode Toggle 渲染

- **优先级**: P1
- **前置条件**: 导航到轻量工作区
- **步骤**:
  1. 检查页面顶部的模式切换
- **预期结果**:
  - 显示两个 Tab 切换：
    - 「📊 分析模式」(Data Agent)
    - 「🔍 探索模式」(Hermes)
  - 默认选中「分析模式」
  - 两个 Tab 之间有明显视觉区分
  - `data-testid`: `mode-toggle`, `mode-toggle-analysis`, `mode-toggle-hermes`
- **相关 Spec**: SPEC-012 §3

### UI-058: Hermes — 切换到探索模式

- **优先级**: P1
- **前置条件**: 在轻量工作区，Hermes 服务已配置
- **步骤**:
  1. 点击「🔍 探索模式」Tab
- **预期结果**:
  - 界面切换到探索模式
  - 聊天输入区保留（但发送目标变为 Hermes 端点）
  - 会话历史切换为 Hermes 会话列表（与 Data Agent 会话隔离）
  - 快捷提示词可能不同或隐藏
  - 状态指示显示「Hermes Online」或「Hermes Offline」
  - `data-testid`: `hermes-chat-area`, `hermes-session-list`

### UI-059: Hermes — 探索模式下发送消息

- **优先级**: P1
- **前置条件**: 已切换到探索模式，Hermes 在线
- **步骤**:
  1. 输入「What are the latest trends in data science?」
  2. 发送
- **预期结果**:
  - 消息发送到 Hermes LLM（非 Data Agent Service）
  - 响应以 SSE 流式返回
  - 实时显示逐字输出
  - Hermes 会话记录写入 `hermes_sessions` 集合
  - `data-testid`: `hermes-input`, `hermes-send-btn`, `hermes-msg-{index}`

### UI-060: Hermes — 探索模式下无法调用 Data Agent 工具

- **优先级**: P1
- **前置条件**: 切换到探索模式
- **步骤**:
  1. 发送「查询华东区销售数据」
  2. 检查 AI 回复
- **预期结果**:
  - 回复中**不包含** Data Agent Tool Call 卡片
  - 回复中**不包含** SQL 代码块
  - 回复为 Hermes 自由文本响应
  - `data-testid`: `hermes-msg-{index}`
- **相关 Spec**: SPEC-012 §5

### UI-061: Hermes — 离线状态提示

- **优先级**: P1
- **前置条件**: Hermes 服务未配置或不可达
- **步骤**:
  1. 点击探索模式 Tab
- **预期结果**:
  - 显示离线状态：「Hermes 服务未连接」
  - 输入框禁用或显示「请先配置 Hermes 服务」
  - `data-testid`: `hermes-offline-badge`
- **相关 Spec**: SPEC-012 §3

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — Hermes SSE 端点、配置状态 API |
| 是否依赖第三方服务 | Yes — Hermes LLM 服务（需 mock） |
| 是否需要特殊测试数据 | Yes — Hermes 在线/离线状态 mock、配置数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Low-Medium — 若 Hermes 兼容 OpenAI API 则直接接入 Mock Service；若不兼容则 UI-059/060 标记人工测试 |
| 是否有无法实现的用例 | 取决于 Hermes 实现：若 OpenAI 兼容 → 全部可测；若不兼容 → UI-059/060 标记人工测试 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/hermes.spec.ts` | HERMES E2E 测试实现 | New |
| `tests/ui/fixtures/hermes.fixture.ts` | Hermes mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P1 用例（UI-057 ~ UI-061）必须通过
- [ ] Mode Toggle UI 符合 SPEC-012 §3 规范
- [ ] 探索模式与分析模式会话完全隔离
- [ ] 探索模式下 Data Agent 工具不可调用

## 8. UI Test / E2E 验收规则

- [ ] **必须** 使用 mock model service（`page.route()` 拦截 `/api/v1/hermes/**`），返回预定义的探索回复
- [ ] **必须** mock Hermes SSE 流式端点，返回分片 mock 文本
- [ ] **必须** 验证切换到探索模式后的 URL 或路由变化
- [ ] **必须** 验证探索模式下消息中不含 Data Agent Tool Call 元素
- [ ] **严禁** 使用真实 Hermes LLM API 进行测试
- [ ] **严禁** 跳过会话隔离验证

参考: `.agent/memory/E2E_TESTING.md`
