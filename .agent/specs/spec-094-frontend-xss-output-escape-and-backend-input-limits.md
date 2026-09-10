# 前端 XSS 输出安全收编（React 结构性安全 + Markdown 协议白名单显式化）

> **SPEC-094** | Status: 📐 已定稿（技术结论闭环，待确认 SafeText 定位后进入实现，2026-09-10）

> **术语红线（本 spec 的核心前提）**：
> - **React 生态没有「字符串 HTML 转义」这回事**。React 的 `{text}` 文本插值是
>   **结构性安全**（`createTextNode` 文本节点，浏览器不解析其中的 `<script>`），
>   不是把 `<` 替换成 `&lt;`。因此**对文本做 `escapeHtml` 反而会 double-escape**。
> - **后端校验一律不动**（结构性限制 + `ValidateXSS` 输入校验均正确，PDF 文字豁免是
>   SPEC-077 §4.4 既定设计）。
> - XSS 防护唯一有效位置是**渲染层**：纯文本靠 React 结构性安全，Markdown 靠
>   react-markdown 默认不渲染 raw HTML + `defaultUrlTransform` 协议过滤。

## 1. 目标

1. 前端新增 **`SafeText` 纯文本语义出口组件**（`components/SafeText.tsx`），把
   「后端/LLM 文本 → DOM」的**纯文本出口**收编为显式边界。**它不做 HTML 字符串转义**
   （React 已结构性安全，转义会 double-escape），作用是语义标注 + 审计锚点。
2. Markdown 出口（chat 正文、task/run 结果）走既有 `Markdown` 组件，仅补 `a` 组件
   **协议白名单显式化**（react-markdown v9 已默认 `defaultUrlTransform` 过滤，此为纵深防御收尾）。
3. **不改后端任何校验**；**不做请求层转义中间件**（double-escape / 破坏 Markdown，见 §3.1）。

## 2. 现状调查结论（2026-09-10 三次深化，已闭环）

### 2.1 后端安全现状（不动）

| 层 | 机制 | 核实位置 | 结论 |
|----|------|---------|------|
| 输入校验 | `ValidateXSS`（chat/KB/task，PDF 文字豁免） | `service/chat/chat_service.go:90`、`handler/knowledge.go`、`handler/task.go:54` | ✅ 不动 |
| 输入审计 | `AuditInput`（PII 脱敏 Presidio + regex 兜底） | `domain/security/auditor.go:116` | ✅ 不动 |
| 输出审计 | `AuditOutput`（PII + `xss` rule：`<script`→`&lt;`） | `domain/security/auditor.go:154`，挂在 `modelcfg/audited.go:103` + `runtime.go:327` | ✅ 不动 |
| 结构性限制 | chat 100KB/5图/2MB、KB 5MB/1MB/10图、task 图片复用 | `service/chat`、`service/knowledge`、`handler/task.go` | ✅ 不动 |

### 2.2 前端渲染安全模型（逐文件核实，含版本）

| 出口 | 现状 | 安全性 |
|------|------|--------|
| `dangerouslySetInnerHTML` | 项目代码**零使用** | ✅ 唯一能把字符串变 HTML 的口子是关死的 |
| React 文本插值 | chat system/user 消息 `{msg.content}`、task 标题、run 标题/描述、toast | ✅ **结构性安全**（`createTextNode`，等效转义） |
| Markdown 出口 | `Markdown.tsx`：react-markdown **^9.1.0** + remark-gfm，**无 `rehype-raw`** | ✅ raw HTML 不渲染（`<script>` 当文本）；v9 默认 `defaultUrlTransform` 过滤 `javascript:`/`data:` |
| Markdown 使用点 | 仅 3 处：`chat/page.tsx`、`agent/runs/[runId]/page.tsx`、`Markdown.tsx` | ✅ 全走安全组件 |

