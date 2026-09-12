# Task PDF 解析 + kb/chat/task Excel 解析

> **SPEC-096** | Status: 📐 深化中（D1~D6 已定稿；暂不实现）

## 1. 目标

1. **task 创建支持 PDF 附件**：仅在「常规创建弹窗」中增加 PDF 上传（「日常总结模版」不加），PDF 解析逻辑与 kb/chat 一致（前端 unpdf 解析出纯文本 + 嵌入配图），解析出的图片数量/尺寸/文本校验限制**合并入 task 本身的文本与图片上传**，逻辑与 chat 的 PDF 解析相同。
2. **task 描述隐藏 PDF 实际内容**：PDF 解析文字不向用户展示，task 列表/详情只显示 📄 卡片，逻辑与 chat 历史显示 PDF 的方式一致。
3. **kb/chat/task 三端新增 Excel 解析**：校验/显示/XSS 豁免逻辑与 PDF 解析一致，仅 chat 历史显示为「Excel」（📊 图标）；Excel **只解析为纯文本、不支持图片**。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-077（chat 附件 PDF） | ✅ | PDF 解析前端链路（unpdf `parsePdf`）+ 后端 `PdfAttachment` + `[PDF:name]` 标签协议，本 spec 复用 |
| SPEC-081（KB URL 导入 / 上传限制） | ✅ | `MaxKBTextBytes/MaxKBImageBytes/MaxKBImageCount` 单一事实源，kb 的 PDF/Excel 文字走 `CreateFromText` |
| SPEC-086（kb_create_doc skill） | ✅ | 复用 `CreateTextDoc` 全流程（长度校验→标题截断→PII 脱敏→GridFS→CreateDoc→异步入队），Excel 文字建 doc 同 PDF |
| SPEC-082（chat/task 取消） | ✅ | task 结构（`Task.Params` + executor `deriveUserMessageFromParams`/`buildTaskContent`）已定型，本 spec 在其上扩展 |

> 无硬阻塞前置；纯前端解析 + 后端 task 扩展，可独立实现。

## 2. 背景（现状，已逐文件核实）

### 2.1 chat PDF 解析（SPEC-077，已实现，本 spec 复用的基准）

| 环节 | 现状 |
|------|------|
| 前端解析 | `frontend/lib/pdf.ts` 的 `parsePdf(file)` 用 unpdf 解析出 `{ text, images }`（text=合并纯文本，images=嵌入配图 dataURL） |
| 文件大小 | `MAX_PDF_BYTES = 20MB`（`frontend/lib/attachment.ts`） |
| 图片合并 | PDF 解析出的配图并入图片附件，共享 `MAX_ATTACHMENT_IMAGES = 5` / `MAX_ATTACHMENT_IMAGE_BYTES = 2MB` 计数 |
| 文本上限 | 用户提示词 + PDF 文字合并 UTF-8 bytes ≤ `MaxChatTextBytes = 100KB`（`domain/chat/contract.go`） |
| 后端 DTO | `PdfAttachment { Name, Text }`；`ChatRequest.Pdfs []PdfAttachment` |
| 标签协议 | `formatPDFText`：`[PDF:name]\n<text>\n[/PDF:name]` 前置到 user 文本（`service/chat/chat_service.go`），LLM 读到文字 |
| 前端隐藏 | `stripPdfBlocks(content)` 用正则剥离 `[PDF:name]…[/PDF:name]`，提取文件名渲染 📄 卡片（`app/chat/page.tsx`），解析文字绝不展示 |
| XSS 豁免 | PDF 文字含 `<script>` 不触发 `ValidateXSS`（SPEC-077 §4.4：移除 AuditInput 的 xss_script 规则，PDF 文字豁免） |

### 2.2 kb PDF/上传（SPEC-081/086，已实现）

