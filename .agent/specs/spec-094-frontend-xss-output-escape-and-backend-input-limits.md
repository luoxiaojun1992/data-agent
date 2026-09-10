# 前端统一 XSS 输出转义（渲染层出口组件）

> **SPEC-094** | Status: 📐 深化调研完成，待拍板 D1~D3 后定稿（2026-09-10）

> **术语红线**：**输入校验（validate）≠ 输出转义（escape）**。本 spec 的最终立场：
> - **后端校验一律不动**（结构性限制 + `ValidateXSS` 输入校验均已正确，PDF 文字豁免是
>   SPEC-077 §4.4 的既定设计）。
> - XSS 防护收敛到**前端输出转义**，且**只在渲染层做**（出口组件），不在请求层做
>   （请求层转义会破坏 Markdown/HTML 并造成 double-escape，详见 §3.1）。

## 1. 目标

1. 前端新增**统一 XSS 输出转义出口组件**（`lib/escape.ts` + `components/SafeText.tsx`），
   把所有「后端/LLM 文本 → DOM」的**纯文本出口**收编为显式安全边界。
2. Markdown 出口（chat 正文、task/run 结果、KB 内容）走既有 `Markdown` 组件
   （react-markdown 默认不渲染 raw HTML），仅补 `a` 组件协议白名单显式化。
3. **不改后端任何校验**；**不引入请求层转义中间件**（技术不可行，见 §3.1）。

## 2. 现状调查结论（2026-09-10 二次深化，已闭环）

### 2.1 后端安全现状（逐文件核实）

| 层 | 机制 | 核实位置 | 结论 |
|----|------|---------|------|
| 输入校验 | `ValidateXSS`（block 明显 XSS 输入，PDF 文字豁免） | `service/chat/chat_service.go:90`、`handler/knowledge.go`、`handler/task.go:54` | ✅ 正确，不动 |
| 输入审计 | `AuditInput`（PII 脱敏 Presidio + fallback regex） | `domain/security/auditor.go:116` | ✅ 正确，不动 |
| 输出审计 | `AuditOutput`（PII 脱敏 + fallback regex，含 `xss` rule） | `domain/security/auditor.go:154` | ⚠️ 见 §2.2 |
| 输出调用点 | LLM 层 `modelcfg/audited.go:103` + runtime 层 `runtime.go:327` | 输出审计挂在 ADK 层 | ✅ 已挂，不动 |
| 结构性限制 | chat 100KB/5 图/2MB、KB 5MB/1MB/10 图、task 图片复用 | `service/chat`、`service/knowledge`、`handler/task.go` | ✅ 正确，不动 |

### 2.2 后端输出侧 XSS 现状（关键发现）

后端 LLM 输出侧**已经存在**一段 XSS sanitize：`domain/security/auditor.go` 的
`OutputRules` 含 `xss` rule（regex `(?i)<\s*script`），`sanitizeByType("xss", s)` 把
`<` `>` 替换为 `&lt;` `&gt;`。该 sanitize 挂在 `AuditOutput`，由 ADK 层
（`modelcfg/audited.go`、`runtime/runtime.go`）在 LLM 输出时调用。

**结论**：
- 它**只覆盖 `<script`**，漏 `<img onerror>` / `<svg onload>` / `<a href=javascript:>`
  等其它 XSS 向量，**不是完整防护**。
- 它**不破坏 Markdown/HTML**：正常 LLM 输出不含 `<script` 字面量；即便代码块里示范
  `<script>`，前端 react-markdown 会把 `&lt;` 按 entity 解码还原为 `<` 文本渲染。
- **按用户决策：保持现状不动**（它不是主防护，主防护在前端渲染层）。

### 2.3 前端渲染管线现状（逐文件核实）

| 出口 | 现状 | 安全性 |
|------|------|--------|
| `dangerouslySetInnerHTML` | 项目代码**零使用**（仅 node_modules） | ✅ |
| Markdown 出口 | `components/Markdown.tsx`：react-markdown + remark-gfm + 自定义组件（无 html 渲染）；`defaultUrlTransform` 过滤 `javascript:`/`data:` | ✅ 安全（默认不渲染 raw HTML） |
| 纯文本插值出口 | chat system/user 消息 `{msg.content}`、task 标题、run 标题/描述、toast | ✅ React JSX 文本插值自动转义 |
| Markdown 使用点 | 仅 3 处：`chat/page.tsx`（正文）、`agent/runs/[runId]/page.tsx`（结果+消息）、`Markdown.tsx` 自身 | ✅ 全走安全组件 |

**总体结论**：当前输出渲染**已隐式安全**。本 spec 的转义组件定位 = **把隐式安全收编
为显式统一出口**（`SafeText` 纯文本 / `Markdown` Markdown），提供审计锚点与未来防护。

## 3. 设计方向

### 3.1 核心判断：请求层「转义中间件」不可行

> 用户方案为「封装统一后端请求组件，挂转义中间件，所有后端返回数据统一转义」。
> 经调研，该方案在 React 生态下**技术上不可行**，原因如下：