**总体结论**：前端输出**已经 XSS 安全**（React 结构性安全 + react-markdown v9 默认安全 +
零 dangerouslySetInnerHTML）。本 spec 的价值是**把隐式安全显式化**：
- `SafeText` 把纯文本出口**命名化**，`git grep SafeText` 可枚举、防未来误改成 HTML 渲染；
- `Markdown` 的 `a` 协议白名单**显式化**（纵深防御，非修 bug）。

## 3. 设计方向

### 3.1 为什么不做请求层「转义中间件」（技术不可行，结论不变）

| # | 障碍 | 说明 |
|---|------|------|
| 1 | **double-escape** | 请求层 `<`→`&lt;` 后，前端 `{text}` 插值时 React 把 `&`→`&amp;`，显示 `&lt;` 乱码；要避免只能改 `dangerouslySetInnerHTML`（引入 XSS 的反模式） |
| 2 | **字段语义不可统一** | 同一响应 `content`（Markdown）与 `title`（纯文本）处理相反，请求层无法感知 |
| 3 | **破坏内容** | 破坏 Markdown 代码块、URL、base64 data URI |

**补充（本次深化新增的根因）**：即便没有上面 3 点，请求层转义在 React 里也**没有意义**——
React 文本插值已经是结构性安全的，多转一层只是 double-escape。**转义的物理位置本就不在数据层，而在渲染层。**

### 3.2 SafeText 定位（关键修正）

`SafeText` **不做 `escapeHtml`**。它是纯文本出口的语义锚点：

```tsx
// components/SafeText.tsx
export default function SafeText({ text, className }: {
  text: string | null | undefined;
  className?: string;
}) {
  // React 文本插值已结构性安全（createTextNode），无需也不应做 HTML 字符串转义
  // （escapeHtml 会与 React 自动转义叠加导致 double-escape）。
  return <span className={className}>{String(text ?? '')}</span>;
}
```

价值：①语义标注「这是纯文本，不走 Markdown」；②`git grep SafeText` 可枚举所有纯文本出口；
③防未来有人把纯文本出口误改成 `dangerouslySetInnerHTML` 或 HTML 渲染库。

### 3.3 Markdown `a` 协议白名单显式化

react-markdown v9 已默认 `defaultUrlTransform` 过滤 `javascript:`/`data:`，自定义 `a` 组件拿到的
`props.href` 已是安全 URL。仍**显式加白名单**（`http:`/`https:`/`mailto:`，其余拒绝）作为
纵深防御收尾，防止未来升级/替换渲染库时丢失默认防护。

## 3.5 待确认决策点

| # | 决策点 | 推荐（默认） | 备选 |
|---|--------|-------------|------|
| D1 | `SafeText` 定位 | **语义锚点，不做字符串转义**（React 已结构性安全，转义会 double-escape） | 显式 `escapeHtml`（会 double-escape，技术不可取） |

> D2（统一请求组件收编）已按用户要求**取消**。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | **No**（后端零改动） |
| 性能影响 | 忽略不计（SafeText 为 O(1) 包装） |
| 是否需要新增 Skill | No |

## 7. 相关文件（预估，定稿时精化）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/components/SafeText.tsx` | 纯文本语义出口组件（不转义） | New |
| `frontend/components/Markdown.tsx` | `a` 协议白名单显式化 | Small |
| `frontend/app/**`（chat/agent 纯文本插值点） | 接入 `SafeText` | Small |

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 本 spec 后端零改动，仅前端；Go UT 无新增（不触发 ut-workflow 增量门禁）。

## 10. 验证标准（定稿时扩展）

1. ✅ 后端安全现状逐文件核实，一律不动。
2. ✅ 前端渲染安全模型核实：React 结构性安全 + react-markdown v9 默认安全 + 零 dangerouslySetInnerHTML。
3. 待确认 D1 → 实现：
   - 纯文本出口统一走 `SafeText`（`git grep` 可枚举）；
   - `Markdown` 的 `a` 协议白名单显式化；
   - `dangerouslySetInnerHTML` 持续保持零使用（可作为 lint 规则固化）。
