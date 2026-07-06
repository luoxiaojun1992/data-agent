# 企业级数据分析 Agent — MVP 开发计划

> **版本**: v2.1 | **日期**: 2026-07-06
>
> 本文档为纯项目执行跟踪，包含任务分解、工时估算和依赖关系。
> 产品需求见 PRD，技术方案见 RFC。

---

## 目录

1. [总体时间线](#1-总体时间线)
2. [Phase 1: 基础框架 (Week 1-2)](#2-phase-1-基础框架-week-1-2)
3. [Phase 2: 核心服务 (Week 3-4)](#3-phase-2-核心服务-week-3-4)
4. [Phase 3: 知识库与解读 (Week 5-6)](#4-phase-3-知识库与解读-week-5-6)
5. [Phase 4: 高级功能 (Week 7-8)](#5-phase-4-高级功能-week-7-8)
6. [Phase 5: 管理后台 (Week 9-10)](#6-phase-5-管理后台-week-9-10)
7. [Phase 6: 测试与优化 (Week 11-12)](#7-phase-6-测试与优化-week-11-12)
8. [团队分工与工时汇总](#8-团队分工与工时汇总)

---

## 1. 总体时间线

```
W1── W2 ── W3 ── W4 ── W5 ── W6 ── W7 ── W8 ── W9 ── W10 ─ W11 ─ W12
| Phase 1 || Phase 2 || Phase 3 || Phase 4 || Phase 5 || Phase 6  |
| 基础框架 || 核心服务 || 知识库   || 高级功能 || 管理后台 || 测试优化  |
├─────────┤├─────────┤├─────────┤├─────────┤├─────────┤├──────────┤
| M1      || M2      || M3      || M4      || M5      || M6       |
```

| 里程碑 | 时间 | 交付物 |
|--------|------|--------|
| **M1** | Week 2 | 基础设施就绪 |
| **M2** | Week 4 | Chat 模式可用（含消息美化渲染 + 增强提示词） |
| **M3** | Week 6 | Agent 模式可用 |
| **M4** | Week 8 | 知识库上线 + 飞书 IM 可用 |
| **M5** | Week 10 | 管理后台完整 |
| **M6** | Week 12 | Alpha Release |

![开发路线图](./diagrams/06_development_roadmap.png)

---

## 2. Phase 1: 基础框架 (Week 1-2)

### Week 1: 项目初始化与基础设施

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P1-01 | Go 项目初始化（Module、目录结构、Makefile） | 2h | - |
| P1-01a | E2E 框架就绪（Playwright 配置 + UI-000 占位用例，保证 CI Pipeline 不报错） | 2h | P1-01 |
| P1-02 | Docker Compose 开发环境（MongoDB/Milvus/SeaweedFS/Redis） | 4h | - |
| P1-03 | 配置管理系统（Viper/YAML + 环境变量覆盖） | 3h | P1-01 |
| P1-04 | 统一日志系统（slog 结构化日志） | 2h | P1-01 |
| P1-05 | MongoDB 连接层（Repository Pattern） | 4h | P1-02 |
| P1-06 | Redis 连接层（缓存 + Stream 操作） | 3h | P1-02 |
| P1-07 | SeaweedFS 连接层（Bucket CRUD + 文件操作） | 3h | P1-02 |
| P1-08 | Milvus 连接层（Collection 管理 + 向量搜索） | 4h | P1-02 |

### Week 2: 中间件与认证

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P1-09 | JWT 认证中间件（签发、验证、刷新） | 4h | P1-04 |
| P1-10 | RBAC 引擎（角色定义、权限校验） | 6h | P1-05, P1-09 |
| P1-11 | HTTP 路由注册（Gin Router + 分组） | 2h | - |
| P1-12 | 审计日志中间件 | 3h | P1-05, P1-09 |
| P1-13 | 安全过滤中间件框架 | 4h | P1-09 |
| P1-14 | 错误码体系统一（errcode + HTTP 映射） | 2h | - |
| P1-15 | CORS / Rate Limit 中间件 | 1h | - |
| P1-16 | 健康检查 API | 1h | P1-05 |

---

## 3. Phase 2: 核心服务 (Week 3-4)

### Week 3: Agent Engine + Skill Registry

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P2-01 | ADK Agent Engine 初始化与配置 | 4h | P1-16 |
| P2-02 | LLM Router 实现（多模型切换、参数配置） | 4h | P2-01 |
| P2-03 | Skill 接口定义与注册中心 | 4h | P2-01 |
| P2-04 | Skill 自动加载器（扫描 skills/ 目录） | 3h | P2-03 |
| P2-05 | SQL Executor Skill（MCP 对接数据库） | 8h | P2-03, P1-05 |
| P2-06 | Stats Engine Skill（回归 + 时间序列, gonum） | 10h | P2-03 |
| P2-07 | 分析结果落库模型与 Repository | 3h | P1-05 |
| P2-08 | Artifact 存储（SeaweedFS + save_artifact Skill） | 5h | P1-07, P2-03 |

### Week 4: Chat Service + Agent Service

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P2-09 | Session Manager（创建、续期、过期清理） | 6h | P1-07 |
| P2-10 | Chat Service（SSE 流式响应） | 6h | P2-01, P2-08 |
| P2-11 | 快捷提示词 CRUD + 按角色展示 | 3h | P1-05 |
| P2-11a | Prompt Enhancement Service（无状态，进度圈→LLM→填充输入框） | 3h | P2-01 |
| P2-11b | Vault Service（AES-256-GCM 加密 + 密钥轮转） | 4h | P1-01 |
| P2-11c | 上下文窗口管理（tiktoken-go token 计数 + LLM 摘要压缩 + KB 结果截断 + 长报告分段生成合并） | 3h | P2-01 |
| P2-12 | Agent Task Queue（Redis Stream） | 4h | P1-06 |
| P2-13 | Agent Worker Pool（并发消费任务） | 6h | P2-11 |
| P2-14 | 任务取消机制（Context Cancel） | 3h | P2-12 |
| P2-15 | 任务进度上报（WebSocket Push） | 4h | P2-12 |
| P2-16 | 任务完成通知（Email + In-app） | 3h | P2-14 |

---

## 4. Phase 3: 知识库与解读 (Week 5-6)

### Week 5: 知识库文档处理

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P3-01 | 文档解析引擎（PDF/Word/Excel/MD/TXT） | 6h | - |
| P3-02 | LLM 语义分片引擎（模型自行判断语义段落边界） | 4h | P3-01, P2-01 |
| P3-03 | LLM Embedding 生成（复用当前 LLM，不引入专用向量模型） | 3h | P3-02 |
| P3-04 | Milvus Collection 创建与写入（doc_id 绑定） | 4h | P3-03, P1-08 |
| P3-05 | 异步 Agent 索引任务（复用 Agent Task Queue） | 4h | P2-12 |
| P3-06 | MongoDB 全文搜索索引建立 | 2h | P1-05 |

### Week 6: 搜索 + 解读 + 聚合

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P3-07 | Milvus 向量相似度搜索 | 3h | P3-04 |
| P3-08 | 混合搜索（Semantic + Fulltext + 重排序） | 5h | P3-07, P3-06 |
| P3-09 | 权限过滤（搜索结果按用户权限裁剪） | 3h | P3-08, P1-10 |
| P3-10 | 分析结果智能解读（KB 检索 + 上下文注入） | 6h | P3-08 |
| P3-11 | 多维度聚合层数据定义与管理 | 5h | P1-05 |
| P3-12 | Email Sender Skill（域名白名单） | 3h | P2-03 |
| P3-13 | 知识库 Web API（上传/删除/搜索 + 索引状态 + doc_id 隔离） | 4h | P3-08 |

---

## 5. Phase 4: 高级功能 (Week 7-8)

### Week 7: 定时任务 + 子 Agent

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P4-01 | Scheduler Service（robfig/cron 集成） | 4h | P2-12 |
| P4-02 | 定时任务 CRUD + 暂停/恢复 | 3h | P4-01 |
| P4-03 | 定时任务执行 → 触发 Agent Service | 3h | P4-01 |
| P4-04 | 失败重试逻辑 + 超时终止 | 2h | P4-03 |
| P4-05 | Sub-Agent 编排基础（A2A 协议） | 6h | P2-01 |
| P4-06 | 子 Agent 会话生命周期管理 | 4h | P2-08, P4-05 |

### Week 8: 高级统计 + 安全 + OpenAPI + IM 集成

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P4-07 | 聚类分析 Skill（K-Means, gonum） | 6h | P2-06 |
| P4-08 | 主成分分析 Skill (PCA, gonum) | 4h | P2-06 |
| P4-09 | 财务分析 Skill（比率计算 + 趋势对比） | 6h | P2-06 |
| P4-10 | Security Audit Layer 完整实现 | 6h | P1-13 |
| P4-11 | 熔断器 Circuit Breaker | 3h | P4-10 |
| P4-12 | OpenAPI → MCP 转换器 | 8h | P2-03 |
| P4-13 | 日志类数据表 TTL 自动过期（MongoDB `expireAfterSeconds`，覆盖审计日志/会话记录/请求日志/通知记录） | 2h | P1-12 |
| P4-14 | 用户/Agent 审计日志完善（IP、Skill 调用） | 4h | P1-12 |
| P4-15 | 报告格式校验（Markdown AST 结构化校验：标题层级、章节存在性 + Agent 修正循环） | 6h | P1-05 |

### Week 8 附加: IM 集成 — 飞书机器人（仅限轻量办公 Chat 模式，MVP）

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P4-16 | 飞书开放平台应用创建与配置（AppID/Secret/事件订阅/权限） | 2h | - |
| P4-17 | IM 模块（internal/service/im/）— 集成在主二进制，Webhook + 签名验证 + go-lark SDK 消息收发（仅接入 Chat API，不接入 Agent/Hermes） | 3h | P4-16 |
| P4-18 | 用户绑定模块（飞书 open_id ↔ 系统 user_id，MongoDB `im_bindings` 集合） | 3h | P1-05, P4-17 |
| P4-20 | 消息路由（IM 消息 → Agent Service Chat API，复用现有 Chat 模式，内部调用） | 3h | P2-10, P4-17 |
| P4-21 | 分析结果卡片格式化（表格 + 关键指标 + 图表链接，飞书卡片 JSON 模板） | 4h | P4-17, P4-20 |
| P4-22 | 快捷指令支持（/分析 /查询 /周报 /帮助） | 2h | P4-20 |
| P4-23 | Agent 异步任务完成 → 飞书消息通知 | 2h | P2-16, P4-17 |
| P4-24 | IM 模块集成测试（主二进制内 Webhook 端到端验证） | 1h | P4-17 |

### Week 8 附加: 统计监控与 ROI

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P4-25 | Redis Stats 计数器（Scheduler 定时直接写入 Redis，记录日志不经过队列；指标：Agent调用次数/模型调用次数/Session次数/Task次数/Token消耗量；Redis 必须开启 AOF+RDB 持久化） | 4h | P4-01, P1-06 |
| P4-26 | Dashboard ROI 计算模块（投入产出比 = 等效节省人时 / AI 总成本，从 Redis Stats 聚合计算） | 2h | P4-25, P5-04 |

---

## 6. Phase 5: 管理后台 (Week 9-10)

### Week 9: 后台前端搭建 + 核心页面

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P5-01 | Next.js 项目初始化 + UI 框架 | 4h | - |
| P5-02 | Admin Layout + 菜单路由 | 3h | P5-01 |
| P5-03 | 登录页 + JWT Token 管理 | 3h | P5-01 |
| P5-04 | Dashboard 可视化看板 | 6h | P5-02 |
| P5-05 | 用户管理页面（CRUD + 角色分配 + 启停） | 5h | P5-02 |
| P5-06 | 权限管理页面（角色 CRUD + 权限映射） | 4h | P5-02 |

### Week 10: 功能页面完善

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P5-07 | 模型配置页面（LLM 选择 + 参数调整） | 3h | P5-02 |
| P5-08 | 任务管理页面（列表 + 进度 + 取消） | 4h | P5-02 |
| P5-09 | 知识库管理页（上传 + 列表 + 搜索 + 状态追踪） | 5h | P5-02 |
| P5-10 | 审计日志查看页（筛选 + 导出） | 4h | P5-02 |
| P5-11 | API 转换审核页面 | 3h | P5-02 |
| P5-12 | 消息美化渲染组件（工具调用卡片/SQL块/表格/图表内嵌） | 6h | P5-02 |
| P5-13 | 增强提示词输入框（AI Suggest 按钮 + 下拉建议面板） | 3h | P5-02 |
| P5-14 | 知识库索引状态展示（pending/indexing/done/failed） | 2h | P5-09 |
| P5-15 | 响应式适配（移动浏览器兼容） | 3h | P5-02 |
| P5-16 | 前后端联调 + Bug 修复 | 8h | All above |

### Week 10 附加: 自由探索模式 (Hermes)

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P5-17 | Hermes Service (Go) 开发 — 转发层 + SSE 透传 | 4h | P5-02 |
| P5-18 | Hermes 容器集成（Docker Compose + 健康检查） | 3h | P5-17 |
| P5-19 | 前端探索模式 UI（mode toggle + 输入转发 + 状态指示） | 3h | P5-02 |
| P5-20 | MongoDB `hermes_sessions` 集合 + 查询 API | 2h | P5-17 |

---

## 7. Phase 6: 测试与优化 (Week 11-12)

### Week 11: 测试

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P6-01 | 单元测试补充（目标覆盖率 > 70%） | 10h | All |
| P6-02 | 集成测试（Service 层 + 数据库交互） | 8h | P6-01 |
| P6-03 | E2E 测试（Playwright: 基于占位框架逐步添加登录→Chat→Agent→结果用例） | 8h | P6-02 |
| P6-04 | golangci-lint 全量检查 + 修复 | 4h | All |
| P6-05 | 依赖安全扫描（govulncheck） | 2h | All |

### Week 12: 优化 + 发布

| ID | 任务 | 工时 | 依赖 |
|----|------|:---:|------|
| P6-06 | 性能压测 + 瓶颈优化 | 8h | P6-03 |
| P6-07 | 安全渗透测试（OWASP Top 10） | 4h | P6-05 |
| P6-08 | Docker 镜像构建 + K8s 部署配置 | 4h | - |
| P6-09 | 内部 Alpha 发布 + 使用培训 | 4h | P6-08 |
| P6-10 | 文档整理（API 文档 + 部署手册 + 用户指南） | 6h | All |

---

## 8. 团队分工与工时汇总

### 团队配置

| 角色 | 人数 | 职责 |
|------|:---:|------|
| 后端工程师 | 2 | 核心服务开发 + AI 代码 Review |
| 全栈/前端 | 1 | 管理后台前端 + API 联调 |
| AI/架构师 | 1（兼职） | 架构决策 + Agent Prompt 设计 + 技术评审 |

### 按阶段分配

| Phase | 后端 | 前端 | 关键产出 |
|-------|:---:|:---:|---------|
| P1 (W1-2) | 2 | 1 (预研) | 基础设施 + 中间件 |
| P2 (W3-4) | 2 | 1 (原型) | Agent Engine + Chat/Agent |
| P3 (W5-6) | 2 | - | 知识库 + 解读 + 聚合 |
| P4 (W7-8) | 2 | 1 (IM联调) | 高级功能 + 安全 + 飞书IM |
| P5 (W9-10) | 1 | 1 (全力) | 管理后台 |
| P6 (W11-12) | 2 | 1 | 测试 + 调优 + 发布 |

### 工时汇总

| Phase | 预估工时 |
|-------|:-------:|
| Phase 1 | ~47h |
| Phase 2 | ~76h |
| Phase 3 | ~55h |
| Phase 4 | ~90h |
| Phase 5 | ~64h |
| Phase 6 | ~56h |
| **总计** | **~388h** |
