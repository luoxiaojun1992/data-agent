# KB 支持 URL 导入（后端解析网页 + 统一上传限制）

> **SPEC-081** | Status: 设计中

## 1. 目标

1. 知识库支持**通过 URL 导入**：用户粘贴网页 URL，**后端服务**解析网页（含 JS 渲染内容），提取文字与图片，自动创建 KB 文档并走现有索引管道。
2. 解析有**超时保护**；不在浏览器本地解析（跨域问题）。
3. 提取出的**文字**与**图片**分别建 doc（文字 1 个 doc，每张图片单独 1 个 doc），**创建逻辑与前端上传 PDF 解析出的文字/图片完全一致**，复用现有 `CreateDoc + GridFS + kb_index 入队` 机制，后续索引逻辑不变。
4. 统一上传限制（URL 导入与浏览器上传同规则，**KB 全场景限制的唯一事实源**）：
   - **标题**：最大 **200 字符**（rune 计，含 txt 上传 / PDF 解析 / URL 导入 / `kb_create_doc`（SPEC-086）所有建 doc 路径）
   - **文字**：最大 **5MB**（含 txt 上传、PDF 解析文本、URL 导入文本）
   - **图片**：最多 **10 张**、每张最大 **1MB**（含图片上传、PDF 解析图片、URL 导入图片）

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 | ✅ | KB 存储/索引管线（GridFS + chunking + embedding + Qdrant）已就绪 |
| SPEC-059/060/061 | ✅ | PDF + 图片上传、图片索引（kb_image use case）已上线 |
| SPEC-068 | ✅ | PII 脱敏（RedactText）在文本上传路径已接入，URL 文本必须复用 |
| SPEC-069/070 | ✅ | 删除级联（向量+图）对新建 doc 无影响；图数据库写入复用现有 AddChunks |
| SPEC-075 | ✅ | KB 列表 q/tag 搜索分页，URL 导入产生的 doc 自然进入列表 |
| SPEC-079 | ✅ | 健康检查无冲突（新增渲染服务不入必探清单） |
| — | — | 无硬阻塞依赖；headless 渲染服务为新增基础设施（§3） |

## 2. 背景

1. **只能上传本地文件**：KB 目前仅支持 txt / PDF / 图片上传。用户想收藏网页内容必须先手动保存/复制，体验差。
2. **浏览器本地解析不可行**：PDF 解析在浏览器本地做（pdf.js），但网页解析若在浏览器本地做会受**跨域限制**（目标站点无 CORS 头时 fetch 被拒）。因此解析必须在后端完成。
3. **上传无任何限制**：现状 txt 直接 `io.ReadAll`、图片 base64 直接 decode，**无大小/数量上限**——超大文件会打爆内存/GridFS，恶意用户可无限上传。需要统一限制。
4. **JS 渲染需求**：现代网页大量内容由 JS 动态渲染，纯 HTTP GET 拿不到正文（现有 `web_fetch` skill 只做静态抓取，不满足）。

## 3. 架构概述

```
浏览器（KB 页 URL 导入表单）
   │ POST /api/v1/knowledge/import-url {url}
   ▼
KB Handler（新增 ImportURL）
   ├─ 1. URL 校验（http/https + SSRF 防护：拒绝内网/回环/云元数据地址）
   ├─ 2. 渲染服务调用（headless chrome sidecar，超时保护）
   │      GET /json/new + page render → 渲染后 HTML + 页面截图（可选）
   ├─ 3. 内容提取
   │      文字：DOM 可读文本（≤5MB，截断）
   │      图片：<img> 绝对 URL 列表（≤10 张，下载后每张 ≤1MB，超限跳过）
   ├─ 4. 复用现有创建路径（与浏览器上传完全一致）
   │      文字 → RedactText → UploadFile(GridFS) → CreateDoc(txt) → EnqueueRaw kb_index
   │      每张图片 → UploadFile(GridFS) → CreateDoc(image) → EnqueueRaw kb_index
   ▼
   返回 { doc_ids: [...], skipped_images: n }（HTTP 200；doc 异步索引与现状一致）
```

**headless 渲染服务**（新增基础设施）：
- `docker-compose.yml` 新增 `headless-chrome` 服务（`chromedp/headless-shell` 或 `browserless/chrome` 镜像，中国镜像源拉取）。
- 后端通过 HTTP 调用渲染（chromedp 直接驱动或 browserless REST/WS）。
- **不加入 SPEC-079 健康检查必探清单**（与 ollama 同档：不探/条件探）；渲染服务不可用时 import-url 返回 503 错误，不影响其他 KB 功能。

