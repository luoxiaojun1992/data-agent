# Chat 附件支持 PDF（解析文字前置 + 图片等价限制）

> **SPEC-077** | Status: 设计已定稿（2026-09-01）

## 1. 目标

让 chat 附件在图片之外支持 PDF：上传 PDF 时复用 KB 的浏览器端解析逻辑（`lib/pdf.ts` 的 `parsePdf`），解析出的**文字在发送时附加到用户输入提示词之前**（前端消息区不显示解析文字，用特殊标签标记用于前端展示 PDF 卡片），解析出的**图片按图片附件的限制处理**（数量/大小上限与直接上传图片完全一致）。**文字总量（用户提示词 + PDF 解析文字）合并限制 100KB**；图片限制保持现状（合并 ≤5 张 × 2MiB）。

> **本 spec 是 chat 附件限制的唯一事实源**：chat 场景的文字（用户提示词 + PDF 解析文字）与图片（用户图 + PDF 解析图）限制均以本 spec 为准，其他 spec（如 task、kb）不引用本 spec 的常量，各自有独立事实源（task → SPEC-087、kb → SPEC-081）。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 知识库系统 | ✅ | `lib/pdf.ts` 的 `parsePdf` 解析逻辑已就绪（KB 上传复用） |
| SPEC-004 Agent 核心引擎 | ✅ | chat 图片附件链路（`ChatRequest.Images` + `buildUserContent`）已就绪 |
| — | — | 无阻塞项 |

## 2. 背景（现状）

- chat 附件当前**仅支持图片**：前端 `accept="image/*"`，限制 `MAX_ATTACHMENT_IMAGES=5`、`MAX_ATTACHMENT_IMAGE_BYTES=2MiB`（与后端 `domainchat.MaxImages=5` / `MaxImageBytes=2MiB` 一致）。
- 后端 `buildUserContent(text, images)`：组装 `[user text part] + [每图一个 InlineData part]`，图片经 `ValidateImages` 校验（≤5 张、每张 ≤2MiB、mime 白名单）。
- PDF 解析已有现成实现：`frontend/lib/pdf.ts` 的 `parsePdf(file)` → `{ text, images }`（浏览器端 unpdf 提取纯文本 + 逐页渲染提取嵌入配图），KB 上传已在复用。

## 3. 架构概述

```
前端 chat 附件（图片 + PDF）
  ├─ PDF 文件 → parsePdf(file) → { text, images }
  ├─ 发送 payload：
  │    message = 用户输入原文
  │    pdfs = [{ name, text }]   ← 新增，text 为解析文字（前端不展示）
  │    images = PDF解析图 + 用户图片（合计 ≤5，每张 ≤2MiB）
  └─ 消息展示：pdfs 元数据 → 渲染 📄 PDF 卡片（显示文件名，不显示解析文字）
          │
          ▼
后端 ChatRequest 加 Pdfs 字段
  └─ buildUserContent：解析文字拼成 text part 放在用户输入前 + 图片 InlineData
```

## 4. API 设计

### 4.1 ChatRequest 扩展

| 字段 | 类型 | 说明 |
|------|------|------|
| `pdfs` | `[]PdfAttachment`，可选 | 新增。`PdfAttachment{ name, text }`：name 用于前端展示，text 为解析文字 |

```go
// PdfAttachment 携带 PDF 附件的文件名与解析文字。
type PdfAttachment struct {
    Name string `json:"name"`
    Text string `json:"text"`
}
```

### 4.2 图片限制（唯一事实源；与直接上传图片完全等价，保持现状）

| 约束 | 值 | 说明 |
|------|-----|------|
| 数量 | ≤ `MaxImages`(5) | **PDF 解析图 + 用户直接上传图片，合并计数** |
| 单张大小 | ≤ `MaxImageBytes`(2MiB) | 超限的 PDF 解析图丢弃（或前端降采样后仍超限则丢弃并提示） |
| 总量 | ≤ `MaxTotalBytes`(5MiB) | 全部图片解码后字节合计（`domainchat.ValidateImages` 既有约束） |

> 复用现有 `domainchat.ValidateImages` 校验，PDF 解析图走同一条图片链路，无需新增校验逻辑。**现有限制已满足需求（有数量+大小+总量限制），保持现状不改**（本次需求「没有才加 1MB/10 张」，已有 5 张×2MiB×5MiB 则不动）。常量已落地于 `internal/domain/chat/image.go`。

