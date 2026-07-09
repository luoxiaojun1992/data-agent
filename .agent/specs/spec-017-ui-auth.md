# UI E2E 测试设计 — 登录与认证 (AUTH)

> **SPEC-017** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 登录与认证模块的 E2E UI 测试用例规范。覆盖登录页的完整 UI 渲染、表单校验、认证流程、JWT Token 管理与登出等核心用户路径。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §4
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-001 ~ UI-010（共 10 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施与认证授权（JWT、RABC 基础） |
| SPEC-013 | ✅ | 管理后台（SSO 按钮路由） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架、data-testid 规范） |

## 3. 测试范围

登录与认证模块包含以下子功能：

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 登录页品牌元素渲染 | UI-001 | P0 |
| 邮箱输入框交互与校验 | UI-002 | P0 |
| 密码输入框交互 | UI-003 | P0 |
| 登录按钮交互与状态 | UI-004 | P0 |
| SSO 单点登录 | UI-005 | P1 |
| 邮箱格式前端校验 | UI-006 | P1 |
| 空字段校验 | UI-007 | P1 |
| 错误凭证处理 | UI-008 | P0 |
| JWT Token 过期自动跳转 | UI-009 | P0 |
| 登出流程 | UI-010 | P0 |

## 4. 测试用例

### UI-001: 登录页 — 品牌元素渲染

- **优先级**: P0
- **前置条件**: 未登录状态访问系统 URL
- **步骤**:
  1. 打开 DataAgent 登录页面
- **预期结果**:
  - 页面背景为纯黑 (`#000000`)
  - 居中显示玻璃卡片（420px 宽，`rgba(255,255,255,0.05)` 填充，`1px rgba(255,255,255,0.10)` 边框）
  - 卡片顶部显示 DA 图标（36x36px 蓝紫渐变圆角方）
  - 图标右侧显示 "DataAgent" 品牌名（18px SemiBold 白色）
  - 标题「登录企业数据分析平台」（20px SemiBold 白色）
  - `data-testid`: `login-card`, `login-logo`, `login-logo-icon`, `login-logo-name`, `login-title`
- **相关设计**: UI原型设计文档 §3.1

### UI-002: 登录页 — 邮箱输入框

- **优先级**: P0
- **前置条件**: 登录页已加载
- **步骤**:
  1. 检查邮箱输入框
  2. 点击输入框使其获得焦点
  3. 输入无效邮箱格式 `abc`
  4. 失去焦点
  5. 输入有效邮箱 `test@company.com`
- **预期结果**:
  - 标签「邮箱地址」（12px SemiBold #7A7A7A）显示在输入框上方
  - Placeholder 显示 `name@company.com`
  - 聚焦时边框变为 `#B1E2FF`
  - 无效格式时显示错误提示（红色文字）
  - 有效格式时错误提示消失
  - `data-testid`: `login-email-label`, `login-email-input`, `login-email-error`
- **相关设计**: UI原型设计文档 §3.1

### UI-003: 登录页 — 密码输入框

- **优先级**: P0
- **前置条件**: 登录页已加载
- **步骤**:
  1. 检查密码输入框
  2. 输入密码
- **预期结果**:
  - 标签「密码」（12px SemiBold #7A7A7A）显示在输入框上方
  - 输入内容以掩码圆点 (`······`) 显示
  - 输入框类型为 `type="password"`
  - `data-testid`: `login-password-label`, `login-password-input`
- **相关设计**: UI原型设计文档 §3.1

### UI-004: 登录页 — 登录按钮

- **优先级**: P0
- **前置条件**: 已输入正确邮箱和密码
- **步骤**:
  1. 输入邮箱 `admin@company.com` 和密码
  2. 点击「登录」按钮
- **预期结果**:
  - 按钮为全宽蓝紫渐变填充（`linear-gradient(135deg, #B1E2FF, #9381FF)`）
  - 按钮文字「登录」为黑色 (`#000`)，15px Bold
  - Hover 时透明度变为 0.9 + 上移 1px
  - 点击后显示 loading 状态（按钮禁用 + 旋转动画）
  - 登录成功后页面跳转到 Dashboard
  - `data-testid`: `login-btn`
- **相关设计**: UI原型设计文档 §3.1

### UI-005: 登录页 — SSO 按钮

- **优先级**: P1
- **前置条件**: 登录页已加载
- **步骤**:
  1. 检查 SSO 按钮
  2. 点击 SSO 按钮
