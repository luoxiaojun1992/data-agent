# 前端 XSS 输出转义组件 + 后端输入限制校验补齐

> **SPEC-094** | Status: 📐 调研完成，待拍板 D1~D5 后定稿（2026-09-10）

> **术语红线**：**输入校验（validate）≠ 输出转义（escape）**。本 spec 的立场：
> 后端只补**结构性限制**（文本长度、图片数量/大小），XSS 校验后端**不做**；
> XSS 防护由**前端输出转义组件**统一承担（渲染任何后端/LLM 返回文本前转义）。

## 1. 目标

1. 前端新增 **XSS 转义组件**（如 `SafeText` / `escapeHtml` 工具函数），用于所有
   后端/LLM 返回内容的**输出转义**（暂不深化，待调查 Markdown 渲染管线后定稿）。
2. 调查并补齐后端输入限制缺口：**KB、chat、task** 三条链路的文本长度、图片
   数量/大小校验必须**后端兜底**（不能只靠前端限制）。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-077 | ✅ | chat 附件/PDF 限制（前端 100KB/5 图/2MB/PDF 20MB）已就绪，后端已有部分 |
| SPEC-081 | ✅ | `security.ValidateXSS` 与 KB 限制常量（MaxKBTitleRunes=200 / MaxKBTextBytes=5MB / MaxKBImageBytes=1MB / MaxKBImageCount=10）已就绪 |
| SPEC-084 | ✅ | task 创建校验（title/description ValidateXSS，e486197）已就绪 |
| — | — | 无阻塞项；立项阶段不实现 |

## 2. 现状调查结论（2026-09-10 全量调研，已闭环）

### 2.1 后端结构性限制现状矩阵（逐文件核实）

| 模块 | 文本长度 | 图片数量 | 图片大小 | XSS（现状） | 核实位置 |
|------|---------|---------|---------|------------|---------|
| **chat 消息** | ✅ 100KB（用户+PDF文字合并，字节） | ✅ ≤5 张 | ✅ ≤2MB/张 + ≤5MB/总量 + MIME 白名单 | ✅ 用户提示词（PDF 文字豁免） | `service/chat/chat_service.go:90,96,232` + `domain/chat/image.go` |
| **chat PDF** | ⚠️ 解析文字并入 100KB | ❌ **无数量上限** | ❌ **文件大小无后端校验**（前端 20MB） | ❌ 豁免（SPEC-077 §4.4 设计如此） | `validateChatTextSize` |
| **KB** | ✅ 正文 5MB / 标题 200 runes 截断 | ⚠️ 用户上传=单文件（天然无批量）；URL 导入 webimport 截断 ≤10 张 | ✅ ≤1MB/张 | ✅ 标题（handler 入口 block） | `handler/knowledge.go:53,69,92` + `service/knowledge/service.go:162,194` |
| **task 创建** | ❌ **title/description/params 均无长度限制** | ✅ ≤5 张（复用 `ValidateImages`） | ✅ ≤2MB/5MB（复用） | ✅ title+description（`handler/task.go:54-61`） | `handler/task.go:46,54` |
| **task 运行** | ❌ 同上（params 直透） | ✅ executor 二次校验（纵深防御） | ✅ 同上 | ❌ params.message（LLM 输入，允许代码样例） | `logic/agent/executor.go:373` + `orchestrator.go:76` |
| **redact API** | ❌ **text 无长度限制**（SPEC-093 新 API 缺口） | — | — | ❌ 无需（脱敏目标文本） | `handler/redact.go`（无 len 检查） |
| **feishu webhook** | ❌ MVP echo 无限制（不落库不进 LLM） | — | — | ❌ 无需（JSON 编码 echo） | `service/im/service.go:111` |

### 2.2 前端输出渲染管线现状

| 项 | 现状 | 结论 |
|----|------|------|
| `dangerouslySetInnerHTML` | **项目代码零使用**（仅 node_modules 第三方） | ✅ React 文本插值全链路自动转义 |
| Markdown 渲染 | `components/Markdown.tsx`：react-markdown + remark-gfm + 自定义组件（无 html 渲染） | ✅ react-markdown 默认**不渲染 raw HTML**（转义为纯文本） |
| 链接安全 | `a` 标签 `target="_blank" rel="noopener noreferrer"`；react-markdown 内置 `defaultUrlTransform` 过滤 `javascript:`/`data:` 危险协议 | ✅ 已防护 |
| **总体结论** | 当前输出渲染**已基本安全**（隐式安全） | spec 的转义组件定位 = **把隐式安全收编为显式统一出口** + 兜底审计 |

### 2.3 缺口清单（后端需补的结构性限制）