- 前端同样用 `parsePdf` 解析 PDF → 文字走 `CreateFromText` 建 doc、配图走 `CreateFromImage`（或 `UploadDoc`）。
- 限制单一事实源（`domain/knowledge/model.go`）：`MaxKBTextBytes=5MB` / `MaxKBTitleRunes=200` / `MaxKBImageBytes=1MB` / `MaxKBImageCount=10`。
- 标题 XSS block（`ValidateXSS`），正文不校验（LLM 输入允许代码样例）。

### 2.3 task 现状（本 spec 的扩展对象）

| 环节 | 现状 | 缺口 |
|------|------|------|
| 创建接口 | `POST /tasks`（`handler/task.go CreateTask`）接收 `title/description/images/cron/schedule/model_id` | 无 `pdfs` 字段 |
| 图片附件 | `Images []domainchat.ImagePart` → `validateRequestImages`（复用 `ValidateImages`，≤5 张/2MB）→ `EncodeImages` 存 `params["images"]` | 已复用 chat 图片校验（D2 定稿：改为 task 特有常量/类型/函数，去 `domainchat` 依赖） |
| 文本 | `description` → `params["description"]`；XSS 校验 title/description | **无文本长度上限**（SPEC-094 已确认缺口） |
| 执行消费 | executor `deriveUserMessageFromParams` 取 text（query/message/prompt/description）+ images；`buildTaskContent` 组装 genai.Content | 无 PDF 文字前置、无 Excel |
| 前端弹窗 | 常规创建弹窗（`agent-task-modal`，title+desc+📎图片+模型+定时）vs 日常总结模版弹窗（`agent-template-daily-summary-title`，独立于常规弹窗） | 常规弹窗无 PDF 上传；日常总结不涉及 |

### 2.4 Excel 现状

- 三端均**无任何 Excel 解析能力**。
- 前端**无 Excel 解析库**（`package.json` 仅有 `unpdf`，无 xlsx/exceljs/read-excel-file）。

### 2.5 三端校验现状矩阵（D2 确认结论，2026-09-12 逐文件核实）

| 校验项 | chat | kb | task（现状 → D2 定稿后） |
|--------|:---:|:---:|:---:|
| 文本长度 | ✅ `MaxChatTextBytes=100KB`（用户+PDF 合并） | ✅ `MaxKBTextBytes=5MB` | ❌ **无** → ✅ 新增 task 特有 `MaxTaskTextBytes=100KB` |
| XSS | ✅ `ValidateXSS`（用户提示词，PDF 文字豁免） | ✅ `ValidateXSS`（标题；正文不校验） | ✅ `ValidateXSS`（title/description，`handler/task.go:54/58`；封装为 task 特有校验入口） |
| 图片数量 | ✅ `MaxImages=5`（`domain/chat/image.go`） | ✅ `MaxKBImageCount=10` | ❌ 复用 chat → ✅ 新增 task 特有 `MaxTaskImages=5` + `ValidateTaskImages` |
| 图片大小 | ✅ `MaxImageBytes=2MB` / `MaxTotalBytes=5MB` | ✅ `MaxKBImageBytes=1MB` | ❌ 复用 chat → ✅ 新增 task 特有 `MaxTaskImageBytes=2MB` / `MaxTaskTotalBytes=5MB` |

> **结论**：chat/kb 四类校验全齐；task 的 XSS 规则调用已存在，但**文本长度与图片校验当前借用 `domainchat` 的常量/函数**，违反「各 domain 边界独立」的分层铁律。D2 定稿：**task 域新增自己特有的校验常量与校验函数**（`internal/domain/task/limits.go`），不再 import `domainchat` —— 文本长度 `MaxTaskTextBytes=100KB`、图片数量/大小 `MaxTaskImages=5`/`MaxTaskImageBytes=2MB`/`MaxTaskTotalBytes=5MB`、特有 `ImagePart` 类型与 `ValidateTaskImages`；XSS 规则仍复用 `security.ValidateXSS`（安全规则单点维护），但由 task 特有校验函数封装调用。

## 3. 需求分解

