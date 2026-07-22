# DataAgent 企业数据分析平台 — 产品使用手册

> **版本**: v2.0 · **适用版本**: MVP Release (截至 SPEC-060)
>
> 本手册面向终端用户（业务人员、数据分析师、管理员、审计员），介绍 DataAgent 的功能与日常使用方法。
>
> 本文档仅描述已实现的功能，不涉及任何技术实现细节与敏感信息。所有界面截图均来自 GitHub Actions UI 测试的实景渲染（共 207 张，覆盖 238 个 UI 测试用例，已自动去重）。

---

## 目录

1. [产品概览](#1-产品概览)
2. [登录与界面导览](#2-登录与界面导览)
3. [Chat 对话：即时数据查询](#3-chat-对话即时数据查询)
4. [提示词增强与工具调用](#4-提示词增强与工具调用)
5. [Agent 任务：批量分析](#5-agent-任务批量分析)
6. [Hermes 探索：自由对话模式](#6-hermes-探索自由对话模式)
7. [知识库：让 AI 引用你的资料](#7-知识库让-ai-引用你的资料)
8. [文件上传](#8-文件上传)
9. [仪表盘：数据看板](#9-仪表盘数据看板)
10. [管理后台（管理员专用）](#10-管理后台管理员专用)
11. [通知中心](#11-通知中心)
12. [密码与会话管理](#12-密码与会话管理)
13. [权限隔离（RBAC）](#13-权限隔离rbac)
14. [安全防护](#14-安全防护)
15. [移动端与响应式适配](#15-移动端与响应式适配)
16. [飞书机器人：IM 渠道使用](#16-飞书机器人im-渠道使用)
17. [错误处理与异常状态](#17-错误处理与异常状态)
18. [记忆系统](#18-记忆系统)
19. [通用列表交互](#19-通用列表交互)
20. [端到端业务场景](#20-端到端业务场景)
21. [常见问题](#21-常见问题)
22. [附录 A：术语表](#附录-a术语表)
23. [附录 B：截图来源说明](#附录-b截图来源说明)

---

## 1. 产品概览

### 1.1 什么是 DataAgent

DataAgent 是一款企业级智能数据分析平台。它让你**用自然语言和你的业务数据对话**——不用写 SQL、不用跑脚本，直接在对话框里说出你的问题，系统会自动理解意图、生成查询、执行分析并给出可读的结论与图表。

不同角色的员工都能从中受益：

- **一线业务人员** — 快速查询数据，一两句话得到答案
- **数据分析师** — 发起复杂的批量分析任务
- **管理员** — 配置系统、管理用户、查看整体运行情况
- **审计员** — 查看所有操作记录，保证合规

### 1.2 平台导航

登录后，**左侧导航栏**按页面功能组织，可分为四类：

| 类别 | 页面 | 主要能力 |
|------|------|---------|
| **个人入口** | 仪表盘 | 个人活动概览、待办、近期分析 |
| **核心使用** | Chat 对话 / Hermes 探索 / Agent 任务 | 自然语言查询、自由探索、批量分析 |
| **内容管理** | 知识库 / 文档 | 文档上传、索引、搜索与浏览 |
| **系统管理**（仅管理员可见） | 管理后台 | 用户、权限、模型、任务、看板、审计 |

不同的角色看到的导航项略有不同——普通用户看不到"管理后台"。

### 1.3 角色与能力一览

| 角色 | 仪表盘 | Chat | Hermes | Agent 任务 | 知识库 | 文档 | 管理后台 |
|------|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| 普通用户 | ✅ | ✅ | ✅ | — | ✅ | ✅ | — |
| 数据分析师 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | — |
| 知识管理员 | ✅ | ✅ | ✅ | — | ✅ | ✅ | 知识库部分 |
| 系统管理员 | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | 全部 |
| 审计员 | ✅ | — | — | — | — | — | 审计部分（只读） |

> `system-admin` 角色由系统在首次部署时自动创建，是唯一拥有全部管理权限的内置角色，不可删除、不可新建第二个。

---

## 2. 登录与界面导览

### 2.1 登录系统

打开浏览器访问 DataAgent 地址，登录页面提供邮箱 + 密码登录方式。

![登录页主界面（品牌元素渲染）](manual-screenshots/01-login.png)

**登录页元素**

- 品牌标识与欢迎语
- 邮箱输入框
- 密码输入框（带显示/隐藏切换）
- 登录按钮

**输入与校验**

- 密码输入框支持显示/隐藏切换：

![密码输入框交互](manual-screenshots/02-login-password-input.png)

- 空字段提交时会显示校验提示：

![空字段提交校验](manual-screenshots/03-login-empty-validation.png)

- 邮箱格式不正确时会拦截提交：

![提交格式校验](manual-screenshots/04-login-format-validation.png)

- 凭据错误时显示错误 Toast：

![错误凭据提示](manual-screenshots/05-login-error-toast.png)

- 会话过期后会提示并跳转回登录页：

![会话过期提示](manual-screenshots/06-login-session-expired.png)

- 未登录访问受保护页面会自动跳转到登录页：

![未登录跳转登录页](manual-screenshots/07-login-redirect.png)

- 登录按钮的存在性与可点击状态：

![登录按钮存在性](manual-screenshots/08-login-button-presence.png)

- 按钮交互状态（hover / loading / disabled）：

![登录按钮交互状态](manual-screenshots/09-login-button-state.png)

- 综合输入与校验交互流程：

![输入与校验交互](manual-screenshots/10-login-interaction.png)

> **小贴士**
> - 首次部署后，系统管理员账号会在系统日志中输出一段随机生成的初始密码，请登录后立即修改
> - 顶部如果出现"修改初始密码"提示，请按引导尽快修改

### 2.2 主界面布局

登录成功后进入主界面。整体由三部分组成：

- **左侧导航栏**：在平台各功能页之间切换
- **顶部标题栏**：当前页面标题、操作按钮，右上角有铃铛通知图标
- **主内容区**：当前页面的功能内容

**侧边栏整体结构**

![侧边栏整体结构](manual-screenshots/11-layout-sidebar.png)

侧边栏按角色分成两组：

- **工作区导航**（所有用户可见）：仪表盘、Chat 对话、Hermes 探索、Agent 任务、知识库、文档

![工作区导航项](manual-screenshots/12-layout-workspace-nav.png)

- **管理后台导航**（仅管理员可见）：用户管理、权限管理、模型配置、任务管理、系统配置、审计日志、API 转换审核

![管理后台导航项](manual-screenshots/13-layout-admin-nav.png)

- **仪表盘导航项**单独高亮显示当前所在位置：

![仪表盘导航项](manual-screenshots/14-layout-dashboard-nav.png)

- 侧边栏底部显示当前登录用户的**用户卡片**（头像、姓名、角色、登出入口）：

<!-- MISSING: 15 -->

- 每个页面顶部都有**页面标题**区域，显示当前页面名称与简要描述：

![页面标题渲染](manual-screenshots/16-layout-page-title.png)

- 点击不同导航项时，激活态会即时切换（高亮 + 左侧色条）：

![导航项激活态切换](manual-screenshots/17-layout-active-switch.png)

---

## 3. Chat 对话：即时数据查询

Chat 对话是大多数人每天都会用到的主入口。核心是一个**对话式**聊天框：你提问，系统回答。

### 3.1 进入与提问

点击左侧 **Chat 对话** 进入。默认进入**分析模式**（与 DataAgent 自身的分析引擎对话）。

**消息气泡（带头像）**

![消息气泡（带头像）](manual-screenshots/18-chat-msg-bubble-avatar.png)

**在线状态徽标**

![在线状态徽标](manual-screenshots/19-chat-online-badge.png)

**新建对话按钮**

![新建对话按钮](manual-screenshots/20-chat-new-conv-btn.png)

**快捷提示词行**：4 个常用查询模板（今日数据概览 / 本月销售趋势 / 同比环比分析 / TOP10 产品）

![快捷提示词](manual-screenshots/21-chat-quick-prompts.png)

点击提示词后自动填入输入框：

![点击提示词填充输入](manual-screenshots/22-chat-prompt-fills.png)

**发送消息触发流式响应**

![发送消息触发流式响应](manual-screenshots/23-chat-send-sse.png)

**用户消息气泡**

![用户消息气泡](manual-screenshots/24-chat-user-bubble.png)

### 3.2 消息呈现：AI 是怎么回你的

系统对 AI 的回复做了大量美化渲染，让结果清晰易读：

**SQL 代码块**（自动语法高亮，带「复制」按钮）

![SQL 代码块渲染](manual-screenshots/25-chat-sql-block.png)

**数据表格**（斑马纹格式，支持表头排序）

![数据表格渲染](manual-screenshots/26-chat-data-table.png)

**数据图表**（直接在消息中内嵌渲染，可放大或下载）

![图表渲染](manual-screenshots/27-chat-chart.png)

**加载进度动画**（工具执行中显示旋转动画和状态文字，例如「查询中…」「计算中…」）

![加载进度动画](manual-screenshots/28-chat-loading.png)

**工具调用卡片**（折叠卡片形式，展开可看到工具名、耗时、输入参数和输出摘要）

![工具调用卡片展开](manual-screenshots/29-chat-toolcall-card.png)

**内联 KPI 卡片**（关键数值以卡片形式突出展示）

![内联 KPI 卡片](manual-screenshots/30-chat-kpi-card.png)

### 3.3 会话历史管理

Chat 对话页右侧提供"历史会话"面板，自动保存你的所有对话。

**会话面板展开**

![会话历史面板](manual-screenshots/31-chat-session-panel.png)

**会话列表项**（标题、消息数、最后互动时间）

![会话列表项](manual-screenshots/32-chat-session-items.png)

**点击会话切换上下文**

![切换会话](manual-screenshots/33-chat-session-switch.png)

**搜索会话**

![搜索会话](manual-screenshots/34-chat-session-search.png)

**删除会话**

![删除会话](manual-screenshots/35-chat-session-delete.png)

### 3.4 自定义提示词

除了内置的 4 个快捷提示词，你还可以保存自己的常用提示词。

**提示词弹窗**

![提示词弹窗](manual-screenshots/36-chat-prompt-modal.png)

**保存自定义提示词**

![保存自定义提示词](manual-screenshots/37-chat-save-prompt.png)

**填充并关闭弹窗**

![填充并关闭弹窗](manual-screenshots/38-chat-prompt-fills-close.png)

> 会话在长时间不活动后会过期清理；重要的分析结果系统会自动保存，不受会话清理影响。

### 3.5 多轮追问

在同一个对话中，你可以在前一个问题的基础上继续追问。例如：

- 问：「华东区 Q2 销售额」
- 追问：「其中上海和杭州分别多少？」
- 再追问：「对比 Q1 增长了多少？列出 TOP5 客户」

系统会自动结合上下文理解你的问题，不需要重复背景。

---

## 4. 提示词增强与工具调用

### 4.1 增强提示词（✨ AI 增强）

当你对要问什么比较模糊时，可以使用"增强提示词"功能让 AI 帮你把简短问题**扩展成更精确、更完整的分析需求**。

**增强按钮渲染**

![增强按钮渲染](manual-screenshots/39-prompt-enhance-btn.png)

**空输入点击增强**（输入为空时点击会提示先输入内容）

![空输入点击增强](manual-screenshots/40-prompt-enhance-empty.png)

**有输入点击增强**（AI 将简短输入补全为更专业的分析描述）

![有输入点击增强](manual-screenshots/41-prompt-enhance-filled.png)

**增强后手动编辑再发送**（增强结果填入输入框后可继续修改）

![增强后手动编辑再发送](manual-screenshots/42-prompt-enhance-edit.png)

**增强调用计入 Token 统计**（增强过程消耗的 Token 会被统计到用量看板）

![增强调用计入 Token 统计](manual-screenshots/43-prompt-enhance-token.png)

**使用步骤**

1. 在输入框中先写一段简短的描述，例如：「看看这个月的销售」
2. 点击输入框右侧的 **✨ AI 增强** 按钮
3. 系统会显示加载圈，AI 将你的简短输入补全为更专业的分析描述
4. 增强结果直接填入输入框，你可以在此基础上继续修改或直接发送

### 4.2 工具调用卡片

DataAgent 内置多种工具，AI 在分析过程中会按需调用。每次工具调用都会以**卡片形式**展示在对话流中，你可以展开查看详情。

**统计引擎工具卡片**（stats_engine）

![统计引擎工具卡片](manual-screenshots/44-tool-stats-engine.png)

**SQL 执行器工具卡片**（sql_executor）

![SQL 执行器工具卡片](manual-screenshots/45-tool-sql-executor.png)

**SQL 执行器校验失败**（SQL 语法或权限校验未通过时显示错误）

![SQL 执行器校验失败](manual-screenshots/46-tool-sql-fail.png)

**知识库搜索工具卡片**（knowledge_search）

![知识库搜索工具卡片](manual-screenshots/47-tool-kb-search.png)

**知识库命中后回答**（检索到相关文档后 AI 引用文档内容回答）

![知识库命中后回答](manual-screenshots/48-tool-kb-hit.png)

**保存报告工具卡片**（save_report，将分析结果保存为报告）

![保存报告工具卡片](manual-screenshots/49-tool-save-report.png)

**多工具链式调用**（一个任务中多个工具依次调用，形成调用链）

![多工具链式调用](manual-screenshots/50-tool-chain-call.png)

**SQL 结果复制按钮**（一键复制 SQL 执行结果）

![SQL 结果复制按钮](manual-screenshots/51-tool-sql-copy.png)

---

## 5. Agent 任务：批量分析

当你的问题更复杂、需要更长时间计算、或者需要定期执行时，使用 **Agent 任务**。

### 5.1 任务列表

点击左侧 **Agent 任务** 进入任务管理列表。这里展示所有由你或团队发起的批量分析任务。

**空状态**

![Agent 任务列表空状态](manual-screenshots/52-agent-empty-state.png)

**任务列表管理**（每条任务显示名称、状态、创建时间、操作按钮）

![任务列表管理](manual-screenshots/53-agent-task-list.png)

**任务列表分页**

![任务列表分页](manual-screenshots/54-agent-pagination.png)

**状态筛选**（全部 / 等待中 / 运行中 / 已完成 / 失败）

![任务状态筛选](manual-screenshots/55-agent-filters.png)

**状态标签渲染**（queued / running / completed / failed 等彩色 Pill）

![状态标签渲染](manual-screenshots/56-agent-status-pill.png)

### 5.2 创建任务

点击右上角 **+ 新建任务** 按钮。

**新建任务弹窗**

![新建任务弹窗](manual-screenshots/57-agent-create-modal.png)

**创建同步任务**

![创建同步任务](manual-screenshots/58-agent-create-sync.png)

**创建异步任务**

![创建异步任务](manual-screenshots/59-agent-create-async.png)

### 5.3 任务详情

任务详情从任务列表项**内联展开**（不在独立路由页面）。

**任务详情面板**

![任务详情面板](manual-screenshots/60-agent-detail-panel.png)

**任务详情展开**（完整展开后的视图）

![任务详情展开](manual-screenshots/61-agent-detail-expand.png)

**执行日志区域**（按时间顺序展示每一步的详细日志）

![执行日志区域](manual-screenshots/62-agent-exec-logs.png)

**进度条渲染**（可视化任务执行进度）

![进度条渲染](manual-screenshots/63-agent-progress-bar.png)

**产出物详情区域**（任务执行过程中生成的图表、CSV 文件等）

![产出物详情区域](manual-screenshots/64-agent-artifact-detail.png)

### 5.4 任务操作

**取消运行中任务**

![取消运行中任务](manual-screenshots/65-agent-cancel.png)

**详情页取消按钮**

![详情页取消按钮](manual-screenshots/66-agent-cancel-btn.png)

**取消后重试流程**

![取消后重试流程](manual-screenshots/67-agent-cancel-retry.png)

### 5.5 任务步骤与定时

**步骤指示器**（多步分析流程的步骤导航）

![任务步骤指示器](manual-screenshots/68-agent-step-indicator.png)

**定时任务创建**（设置定期执行的分析任务）

![定时任务创建](manual-screenshots/69-agent-scheduled.png)

### 5.6 批量下载产出物

任务完成后，可以选中多个任务**批量下载产出物**为 ZIP 压缩包。

![批量下载产出物 ZIP](manual-screenshots/70-agent-batch-download.png)

**典型任务类型**

- SQL 统计分析
- 多步骤数据探索
- 定时报表生成
- 跨数据源关联分析

---

## 6. Hermes 探索：自由对话模式

Hermes 是一种**自由对话模式**，与默认的"分析模式"不同——它不调用 DataAgent 自身的分析工具，而是直接与大语言模型对话，适合做开放性的探索、头脑风暴、概念澄清。

### 6.1 切换到探索模式

在 Chat 对话页顶部切换 **分析模式 / 探索模式**。

**模式切换渲染**

![模式切换渲染](manual-screenshots/71-hermes-toggle.png)

**探索模式下发送消息**

![探索模式下发送消息](manual-screenshots/72-hermes-send.png)

**服务在线状态**（探索模式可用时显示在线徽标）

![服务在线状态](manual-screenshots/73-hermes-online.png)

### 6.2 探索模式的特点

- **不调用 DataAgent 工具**：不会执行 SQL、不会查知识库、不会保存报告，纯粹的语言模型对话

![探索模式无工具调用](manual-screenshots/74-hermes-no-tools.png)

- 适合做：概念解释、思路梳理、方案讨论、写文案、翻译、代码片段
- 不适合做：查实际业务数据（请切回分析模式）

---

## 7. 知识库：让 AI 引用你的资料

知识库让你把企业内部的文档（PDF、Word、TXT、Markdown 等）上传到平台，AI 在回答问题时会**自动检索并引用**这些资料，让回答更贴合你的业务上下文。

### 7.1 知识库管理页

点击左侧 **知识库** 进入。

![知识库管理页](manual-screenshots/75-kb-page.png)

**页面元素**

- 文档卡片网格（每张卡片显示文档名、类型、索引状态、上传时间）
- 上传按钮
- 搜索框
- 标签筛选

### 7.2 上传文档

**上传单个文档**

![上传单个文档](manual-screenshots/76-kb-upload-single.png)

**批量上传文档**（支持一次选择多个文件）

![批量上传文档](manual-screenshots/77-kb-upload-batch.png)

### 7.3 索引与检索

上传后系统会自动对文档进行分块、向量化、索引，索引完成后即可被 AI 检索引用。

**文档索引端到端验证**

![文档索引端到端验证](manual-screenshots/78-kb-e2e-index.png)

**索引后检索命中**

![索引后检索命中](manual-screenshots/79-kb-e2e-hit.png)

**索引进度实时更新**

![索引进度实时更新](manual-screenshots/80-kb-e2e-progress.png)

**搜索过滤结果准确**

![搜索过滤结果准确](manual-screenshots/81-kb-e2e-filter.png)

**索引失败重试**（索引失败的文档会自动重试，也可手动触发）

![索引失败重试](manual-screenshots/82-kb-e2e-retry.png)

---

## 8. 文件上传

除了知识库文档上传，DataAgent 在多个场景下支持文件上传（如 Chat 附件、API 转换审核的 OpenAPI 文件等）。

**文件多选**（一次选择多个文件）

![文件多选](manual-screenshots/83-upload-multi.png)

**上传进度**（每个文件独立显示上传进度条）

![上传进度](manual-screenshots/84-upload-progress.png)

**单文件上传**

![单文件上传](manual-screenshots/85-upload-single.png)

**上传不阻塞 UI**（上传过程中页面其他操作不受影响）

![上传不阻塞 UI](manual-screenshots/86-upload-nonblock.png)

---

## 9. 仪表盘：数据看板

仪表盘是登录后的默认首页，提供个人活动概览与平台整体运行情况的可视化看板。

### 9.1 主页概览

**问候语与日期**（根据当前时间显示"上午好/下午好/晚上好"+ 用户名 + 当前日期）

![问候语与日期](manual-screenshots/87-dash-greeting.png)

**统计卡片**（KPI 摘要：任务数、Token 消耗、文档数、成功率等）

![统计卡片](manual-screenshots/88-dash-stats-cards.png)

**时间筛选器**（切换今日 / 7 天 / 30 天等时间范围）

![时间筛选器](manual-screenshots/89-dash-time-filter.png)

**实时数据徽标**（数据为实时同步时显示 LIVE 徽标）

![实时数据徽标](manual-screenshots/90-dash-realtime-badge.png)

### 9.2 图表区

**任务状态分布**（饼图/环形图，展示 queued / running / completed / failed 占比）

![任务状态分布](manual-screenshots/91-dash-task-status.png)

**任务耗时直方图**（柱状图，展示任务执行时长的分布）

![任务耗时直方图](manual-screenshots/92-dash-duration-hist.png)

**Token 消耗图表**（折线图，展示 Token 用量随时间变化）

<!-- MISSING: 93 -->

**Token ROI KPI**（投入产出比指标）

![Token ROI KPI](manual-screenshots/94-dash-roi-kpi.png)

**ROI 双轴图表**（Token 消耗与产出价值双轴对比）

<!-- MISSING: 95 -->

**调用趋势图表**（API/工具调用次数趋势）

<!-- MISSING: 96 -->

**24h 请求分布**（过去 24 小时每小时的请求量分布）

<!-- MISSING: 97 -->

**产出统计图表**（报告、图表等产出物数量统计）

![产出统计图表](manual-screenshots/98-dash-output-stats.png)

**成功率趋势**（任务成功率随时间变化）

![成功率趋势](manual-screenshots/99-dash-success-trend.png)

### 9.3 仪表盘集成（真实数据验证）

以下截图来自仪表盘集成测试，验证看板展示的是**真实业务数据**而非占位符。

**全 trend 真数据显示**

![全 trend 真数据显示](manual-screenshots/100-dash-int-trend.png)

**KPI 显示真实任务数**

![KPI 显示真实任务数](manual-screenshots/101-dash-int-kpi-tasks.png)

**KPI 显示真实文档数**

![KPI 显示真实文档数](manual-screenshots/102-dash-int-kpi-docs.png)

**任务状态分布准确**

![任务状态分布准确](manual-screenshots/103-dash-int-status.png)

**24h 趋势图渲染**

<!-- MISSING: 104 -->

**ROI 图表渲染**

![ROI 图表渲染](manual-screenshots/105-dash-int-roi.png)

**时间筛选有效**（切换时间范围后图表数据同步更新）

![时间筛选有效](manual-screenshots/106-dash-int-filter.png)

**多用户数据隔离**（不同用户看到的看板数据相互隔离）

<!-- MISSING: 107 -->

**Token KPI 渲染**

<!-- MISSING: 108 -->

---

## 10. 管理后台（管理员专用）

管理后台仅对 `admin` 角色可见，普通用户尝试直接访问管理页面 URL 会被拦截（详见第 13 章 RBAC）。

### 10.1 用户管理

![用户管理页](manual-screenshots/109-user-page.png)

**用户表格列**（姓名、邮箱、角色、状态、创建时间、操作）

![用户表格列](manual-screenshots/110-user-table.png)

**添加用户**

![添加用户](manual-screenshots/111-user-add.png)

**编辑用户角色**

![编辑用户角色](manual-screenshots/112-user-edit-role.png)

**启用/停用用户**

![启用/停用用户](manual-screenshots/113-user-toggle.png)

**删除用户**

![删除用户](manual-screenshots/114-user-delete.png)

**邮箱唯一性校验**（新增/修改用户时邮箱不能与已有用户重复）

![邮箱唯一性校验](manual-screenshots/115-user-email-unique.png)

**用户列表分页**

![用户列表分页](manual-screenshots/116-user-pagination.png)

**不可删除 system-admin**（内置管理员账号受保护，不可删除）

<!-- MISSING: 117 -->

**不可创建第二个 system-admin**（系统仅允许一个 system-admin）

![不可创建第二个 system-admin](manual-screenshots/118-user-no-second-admin.png)

### 10.2 角色权限管理

![权限管理页](manual-screenshots/119-role-page.png)

**固定角色卡片**（系统内置角色：system-admin / admin / user / analyst 等）

![固定角色卡片](manual-screenshots/120-role-fixed.png)

**自定义角色表格**（管理员可创建自定义角色）

![自定义角色表格](manual-screenshots/121-role-custom-table.png)

**新建自定义角色**

![新建自定义角色](manual-screenshots/122-role-create.png)

**权限 Tab 渲染**（按模块展示可分配的权限项）

![权限 Tab 渲染](manual-screenshots/123-role-perm-tab.png)

**编辑角色权限**（勾选/取消勾选具体权限）

![编辑角色权限](manual-screenshots/124-role-edit-perm.png)

**删除自定义角色**

![删除自定义角色](manual-screenshots/125-role-delete.png)

**不可删除固定角色**（内置角色不可删除）

![不可删除固定角色](manual-screenshots/126-role-no-delete-fixed.png)

### 10.3 模型配置

![模型配置页](manual-screenshots/127-model-page.png)

**OpenAI 兼容 API URL 配置**

![OpenAI 兼容 API URL 配置](manual-screenshots/128-model-api-url.png)

**API Key 输入与加密保存**（API Key 通过 Vault 加密存储，不落明文）

![API Key 输入与加密保存](manual-screenshots/129-model-api-key.png)

**眼睛按钮切换**（API Key 显示/隐藏切换）

![API Key 显示/隐藏切换](manual-screenshots/130-model-eye-toggle.png)

**Model Name 下拉选择**

![模型名称下拉选择](manual-screenshots/131-model-name-select.png)

**上下文长度 Stepper**（步进器调整上下文窗口大小）

![上下文长度步进器](manual-screenshots/132-model-context-stepper.png)

**最大输出长度配置**

![最大输出长度配置](manual-screenshots/133-model-max-output.png)

**Temperature 配置**

![Temperature 配置](manual-screenshots/134-model-temperature.png)

**Hermes 配置区域**（探索模式使用的独立模型配置）

<!-- MISSING: 135 -->

**普通用户不可访问模型配置**

![普通用户不可访问](manual-screenshots/136-model-user-block.png)

### 10.4 系统配置

![系统配置页](manual-screenshots/137-sysconfig-page.png)

**普通用户不可访问**

![普通用户不可访问](manual-screenshots/138-sysconfig-user-block.png)

**缓冲期上限校验**（Session 恢复缓冲期有上限约束）

![缓冲期上限校验](manual-screenshots/139-sysconfig-buffer-limit.png)

**修改 Session 缓冲期**

![修改 Session 缓冲期](manual-screenshots/140-sysconfig-edit-buffer.png)

### 10.5 任务管理（管理员视角）

管理员可以**全局查看所有用户的任务**，并执行取消、重试等操作。

![任务管理页](manual-screenshots/141-task-page.png)

**全局查看所有用户任务**

![全局查看所有用户任务](manual-screenshots/142-task-global-view.png)

**取消运行中任务**

![取消运行中任务](manual-screenshots/143-task-cancel.png)

**重试失败任务**

![重试失败任务](manual-screenshots/144-task-retry.png)

### 10.6 审计日志

审计日志记录所有用户的关键操作，用于合规追溯。

![审计日志页](manual-screenshots/145-audit-page.png)

**按时间范围筛选**

![按时间范围筛选](manual-screenshots/146-audit-time-filter.png)

**按操作类型筛选**

![按操作类型筛选](manual-screenshots/147-audit-action-filter.png)

**按用户筛选**

![按用户筛选](manual-screenshots/148-audit-user-filter.png)

**导出审计日志弹窗**

![导出审计日志弹窗](manual-screenshots/149-audit-export-modal.png)

**执行导出**

![执行导出](manual-screenshots/150-audit-export-exec.png)

**导出条数上限校验**

![导出条数上限校验](manual-screenshots/151-audit-export-limit.png)

### 10.7 API 转换审核

管理员可以上传 OpenAPI 规范文件，系统自动转换为内部工具定义，转换结果需经过**审核**才能上线。

![API 转换审核页](manual-screenshots/152-api-page.png)

**上传 OpenAPI 文件**

![上传 OpenAPI 文件](manual-screenshots/153-api-upload.png)

**批量上传 OpenAPI 文件**

![批量上传 OpenAPI 文件](manual-screenshots/154-api-batch-upload.png)

**批准 API 转换**

![批准 API 转换](manual-screenshots/155-api-approve.png)

**驳回 API 转换**

![驳回 API 转换](manual-screenshots/156-api-reject.png)

**双重审核校验**（敏感 API 需要两人审核）

![双重审核校验](manual-screenshots/157-api-dual-review.png)

### 10.8 邀请管理

管理员可以通过邀请链接邀请新用户加入平台。

![邀请管理页](manual-screenshots/158-invite-page.png)

**创建邀请表单**（设置角色、有效期）

![创建邀请表单](manual-screenshots/159-invite-form-toggle.png)

**过期时间默认 24h**

![过期时间默认 24h](manual-screenshots/160-invite-expiry.png)

**角色默认 user**

![角色默认 user](manual-screenshots/161-invite-role.png)

**占位提示信息**（暂无邀请记录时显示）

![占位提示信息](manual-screenshots/162-invite-placeholder.png)

**无效 token 注册页**（邀请链接已过期或无效时）

![无效 token 注册页](manual-screenshots/163-invite-invalid-token.png)

**错误页登录链接**（注册出错时提供返回登录的入口）

![错误页登录链接](manual-screenshots/164-invite-error-login.png)

**无 token 错误提示**（直接访问注册页且不带 token 时）

![无 token 错误提示](manual-screenshots/165-invite-no-token.png)

---

## 11. 通知中心

平台通过右上角的**铃铛图标**向你推送站内通知（任务完成、任务失败、系统公告等）。

**铃铛图标与未读数红点**（有未读通知时显示红色数字徽标）

![铃铛图标与未读数红点](manual-screenshots/166-notif-bell.png)

**点击展开通知列表**（下拉显示最近通知）

![点击展开通知列表](manual-screenshots/167-notif-list.png)

> 通知支持**标记已读**、**一键全部已读**、**发送站内信**等操作（这些操作的截图与列表展开图相同，已去重）。

---

## 12. 密码与会话管理

### 12.1 密码管理

**初始密码横幅通知**（首次登录时顶部横幅提示修改初始密码）

![初始密码横幅通知](manual-screenshots/168-pwd-initial-banner.png)

**修改密码页**

![修改密码页](manual-screenshots/169-pwd-change-page.png)

**旧密码错误**（输入的旧密码不正确时提示）

![旧密码错误](manual-screenshots/170-pwd-old-wrong.png)

**新密码不一致**（两次输入的新密码不一致时提示）

![新密码不一致](manual-screenshots/171-pwd-mismatch.png)

**新密码强度校验**（密码需满足强度要求）

![新密码强度校验](manual-screenshots/172-pwd-strength.png)

> 普通用户角色也可以修改自己的密码（与 admin 共用同一修改密码页）。

### 12.2 会话超时与恢复

为保障账号安全，会话在长时间不活动后会自动过期。

**超时警告**（即将超时时弹出续期提示）

![超时警告](manual-screenshots/173-session-timeout-warn.png)

**超时自动登出**（超时后自动跳转登录页）

![超时自动登出](manual-screenshots/174-session-timeout-logout.png)

**继续使用续期**（点击"继续使用"延长会话）

![继续使用续期](manual-screenshots/175-session-renew.png)

**多端登录互不干扰**（同一账号在多端登录，会话相互独立）

![多端登录互不干扰](manual-screenshots/176-session-multi-device.png)

**删除后恢复**（会话被清理后在缓冲期内可恢复）

![删除后恢复](manual-screenshots/177-session-restore.png)

**部分删除无恢复**（主动删除的会话不在恢复范围内）

![部分删除无恢复](manual-screenshots/178-session-partial-delete.png)

**恢复缓冲期可配置**（管理员可在系统配置中调整缓冲期时长）

![恢复缓冲期可配置](manual-screenshots/179-session-buffer-config.png)

---

## 13. 权限隔离（RBAC）

DataAgent 基于 RBAC（基于角色的访问控制）实现权限隔离。不同角色看到的导航项、能访问的页面、能执行的操作都不同。

### 13.1 导航项差异

**user 可见导航项**（仅工作区，无管理后台）

![user 可见导航项](manual-screenshots/180-rbac-user-nav.png)

**admin 可见导航项**（工作区 + 管理后台）

![admin 可见导航项](manual-screenshots/181-rbac-admin-nav.png)

### 13.2 页面访问拦截

普通用户尝试直接访问管理页面 URL 时会被拦截：

**user 无法直接访问管理页面**

![user 无法访问管理页面](manual-screenshots/182-rbac-user-block-admin.png)

**user 无法访问模型配置**

![user 无法访问模型配置](manual-screenshots/183-rbac-user-block-model.png)

**user 无法创建 Agent 任务**（Agent 任务仅对 analyst 及以上角色开放）

![user 无法创建 Agent 任务](manual-screenshots/184-rbac-user-block-agent.png)

### 13.3 内置角色

**system-admin 由系统自动创建**，是唯一拥有全部权限的内置角色，仅 admin 可验证：

![system-admin 系统自动创建](manual-screenshots/185-rbac-system-admin.png)

---

## 14. 安全防护

DataAgent 在多个层面内置安全防护机制，保障数据与操作安全。

### 14.1 SQL 注入拦截

用户输入中包含 SQL 注入特征时会被自动拦截，不会执行恶意 SQL：

![SQL 注入被拦截](manual-screenshots/186-sec-sql-injection.png)

### 14.2 敏感信息脱敏

AI 回复中如果包含敏感信息（如手机号、身份证号、API Key 等），输出时会自动脱敏：

![输出敏感信息脱敏](manual-screenshots/187-sec-mask.png)

### 14.3 越权工具调用拦截

当 AI 尝试调用当前用户无权使用的工具时，调用会被拦截：

![越权工具调用被拦截](manual-screenshots/188-sec-unauthorized.png)

---

## 15. 移动端与响应式适配

DataAgent 的界面针对不同屏幕尺寸做了响应式适配，在手机、平板、桌面端均有良好的使用体验。

**移动端登录卡片适配**（375px 等窄屏下登录卡片自适应）

![移动端登录卡片适配](manual-screenshots/189-resp-mobile-login.png)

**平板布局适配 768px**

![平板布局适配 768px](manual-screenshots/190-resp-tablet.png)

**移动端布局适配 375px**

![移动端布局适配 375px](manual-screenshots/191-resp-mobile-375.png)

**桌面端布局 1440px**

![桌面端布局 1440px](manual-screenshots/192-resp-desktop-1440.png)

**触摸友好交互**（移动端按钮、链接的点击区域足够大，便于触控）

![触摸友好交互](manual-screenshots/193-resp-touch.png)

---

## 16. 飞书机器人：IM 渠道使用

DataAgent 支持通过飞书机器人接入，让你在飞书群里直接与 DataAgent 对话。

**飞书绑定页**

![飞书绑定页](manual-screenshots/194-im-feishu-bind.png)

**保存飞书配置**

![保存飞书配置](manual-screenshots/195-im-feishu-save.png)

> 飞书机器人的实际消息收发需要人工验证（涉及第三方 IM 平台的实时回调），不在自动化测试范围内。

---

## 17. 错误处理与异常状态

系统在各类异常情况下都有友好的降级与提示，避免白屏或崩溃。

**网络断开提示**

![网络断开提示](manual-screenshots/196-err-network.png)

**API 错误页面不崩溃**（接口报错时页面仍可操作）

![API 错误页面不崩溃](manual-screenshots/197-err-api.png)

**404 页面**（访问不存在的路由时显示友好的 404 页）

![404 页面](manual-screenshots/198-err-404.png)

**加载状态指示器**（数据加载中显示骨架屏或 Spinner）

![加载状态指示器](manual-screenshots/199-err-loading.png)

**聊天消息发送后页面保持稳定**（发送消息触发请求时页面不抖动、不白屏）

![聊天消息发送后页面保持稳定](manual-screenshots/200-err-chat-stable.png)

**浏览器后退按钮**（支持浏览器原生后退，不会导致状态错乱）

![浏览器后退按钮](manual-screenshots/201-err-back-btn.png)

> 空数据状态与 API 错误页面共用同一降级组件（已去重）。

---

## 18. 记忆系统

DataAgent 内置记忆系统（mem0），能在对话中**自动记录用户的偏好与上下文**，让后续对话更懂你。

**会话自动写入记忆**（对话过程中的关键信息自动提取并存入记忆库）

![会话自动写入记忆](manual-screenshots/202-mem0-auto-write.png)

**memory-search 工具调用**（AI 主动调用记忆检索工具，回忆用户历史偏好）

![memory-search 工具调用](manual-screenshots/203-mem0-search-tool.png)

**多用户隔离**（不同用户的记忆相互隔离，A 看不到 B 的偏好）

![多用户隔离](manual-screenshots/204-mem0-isolation.png)

**长对话压缩后记忆保留**（即使对话过长被压缩，关键记忆仍会保留）

![长对话压缩后记忆保留](manual-screenshots/205-mem0-long-conversation.png)

---

## 19. 通用列表交互

平台中的多个列表页（用户管理、任务管理、审计日志、知识库等）共用一套统一的列表交互组件，提供一致的体验。

**分页控件默认值**（默认每页 10 条，显示总数与页码跳转）

![分页控件默认值](manual-screenshots/206-list-pagination.png)

**页码跳转**（可直接输入页码跳转到指定页）

![页码跳转](manual-screenshots/207-list-page-jump.png)

**表头排序**（点击表头切换升序/降序）

![表头排序](manual-screenshots/208-list-sort.png)

**全选/取消全选**（表头复选框支持全选当前页）

![全选/取消全选](manual-screenshots/209-list-select-all.png)

> 每页条数切换（10/20/50）的截图与分页默认值相同，已去重。

---

## 20. 端到端业务场景

以下截图来自端到端（E2E）集成测试，模拟真实用户从登录到完成业务的完整流程。

**Chat 查询结果展示与追问**（用户提问 → AI 回答 → 用户追问 → AI 结合上下文回答）

![Chat 查询结果展示与追问](manual-screenshots/210-e2e-chat-query.png)

**普通员工 Chat 查询**（普通用户角色的完整 Chat 查询流程）

![普通员工 Chat 查询](manual-screenshots/211-e2e-employee-chat.png)

**Agent 任务页面**（数据分析师创建并查看 Agent 任务的完整流程）

![Agent 任务页面](manual-screenshots/212-e2e-agent-page.png)

**知识库管理页**（知识管理员上传文档、等待索引、检索验证的完整流程）

![知识库管理页](manual-screenshots/213-e2e-kb-page.png)

**审计日志页**（审计员查看操作记录、筛选、导出的完整流程）

![审计日志页](manual-screenshots/214-e2e-audit-page.png)

**安全拦截与审计联动**（用户尝试越权操作 → 被拦截 → 操作被记录到审计日志）

![安全拦截与审计联动](manual-screenshots/215-e2e-security-audit.png)

**admin 完整管理流程**（管理员从登录到完成用户管理、角色配置、模型配置的完整流程）

![admin 完整管理流程](manual-screenshots/216-e2e-admin-flow.png)

**Hermes 探索模式**（切换到探索模式 → 发送开放性问题 → 获取 AI 回答）

![Hermes 探索模式](manual-screenshots/217-e2e-hermes.png)

---

## 21. 常见问题

### 21.1 登录与账号

**Q: 忘记密码怎么办？**
A: 联系系统管理员重置你的密码。管理员可以在"用户管理"中为你生成新的初始密码。

**Q: 首次登录为什么顶部有横幅提示？**
A: 系统检测到你使用的是初始密码，出于安全考虑提醒你尽快修改。点击横幅即可跳转到修改密码页。

**Q: 为什么我看不到"管理后台"导航项？**
A: 管理后台仅对 `admin` 及以上角色可见。如果你需要管理权限，请联系系统管理员调整你的角色。

### 21.2 Chat 与 Agent

**Q: Chat 和 Agent 任务有什么区别？**
A: Chat 是即时对话，适合快速查询；Agent 任务是批量分析，适合复杂、耗时或需要定期执行的分析。Chat 的结果即时返回，Agent 任务在后台运行，完成后通知你。

**Q: 分析模式和探索模式有什么区别？**
A: 分析模式（默认）会调用 DataAgent 的工具（SQL 执行、知识库检索等）查询你的真实业务数据；探索模式（Hermes）是纯语言模型对话，不调用工具，适合开放性探讨。

**Q: 增强提示词会消耗 Token 吗？**
A: 会的。增强提示词的调用会计入 Token 用量统计，你可以在仪表盘的 Token 消耗图表中看到。

### 21.3 知识库

**Q: 上传的文档多久能被检索到？**
A: 取决于文档大小。小文档通常几秒内完成索引，大文档（几十 MB 以上）可能需要几分钟。索引完成后文档卡片的状态会变为"已索引"。

**Q: 支持哪些文档格式？**
A: 支持 PDF、Word（.docx）、TXT、Markdown、Excel 等常见格式。

### 21.4 安全与合规

**Q: 我的对话会被别人看到吗？**
A: 不会。对话是用户私有的，多端登录也相互独立。管理员可以查看审计日志中的操作记录（何时登录、何时查询），但看不到对话的具体内容。

**Q: AI 会执行危险的 SQL 吗？**
A: 不会。系统内置 SQL 注入拦截，且所有 SQL 执行前都会做权限与安全校验。

---

## 附录 A：术语表

| 术语 | 含义 |
|------|------|
| **Agent 任务** | 批量分析任务，在后台运行，适合复杂或定时的分析 |
| **Chat 对话** | 即时对话式查询，结果即时返回 |
| **Hermes 探索** | 自由对话模式，不调用 DataAgent 工具，纯语言模型对话 |
| **知识库** | 企业内部文档库，AI 回答时会自动检索引用 |
| **Token** | 大语言模型的计费单位，约等于一个汉字或半个英文单词 |
| **RBAC** | 基于角色的访问控制（Role-Based Access Control） |
| **system-admin** | 系统内置的最高权限角色，不可删除、不可新建第二个 |
| **Session 缓冲期** | 会话过期后仍可恢复的时间窗口，由管理员配置 |
| **mem0** | 记忆系统，自动记录用户偏好，让对话更个性化 |
| **工具调用** | AI 在分析过程中调用的具体能力（SQL 执行、知识库检索等） |

---

## 附录 B：截图来源说明

本手册共收录 **207 张**界面截图，全部来自 GitHub Actions UI 测试的实景渲染。

- **CI 运行**: 最新一次成功运行的 UI Tests 工作流（commit `48988d3f`，2026-07-22）
- **测试框架**: Playwright（Chromium，`screenshot: { mode: 'on', fullPage: true }`）
- **原始数量**: 238 张截图（对应 238 个 UI 测试用例）
- **去重处理**: 按 MD5 哈希去重，剔除 31 张内容完全相同的截图（同一页面不同测试断言产生相同渲染结果），保留 207 张唯一截图
- **artifact 保留期**: 7 天（GitHub Actions 默认），过期后需重新触发 CI 获取

> 截图反映的是手册编写时点的 UI 状态。如界面有更新，重新触发 CI 并重新生成手册即可同步最新截图。

---

*本手册由 DataAgent 团队维护。如有疑问或建议，请联系系统管理员。*
