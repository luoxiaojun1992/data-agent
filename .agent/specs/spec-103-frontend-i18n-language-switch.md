# SPEC-103 纯前端国际化（i18n 翻译 + 语言切换）

> **SPEC-103** | Status: ✅ 已实现（2026-09-23 实现并部署验证，commit 87c525e）
> 日期：2026-09-18 立项 → 2026-09-18 设计定稿 → 2026-09-23 实现+部署+E2E 验证

## 1. 目标

1. **纯前端国际化**：前端全部 UI 文案翻译化（文案从 TSX 硬编码中抽离为语言资源），支持多语言界面。
2. **语言切换**：用户可在界面切换语言（默认中文），选择持久化，全站即时生效。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-076 前端主题切换（localStorage 持久化 + 全局状态 + 防闪烁） | ✅ | 语言切换与主题切换同模式（状态 + 持久化 + 顶层 Provider），可直接参考其实现范式 |
| — | — | 纯前端，无后端前置 |

## 2. 背景与动机

- 当前前端所有文案**硬编码中文**（TSX 内直接写死），无国际化能力。
- 产品面向上海外贸企业市场（见商业定位），存在英文界面需求。
- 语言切换与已有主题切换（SPEC-076）同属「用户偏好 + 纯前端」模式，实现路径可复用。

## 3. 方向（立项不展开，仅列范围与待定项）

### 3.1 范围界定（D3 已定稿）

- **翻译**：前端静态 UI 文案（菜单、按钮、表单标签、提示、空态、错误提示等）。
- **不翻译——后端返回的一切内容**（D3 定稿）：LLM 对话内容、记忆、知识库文档、artifact、task（定义/结果）、配置内容、审计日志 `action_desc`、通知内容、后端错误消息等——**全部原样显示，不做前端翻译**。
- **不翻译——有功能语义的文字**（D5 定稿）：HTML/JS 代码、代码块内容、markdown 源码、className/data-testid、API 路径、日期格式串等。

### 3.2 方向性设计（D1/D2/D4 已定稿）

- **技术方案（D1）**：**next-intl**——Next.js App Router 最成熟的国际化库（App Router 原生支持、SSR 友好、TS 类型安全、MIT license 合规）。不用 i18next/自研。
- **语言范围（D2）**：`zh`（默认）+ `en`。
- 语言资源字典化（`zh.json`/`en.json`，next-intl messages 结构）。
- 语言状态存 **cookie**（SSR 可读）+ localStorage 同步（D4，见 §11 解释），next-intl 标准 cookie 机制。
- 语言切换入口 UI（与主题切换并列或就近），切换即时生效。
- 防闪烁：参考 SPEC-076 的 inline script 范式（D4）。

## 4. 待定决策点

> 5 个决策点已全部定稿（2026-09-18），见 §11 D1~D5。本章保留为历史记录。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（纯前端；语言偏好 cookie + localStorage） |
| 是否影响现有 API | No（后端返回内容一律不翻译，D3） |
| 性能影响 | 语言字典按需加载/打包，影响可控 |
| 是否需要新增 Skill | No |
| 数据迁移 | 无 |
| 新依赖 license | next-intl = MIT ✅（符合 license 红线） |

## 7. 相关文件（方向）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/lib/i18n/*`（新） | next-intl 配置（messages 字典 + requestConfig + cookie 读写） | Medium |
| `frontend/app/layout.tsx` / `providers.tsx` | 挂载 NextIntlClientProvider + inline 防闪烁 script | Low |
| `frontend/middleware.ts`（新/改） | next-intl 语言路由/cookie 处理（如采用） | Low |
| 全站页面 `app/**` | 硬编码中文替换为翻译 key（一次性全站，D5） | High |
| 语言切换组件 | 切换 UI + cookie/localStorage 持久化 | Low |

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增语言切换交互时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

> 纯前端 spec，无 Go 代码变更——Go UT（§9.5）豁免。

## 10. 验证标准

1. 切换语言后全站 UI 文案即时切换，刷新后保持（cookie + localStorage）。
2. 默认中文；无闪中变英（SSR 按 cookie 渲染，D4）。
3. 未翻译 key 有降级（回退 zh 或显示 key）。
4. 后端返回内容（LLM/记忆/KB/artifact/task/配置/通知/错误）**原样显示不翻译**（D3）。
5. **只翻译纯文字**（D5）：代码块/类名/testid/API 路径/markdown 源码等有功能语义的文字不被翻译；翻译前后 DOM 结构、className、data-testid、样式不变。
6. **长度自适应**（D5）：中英切换后按钮/标签/表头/弹窗不错位、不溢出——不写死宽度、长英文单词可换行（word-break）、表格列宽自适应。
7. 新增文案强制走字典（约定/审查）。

