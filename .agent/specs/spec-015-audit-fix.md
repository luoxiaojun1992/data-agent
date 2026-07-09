# 代码审核修复 — main 分支与 Spec/PRD/RFC/设计稿一致性修复

> **SPEC-015** | Status: 已实现 | 依赖: SPEC-001 ~ SPEC-014（全部已实现）

## 目标

基于 main 分支代码实现与 Spec/PRD/RFC/UI 原型设计文档的一致性审计结果，修复所有 P0 阻塞问题、P1 重大缺口和 P2 重要遗漏，使代码实现完全遵照设计文档。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-001 ~ SPEC-014 | ✅ | 全部已设计并标记「已实现」 |
| PRD-企业数据分析Agent-MVP.md | ✅ | v1.0, 25 个功能模块 |
| RFC-企业数据分析Agent-技术方案.md | ✅ | v1.1, 19 个章节 |
| UI原型设计文档.md | ✅ | v1.1, 12 页面 + 设计系统 |
| prototype.html | ✅ | 交互原型，Glass Vision OS Aurora 主题 |

---

## 审计结果总览

| 级别 | 数量 | 说明 |
|:----:|:----:|------|
| **P0 阻塞** | 3 | 前端缺失、Skill Registry 未集成、Login 占位 |
| **P1 重大** | 5 | Hermes 位置错误、AgentService 内存任务、Scheduler 缺失、飞书 Mock、Skill 缺失 |
| **P2 重要** | 9 | tiktoken、Vault、Stats、Session 持久化、WebSocket、Model Config、Milvus、Admin API、Security 集成 |
| **P3 优化** | 6 | 子Agent编排、ZIP下载、批量上传、Email Skill、OpenAPI-MCP、通知服务 |

---

## 详细问题清单

### P0 — 阻塞级（必须立即修复）

#### P0-1: 前端代码完全缺失

**现状**: `frontend/` 目录为空。UI原型设计文档定义了 12 个页面（登录、Chat工作区、Agent任务列表、任务详情、Dashboard、用户管理、角色管理、模型配置、任务管理、知识库、审计日志、API转换审核），prototype.html 提供了完整交互原型。RFC 第 4 章定义了 Next.js 14 + React 18 前端架构。

**Spec 要求**:
- SPEC-013 第 1 节: "Next.js 14 项目初始化，Admin Layout + 菜单路由（侧边栏 7 项导航）"
- SPEC-013 第 2 节: 12 个管理后台页面
- UI原型设计文档第 3 节: 每页的布局结构、元素清单、PRD 功能映射
- PRD F-04: Chat 模式 SSE 流式、消息美化、会话历史侧边栏、快捷提示
- PRD F-05: Agent 模式批量任务界面

**影响**: 用户无法使用任何系统功能。所有 E2E 测试用例无法编写（当前仅 UI-000 占位）。

#### P0-2: Skill Registry 未与 Skill 实现集成

**现状**: `main.go:121` 使用 `agent.NewSkillRegistryAdapter()`（engine.go:282-297），其 `Get()` 方法返回 `"skill %q not found (skills will be implemented in SPEC-008)"`。4 个 Skill（sql_executor, stats_engine, knowledge_search, save_report）已实现但从未注册到 Registry。

**Spec 要求**:
- SPEC-004 第 2 节: "Skill 自动加载器：扫描 `skills/` 目录，按子目录名注册"
- SPEC-008: "所有 Skill 包装层...必须通过 Registry.Register() 注册"

**影响**: Agent Engine 无法调用任何 Skill，所有数据分析能力不可用。

#### P0-3: Login/Auth 端点全为占位实现

**现状**: `main.go:220-223` login handler 返回 `{"message": "login endpoint placeholder"}`，用户管理端点也是占位。无用户注册、无登录验证、无 Token 签发流程。

**Spec 要求**:
- SPEC-003 第 6.4 节: "JWT 签发、验证、刷新"
- SPEC-003 第 6.5 节: "RBAC 引擎，角色定义（MongoDB roles 集合），权限校验中间件"
- PRD F-11: "登录页（邮箱+密码 + SSO），用户管理 CRUD + 角色分配 + 启停"

