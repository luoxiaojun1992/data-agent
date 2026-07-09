# UI E2E 测试设计 — 增强提示词 (PROMPT)

> **SPEC-033** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 增强提示词功能的 E2E UI 测试用例规范。覆盖增强按钮渲染、点击增强按钮（空输入/有输入）、增强后手动编辑再发送和增强调用不计入 Token 统计。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §20
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-156 ~ UI-160（共 5 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-004 | ✅ | Agent 核心引擎（增强提示词逻辑） |
| SPEC-008 | ✅ | Skill 实现层（增强 Skill） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-019 | ✅ | CHAT（增强按钮位于 Chat 页面） |
| SPEC-043 | 📐 | Mock Model Service（**P8 前置依赖**） |

### Mock Model Service 策略

> **硬性约束**: 增强提示词调用 LLM 优化用户输入，**严禁使用真实 LLM API**。

| 测试场景 | Mock 方式 | 说明 |
|----------|----------|------|
| 增强提示词 | `page.route('**/api/v1/chat/enhance')` → mock 增强结果 | 返回预定义的优化后文本 |
| 增强 loading | mock 延迟 1~2 秒后返回 | 验证 spinner 动画渲染 |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 增强按钮渲染 | UI-156 | P0 |
| 点击增强按钮（输入为空） | UI-157 | P0 |
| 点击增强按钮（有输入） | UI-158 | P0 |
| 增强后手动编辑再发送 | UI-159 | P1 |
| 增强调用不计入 Token 统计 | UI-160 | P2 |

## 4. 测试用例

### UI-156: Prompt — 增强按钮渲染

- **优先级**: P0
- **前置条件**: 导航到轻量工作区
- **步骤**:
  1. 检查输入框右侧
- **预期结果**:
  - 显示「✨ 增强」按钮
  - 按钮样式：蓝紫渐变半透明 + 蓝边框 + `#B1E2FF` 文字
  - `data-testid`: `chat-enhance-btn`
- **相关设计**: UI原型设计文档 §3.2, PRD §F-18

### UI-157: Prompt — 点击增强按钮（输入为空）

- **优先级**: P0
- **前置条件**: 输入框为空
- **步骤**:
  1. 点击「✨ 增强」按钮
- **预期结果**:
  - 按钮进入 loading 状态（旋转 spinner 动画，0.8s 周期）
  - 按钮文字隐藏，仅显示 spinner
  - 无请求发出或提示「请先输入内容」
  - `data-testid`: `chat-enhance-btn`

### UI-158: Prompt — 点击增强按钮（有输入）

- **优先级**: P0
- **前置条件**: 输入框中有文字「看看这个月的销售」
- **步骤**:
  1. 点击「✨ 增强」
  2. 等待响应（< 2 秒 — PRD §F-18）
- **预期结果**:
  - 按钮显示 loading spinner（旋转动画，0.8s）
  - 增强结果直接填充输入框（替换原始文本），用户可继续手动编辑后提交
  - 响应时间 < 2 秒（PRD §F-18 验收标准）
  - **无 Session 创建、无对话历史记录**（无状态）
  - `data-testid`: `chat-enhance-btn`, `chat-input`
- **相关 PRD**: PRD §F-18

### UI-159: Prompt — 增强后手动编辑再发送

- **优先级**: P1
- **前置条件**: 增强提示词后输入框已填入增强文本
- **步骤**:
  1. 手动编辑增强后的文本
  2. 点击发送
- **预期结果**:
  - 手动编辑的内容被发送
  - 正常进入 Chat 对话流程
  - `data-testid`: `chat-input`, `chat-send-btn`

### UI-160: Prompt — 增强调用不计入 Token 统计

- **优先级**: P2
- **前置条件**: 增强功能正常
- **步骤**:
  1. 查看 Dashboard Token 消耗统计（增强前）
  2. 执行一次增强操作
  3. 查看 Token 消耗（增强后）
- **预期结果**:
  - 增强 LLM 调用不计入 Chat/Agent 的 Token 统计
  - Dashboard Token 消耗数值不变
  - `data-testid`: `dashboard-kpi-token-today`
- **相关 PRD**: PRD §F-18

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 增强提示词 API 使用 mock model service 返回预定义增强结果 |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 增强请求/响应 mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Low — mock model service 返回固定增强文本，无需复杂的流式响应 |
| 是否有无法实现的用例 | 无 — 增强功能通过 mock model service 返回可控的预定义文本 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/prompt.spec.ts` | PROMPT E2E 测试实现 | New |
| `tests/ui/fixtures/prompt.fixture.ts` | Prompt mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-156 ~ UI-158）必须通过
- [ ] 所有 P1 用例（UI-159）必须通过
- [ ] 所有 P2 用例（UI-160）建议通过
- [ ] 增强功能 UI 100% 符合 PRD §F-18

## 8. UI Test / E2E 验收规则

- [ ] **必须** 使用 mock model service（`page.route()` 拦截 `/api/v1/chat/enhance`），返回预定义的增强文本
- [ ] **必须** mock 1~2 秒延迟后返回响应，验证 loading spinner 动画
- [ ] **必须** 验证增强按钮的 loading spinner 动画
- [ ] **必须** 验证增强后输入框内容替换（非追加）
- [ ] **必须** 验证增强不创建新 Session
- [ ] **严禁** 使用真实 LLM API 进行增强测试
- [ ] **严禁** 跳过空输入边界情况

参考: `.agent/memory/E2E_TESTING.md`
