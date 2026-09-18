# SPEC-103 纯前端国际化（i18n 翻译 + 语言切换）

> **SPEC-103** | Status: ✅ 设计定稿（D1~D5 全定稿；暂不实现）
> 日期：2026-09-18 立项 → 2026-09-18 设计定稿

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

### D4 — SSR 水合一致性（cookie + 防闪烁）

- 背景：SSR 在服务器渲染、读不到 localStorage → 若仅存 localStorage，首屏按默认中文渲染、客户端接管后变英文 → 闪烁 + hydration mismatch 警告（SPEC-076 主题切换踩过的同类坑）。
- 定稿：语言状态存 **cookie**（SSR 可读，服务器按正确语言渲染首屏）+ localStorage 镜像（客户端快速读）；采用 next-intl 标准 cookie 机制（必要时 `frontend/middleware.ts` 处理）。
- 防闪烁：参考 SPEC-076 的 inline script 范式（React 水合前设置 `lang` 属性）。

### D5 — 一次性全站翻译 + 质量约束

- **一次性**完成全站 UI 文案翻译，**不分期**。
- **只翻译纯文字**：不翻译 HTML/JS 代码、代码块、markdown 源码、className、data-testid、API 路径、日期格式串等有功能语义的内容。
- **不影响样式/结构/显示效果**：仅替换文本内容；DOM 结构、className、testid、样式规则不变。
- **长度变化自适应**：中↔英文字长度差异大，按钮/标签/表头/弹窗/表格不得写死宽度，长英文单词允许换行（word-break），布局用 flex/min-width 保证切换后不错位、不溢出。