**影响**: 系统完全无法认证，任何需要 JWT 的 API 都不可访问。

---

### P1 — 重大缺口

#### P1-1: Hermes 服务位置错误

**现状**: Hermes 在主二进制中作为 Proxy route（`main.go:211`），非独立服务。

**Spec 要求（SPEC-012 第 1 节）**:
- "Go 轻量转发层（`cmd/hermes/main.go`）"
- "与 Agent Service 无耦合，独立部署"
- "Dev Docker（可本地启动 Hermes 容器）"

**修复方向**: 创建 `cmd/hermes/main.go` 独立入口，从主二进制移除 Hermes route，添加 Docker Compose Hermes 容器。

#### P1-2: AgentService 使用内存 Map 而非 Redis Stream

**现状**: `agent_service.go:43` `var tasks = make(map[string]*AgentTask)` 使用内存存储异步任务，`go func()` 直接启动 goroutine 而非入队。

**Spec 要求（SPEC-004 第 4 节 + SPEC-009 第 1 节）**:
- "Agent 异步模式：创建 AgentTask → 入队（见 SPEC-009）→ 立即返回 task_id"
- "使用 Redis Stream，Consumer Group 模式保证 at-least-once 消费"

**修复方向**: AgentService.CreateAgentTask 应调用 TaskService.CreateTask（已有实现），将任务入队 Redis Stream。

#### P1-3: Scheduler 调度器未实现

**现状**: 零代码。ScheduledTask 模型定义存在（`task/model.go:77-90`）但无调度逻辑。

**Spec 要求（SPEC-009 第 5 节）**:
- "robfig/cron v3：标准 cron 表达式解析 + 秒级精度"
- "调度流程：Cron 触发 → 创建 AgentTask → 入队 Redis Stream（复用 Task Queue）"
- "ScheduledTask CRUD API"

**修复方向**: 实现 `internal/scheduler/` 调度器，使用 robfig/cron，提供 CRUD API。

#### P1-4: 飞书 IM 集成仅为 Echo Mock

**现状**: `im/service.go:161` 仅返回 `"收到消息: " + msg.Text`，未集成 Agent Chat API。

**Spec 要求（SPEC-011 第 4 节）**:
- "IM 消息 → Agent Service Chat API（内部调用，非 HTTP）"
- "快捷指令支持：/分析 /查询 /周报 /帮助"
- "分析结果 → 飞书卡片 JSON 格式化"
- SPEC-011 第 3 节: "用户绑定，im_bindings 集合"

**修复方向**: 集成 go-lark SDK，实现消息路由到 ChatService，支持卡片消息和快捷指令。

#### P1-5: 缺失 5 个 Skill 实现

**现状**: 仅有 4 个 Skill（sql_executor, stats_engine, knowledge_search, save_report），缺失 5 个：

| 缺失 Skill | Spec 要求 | 修复方向 |
|-----------|----------|---------|
| `save_artifact` | SPEC-008 第 5 节 | 上传文件到 SeaweedFS + MongoDB 元数据 |
| `workspace_read/write/exec` | SPEC-008 第 6 节 | 按 session 隔离的文件读写 + 沙箱执行 |
| `prompt_enhancement` | SPEC-008 第 7 节 | 无状态 LLM 增强，生成 3 个建议版本 |
| `email_sender` | SPEC-008 第 8 节 | 域名白名单 + 异步 goroutine 发送 |
| `openapi_to_mcp` | SPEC-008 第 9 节 | kin-openapi 解析 → MCP Tool 生成 → 审核流程 |

---

### P2 — 重要遗漏

#### P2-1: 上下文窗口管理未使用 tiktoken-go

**现状**: `context_manager.go:33-46` 使用粗略启发式 `EstimateTokens()`（~4 chars/token），而非 tiktoken-go。

**Spec 要求（SPEC-004 第 7 节）**: "tiktoken-go token 计数（按模型动态选择 encoder）"