## 4. API 设计

### 4.1 新增：URL 导入

| Method | Path | Description |
|--------|------|-------------|
| POST | `/api/v1/knowledge/import-url` | 解析 URL 并创建 KB 文档（认证：JWT） |

请求：

```json
{ "url": "https://example.com/article" }
```

响应（200）：

```json
{
  "doc_ids": ["uuid-text", "uuid-img-1", "uuid-img-2"],
  "text_doc_id": "uuid-text",
  "image_doc_ids": ["uuid-img-1", "uuid-img-2"],
  "skipped_images": 3,
  "text_bytes": 12345
}
```

错误语义（HTTP 400/422/502/503）：
- 400：URL 非法（非 http/https、host 解析失败）
- 403：SSRF 拦截（内网/回环/链路本地/云元数据地址）
- 422：页面无有效内容（文字为空且无图片）
- 502：目标站点抓取失败（超时/DNS/连接失败）
- 503：渲染服务不可用

### 4.2 既有接口补限制（向后兼容）

`POST /knowledge/docs`（浏览器上传路径）增加同样的服务端校验：
- 标题：>200 rune → 截断到 200 rune（标题是 label，截断无损，见 §5.3）
- 文本（multipart `file`）：>5MB → 400「文本超过 5MB 上限」
- 图片（`file_base64`）：解码后 >1MB → 400「图片超过 1MB 上限」；单次上传只接受单文件（现状如此，不改语义）

### 4.3 限制常量（唯一事实源）

> **本 spec 是 KB 全场景限制的唯一事实源**：web 端上传、PDF 解析、URL 导入、`kb_create_doc`（SPEC-086）均引用此处的常量，其他 spec 不重复定义。

| 常量 | 值 | 适用 |
|------|-----|------|
| `MaxKBTitleRunes` | 200（rune 计） | 所有 KB 文档标题（txt 上传 / PDF 解析 / URL 导入 / kb_create_doc） |
| `MaxKBTextBytes` | 5 MB (5×1024×1024) | txt 上传 / PDF 解析文本 / URL 导入文字 / kb_create_doc content |
| `MaxKBImageCount` | 10 | PDF 解析图片 / 直接图片上传批次 / URL 导入图片 |
| `MaxKBImageBytes` | 1 MB (1×1024×1024) | 单张图片 |
| `ImportURLRenderTimeout` | 30s | **整体解析网页超时**（端到端：从 headless 开始加载 URL 到取得最终渲染 DOM 的总时长；**非单个 HTTP 请求超时**） |
| `ImportURLTotalTimeout` | 120s | 整次导入（含渲染 + 文字提取 + 图片下载 + 建 doc）的端到端总预算 |
| `ImportURLImageDownloadTimeout` | 10s/张 | 单张图片**整体下载**超时（从发起下载到拿全该图片字节） |

## 5. 详细设计

### 5.1 URL 校验与 SSRF 防护（安全必备）

- 仅允许 `http://` / `https://`。
- 解析 host → DNS 解析 → 拒绝：回环（127.0.0.0/8, ::1）、私网（10/8, 172.16/12, 192.168/16）、链路本地（169.254/16，含云元数据 169.254.169.254）、0.0.0.0、组播/保留段。
- DNS 重绑定缓解：渲染请求前先解析校验 IP，再向该 IP 发起请求并复用连接（或至少渲染服务侧同样执行私网校验，双保险）。

### 5.2 JS 渲染与内容提取

- 渲染：调用 headless-chrome 打开 URL，等待网络空闲/固定延迟，取渲染后 DOM。
- 文字提取：渲染后 body 的 `innerText`（或可读性库抽正文），压缩空白，截断至 5MB。
- 图片提取：DOM 中 `<img>` 的绝对 URL（src / data-src / srcset 首个），去重，跳过 data URI 中的大图与图标类（可选启发式：宽高 < 50px 跳过）。逐个下载：超 1MB 跳过、超 10 张后停止。
- 全部子任务带超时保护（§4.3），总流程 120s 兜底。

### 5.2.1 超时语义（红线：整体 vs 单请求）