| # | 子需求 | 涉及端 | 关键点 |
|---|--------|--------|--------|
| R1 | task 常规创建弹窗增加 PDF 上传 | task | 复用 chat 的 `parsePdf`；图片合并入 task 图片、文本合并入 task 文本 |
| R2 | task 描述隐藏 PDF 内容 | task | 只显示 📄 卡片，不显示 PDF 文字 |
| R3 | kb/chat/task 增加 Excel 解析 | 三端 | 纯文本、无图片；chat 历史显示 📊；XSS 豁免同 PDF |

## 4. 详细设计（立项级，待深化）

### 4.1 R1 — task PDF 解析（复用 chat 逻辑）

- 前端：常规创建弹窗增加「📄 添加 PDF」入口，复用 `parsePdf(file)` 解析出 `{ text, images }`。
  - PDF 文件大小 ≤ `MAX_PDF_BYTES`（20MB）。
  - 解析出的图片并入 task 图片附件，受 task 特有图片校验常量约束（`MaxTaskImages=5` / `MaxTaskImageBytes=2MB` / `MaxTaskTotalBytes=5MB`，D2 已定稿 task 域独立常量）：PDF 配图 + 手动上传图片合并计数。
  - 解析出的文字与 description 合并，受 task 特有 `MaxTaskTextBytes = 100KB` 上限约束（D2 已定稿）。
- 后端：`CreateTask` 请求体新增 `pdfs []PdfAttachment`（复用 `domain/chat.PdfAttachment`），图片照旧走 `params["images"]`（改用 task 特有 `EncodeImages`），PDF 文字按 D1（B）以 `[PDF:name]…[/PDF:name]` 标签拼进 description 存储。
- **日常总结模版弹窗不加 PDF 入口**（红线，用户明确）。

### 4.2 R2 — task 描述隐藏 PDF 内容（D1 已定稿 B）

目标：task 列表/详情展示 description 时**只显示 📄 卡片、绝不显示 PDF 解析文字**。两种落地方式（D1 已定稿 B）：

| 方案 | 存储 | 前端显示 | executor 消费 | 备注 |
|------|------|---------|--------------|------|
| A | PDF 文字单独存 `params["pdfs"]`（`[{name,text}]`），description 保持纯净 | description 天然不含 PDF 文字；从 `params["pdfs"]` 拿文件名渲染 📄 卡片 | `deriveUserMessageFromParams` 额外取 `params["pdfs"]`，`buildTaskContent` 用 `formatPDFText` 前置标签（复用 chat 逻辑） | 结构性隐藏，description 不污染；但需新增 `params["pdfs"]` 字段 + executor 多一处消费 |
| **B ✅** | PDF 文字以 `[PDF:name]…[/PDF:name]` 标签拼进 description 存 DB | 前端 `stripPdfBlocks` 剥离 + 📄 卡片（完全复用 chat） | executor 直接用 description 作 text | **与 chat 存储协议完全一致**，复用度最高；description 含标签 |

> **D1 定稿 B**（2026-09-12）：与 chat 存储协议完全同构，前端 `stripPdfBlocks` 剥离 + 📄 卡片、executor 直接用 description 作 text，复用度最高、无额外 `params["pdfs"]` 字段；「description 被污染」可控（前端展示必剥离标签）。

### 4.3 R3 — Excel 解析（kb/chat/task 三端）

- **前端解析**：新增 Excel 解析库 `read-excel-file`（D5 已定稿，MIT），把 `.xlsx` 解析为**纯文本**（逐 sheet 拼接单元格文本，不提取图片）。`read-excel-file` **仅支持 `.xlsx`、不支持 `.xls`**（D5 已收敛，`.xls` 不在范围内）。
  - 复用 `parsePdf` 的返回形态思路，新增 `parseExcel(file): Promise<{ text: string }>`（**无 images**）。
- **标签协议**：与 `[PDF:name]…[/PDF:name]` 对称，新增 `[Excel:name]…[/Excel:name]`；前端剥离函数从 `stripPdfBlocks` 泛化为同时处理 PDF/Excel 标签（返回 `{ text, pdfs, excels }`）。
- **三端落地**：