## 11. 设计定稿记录（2026-09-18，D1~D5 全定稿）

### D1 — 技术方案：next-intl（最成熟的现有库）

- 采用 **next-intl**：Next.js App Router 生态最成熟的国际化库（App Router 原生支持、SSR/服务端组件友好、TS 类型安全、社区活跃），MIT license ✅（符合项目 license 红线）。
- 不用 i18next（通用型但 App Router 集成需自搭）、不自研。

### D2 — 语言范围

- `zh`（默认）+ `en`，双语起步。

### D3 — 翻译边界：后端返回的一切都不翻译

- **后端返回内容一律原样显示**：LLM 对话、记忆、KB 文档、artifact、task 定义/结果、配置内容、审计 `action_desc`、通知、后端错误消息等——**不做前端翻译**。
- 只翻译前端静态 UI 文案。

### D4 — SSR 水合一致性（cookie + 防闪烁，2026-09-18 定稿 + 副作用确认）

- 背景：SSR 在服务器渲染、读不到 localStorage → 若仅存 localStorage，首屏按默认中文渲染、客户端接管后变英文 → 闪烁 + hydration mismatch 警告（SPEC-076 主题切换踩过的同类坑）。且 i18n 文案是**服务端组件渲染进 HTML 的静态文本**，inline script 无法改文本，必须让 SSR 知道语言。
- 定稿：语言状态存 **cookie**（SSR 可读，服务器按正确语言渲染首屏）+ localStorage 镜像（客户端快速读）；采用 next-intl 标准 cookie 机制（必要时 `frontend/middleware.ts` 处理）。
- 防闪烁：参考 SPEC-076 的 inline script 范式（React 水合前设置 `lang` 属性）。
- **副作用与成本确认（2026-09-18）**：
  - Cookie 大小：仅 `"zh"/"en"`（≈2 字节），远低于 4KB 限制，无问题。
  - SameSite/跨域：同域部署（nginx 反代），SameSite=Lax 足够，无跨站问题。
  - 后端影响：**零**（后端不读该 cookie，D3 无联动）。
  - 依赖要求：next-intl 标准机制；必要时 Next.js 内置 middleware.ts，**无额外包**。
  - 副作用：清 cookie/换浏览器后语言重置默认 zh——与 localStorage 行为相同，可接受。
  - 迁移成本：无存量（i18n 为全新功能）。

### D4 附注 — 主题/自动脱敏**不**迁移 cookie（2026-09-18 确认）

- **主题切换（SPEC-076）**：切换的只是 `data-theme` 属性（CSS 变量），现有 inline script 在首帧前读 localStorage 设属性，**已部署验证有效、无闪烁**——不需要 cookie，改 cookie 徒增迁移成本与同步复杂度。
- **自动脱敏（SPEC-093）**：纯客户端 checkbox 状态，不影响 SSR 渲染文本——保持现状（mount 时读 localStorage）。
- **自动脱敏默认值依赖 API 健康度**：在**浏览器前端处理**（开关打开时才调脱敏 API、失败走现有报错降级；与语音按钮 `isVoiceInputSupported()` 检测同模式），不进 SSR/后端。
- 结论：i18n 用 cookie 是因为文案是 SSR 渲染的静态文本（cookie 是唯一让 SSR 知道语言的手段）；主题/脱敏不属于此类，现有方案即最优。

### D5 — 一次性全站翻译 + 质量约束

- **一次性**完成全站 UI 文案翻译，**不分期**。
- **只翻译纯文字**：不翻译 HTML/JS 代码、代码块、markdown 源码、className、data-testid、API 路径、日期格式串等有功能语义的内容。
- **不影响样式/结构/显示效果**：仅替换文本内容；DOM 结构、className、testid、样式规则不变。
- **长度变化自适应**：中↔英文字长度差异大，按钮/标签/表头/弹窗/表格不得写死宽度，长英文单词允许换行（word-break），布局用 flex/min-width 保证切换后不错位、不溢出。

## 12. 实现记录（2026-09-23，commit 87c525e）

### 12.1 交付内容