- `ImportURLRenderTimeout`（30s）是**「整体解析该 URL web page」的端到端超时**：计时从 headless-chrome 开始加载目标 URL 起，到页面渲染完成（网络空闲/固定延迟后取得最终 DOM）为止的**总时长**。它是一个**整体预算**，覆盖页面加载 HTML、CSS、JS、图片、XHR/fetch 等**全部子请求的累计耗时**。
- **不是**渲染服务内部每个单独 HTTP 请求（如单张图片、单个 JS/CSS 文件、单个 XHR）各自独立设 30s 超时。子请求自身的超时（TCP 建连、单文件下载）由渲染引擎/chrome 默认网络超时控制，远小于整体预算，并受整体预算兜底。
- `ImportURLTotalTimeout`（120s）是**整次 import-url 的端到端总预算**（渲染 ≤30s + 文字提取 + 图片逐个下载 ≤10s/张×10 + 建 doc/入队），任一步导致总时长超过 120s 即整体失败（502）。
- `ImportURLImageDownloadTimeout`（10s）是**单张图片的整体下载超时**（从发起下载到拿全该图片字节），同样不是每个 TCP 数据包/连接的超时。

### 5.3 复用现有创建路径（红线：不得复制第二套逻辑）

- 文字 doc：**完全复用** `UploadDoc` 的 Path 1 逻辑——`RedactText`（PII 脱敏）→ `UploadFile`（GridFS）→ `CreateDoc(txt)` → `EnqueueRaw("kb_index", KBIndexPayload{DocID, GridFSFileID})`。
- 图片 doc：**完全复用** `UploadDoc` 的 Path 2 逻辑——`UploadFile(GridFS)` → `CreateDoc(image)` → 入队。
- 实现方式：将 `UploadDoc` 内联逻辑抽为 service 方法 `CreateFromText(ctx, userID, title, fileName, text, sizeBytes) (doc, error)` 与 `CreateFromImage(ctx, userID, title, fileName, data []byte, mimeType) (doc, error)`，`UploadDoc` 与 `ImportURL` 都调用这两个方法（单一来源，杜绝分叉）。
- 标题规则：`<页面标题或 URL host>-text` 与 `-img-{n}`，与 PDF 上传的 `-{编号}` 风格一致；标题长度 ≤ `MaxKBTitleRunes`（200 rune），超长截断（截断到 200 rune，不报错——标题非核心内容，静默截断可接受，与正文「整体拒绝」策略不同）。
- 后续索引（chunking/embedding/Qdrant/ArcadeDB 图写入）**零改动**。

### 5.4 前端 KB 页 URL 导入 UI（独立按钮 + 独立弹窗）

- **独立按钮**：KB 页顶部新增「导入网址」按钮（`data-testid="kb-import-url-btn"`），与现有「上传文档」按钮并列，样式沿用 SPEC-078 `primaryButtonStyle` 体系的次级按钮。不侵入现有上传弹窗。
- **独立弹窗**（`data-testid="kb-import-url-modal"`）：
  - URL 输入框（`data-testid="kb-import-url-input"`）+ 「导入」提交按钮（`kb-import-url-submit`）+ 取消；
  - 弹窗内状态流转：空 URL 禁用提交；提交中显示 loading 并禁用按钮（导入为后端渲染，耗时可能达 120s，必须有进行中反馈）；
  - 成功后：关闭弹窗 + 刷新列表 + toast「导入完成（N 个文档）」，`skipped_images > 0` 时附加「（跳过 M 张超限图片）」提示；
  - 失败：弹窗内/ toast 展示后端错误语义（URL 非法 / SSRF 拦截 / 抓取失败 / 渲染服务不可用），弹窗不关闭，允许修改 URL 重试。
- **浏览器上传侧同步加限制**（与后端一致，避免上传后才被拒）：
  - txt/PDF 文本 >5MB → 前端拒绝并提示；
  - 图片：单张 >1MB 拒绝；单批次（含 PDF 解析出的图片）>10 张时超出的跳过并提示；
- 前端限制仅为体验优化，**后端校验是权威**（红线：不可只信前端）。

### 5.5 数据模型

无新集合。`knowledge_docs` 复用（file_type 用现有 `txt` / `image`；可选新增 `source_url` 字段记录来源，默认空，供未来追溯——列为可选项，不阻塞）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 knowledge_docs；`source_url` 可选字段） |
| 是否影响现有 API | Yes（`/knowledge/docs` 加服务端限制；新增 `/knowledge/import-url`） |
| 性能影响 | 低（导入是用户触发的一次性操作；渲染在独立 sidecar，不占主服务内存） |
| 是否需要新增 Skill | No（不是 ADK 工具，是 API） |
| 新增基础设施 | Yes（docker-compose `headless-chrome` 服务） |
| 新增依赖 | Go：chromedp 或 browserless HTTP client（二选一，见下） |
| 风险 | ① 镜像体积/拉取（国内源已配）；② 渲染服务单点（不可用仅影响 import-url）；③ SSRF 是安全关键点，必须测试覆盖 |

