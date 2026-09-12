# pptx skill 优化 — 同时支持 Markdown 与 HTML

> **SPEC-098** | Status: 立项（待深化；暂不实现）

## 1. 目标

1. **pptx_generator skill 同时支持 Markdown 与 HTML 两种内容格式**：LLM 可选任一格式生成 `.pptx`，默认兼容旧的 markdown 调用。
2. **在 skill 描述中明确「HTML 生成的 PPT 更精美」**：引导 LLM 在需要精美排版的场景优先使用 HTML，并给出 HTML 用法指导（标签 → 幻灯片元素的映射 + 行内样式支持）。
3. **同步原始 seed 数据**：更新 `predefinedSkills()` 中 `pptx_generator` 的描述，使 skill 管理页 / `skill_search` 可发现「支持 HTML」这一能力。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-008（Skill 实现层） | ✅ | `pptx_generator` 作为 ADK function tool 的注册范式（`internal/adk/tools/tools.go` + `predefinedSkills`） |
| SPEC-086/087（skill seed 同步范式） | ✅ | 「新增/修改 skill 同步 `predefinedSkills` 原始 seed 数据」的既有约定 |
| genppt v0.0.0-20260101075009-61df96324be9 | ✅ | 底层库已内置 `FromHTMLWithOptions`，本次无需升级依赖 |

> 无硬阻塞前置；纯后端逻辑扩展 + tool 描述/seed 更新，可独立实现。

## 2. 背景（现状与根因，已逐文件核实）

### 2.1 现状

| 环节 | 现状 | 文件 |
|------|------|------|
| 生成逻辑 | 仅 `genppt.FromMarkdownWithOptions(markdown, opts)` | `internal/logic/pptx/pptx.go` |
| tool 参数 | `content`（markdown）+ `file_name` | `internal/adk/tools/tools.go:1310-1313` |
| tool 描述 | 「from markdown content … # slide titles, - bullet points」 | `tools.go:1070` |
| seed 描述 | 「从 markdown 内容生成 .pptx PowerPoint 文件」 | `internal/service/skill/config.go:166` |

### 2.2 根因：markdown 转换能力有限

genppt 的 markdown 转换器（`markdown.go`）只支持 5 种块，样式由 `MarkdownOptions` **全局统一**控制：

| blockType | 来源语法 | 能否逐元素定制样式 |
|-----------|---------|:---:|
| `heading` | `#` / `##` | ❌ 全局 HeadingColor |
| `text` | 正文 | ❌ 全局 BodyColor |
| `bullet` | `-` / `1.` | ❌ |
| `code` | ``` ``` ``` | ❌ 全局 CodeBackground |
| `image` | `![](url)` | ❌ 无圆角/浮动/定位 |

**缺陷**：① 无表格；② 无法逐元素指定颜色/字号/对齐/背景；③ 无法定制单页背景色。LLM 只能堆叠标题+列表，成品单调、无层次、无视觉重点 →「丑」。

### 2.3 genppt 已内置 HTML 转换（本次复用的能力）

genppt 的 `html.go` 提供了 `FromHTML` / `FromHTMLWithOptions`，能力显著更强：

| HTML 元素 | 映射 | 行内样式支持 |
|-----------|------|:---:|
| `<h1>` | 新幻灯片（标题） | `color` / `background-color` / `text-align` |
| `<h2>`~`<h6>` | 页内小节标题 | `color` / `font-size` / `text-align` / `background-color` |
| `<p>` | 正文 | `color` / `font-size` / `text-align` / `background-color` |
| `<ul>/<ol>/<li>` | 列表 | `li` 支持 `color` |
| `<pre>/<code>` | 代码块 | — |
| `<table>` | **表格**（markdown 不支持） | — |
| `<img>` | **图片**（base64/URL/本地） | `border-radius` / `float:left\|right` / `width` / `height` |
| `<hr>` / `<section>` | 手动分页 | — |
| `<body>` / `<div>` / `<style>` | 幻灯片背景色 | `background-color` |

另有**两遍渲染自动缩放**（内容过高自动 scale 到一页内），markdown 路径无此能力。

> **结论**：genppt 的 HTML 转换器才是「精美」的正确打开方式；当前 skill 只是没接上它。本次改动**不升级依赖、不引入新库**，纯粹把已存在的 `FromHTMLWithOptions` 暴露给 LLM。

## 3. 需求分解

| # | 子需求 | 涉及端 | 关键点 |
|---|--------|--------|--------|
| R1 | pptx 生成逻辑支持 HTML | 后端 logic | 新增 `GenerateHTML`，复用 `FromHTMLWithOptions` |
| R2 | tool 增加格式选择 | 后端 tool | `format` 参数（markdown/html）+ 描述/jsonschema 更新 |
| R3 | 同步原始 seed 数据 | 后端 seed | `predefinedSkills` 更新 pptx_generator 描述 |

## 4. Skill 接口设计（pptx_generator tool）

| 参数 | 类型 | 必填 | 说明 |
|------|------|:---:|------|
| `content` | string | ✅ | 演示文稿内容；`format=html` 时为 HTML 片段，`format=markdown` 时为 Markdown |
| `format` | string | ❌ | `"html"`（推荐，更精美）或 `"markdown"`（默认，向后兼容） |
| `file_name` | string | ❌ | 输出文件名（默认 `presentation.pptx`） |

- `format` 缺省时按 `markdown` 处理（向后兼容旧的 LLM 调用与既有测试）。
- 返回值不变：`{ path: "presentation.pptx" }`（session workspace 相对路径）。

## 5. 详细设计

### 5.1 生成逻辑（`internal/logic/pptx/pptx.go`）

```go
// 保留：markdown → pptx（向后兼容）
func Generate(markdown string, outputPath string) error { /* 现有实现不变 */ }