| 项 | 内容 |
|----|------|
| 依赖 | next-intl ^3.26.5（MIT），`createNextIntlPlugin('./lib/i18n/request.ts')` 接线 next.config.js，`output: 'standalone'` 不变 |
| i18n 基础设施 | `lib/i18n/config.ts`（LOCALE_COOKIE='NEXT_LOCALE' / LOCALE_STORAGE_KEY='data-agent-locale' / Locale='zh'\|'en' / DEFAULT_LOCALE='zh'）+ `lib/i18n/request.ts`（getRequestConfig 读 cookie）+ `lib/i18n/messages/{zh,en}.json`（40 个命名空间，key 完全对齐 0 错位） |
| 布局接线 | `app/layout.tsx` 改 async：`getLocale()`+`getMessages()` → `NextIntlClientProvider` 包裹；防闪烁 inline script 水合前对齐 `<html lang>`；SSR `lang={locale==='en'?'en':'zh-CN'}` + suppressHydrationWarning |
| 切换组件 | `app/components/LanguageToggle.tsx`：写 cookie（max-age=31536000, SameSite=Lax）+ localStorage 镜像 → `startTransition(() => router.refresh())`；按钮固定显示「目标语言」（zh 显 EN / en 显 中文），置于 providers.tsx 主题切换之后 |
| 全站翻译 | 40+ 页面/组件硬编码中文抽离为 `t()` 调用（chat/dashboard/knowledge/memory/feishu/agent×3/admin×12 等）；模块级中文常量（STATUS_LABELS 等）移入组件或改 t() 映射 |
| E2E | `tests/ui/i18n.spec.ts` 3 用例（UI-i18n-1 默认中文 / UI-i18n-2 切换即时生效 / UI-i18n-3 cookie 持久化）——**3 passed** |

### 12.2 翻译边界落实（D3/D5 审查结论）

- 全站 JSX 用户可见中文清零；剩余中文均为注释（允许）。
- 两处有意保留的非 UI 中文：`agent/page.tsx` LLM 模板提示词 `params.message`（发给后端执行，非 UI）；`admin/settings` `(使用默认值)`（后端配置比较值）。
- 依赖中文字符串判断的逻辑改为结构化判断（如 `toast.includes('失败')` → `toast.type === 'error'`）。

### 12.3 部署与 E2E 验证

- 测试服务器：git pull → `docker compose build frontend`（26 页编译通过）→ `up -d frontend` → restart nginx。
- SSR 验证：默认 cookie 渲染中文（19 处「登录」）；`Cookie: NEXT_LOCALE=en` 渲染英文（12 处 Email / 4 处 Sign in）——cookie 驱动 SSR 生效。
- E2E（本地 ssh 隧道 + 系统 Chrome）：3/3 passed（9.0s）。

### 12.4 E2E 环境踩坑与测试基建增强（playwright.config.ts）

1. **本地浏览器版本缺失**：@playwright/test 1.61.1 需 chromium-1228，本机缓存无此版本且 CDN 下载极慢 → config 支持 `PW_CHANNEL=chrome` 复用系统 Chrome（CI 不设该变量，行为不变）。
2. **本地代理劫持**：环境注入 `HTTP_PROXY/HTTPS_PROXY=127.0.0.1:57022`（WorkBuddy 沙箱代理），浏览器内 API 请求被代理接管 → 全部失败（「服务离线」+ 登录失败被前端 catch 误报为「邮箱或密码错误」；curl 直连不受影响）。config 支持 `PW_NO_PROXY=1` 注入 `--no-proxy-server` 强制直连。
3. **API base 覆盖**：spec 硬编码 `API_BASE='http://data-agent:8080/api/v1'`（CI docker 网络内），支持 `API_BASE` env 覆盖供本地 ssh 隧道使用（默认值不变，CI 零影响）。
4. **SPEC-084 遗留**：自注册已禁用（邀请制），spec 的 beforeAll 改为「校验预置用户可登录」（MongoDB 预置 `e2e-i18n@test.local`，bcrypt $2a$ cost 10，role=admin），支持 `E2E_USERNAME/E2E_PASSWORD` 覆盖。
5. **RBAC 导航过滤**：`nav-*` 侧边栏项受 `sidebar:*` 权限过滤，预置用户无 RBAC 角色时不渲染——断言锚点改用不受权限影响的 `page-title` / `dashboard-stat-kb` / `language-toggle`。

### 12.5 既有测试债务（非本 spec 范围，待专项处理）

- 整个 E2E 套件（auth/invite/chat 等）仍调用 SPEC-084 已删除的 `POST /auth/register`（现 404）——CI ui-tests 的存量用例依赖需统一迁移到邀请制/预置用户模式。
- 本地跑 E2E 时 3000 隧道须指向 nginx :80（`/api/v1` 反代在 nginx），非 frontend :3000。