**渲染方案二选一（立项推荐）**：
- **A. browserless/chrome sidecar**（HTTP/REST 调用，后端零浏览器依赖，镜像 ~1GB，社区成熟）——推荐，隔离最好；
- B. chromedp 直接驱动（后端容器内嵌 chrome 二进制，镜像体积 +500MB，后端内存压力大）——备选。

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/api/handler/knowledge.go` | 新增 `ImportURL`；`UploadDoc` 增加限制校验 | Medium |
| `internal/service/knowledge/` | 抽 `CreateFromText` / `CreateFromImage`；新增 URL 解析服务或 logic | Medium |
| `internal/logic/webimport/`（新） | URL 校验 + SSRF + 渲染调用 + 内容提取（L1 可测部分尽量纯函数） | New/Large |
| `docker-compose.yml` | 新增 `headless-chrome` 服务 | Small |
| `frontend/app/knowledge/page.tsx` | URL 导入 UI + 前端限制校验 | Medium |
| `internal/api/handler/knowledge_test.go` 等 | ImportURL / 限制 / SSRF 单测 | Medium |
| `tests/ui/knowledge.spec.ts` 等 | E2E：URL 导入、超限拒绝 | Medium |

## 8. 测试策略

1. **Unit tests**（Go）：
   - SSRF 判定（纯函数）：公网/内网/回环/链路本地/云元数据/非法 URL 全表驱动；
   - 限制校验：5MB 文本、10 张 × 1MB 图片的边界（恰好等于/超出 1 字节）；
   - 内容提取：HTML→文字（空白压缩、5MB 截断）、图片 URL 提取（src/srcset/去重/数量上限）；
   - `CreateFromText` / `CreateFromImage`：与 UploadDoc 路径行为一致（mock GridFS/service）。
2. **Integration tests**：SSRF/渲染走 mock（httptest 假渲染服务），不依赖真实 chrome。
3. **E2E tests**（`tests/ui/`，编号 UI-XXX）：KB 页「导入网址」独立按钮/弹窗交互（打开、提交、成功关闭+列表刷新、失败留窗可重试）、超限文本被拒。
4. **审计**：`.agent/skills/go-ut-audit`。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | SSRF 判定、限制校验、HTML 提取 |
| L2 | 依赖接口，可 mock | **100%** | import-url service、CreateFromText/Image |
| L3 | 依赖 MongoDB/HTTP | **98%** | handler、GridFS 路径 |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc`（非 `ApplyMethodReturn`）验证 handler→service 参数传递正确性
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因并记录到 spec 中）
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

参考:
- `.agent/specs/spec-045-go-service-ut.md`
- `.agent/skills/go-ut-audit/SKILL.md`

## 10. 验证标准

1. 导入一个含 JS 渲染内容的公网页面 → 文字 doc 内容包含 JS 渲染出的正文；图片 doc 数量 = 页面有效图片数（≤10）。
2. 导入一个静态无图页面 → 1 个 txt doc；无图页面 → image_doc_ids 为空。
3. 超限：文字 >5MB 被截断到 5MB 创建成功（而非报错）；第 11 张图被跳过（skipped_images 计数正确）；>1MB 的单图被跳过。
4. SSRF：导入 `http://127.0.0.1/`、`http://169.254.169.254/`、`http://10.x.x.x/` 均返回 403，且未发出实际请求。
5. 渲染服务宕机 → import-url 返回 503，KB 其他功能（上传/列表/搜索）不受影响。
6. 浏览器上传路径：>5MB 文本、>1MB 图片被前后端一致拒绝；PDF 一次解析出 12 张图时仅前 10 张创建。
7. 导入产生的 doc 与上传产生的 doc 在列表、索引、删除（级联向量+图）行为完全一致。
8. KB 页「导入网址」为独立按钮 + 独立弹窗（testid：`kb-import-url-btn` / `kb-import-url-modal` / `kb-import-url-input` / `kb-import-url-submit`），上传弹窗不受影响。
9. `go test ./internal/...` 全绿；覆盖率 ≥98%；E2E 通过。