| 端 | Excel 文字去向 | 图片 | 显示 |
|----|---------------|------|------|
| kb | `CreateFromText` 建 doc（`FileType=txt`，纯文本） | 不支持 | 标题/文件名，无图片 |
| chat | 前置到 user 文本（`[Excel:name]` 标签），合并入 `MaxChatTextBytes` | 不支持 | 历史显示 📊 卡片（非 📄） |
| task | 按 D1（B）以 `[Excel:name]…[/Excel:name]` 标签拼进 description（LLM 消费），合并入 `MaxTaskTextBytes` 上限 | 不支持 | 描述/详情显示 📊 卡片 |

- **校验/XSS 豁免与 PDF 一致**：Excel 解析文字同样**豁免 `ValidateXSS`**（单元格可能含 `<script>`/HTML 片段，属正常数据）；文件大小、文本长度、图片计数限制与 PDF 对齐。
- **Excel 不支持图片**：`parseExcel` 不提取嵌入图片，不并入图片附件。

### 4.4 文件类型判断

- 复用/扩展 `frontend/lib/pdf.ts` 的 `isPdfFile` / `SUPPORTED_EXTENSIONS`，新增 `isExcelFile(fileName)`（仅 `.xlsx`，D5 已收敛不支持 `.xls`）。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（task 复用 `Task.Params` 存 pdfs/excel 文字） |
| 是否影响现有 API | Yes：`POST /tasks` 请求体新增 `pdfs`（及 excel 文字字段） |
| 是否需要新增 Skill | No（纯前端解析 + task handler/service 扩展） |
| 是否需要后端改动 | Yes：task handler（`pdfs` 字段 + 特有校验常量 + XSS 豁免 + 文本上限）+ executor（`params["pdfs"]` 消费 + `formatPDFText` 前置） |
| 性能影响 | 前端 PDF/Excel 解析占用浏览器内存；task 文本上限兜底，不影响现有链路 |
| License | unpdf（PDF，已用）；Excel 库 `read-excel-file`（MIT，✅ 合规，D5 已定稿；**严禁** GPL/AGPL/SSPL） |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/lib/pdf.ts`（或新建 `attachment.ts` 扩展） | 新增 `isExcelFile` / `parseExcel`（纯文本，无图片） | Small |
| `frontend/lib/attachment.ts` | 新增 Excel 类型/限制常量 | Small |
| `frontend/app/agent/page.tsx` | 常规创建弹窗加 PDF 上传 + 图片合并；日常总结模版不加 | Medium |
| `frontend/app/chat/page.tsx` | 加 Excel 附件解析 + 📊 卡片；`stripPdfBlocks` 泛化处理 Excel 标签 | Medium |
| `frontend/app/knowledge/page.tsx` | 加 Excel 上传解析 → `CreateFromText` | Medium |
| `internal/domain/task/limits.go` | **新增** task 特有校验常量/类型/函数（`MaxTaskTextBytes`/`MaxTaskImages`/`MaxTaskImageBytes`/`MaxTaskTotalBytes`/`ImagePart`/`ValidateTaskImages`/`ValidateTaskText`），去 `domainchat` 依赖 | Small |
| `internal/domain/chat/contract.go` | 复用 `PdfAttachment`；可能新增 Excel 标签常量 | Small |
| `internal/api/handler/task.go` | `CreateTask` 加 `pdfs`/excel 字段；图片/文本校验改用 task 特有常量（去 `domainchat` import）+ XSS 豁免 | Medium |
| `internal/logic/agent/executor.go` | `deriveUserMessageFromParams` 取 pdfs/excel + `buildTaskContent` 前置 | Medium |
| `internal/service/task/service.go` | `CreateTask` 透传 pdfs/excel 到 `params` | Small |
| `frontend/package.json` | 新增 Excel 解析库（D5） | Small |

## 7. 测试策略

1. **Unit tests（Go）**：task handler `CreateTask` 的 pdfs 字段解析/文本上限/XSS 豁免；task 域 `ValidateTaskImages`/`ValidateTaskText`；executor `buildTaskContent` 的 PDF/Excel 标签前置；`formatPDFText`/`formatExcelText`。
2. **E2E tests（前端）**：task 创建带 PDF → 描述显示 📄 不显示 PDF 文字 → 执行成功；chat 带 Excel → 历史显示 📊；kb 上传 Excel → 建 doc。
3. **手动验证**：真实 PDF/Excel 文件解析效果。

## 8. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9. 验证标准

1. task 常规创建弹窗可上传 PDF，解析文字 + 配图分别并入 task 文本/图片（计数/尺寸校验生效，走 task 特有常量）。
2. 日常总结模版弹窗**无** PDF 上传入口（红线）。
3. task 列表/详情展示 description 时**只显示 📄 卡片，不显示 PDF 文字**。
4. task 执行时 LLM 能读到 PDF 文字（标签前置生效）。
5. kb/chat/task 均可上传 `.xlsx`，解析为纯文本（无图片）；`.xls` 不支持（前端拒绝）。
6. chat 历史中 Excel 附件显示 📊 卡片（非 📄）。
7. Excel 解析文字含 `<script>` 不触发 XSS（豁免同 PDF）。
8. 三端 PDF/Excel 的文件大小、文本长度、图片计数限制一致生效；task 的「description（用户原文 + PDF 文字 + Excel 文字合并）」≤ task 特有 `MaxTaskTextBytes=100KB`。

## 10. 待定稿决策点（深化阶段拍板）

| # | 决策点 | 说明 |
|---|--------|------|
| D1 | PDF 文字存储方式 | ✅ **已定稿（2026-09-12）**：**B 方案**——PDF 文字以 `[PDF:name]…[/PDF:name]` 标签拼进 description 存 DB（与 chat 存储协议完全一致；前端 `stripPdfBlocks` 剥离 + 📄 卡片；executor 直接用 description 作 text） |
| D2 | task 校验常量归属 | ✅ **已定稿（2026-09-12）**：**task 域新增自己特有的校验常量与校验函数**（新建 `internal/domain/task/limits.go`），**不再复用 `domainchat` 的常量/函数**（分层铁律：domain 边界独立）。文本长度 `MaxTaskTextBytes=100KB`（对齐 chat，常量独立定义）；图片 `MaxTaskImages=5` / `MaxTaskImageBytes=2MB` / `MaxTaskTotalBytes=5MB` + 特有 `ImagePart` 类型 + `ValidateTaskImages`；XSS 规则仍复用 `security.ValidateXSS`（安全规则单点维护），由 task 特有 `ValidateTaskText` 封装调用（XSS 仅校验用户手输原文，PDF/Excel 解析文字豁免，同 chat）。校验对象 = 整个 description（用户原文 + PDF 文字 + Excel 文字合并，UTF-8 bytes ≤ 100KB）。详见 §2.5 校验矩阵 |
| D3 | Excel 标签协议 | ✅ **已定稿（2026-09-12）**：`[Excel:name]…[/Excel:name]`（对称 PDF，name 用于前端 📊 卡片、text 供 LLM） |
| D4 | Excel 解析范围 | ✅ **已定稿（2026-09-12）**：**全部 sheet 拼接**（按 sheet 顺序逐 sheet 拼单元格文本）；单元格类型序列化方式（数字/日期/公式）留实现阶段细化 |
| D5 | Excel 解析库选型与格式范围 | ✅ **已定稿（2026-09-12）**：**`read-excel-file`**（MIT，持续维护），**仅 `.xlsx`、不支持 `.xls`**（`.xls` 不在本 spec 范围内，前端 `isExcelFile` 仅放行 `.xlsx`）。**导入 = npm `package.json`**（`npm install read-excel-file`，Next.js 走 bundler 直接装）；CDN standalone 版仅官方 README 建议非 bundler 场景用，本项目不用 |
| D6 | kb Excel 建 doc 的 FileType | ✅ **已定稿（2026-09-12）**：复用 `txt`（纯文本），不新增 `xlsx` 类型 |