### 4.3 文字限制（唯一事实源；新增，本次补齐）

| 约束 | 值 | 说明 |
|------|-----|------|
| 文字总量 | ≤ `MaxChatTextBytes`(100KB) | **用户提示词 + 全部 PDF 解析文字，合并计数**（UTF-8 字节数） |

- 前端发送前合并校验：`text 字节数 + Σ pdfs[].text 字节数` > 100KB → 拒绝发送并 toast 提示「消息文字超过 100KB 上限」。
- 后端权威校验：`ChatRequest.Text` + `ChatRequest.Pdfs[].Text` 合并字节数 > 100KB → HTTP 400，错误信息含实际字节数与上限。
- 超限 PDF 解析文字策略：**整体拒绝优先**（不要静默截断——静默截断会造成上下文丢失且用户无感知）。前端逐 PDF 截断作为可选优化不列入本 spec。
- 直接上传图片不受文字限制影响（图片走 §4.2）；用户提示词与 PDF 文字是同一 100KB 池子。

### 4.4 用户提示词 XSS 校验（安全；新增，本次补齐）

> **chat 用户直接输入的提示词必须做 XSS 校验**，但**明确排除 PDF 解析文字**（见下方红线）。

| 字段 | 校验函数 | 落点 | 语义 |
|------|---------|------|------|
| 用户提示词（`req.Messages` 中 `Role=="user"` 的文本，即 `lastText`） | `security.ValidateXSS` | chat 请求入口：handler `HandleChat` 绑定 `ChatRequest` 后 / service `prepareRun` 提取 `lastText` 后、组装 LLM content 前 | 含 XSS 载荷 → 400「消息包含非法内容」 |

- 复用 `security.ValidateXSS`（定义于 SPEC-081 §4.4，`internal/domain/security` 包），**不重复实现**；语义 = block（拒绝），与 LLM 层 `AuditInput` 的 XSS block 一致。
- 落点天然在 PDF 文字附加**之前**：XSS 校验只针对用户直接输入文本，PDF 解析文字在 service 层 `buildUserContent` 才前置，故校验顺序正确、无需额外剥离。

> **范围排除（红线）**：**PDF 解析文字不做 XSS 校验**——PDF 是文档内容，可能天然含 HTML/JS 示例（如技术文档的代码片段），XSS block 会误伤正常附件。XSS 校验仅针对用户手动键入的提示词。

**纵深防御关系**：handler 层 `ValidateXSS`（第一道门，提前拦截）→ LLM 层 `AuditInput`（第二道门，SPEC-068，进 LLM 前兜底）。两者语义一致（block）、互不替代。

## 5. 详细设计

### 5.1 前端：PDF 附件解析与发送

- 附件选择框 `accept` 增加 `application/pdf, .pdf`
- PDF 文件 → `parsePdf(file)` → `{ text, images }`
- 图片合并：`PDF解析图 + 用户图片`，去重后按 `MAX_ATTACHMENT_IMAGES` 截断，逐张校验 `MAX_ATTACHMENT_IMAGE_BYTES`（超出丢弃并 toast 提示）
- 发送 payload 组装：`pdfs: [{ name: file.name, text }]` + `images`（合并后）

### 5.2 后端：解析文字前置

`buildUserContent` 扩展：在用户 text part **之前**插入 PDF 解析文字的 text part（每个 PDF 一个 part，或合并为一个 part），图片 InlineData 逻辑不变。

```
parts = [
  (pdf.text 非空时) text part( pdf.text ),
  (用户输入非空时) text part( text ),
  image InlineData part × N,
]
```

### 5.3 前端展示：特殊标签标记 PDF

- 前端**本地 message 对象**新增 `pdfs?: { name: string }[]`（只存 name，不存 text），渲染时显示 📄 文件卡片，**不显示解析文字**。
- 历史消息（Messages API 拉取）：后端 session events 里的 user content 含解析文字前缀，前端识别**特殊标签**剥离。

**特殊标签协议**（前端与后端约定）：

```
解析文字拼入提示词时用标签包裹：
[PDF:filename.pdf]
<解析文字>
[/PDF:filename.pdf]
```