| # | 障碍 | 说明 |
|---|------|------|
| 1 | **double-escape** | 请求层把 `<` → `&lt;` 后，前端 `{text}` 插值渲染，React 会把 `&` 再转义为 `&amp;`，最终显示 `&lt;script&gt;` 乱码。要避免只能改用 `dangerouslySetInnerHTML` —— 这恰恰是引入 XSS 的反模式 |
| 2 | **字段语义不可统一** | 同一响应里 `content`（Markdown）与 `title`/`username`（纯文本）处理方式相反：Markdown 不能 HTML 转义（会破坏内联 HTML 与代码块），纯文本才需要。请求层无法感知字段语义，只能维护脆弱的白名单/黑名单清单 |
| 3 | **破坏内容** | 全量转义会破坏 Markdown 语法（代码块 ` ``` `、内联代码里的 `<` `>`）、URL、base64 data URI 等非文本字段 |

**正确统一出口 = 渲染层组件**（XSS 的物理位置本来就在渲染层）：纯文本走 `SafeText`，
Markdown 走 `Markdown`。React 文本插值本就自动转义，`SafeText` 的作用是把这层隐式安全
**显式化、可 grep 枚举、防未来误改**。

> 「统一请求组件」本身可保留为**独立优化项**（收编 `apiFetch` 为 `lib/http.ts`，
> 自动 JSON 解析/错误/401 处理），但**与 XSS 解耦**——它负责「请求收口」，不负责「转义」。

### 3.2 前端实现

- **`lib/escape.ts`**：`escapeHtml(s)` 纯函数（`& < > " '` 五元转义），L1 可单测。
- **`components/SafeText.tsx`**：纯文本出口组件 `<SafeText text={...} />`，内部
  `escapeHtml` 后经 React 插值渲染（即便 React 再转义一次，五元转义对已转义实体幂等
  ——`escapeHtml` 不重复转义 `&lt;` 中的 `&`，见 §3.3 幂等性）。
- **`components/Markdown.tsx`**：`a` 组件加显式协议白名单
  （`http:`/`https:`/`mailto:`，其余 `javascript:`/`data:` 等拒绝），作为显式化收尾。

### 3.3 escapeHtml 幂等性（避免 double-escape）

`escapeHtml` 必须先判断「是否已转义」：对已含 `&lt;`/`&gt;`/`&amp;` 的文本不重复转义，
保证 `SafeText` 与 React 插值、以及后端 `AuditOutput` 已转义的 `&lt;script` 组合时**幂等**，
不会出现 `&amp;lt;` 乱码。实现为：先转义裸 `&`（排除已转义实体），再转 `< > " '`。

## 3.5 待拍板决策点

| # | 决策点 | 推荐（默认） | 备选 |
|---|--------|-------------|------|
| D1 | **转义出口位置** | **渲染层出口组件**（`SafeText` + `Markdown`，请求层转义因 double-escape/破坏 Markdown 不可行） | 请求层中间件（需字段白名单 + 改用 dangerouslySetInnerHTML，反模式，不建议） |
| D2 | 统一请求组件收编 | 一并封装 `lib/http.ts`（自动 JSON/错误/401），但**不挂转义**，与 XSS 解耦 | 维持现状 `apiFetch` 不动 |
| D3 | `SafeText` 是否显式转义 | **显式 `escapeHtml`**（幂等实现），作为审计收编层，出口即安全 | 仅透传（依赖 React 隐式转义，不新增组件价值） |

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | **No**（后端零改动） |
| 性能影响 | 忽略不计（O(n) 转义，仅纯文本出口） |
| 是否需要新增 Skill | No |

## 7. 相关文件（预估，定稿时精化）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/lib/escape.ts` | `escapeHtml` 幂等纯函数 | New |
| `frontend/components/SafeText.tsx` | 纯文本出口组件 | New |
| `frontend/components/Markdown.tsx` | `a` 协议白名单显式化 | Small |
| `frontend/app/**`（chat/agent/knowledge 纯文本插值点） | 接入 `SafeText` | Small |
| `frontend/lib/http.ts`（若 D2 采纳） | 统一请求客户端（与 XSS 解耦） | New |

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 本 spec 后端零改动，仅前端；Go UT 无新增（不触发 ut-workflow 增量门禁）。

## 10. 验证标准（定稿时扩展）

1. ✅ 后端安全现状（输入校验 + 输出 AuditOutput xss sanitize + 结构性限制）已逐文件核实，均不动。
2. ✅ 前端渲染管线现状（零 dangerouslySetInnerHTML、react-markdown 安全、React 插值自动转义）已核实。
3. 待拍板 D1~D3 → 定稿 → 实现：
   - 前端纯文本出口统一走 `SafeText`（`git grep` 可枚举）；
   - `Markdown` 的 `a` 组件协议白名单显式化；
   - `escapeHtml` 幂等单测（含「已转义实体不重复转义」用例）。
