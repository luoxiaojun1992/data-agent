# UI E2E 测试设计 — 模型配置 (MODEL)

> **SPEC-025** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 模型配置模块的 E2E UI 测试用例规范。覆盖模型配置页渲染、OpenAI 兼容 API URL 配置、API Key 输入与 Vault 加密、眼睛按钮切换可见性、Model Name 下拉选择、上下文长度/最大输出长度/Temperature/Top-P 配置、Hermes 配置区域和仅 system_admin 可访问的权限控制。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §12
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-093 ~ UI-103（共 11 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（Vault 密钥管理） |
| SPEC-004 | ✅ | Agent 核心引擎（模型配置依赖） |
| SPEC-012 | ✅ | Hermes（Hermes 配置区域） |
| SPEC-013 | ✅ | 管理后台（模型配置页面定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-043 | 📐 | Mock Model Service（**P8 前置依赖**） |

### Mock Model Service 策略

> **硬性约束**: 模型配置页测试**不连接真实 LLM**。配置保存验证通过检查表单回显和 API 响应，不通过实际发起模型调用来验证。

| 测试场景 | Mock 方式 | 说明 |
|----------|----------|------|
| API Key 加密/解密 | `page.route('**/api/v1/vault/**')` → mock Vault 响应 | 返回模拟的加密/解密结果 |
| 配置保存 | `page.route('**/api/v1/model-config/**')` → mock 保存成功 | 验证表单回显正确值 |
| Hermes 配置 | `page.route('**/api/v1/hermes/config/**')` → mock 配置保存 | 不实际连接 Hermes 服务 |
| 眼睛切换 | 前端本地 toggle input type | 不需要后端 API |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 模型配置页渲染 | UI-093 | P0 |
| OpenAI 兼容 API URL 配置 | UI-094 | P0 |
| API Key 输入与 Vault 加密 | UI-095 | P0 |
| 眼睛按钮切换 API Key 可见性 | UI-096 | P0 |
| Model Name 下拉选择 | UI-097 | P0 |
| 上下文长度配置（Stepper） | UI-098 | P1 |
| 最大输出长度配置 | UI-099 | P1 |
| Temperature 配置 | UI-100 | P1 |
| Top-P 配置 | UI-101 | P1 |
| Hermes 配置区域 | UI-102 | P1 |
| 仅 system_admin 可访问 | UI-103 | P0 |

## 4. 测试用例

### UI-093: Model — 模型配置页渲染

- **优先级**: P0
- **前置条件**: 以 system_admin 身份登录，点击「模型配置」
- **步骤**:
  1. 检查页面
- **预期结果**:
  - Header：「模型配置」
  - 2 个配置卡片区域：
    1. 默认 LLM 模型配置
    2. Hermes 自由探索模式配置（独立服务 Badge）
  - `data-testid`: `model-page-header`, `model-llm-card`, `model-hermes-card`
- **相关设计**: UI原型设计文档 §3.8

### UI-094: Model — OpenAI 兼容 API URL 配置

- **优先级**: P0
- **前置条件**: 模型配置页
- **步骤**:
  1. 检查「OpenAI 兼容 API URL」配置行
  2. 修改为 `https://custom-api.example.com/v1`
  3. 点击保存