- **预期结果**:
  - 登录按钮和 SSO 按钮之间有分隔线："或"文字 + 左右横线
  - SSO 按钮为透明背景 + 白色边框，14px #7A7A7A 文字
  - 文字「企业 SSO 单点登录」
  - Hover 时背景变半透明白 + 文字变白
  - 点击跳转到 SSO 认证页面（或显示 SSO 登录窗口）
  - `data-testid`: `login-divider`, `login-sso-btn`
- **相关设计**: PRD §2.3

### UI-006: 登录 — 邮箱格式校验错误

- **优先级**: P1
- **前置条件**: 登录页已加载
- **步骤**:
  1. 输入不符合邮箱格式的文本（如 `notanemail`）
  2. 点击登录按钮
- **预期结果**:
  - 邮箱输入框下方显示错误提示：「请输入有效的邮箱地址」
  - 登录按钮不触发请求
  - `data-testid`: `login-email-error`

### UI-007: 登录 — 空字段校验

- **优先级**: P1
- **前置条件**: 登录页已加载
- **步骤**:
  1. 不输入任何内容，直接点击登录按钮
- **预期结果**:
  - 邮箱输入框下方显示：「请输入邮箱地址」
  - 密码输入框下方显示：「请输入密码」
  - `data-testid`: `login-email-error`, `login-password-error`

### UI-008: 登录 — 错误凭证提示

- **优先级**: P0
- **前置条件**: 登录页已加载
- **步骤**:
  1. 输入不存在的邮箱和随机密码
  2. 点击登录
- **预期结果**:
  - 显示错误提示（红色 toast 或内联错误）：「邮箱或密码错误」
  - 登录按钮恢复可用状态（取消 loading）
  - 密码输入框内容被清空
  - `data-testid`: `login-error-toast`

### UI-009: 登录 — JWT Token 过期自动跳转

- **优先级**: P0
- **前置条件**: 已登录但 JWT Token 已过期
- **步骤**:
  1. 模拟 Token 过期
  2. 尝试访问任何需要认证的页面
- **预期结果**:
  - 自动重定向到登录页面
  - 显示提示：「登录已过期，请重新登录」
  - `data-testid`: `login-session-expired-toast`
- **相关 Spec**: SPEC-003 §6.4

### UI-010: 登出

- **优先级**: P0
- **前置条件**: 已登录
- **步骤**:
  1. 点击侧边栏底部用户卡片区域
  2. 点击「登出」按钮
- **预期结果**:
  - 清除 JWT Token（localStorage）
  - 重定向到登录页面
  - 无法通过浏览器后退按钮回到已登录状态
  - `data-testid`: `nav-user-card`, `nav-logout-btn`
- **相关 PRD**: PRD §2.3

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 登录 API（/api/v1/auth/login）、SSO 跳转 |
| 是否依赖第三方服务 | Yes — SSO 身份提供商（需 mock） |
| 是否需要特殊测试数据 | Yes — 预置测试账号、过期 Token |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无（E2E 测试独立运行） |
| 实现复杂度 | Medium — JWT 过期模拟需要特殊处理 |
| 是否有无法实现的用例 | UI-005 (SSO): 需 mock SSO 跳转；UI-009 (Token 过期): 需通过 API 或 cookie 操控实现 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/auth.spec.ts` | AUTH E2E 测试实现 | New |
| `tests/ui/fixtures/auth.fixture.ts` | 认证测试 fixture（Token 管理） | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-001 ~ UI-004, UI-008 ~ UI-010）必须通过
- [ ] 所有 P1 用例（UI-005 ~ UI-007）必须通过
- [ ] CI Pipeline 中 sonar-check → ui-tests 均通过
- [ ] 登录流程 UI 100% 符合 UI 原型设计文档 §3.1 规范
- [ ] 所有 data-testid 属性按规范命名并存在于 DOM 中

## 8. UI Test / E2E 验收规则

- [ ] **必须** 为每个测试用例编写独立 Playwright test
- [ ] **必须** 使用 `page.route()` mock 后端 API 响应
- [ ] **必须** 使用 `data-testid` 选择器，禁止使用 CSS class / XPath
- [ ] **严禁** 删除/降级测试用例
- [ ] **严禁** 以占位用例顶替真实功能测试
- [ ] **严禁** 用真实 API 调用代替模拟（除授权场景外）

参考: `.agent/memory/E2E_TESTING.md`