前端渲染历史消息时识别 `[PDF:` 标签块 → 剥离文字 → 显示 `📄 filename.pdf` 卡片。

### 5.4 边界与限制

| 项 | 规则 |
|----|------|
| PDF 文件大小上限 | 建议 `MAX_PDF_BYTES = 20MiB`（前端 `parsePdf` 前校验；KB 上传当前无硬限制，此处单独定义，超限 toast 拒绝） |
| 文字总量上限 | **100KB（用户提示词 + PDF 解析文字合并，UTF-8 字节）**，前后端双重校验，超限整体拒绝（§4.3） |
| 图片数量/大小 | **保持现状**：合并 ≤5 张、每张 ≤2MiB（§4.2） |
| 无文字的 PDF（纯扫描件） | `text` 为空时仍按图片附件处理（解析图照常），`pdfs[].text` 为空不前置文字 |
| 解析图超 5 张 | 仅保留前 N 张使合并后 ≤5，其余丢弃并提示 |
| 解析图超 2MiB | 丢弃该图并提示（或前端降采样，作为可选项） |
| Session title 污染 | `createNewSession` 的 title 取 `lastText`（用户输入原文，不含 PDF 文字），PDF 文字不进入 title |

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | Yes（`ChatRequest` 加 `pdfs` 字段，向后兼容：不传时行为不变） |
| 性能影响 | PDF 解析在浏览器端（unpdf），与 KB 上传同源；文字 100KB 合并上限约束超长 PDF 的 token 占用 |
| 是否需要新增 Skill | No |
| 是否需要后端改动 | Yes（`buildUserContent` + `ChatRequest` + title 提取 + 100KB 文字校验） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/chat/contract.go` | 新增 `PdfAttachment` + `ChatRequest.Pdfs` + `MaxChatTextBytes=100KB` 常量 | Small |
| `internal/service/chat/chat_service.go` | `buildUserContent` 前置解析文字 + title 提取 + 100KB 合并文字校验 | Medium |
| `frontend/lib/attachment.ts` | 新增 PDF 附件类型 + 限制常量（MAX_PDF_BYTES、MAX_CHAT_TEXT_BYTES） | Small |
| `frontend/app/chat/page.tsx` | 附件 accept 加 PDF、parsePdf 调用、发送 payload、pdf 卡片展示、发送前文字合并校验 | Medium |
| `internal/service/chat/chat_service_test.go` | 覆盖 pdf_text 前置 + 图片合并 + 100KB 边界 | Medium |

## 8. 测试策略

1. **Unit tests（Go）**：`buildUserContent` 验证 PDF 文字 part 顺序（在用户输入前）+ 图片合并；`PdfAttachment` 序列化；100KB 合并校验边界（恰好 100KB 通过 / +1 字节拒绝 / 多 PDF 合并计数）。
2. **E2E tests**：`UI-xxx` 覆盖 chat 上传 PDF → 发送 → 前端显示 PDF 卡片且不显示解析文字 → 后端收到前置文字；超 100KB 文字被前端拒绝。
3. **审计**：`.agent/skills/go-ut-audit`。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5 Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体 | **100%** | PdfAttachment 组装 |
| L2 | 依赖接口，可 mock | **100%** | chat service |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `service/*`, `api/handler/*` |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc` 验证 handler→service 参数传递正确性
- [ ] **必须** Service 测试的写操作验证写入内容字段和值
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证实际结果

## 10. 验证标准

1. chat 可上传 PDF，前端显示 📄 PDF 卡片（文件名），**不显示解析文字**。
2. 发送后，后端用户 content 中解析文字位于用户输入之前。
3. PDF 解析图 + 用户图片合计 ≤5 张、每张 ≤2MiB，超限正确丢弃并提示（保持现状）。
4. 纯图片上传行为完全不变（向后兼容）。
5. 文字合并（提示词 + PDF 解析文字）恰好 100KB 可发送；100KB+1 字节被前端拒绝、后端返回 400。
6. 直接上传图片与用户提示词适用同一限制池：多 PDF 合计文字按字节合并计算，不按单个 PDF 单独计算。
7. XSS 校验：用户提示词含 `<script>` / `<img onerror>` / `javascript:` 载荷 → 400 拒绝；PDF 解析文字含同样内容**不**触发拒绝（范围排除）；普通提示词正常发送。