// 新增：html → pptx
func GenerateHTML(html string, outputPath string) error {
    dir := filepath.Dir(outputPath)
    os.MkdirAll(dir, 0700)
    opts := genppt.DefaultHTMLOptions()
    opts.TitleColor = "#1E3A5F"      // 与 markdown 的深色系对齐（实现阶段可微调）
    opts.HeadingColor = "#1E3A5F"
    opts.BodyColor = "#333333"
    opts.SlideBackground = "#FFFFFF"
    pres := genppt.FromHTMLWithOptions(html, opts)
    return pres.WriteFile(outputPath)
}
```

### 5.2 tool 层（`internal/adk/tools/tools.go`）

- `PPTXGeneratorArgs` 新增 `Format string` 字段。
- `pptxGenerator` 函数按 `format` 分发：`html` → `pptxpkg.GenerateHTML`，否则 `pptxpkg.Generate`。
- **description 更新**（toolSpec 里的两处 Name/Description 同文案）：明确「支持 markdown 与 html；**html 更精美，推荐**」并附 HTML 用法指导（见 5.4）。
- jsonschema 更新：`content` 描述按 format 双语义；`format` 描述说明取值与推荐。

### 5.3 seed 同步（`internal/service/skill/config.go`）

`predefinedSkills()` 中 `pptx_generator` 的 `Description` 由「从 markdown 内容生成」改为「从 markdown 或 HTML 内容生成 .pptx（HTML 支持表格/图片/逐元素样式，更精美，推荐）」。

> ⚠️ **同步机制注意（D3 已定稿）**：`SeedSkills` 幂等逻辑「已存在则跳过」——线上 DB 中已存在的 `pptx_generator` 记录**不会**因 seed 改动自动更新描述。故 D3 定稿为：**① 代码层更新 `predefinedSkills()` 原始 seed 数据（保证新建环境/新 seed 正确）；② 实现时写一次性脚本直接更新线上 DB 中 `pptx_generator` 的 description**，不走 SeedSkills 幂等跳过逻辑。脚本定位：按 skill `name=pptx_generator`（或 `_id`）更新 `description` 字段，与代码 seed 保持一致。

### 5.4 HTML 用法指导（写入 tool description / jsonschema）

给 LLM 的 HTML → PPT 映射约定（genppt HTML 解析器实际支持、已核实）：

```
一张幻灯片 = 一个 <h1> 块（其后内容归该页）；<hr> 或 <section> 手动分页。
常用元素：
  <h1 style="color:#1E3A5F;text-align:center">页标题</h1>   — 新页 + 标题
  <h2>小节标题</h2>                                          — 页内子标题
  <p style="color:#333;font-size:18px">正文段落</p>
  <ul><li style="color:#C0392B">要点</li></ul>               — 列表
  <table><tr><th>表头</th></tr><tr><td>数据</td></tr></table> — 表格
  <img src="data:image/png;base64,..." style="border-radius:8px;float:right">
  <pre><code>代码</code></pre>
  <body style="background-color:#F4F6F8">                   — 全局背景色