**修复方向**: 集成 tiktoken-go，按模型选择 encoder（cl100k_base / o200k_base / p50k_base）。

#### P2-2: 无 Vault 密钥加密集成

**现状**: API Key 从环境变量明文读取。`agent/vault.go` 存在但未使用。

**Spec 要求（SPEC-004 第 8 节）**: "AES-256-GCM 加密存储：API Key / DB 密码 / 第三方 Secret"

**修复方向**: 实现 Vault 集成或 AES-256-GCM 本地加密存储，Migrate 环境变量中的 Secret 到加密存储。

#### P2-3: 无 Redis Stats 收集器

**现状**: `monitor/monitor.go` 仅有进程级指标（内存、goroutines、uptime）。无 Agent 调用统计、Token 消耗、AI 成本。

**Spec 要求（SPEC-010 第 1 节）**: 6 类 Redis Stats 计数器（agent_calls, model_calls, sessions, tasks, tokens, cost），Dashboard ROI 计算 API。

**修复方向**: 实现 Redis Stats 收集器，Scheduler 每 5 分钟写入，提供 Dashboard Stats/ROI API。

#### P2-4: Session Manager 仅内存存储

**现状**: `chat/session.go` 使用 `map[string]*Session`，服务重启全部丢失。

**Spec 要求（RFC 第 12 节）**: "Session lifecycle: create → active → idle 30min → expired → grace period 24h → cleanup"，MongoDB 持久化 + TTL 索引。

**修复方向**: Session 持久化到 MongoDB `sessions` 集合，TTL 索引自动清理。

#### P2-5: 无 WebSocket 任务进度推送

**现状**: 无 WebSocket 实现。任务进度仅通过轮询 `GET /tasks/{task_id}` 获取。

**Spec 要求（SPEC-009 第 3 节）**: "WebSocket 通道：客户端连接 → 订阅 task:{task_id}:progress"

**修复方向**: 实现 WebSocket endpoint，Worker 定时汇报进度，前端实时渲染进度条。

#### P2-6: 模型配置无 MongoDB 持久化/热加载

**现状**: `main.go:99-109` 仅从环境变量注册模型，无法运行时修改。

**Spec 要求（SPEC-004 第 1 节）**: "数据库配置（后台设置）> 环境变量默认值"，"MongoDB model_configs 集合"，"后台修改后立即生效（Watcher 监听或配置热加载）"

**修复方向**: 模型配置 CRUD API + MongoDB 持久化 + 热加载机制。

#### P2-7: Milvus 向量搜索未集成到知识库搜索

**现状**: `knowledge/service.go:162-167` semanticSearch 返回 nil（占位）。

**Spec 要求（SPEC-006 第 4 节 + 第 6 节）**: "Milvus 向量索引...混合搜索 RRF fusion"

**修复方向**: 集成 Milvus client，实现语义向量搜索，与 MongoDB 全文搜索做 RRF 融合。

#### P2-8: 管理后台 API 路由不完整

**现状**: `main.go:239-243` admin routes 仅有占位 Dashboard endpoint。缺失：模型配置 CRUD、角色管理 API、审计日志查询、通知 API、知识库管理 API。

**Spec 要求（SPEC-013 第 2 节）**: 12 个管理后台页面对应全量 API。

**修复方向**: 实现完整 Admin API 路由组。

#### P2-9: Security Auditor 未校验 Skill 调用

**现状**: Security Auditor 已实现 `AuditToolCall()` 方法，但 Agent Engine 的 `Run()` 方法（engine.go:172-208）中从未在被调用的 Skill 执行前调用此方法。

**Spec 要求（SPEC-004 第 5 节）**: "Tool Call 审计（Skill 执行前）：拦截高风险 Skill 调用，记录所有 Skill 调用链到 security_alerts 集合"

**修复方向**: 在 Engine.Run() 中每个 Tool Call 执行前调用 `security.AuditToolCall()`。

---

### P3 — 优化项