- **预期结果**:
  - 标签「OpenAI 兼容 API URL」(14px #7A7A7A)
  - 默认值 `https://api.openai.com/v1`
  - 配置行底部有 1px 玻璃分割线
  - 修改保存后 API 地址更新
  - `data-testid`: `model-api-url-label`, `model-api-url-input`, `model-save-btn`
- **相关设计**: UI原型设计文档 §3.8

### UI-095: Model — API Key 输入与 Vault 加密

- **优先级**: P0
- **前置条件**: 模型配置页
- **步骤**:
  1. 检查 API Key 配置行
  2. 检查输入框类型
  3. 输入 API Key 并保存
  4. 重新加载页面，检查 API Key 显示
- **预期结果**:
  - 输入框类型为 `password`，掩码显示为 `●●●●●●●●●●`
  - 右侧有眼睛图标按钮（eye toggle）
  - 点击眼睛按钮 → 调用 Vault 解密 → 短暂显示原文（F-20）
  - 保存后 API Key 存入 Vault，业务表不存明文（F-20）
  - 重新加载后默认掩码显示
  - `data-testid`: `model-api-key-input`, `model-api-key-eye-toggle`, `model-api-key-masked`
- **相关 PRD**: PRD §F-20

### UI-096: Model — 眼睛按钮切换 API Key 可见性

- **优先级**: P0
- **前置条件**: API Key 已保存
- **步骤**:
  1. 点击眼睛图标按钮（eye toggle）
  2. 再次点击眼睛图标
- **预期结果**:
  - 第一次点击：输入框类型切换为 `text`，显示明文 API Key
  - 眼睛图标变化（睁眼/闭眼）
  - 第二次点击：切换回 `password`，恢复掩码
  - 3-5 秒后自动恢复掩码（安全考虑）
  - `data-testid`: `model-api-key-eye-toggle`, `model-api-key-input`
- **相关 PRD**: PRD §F-20

### UI-097: Model — Model Name 下拉选择

- **优先级**: P0
- **前置条件**: 模型配置页
- **步骤**:
  1. 点击 Model Name 下拉框
  2. 选择「Claude 3.5 Sonnet」
- **预期结果**:
  - 下拉列表显示可用模型：
    - GPT-4o
    - GPT-4o-mini
    - Claude 3.5 Sonnet
    - Gemini 2.0 Flash
  - 选择后下拉关闭，选中值显示
  - `data-testid`: `model-name-select`
- **相关设计**: UI原型设计文档 §3.8

### UI-098: Model — 上下文长度配置（Stepper）

- **优先级**: P1
- **前置条件**: 模型配置页
- **步骤**:
  1. 检查「上下文长度限制」配置行
  2. 点击 + 按钮 3 次
  3. 点击 - 按钮 1 次
- **预期结果**:
  - 标签「上下文长度限制」
  - 默认值 128K tokens
  - +/- Stepper 按钮（28x28px 圆角方）
  - 数值以 IBM Plex Mono 字体居中显示
  - 点击 + 数值增加，点击 - 数值减少
  - `data-testid`: `model-context-len`, `model-context-len-plus`, `model-context-len-minus`
- **相关设计**: UI原型设计文档 §3.8

### UI-099: Model — 最大输出长度配置

- **优先级**: P1
- **前置条件**: 模型配置页
- **步骤**:
  1. 检查最大输出长度配置
- **预期结果**:
  - 标签「最大输出长度」
  - 默认值 16K tokens
  - 同样为 Stepper 控件
  - `data-testid`: `model-max-output`, `model-max-output-plus`, `model-max-output-minus`

### UI-100: Model — Temperature 配置

- **优先级**: P1
- **前置条件**: 模型配置页
- **步骤**:
  1. 检查 Temperature 配置行
- **预期结果**:
  - 标签「Temperature」
  - 默认值 0.7
  - 支持小数输入或滑块控件
  - `data-testid`: `model-temperature`

### UI-101: Model — Top-P 配置

- **优先级**: P1
- **前置条件**: 模型配置页
- **步骤**:
  1. 检查 Top-P 配置行
- **预期结果**:
  - 标签「Top-P」
  - 默认值 0.95
  - 支持小数输入或滑块控件
  - `data-testid`: `model-top-p`

### UI-102: Model — Hermes 配置区域

- **优先级**: P1
- **前置条件**: 模型配置页
- **步骤**:
  1. 检查 Hermes 配置卡片
- **预期结果**:
  - 卡片标题：Hermes 自由探索模式
  - 右上角「独立服务」Badge
  - 配置项：Hermes API URL + Hermes API Key（Vault 加密）
  - 默认模型显示为禁用状态 `hermes-3-70b`
  - `data-testid`: `model-hermes-card`, `model-hermes-url`, `model-hermes-api-key`, `model-hermes-badge`
- **相关设计**: UI原型设计文档 §3.8, SPEC-012

### UI-103: Model — 仅 system_admin 可访问

- **优先级**: P0
- **前置条件**: 以 admin 或 user 身份登录
- **步骤**:
  1. 检查侧边栏是否显示「模型配置」导航项
  2. 尝试直接访问模型配置 URL
- **预期结果**:
  - 「模型配置」不出现在侧边栏中
  - 直接访问 URL 返回 403 或重定向到 Dashboard
  - `data-testid`: `sidebar`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 模型配置 CRUD API、Vault 解密 API |
| 是否依赖第三方服务 | Yes — Vault 密钥管理（需 mock） |
| 是否需要特殊测试数据 | Yes — 加密的 API Key mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | **High** — `UI-095/UI-096` Vault 加密/解密交互需要 mock 复杂的加解密流程；`UI-096` 自动恢复掩码的 3-5 秒倒计时测试 |
| 是否有无法实现的用例 | **待确认** — `UI-095` Vault 解密需要 mock `/api/v1/vault/decrypt` 端点返回明文；`UI-096` 自动恢复掩码需要 `page.waitForTimeout` 或观察 input type 变化 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/model.spec.ts` | MODEL E2E 测试实现 | New |
| `tests/ui/fixtures/model.fixture.ts` | Model mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-093 ~ UI-097, UI-103）必须通过
- [ ] 所有 P1 用例（UI-098 ~ UI-102）必须通过
- [ ] 模型配置 UI 100% 符合 UI 原型设计文档 §3.8
- [ ] API Key 加密/解密流程正确

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端 Model Config API 和 Vault API
- [ ] **必须** 验证 API Key 输入框的 password 类型
- [ ] **必须** 测试眼睛按钮切换的 input type 变化
- [ ] **必须** 验证仅 system_admin 可访问的权限控制
- [ ] **严禁** 使用真实的 Vault 服务
- [ ] **严禁** 在测试代码中硬编码真实 API Key

参考: `.agent/memory/E2E_TESTING.md`