行内样式：color / font-size(px) / text-align / background-color / border-radius / float
```

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | 否（tool 内部行为；`format` 为可选向后兼容参数） |
| 是否需要新增 Skill | No（扩展现有 `pptx_generator`） |
| 是否需要后端改动 | Yes：`logic/pptx` + `adk/tools` + `service/skill`（seed 描述） |
| 是否需要升级依赖 | No（genppt 已内置 `FromHTMLWithOptions`） |
| License | genppt = MIT ✅（不新增任何依赖） |
| 性能影响 | HTML 解析与 markdown 同级（`x/net/html` 标准解析），可忽略 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/logic/pptx/pptx.go` | 新增 `GenerateHTML`（复用 `FromHTMLWithOptions`） | Small |
| `internal/adk/tools/tools.go` | `PPTXGeneratorArgs` 加 `format` + 分发逻辑 + description/jsonschema 更新 | Medium |
| `internal/service/skill/config.go` | `predefinedSkills` 的 pptx_generator 描述更新 | Small |
| `internal/logic/pptx/pptx_test.go`（新增） | `GenerateHTML` 单测（有效 HTML → 产出合法 .pptx；空内容报错） | New |

## 8. 测试策略

1. **Unit tests（Go）**：
   - `GenerateHTML`：给定含 `<h1>/<ul>/<table>/<img>` 的 HTML → 产出 `.pptx` 文件（校验文件存在、大小 > 0、zip 头 `PK`）。
   - `GenerateHTML`：空 HTML → 返回错误（或产出空文稿，实现阶段定语义）。
   - `pptxGenerator`（tool）：`format=html` 分发到 `GenerateHTML`、`format=markdown`/缺省分发到 `Generate`（用 gomonkey 断言分发）。
2. **E2E / 冒烟**（可选）：真实 session 调 `pptx_generator` 分别用 markdown / html 各生成一次，人工检查 HTML 版排版更佳。
3. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量。

## 9. UI Test / E2E 验收规则

> 本 spec 为后端 tool 行为 + seed 描述变更，无前端交互改动；不新增 `tests/ui/` 用例。如后续涉及前端展示 skill 描述的改动，再补 E2E。

- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5 Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | `logic/pptx`（纯函数 + genppt 库调用） | **100%** |
| L3 | `adk/tools`（tool 分发） | **98%**（CI gate） |

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/logic/pptx/... ./internal/adk/tools/...` 通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考: `.agent/specs/spec-045-go-service-ut.md`、`.github/workflows/ut-workflow.yml`

## 10. 验证标准

1. `pptx_generator` 传 `format=html` + 含表格/图片/行内样式的 HTML → 产出合法 `.pptx`（文件可被 PowerPoint/WPS 打开）。
2. `pptx_generator` 传 `format=markdown`（或不传 format）→ 行为与现状一致（向后兼容）。
3. skill 描述（`skill_detail` / `skill_search`）能搜到「支持 HTML」「HTML 更精美」等关键词。
4. `predefinedSkills` 的 pptx_generator 描述已更新（原始 seed 数据同步）。
5. 线上 DB 的 pptx_generator 描述与代码 seed 一致（见 D3）。

## 11. 待定稿决策点（深化阶段拍板）

| # | 决策点 | 说明 |
|---|--------|------|
| D1 | format 参数方式 | 推荐 **显式 `format` 参数**（markdown/html，默认 markdown）；备选：自动 sniff（检测 `<h1>`/`<table>`/`<html>`）。显式更清晰、零误判、向后兼容 |
| D2 | HTML 默认配色 | 推荐沿用当前 markdown 深色系（`#1E3A5F` 标题 / `#333` 正文 / `#FFFFFF` 背景）保持视觉统一；是否让 LLM 通过 `<body style>` 自行覆盖背景 |
| D3 | seed 同步机制 | `SeedSkills` 幂等「已存在则跳过」不会更新线上旧描述。方案 A：实现时一次性手动同步线上 `pptx_generator` 描述；方案 B：将内置 skill 描述设为「随代码版本」、增强 seed 逻辑覆盖（会覆盖用户自定义，需权衡） |