| ID | 问题 | Spec 依据 |
|----|------|----------|
| P3-1 | 无子 Agent 编排（A2A 协议） | SPEC-004 第 9 节（Phase 4 实现，可推迟） |
| P3-2 | 无 Artifact 批量下载（ZIP） | PRD F-21 |
| P3-3 | 无批量文件上传 | PRD F-22 |
| P3-4 | 无 email_sender Skill | SPEC-008 第 8 节 |
| P3-5 | 无 OpenAPI-to-MCP 转换器 | SPEC-008 第 9 节 |
| P3-6 | 无 Prompt Enhancement Skill | SPEC-008 第 7 节 |
| P3-7 | 无站内信通知服务（模型已定义） | SPEC-013 第 4 节 |
| P3-8 | RBAC 未覆盖全部路由 | SPEC-003 第 6.5 节 |
| P3-9 | `configs/config.yaml` 缺少 Milvus/Vault 配置项 | SPEC-003 第 3 节 |

---

## PRD 功能覆盖率

| PRD Feature | 描述 | 状态 |
|-------------|------|:----:|
| F-01 | MCP 数据集成 | ❌ |
| F-02 | 历史数据分析 | ⚠️ Logic 层已实现，未连接数据源 |
| F-03 | 智能解读 | ⚠️ KB 搜索占位 |
| F-03-1 | 报告格式校验 | ✅ |
| F-04 | Chat SSE 流式 | ✅ |
| F-05 | Agent 批量任务 | ⚠️ 内存实现，未用 Redis Stream |
| F-06 | 定时任务 | ❌ |
| F-07 | Session 管理 | ⚠️ 仅内存，未持久化 |
| F-08 | 知识库 | ⚠️ Milvus 未集成 |
| F-09 | 邮件发送 | ❌ |
| F-10 | API-to-MCP | ❌ |
| F-11 | Auth & RBAC | ⚠️ Login 占位，RBAC 不完整 |
| F-12 | 审计日志 | ✅ |
| F-13 | 安全审核层 | ⚠️ Skill 调用未审计 |
| F-14 | 子 Agent 协作 | ❌ (推迟) |
| F-15 | 管理后台 | ❌ 前端缺失 |
| F-16 | 移动端适配 | ❌ |
| F-17 | Artifact 管理 | ✅ |
| F-18 | Prompt 增强 | ❌ |
| F-19 | 列表管理规范 | ⚠️ 部分实现 |
| F-20 | Vault 密钥管理 | ❌ |
| F-21 | ZIP 批量下载 | ❌ |
| F-22 | 批量上传 | ❌ |
| F-23 | Hermes 探索 | ⚠️ 位置错误（应为独立服务） |
| F-24 | 飞书 IM | ⚠️ Echo Mock |
| F-25 | 站内信通知 | ⚠️ 模型已定义，服务未实现 |

**图例**: ✅ 完整实现 | ⚠️ 部分实现 | ❌ 未实现

---

## 修复计划（按优先级、分阶段）

### Phase 1: 解除阻塞（P0 — 预计 40h）

| 任务 | 涉及文件 | 预估 |
|------|---------|:---:|
| P0-2: Skill Registry 集成 | `internal/domain/agent/engine.go`, `main.go` | 4h |
| P0-3: Login/Auth 实现 | `internal/api/handler/auth.go`, `internal/service/auth/` | 8h |
| P0-1: 前端初始化 + 核心页面 | `frontend/` (新建) | 28h |
| **Phase 1 合计** | | **40h** |

### Phase 2: 核心功能补充（P1 — 预计 55h）

| 任务 | 涉及文件 | 预估 |
|------|---------|:---:|
| P1-2: 整合 AgentService ↔ TaskService | `internal/service/agent/agent_service.go` | 6h |
| P1-1: Hermes 独立服务 | `cmd/hermes/main.go`, `main.go` | 6h |
| P1-3: Scheduler 实现 | `internal/scheduler/` (新建) | 12h |
| P1-4: 飞书 IM 集成 Chat API | `internal/service/im/service.go` | 12h |
| P1-5: 补充 5 个 Skill | `skills/save_artifact/`, `skills/workspace_*/`, `skills/prompt_enhancement/`, `skills/email_sender/`, `skills/openapi_to_mcp/` | 19h |
| **Phase 2 合计** | | **55h** |