| # | 缺口 | 建议方案 | 优先级 |
|---|------|---------|:---:|
| G1 | task title/description/params 文本长度 | title ≤200 runes（对齐 KB）；description/message ≤100KB（对齐 chat） | P0 |
| G2 | chat PDF 数量上限 | ≤5 个（对齐图片数量）；文字 100KB 已兜底 | P0 |
| G3 | chat PDF「文件大小」后端校验 | **不可复现**：后端收到的 Pdfs 只有 `{name, text}`（前端已解析），文件本身不进后端。后端能兜底的只有「解析文字字节数」（已并入 100KB）。前端 20MB 限制保持前端职责，spec 记录此边界 | 无需后端改动 |
| G4 | redact API text 长度 | ≤100KB（对齐 chat） | P0 |
| G5 | feishu webhook body 大小 | `http.MaxBytesReader`（如 1MB）；MVP echo 阶段低优先 | P2 |

### 2.4 待拍板决策点

| # | 决策点 | 建议（默认） | 备选 |
|---|--------|-------------|------|
| D1 | task 文本长度取值 | title ≤200 runes、description/message ≤100KB | 统一 64KB |
| D2 | chat PDF 数量上限 | ≤5（与图片对齐） | ≤10 |
| D3 | 现有后端 `ValidateXSS`（chat 提示词/KB 标题/task title+description 3 处）去留 | **保留**为纵深防御（输入侧拒绝明显攻击，输出侧转义兜底，双保险） | 按 spec 立场移除、纯输出转义 |
| D4 | redact API 长度 | ≤100KB | 与 chat 解耦取 1MB |
| D5 | SafeText 组件形态 | `escapeHtml` 工具函数 + `<SafeText>` 包装组件（纯文本出口），Markdown 出口走 react-markdown（已转义，仅补 href 协议白名单显式化） | 侵入 Markdown 渲染管线 |

## 3. 设计方向（调研已闭环，待 D1~D5 拍板后定稿）

### 3.1 前端 XSS 输出转义组件

- 核心：`lib/escape.ts`（`escapeHtml` 纯函数，L1 可单测）+ `components/SafeText.tsx`
  （纯文本出口包装）。所有「后端/LLM 文本 → DOM」的**纯文本出口**统一走该组件。
- Markdown 出口（chat 消息正文、KB 内容、task 结果）：react-markdown 已默认转义
  raw HTML，**不侵入渲染管线**；仅将 `a` 组件显式加协议白名单
  （`http/https/mailto`，`javascript:`/`data:` 拒绝）作为显式化收尾（D5）。
- 出口枚举（定稿时逐一点名）：chat 消息正文（Markdown）、session 标题、
  KB 文档标题/内容、task 标题/描述/结果、通知消息、human channel 消息。

### 3.2 后端输入限制补齐

- 只补结构性限制：G1（task 长度）/G2（chat PDF 数量）/G4（redact 长度），
  对齐现有 domain 常量，单一事实源（`domain/chat` / `domain/task`）。
- G3 结论：PDF 文件大小后端不可复现（文件不进后端），记录边界、不做改动。
- 现有 `ValidateXSS` 3 处校验点按 D3 决定去留（建议保留为纵深防御）。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | 可能（task 长度限制等新增 400 错误映射） |
| 性能影响 | 忽略不计（O(n) 长度检查） |
| 是否需要新增 Skill | No |

## 7. 相关文件（预估，定稿时精化）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `frontend/components/SafeText.tsx`（或 lib） | XSS 输出转义组件 | New |
| `frontend/app/**`（chat/kb/task/notification 等渲染出口） | 接入转义组件 | Medium |
| `internal/domain/task` + `internal/service/task` | task 文本长度限制 | Small |
| `internal/api/handler/chat.go` / `service/chat` | chat PDF 大小限制（如需） | Small |

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
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `logic/sql`, `logic/openapi`, `logic/report`, `config` |
| L2 | 依赖接口，可 mock | **100%** | `queue/`, service interfaces |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `service/*`, `api/handler/*` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc`（非 `ApplyMethodReturn`）验证 handler→service 参数传递正确性
- [ ] **必须** Service 测试的写操作（`UpdateOne`, `InsertOne` 等）验证写入内容的字段和值
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因并记录到 spec 中）
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

### 测试模式

- Handler: `httptest.NewRecorder` + `gin.CreateTestContext` + real handler → mock service
- Service: 直接注入 mock repository / 使用 `gomonkey` 模拟 MongoDB collection
- Logic (L1): 纯 table-driven test，无 mock 依赖

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考:
- `.agent/specs/spec-045-go-service-ut.md` — Go UT 全覆盖 spec
- `.agent/skills/go-ut-audit/SKILL.md` — UT 审计 skill
- `.github/workflows/ut-workflow.yml` — CI UT workflow

## 10. 验证标准（调研级，定稿时扩展）

1. ✅ 待调查清单 5 项已全部闭环（见 §2.1/§2.2 逐文件核实结论）。
2. 待拍板 D1~D5 → 定稿 → 实现：
   - 前端所有纯文本渲染出口走 `SafeText`（`git grep` 可枚举）；
   - Markdown `a` 组件协议白名单显式化；
   - 后端 G1/G2/G4 缺口补齐，handler/service 层兜底 + 单测覆盖（对齐 L3 98% 底线）。