### Phase 3: 质量与完整性（P2 — 预计 60h）

| 任务 | 涉及文件 | 预估 |
|------|---------|:---:|
| P2-1: tiktoken-go 集成 | `internal/service/chat/context_manager.go` | 4h |
| P2-2: Vault 密钥加密 | `internal/domain/agent/vault.go`, `main.go` | 8h |
| P2-3: Redis Stats 收集器 + Dashboard API | `internal/logic/stats_collector.go`, `internal/service/admin/stats.go` | 8h |
| P2-4: Session MongoDB 持久化 | `internal/service/chat/session.go` | 6h |
| P2-5: WebSocket 进度推送 | `internal/api/handler/ws.go`, Worker 改造 | 10h |
| P2-6: 模型配置 CRUD + 热加载 | `internal/service/admin/model_config.go` | 8h |
| P2-7: Milvus 向量搜索集成 | `internal/service/knowledge/service.go` | 8h |
| P2-8: Admin API 完整路由 | `main.go`, handler 文件 | 6h |
| P2-9: Security Auditor 集成 Skill | `internal/domain/agent/engine.go` | 2h |
| **Phase 3 合计** | | **60h** |

### Phase 4: 优化完善（P3 — 预计 40h）

| 任务 | 涉及文件 | 预估 |
|------|---------|:---:|
| P3-1~P3-9: 各项优化 | 多个文件 | 40h |

---

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|:---:|
| `frontend/` | React/Next.js 前端全量 | 新建 |
| `internal/service/auth/` | 认证服务 | 新建 |
| `internal/api/handler/auth.go` | 登录/注册 Handler | 新建 |
| `internal/api/handler/ws.go` | WebSocket Handler | 新建 |
| `cmd/hermes/main.go` | Hermes 独立入口 | 新建 |
| `internal/scheduler/` | Cron 调度器 | 新建 |
| `internal/logic/stats_collector.go` | Redis Stats 收集器 | 新建 |
| `internal/service/admin/` | 管理后台 API | 新建 |
| `skills/save_artifact/` | Skill | 新建 |
| `skills/workspace_read/` | Skill | 新建 |
| `skills/workspace_write/` | Skill | 新建 |
| `skills/workspace_exec/` | Skill | 新建 |
| `skills/prompt_enhancement/` | Skill | 新建 |
| `skills/email_sender/` | Skill | 新建 |
| `skills/openapi_to_mcp/` | Skill | 新建 |
| `internal/domain/agent/engine.go` | Skill Registry 集成 + Security 集成 | 修改 |
| `internal/service/agent/agent_service.go` | 整合 TaskService | 修改 |
| `internal/service/chat/session.go` | MongoDB 持久化 | 修改 |
| `internal/service/chat/context_manager.go` | tiktoken-go 替换启发式 | 修改 |
| `internal/service/im/service.go` | go-lark SDK 集成 | 重写 |
| `internal/service/knowledge/service.go` | Milvus 集成 | 修改 |
| `internal/domain/agent/vault.go` | AES-GCM 集成 | 修改 |
| `main.go` | 路由整合、移除 Hermes route | 修改 |

---

## 验证标准

1. `frontend/` 目录包含 Next.js 14 项目，`npm run dev` 可启动
2. 登录页可正常登录，JWT Token 签发并返回
3. Chat SSE 流式返回，前端实时渲染
4. Agent 异步任务创建 → 入队 Redis Stream → Worker 执行 → 前端查看到结果
5. Skill Registry 自动加载 `skills/` 目录下所有 Skill
6. `go test ./...` 全部通过
7. E2E 用例覆盖：登录 → Chat → Agent 任务 → 结果查看
8. UI 风格遵循玻璃极光深色主题（与 prototype.html 一致）
