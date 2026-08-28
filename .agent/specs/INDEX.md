# Spec Index

所有设计规格文档的中央注册表，统一编号管理。

## 编号规则

- 格式: `SPEC-XXX`，三位数字，从 `001` 递增
- 每个独立设计文档分配一个编号，终身不可变
- Sub-directory 下的 spec 共享同一功能域

---

## 索引表

| 编号 | 标题 | Phase | 文件路径 | 状态 |
|------|------|:-----:|----------|------|
| SPEC-001 | 项目初始化与文档架构 | — | [spec-001-project-init.md](spec-001-project-init.md) | ✅ 已实现 |
| SPEC-002 | CI/CD 环境与工具链 | 前置 | [spec-002-ci-environment.md](spec-002-ci-environment.md) | ✅ 已实现 |
| SPEC-003 | 基础设施与认证授权 | **P1** | [spec-003-infrastructure.md](spec-003-infrastructure.md) | ✅ 已实现 |
| SPEC-004 | Agent 核心引擎（含安全审计） | **P2** | [spec-004-agent-engine.md](spec-004-agent-engine.md) | ✅ 已实现 |
| SPEC-005 | Artifact 存储与工作区 | **P2** | [spec-005-artifact-storage.md](spec-005-artifact-storage.md) | ✅ 已实现 |
| SPEC-006 | 知识库系统 | **P3** | [spec-006-knowledge-base.md](spec-006-knowledge-base.md) | ✅ 已实现 |
| SPEC-007 | 数据分析 Logic 层 | **P3** | [spec-007-data-analysis-logic.md](spec-007-data-analysis-logic.md) | ✅ 已实现 |
| SPEC-008 | Skill 实现层 | **P4** | [spec-008-skill-implementations.md](spec-008-skill-implementations.md) | ✅ 已实现 |
| SPEC-009 | 任务队列与调度基础设施 | **P4** | [spec-009-task-queue-scheduler.md](spec-009-task-queue-scheduler.md) | ✅ 已实现 |
| SPEC-010 | 系统统计监控 | **P4** | [spec-010-stats-monitoring.md](spec-010-stats-monitoring.md) | ✅ 已实现 |
| SPEC-011 | IM 集成（飞书） | **P4** | [spec-011-im-integration.md](spec-011-im-integration.md) | ✅ 已实现 |
| SPEC-012 | Hermes 自由探索 | **P5** | [spec-012-hermes-explore.md](spec-012-hermes-explore.md) | ✅ 已实现 |
| SPEC-013 | 管理后台 | **P5** | [spec-013-admin-dashboard.md](spec-013-admin-dashboard.md) | ✅ 已实现 |
| SPEC-014 | 测试体系 | **P6** | [spec-014-testing.md](spec-014-testing.md) | ✅ 已实现 |
| SPEC-015 | 代码审核修复（一致性修复） | **P7** | [spec-015-audit-fix.md](spec-015-audit-fix.md) | ✅ 已实现 |
| SPEC-016 | Docker Compose 配置修复 | **P7** | [spec-016-docker-compose-fix.md](spec-016-docker-compose-fix.md) | ✅ 已实现 |
| SPEC-017 | UI E2E — 登录与认证 (AUTH) | **P8** | [spec-017-ui-auth.md](spec-017-ui-auth.md) | ✅ 已实现 |
| SPEC-018 | UI E2E — 布局与导航 (LAYOUT) | **P8** | [spec-018-ui-layout.md](spec-018-ui-layout.md) | ✅ 已实现 |
| SPEC-019 | UI E2E — Chat 模式 (CHAT) | **P8** | [spec-019-ui-chat.md](spec-019-ui-chat.md) | ✅ 已实现 |
| SPEC-020 | UI E2E — Agent 模式 (AGENT) | **P8** | [spec-020-ui-agent.md](spec-020-ui-agent.md) | ✅ 已实现
| SPEC-021 | UI E2E — Hermes自由探索 (HERMES) | **P8** | [spec-021-ui-hermes.md](spec-021-ui-hermes.md) | ✅ 已实现
| SPEC-022 | UI E2E — 数据看板 (DASH) | **P8** | [spec-022-ui-dashboard.md](spec-022-ui-dashboard.md) | ✅ 已实现 |
| SPEC-023 | UI E2E — 用户管理 (USER) | **P8** | [spec-023-ui-user.md](spec-023-ui-user.md) | ✅ 已实现 |
| SPEC-024 | UI E2E — 权限管理 (ROLE) | **P8** | [spec-024-ui-role.md](spec-024-ui-role.md) | ✅ 已实现 |
| SPEC-025 | UI E2E — 模型配置 (MODEL) | **P8** | [spec-025-ui-model.md](spec-025-ui-model.md) | ✅ 已实现 |
| SPEC-026 | UI E2E — 系统配置 (SYSCONFIG) | **P8** | [spec-026-ui-sysconfig.md](spec-026-ui-sysconfig.md) | ✅ 已实现 |
| SPEC-027 | UI E2E — 任务管理 (TASK) | **P8** | [spec-027-ui-task.md](spec-027-ui-task.md) | ✅ 已实现 |
| SPEC-028 | UI E2E — 知识库管理 (KB) | **P8** | [spec-028-ui-kb.md](spec-028-ui-kb.md) | ✅ 已实现 |
| SPEC-029 | UI E2E — 审计日志 (AUDIT) | **P8** | [spec-029-ui-audit.md](spec-029-ui-audit.md) | ✅ 已实现 |
| SPEC-030 | UI E2E — API 转换审核 (API) | **P8** | [spec-030-ui-api.md](spec-030-ui-api.md) | ✅ 已实现 |
| SPEC-031 | UI E2E — 站内信系统 (NOTIF) | **P8** | [spec-031-ui-notif.md](spec-031-ui-notif.md) | ✅ 已实现 |
| SPEC-032 | UI E2E — 密码管理 (PWD) | **P8** | [spec-032-ui-pwd.md](spec-032-ui-pwd.md) | ✅ 已实现 |
| SPEC-033 | UI E2E — 增强提示词 (PROMPT) | **P8** | [spec-033-ui-prompt.md](spec-033-ui-prompt.md) | ✅ 已实现 |
| SPEC-034 | UI E2E — IM 集成飞书 (IM) | **P8** | [spec-034-ui-im.md](spec-034-ui-im.md) | ✅ 已实现 |
| SPEC-035 | UI E2E — 列表管理通用规范 (LIST) | **P8** | [spec-035-ui-list.md](spec-035-ui-list.md) | ✅ 已实现 |
| SPEC-036 | UI E2E — 批量文件上传 (UPLOAD) | **P8** | [spec-036-ui-upload.md](spec-036-ui-upload.md) | ✅ 已实现 |
| SPEC-037 | UI E2E — Session 管理 (SESSION) | **P8** | [spec-037-ui-session.md](spec-037-ui-session.md) | ✅ 已实现 |
| SPEC-038 | UI E2E — 安全审查层 (SEC) | **P8** | [spec-038-ui-security.md](spec-038-ui-security.md) | ✅ 已实现 |
| SPEC-039 | UI E2E — 角色权限访问控制 (RBAC) | **P8** | [spec-039-ui-rbac.md](spec-039-ui-rbac.md) | ✅ 已实现 |
| SPEC-040 | UI E2E — 响应式设计 (RESP) | **P8** | [spec-040-ui-responsive.md](spec-040-ui-responsive.md) | ✅ 已实现 |
| SPEC-041 | UI E2E — 错误状态与边界条件 (ERR) | **P8** | [spec-041-ui-error.md](spec-041-ui-error.md) | ✅ 已实现 |
| SPEC-042 | UI E2E — 端到端场景测试 (E2E) | **P8** | [spec-042-ui-e2e-scenarios.md](spec-042-ui-e2e-scenarios.md) | ✅ 已实现 |
| SPEC-043 | Mock Model Service — 测试用模型模拟服务 | **P8 前置** | [spec-043-mock-model-service.md](spec-043-mock-model-service.md) | ✅ 已实现 |
| SPEC-044 | 邀请注册系统 — 移除自由注册，改为邀请制 | **P9** | [spec-044-invite-registration.md](spec-044-invite-registration.md) | ✅ 已实现 |
| SPEC-045 | Go Service 单元测试全覆盖 — 98% 底线，CI 门禁 | **P10** | [spec-045-go-service-ut.md](spec-045-go-service-ut.md) | ✅ 已实现 |
| SPEC-046 | UI E2E 测试增强与真实集成验证（KB 索引 / 工具调用 / Dashboard 数据） | **P11** | [spec-046-ui-test-integration.md](spec-046-ui-test-integration.md) | ✅ 已实现 |
| SPEC-047 | 主分支 UI 截图审查与布局修复（9 个 bug） | **P11** | [spec-047-ui-screenshot-bugfix.md](spec-047-ui-screenshot-bugfix.md) | 📐 设计中 |
| SPEC-048 | 引擎层迁移 Google ADK — ReAct loop / Session 压缩 / 模型路由 | **P11** | [spec-048-adk-migration.md](spec-048-adk-migration.md) | ✅ 已实现 |
| SPEC-049 | 统一模型配置与多模型能力体系（提示词/能力描述/token 倍率 + KB embedding 索引） | **P12** | [spec-049-unified-model-config.md](spec-049-unified-model-config.md) | ✅ 已实现 |
| SPEC-050 | Go 1.26 升级与 adk-go-memory 迁移（含记忆相似度合并） | **P12** | [spec-050-go126-memory-migration.md](spec-050-go126-memory-migration.md) | ✅ 已实现 |
| SPEC-051 | LLM 全链路 Token 统计与 Redis 缓存 | **P12** | [spec-051-llm-token-stats-cache.md](spec-051-llm-token-stats-cache.md) | ✅ 已实现 |
| SPEC-052 | 多模型路由与用途关联（Chat/Task/Embedding/压缩摘要） | **P13** | [spec-052-model-routing.md](spec-052-model-routing.md) | 📐 设计中 |
| SPEC-053 | 会话存储、记忆压缩与 KB 索引逻辑对齐（Chat/Hermes 双轨） | **P13** | [spec-053-session-memory-kb-alignment.md](spec-053-session-memory-kb-alignment.md) | 📐 设计中 |
| SPEC-054 | Sysconfig RBAC 权限不足修复（admin 访问也显示 insufficient permissions） | **P13** | [spec-054-sysconfig-rbac-fix.md](spec-054-sysconfig-rbac-fix.md) | 📐 设计中 |
| SPEC-055 | 分层架构重构（Controller→Service→Repository→Infra） | **P14** | [spec-055-layer-refactoring.md](spec-055-layer-refactoring.md) | ✅ 已实现 |
| SPEC-056 | 分层语义纠正（一）：domain ID 解耦 / service SDK 清理 / middleware 解耦 / IMBind 补全 | **P15** | [spec-056-layer-semantics-correction.md](spec-056-layer-semantics-correction.md) | ✅ 已实现 |
| SPEC-057 | domain model 全量去 bson tag + infra 转换层全量改造 | **P15** | [spec-057-domain-bson-tag-removal.md](spec-057-domain-bson-tag-removal.md) | ✅ 已实现 |
| SPEC-058 | 分层语义纠正（二）：logic 编排层 / chat 解耦 gin / service 扁平化 / main.go 迁移 / 覆盖率 98% | **P15** | [spec-058-layer-orchestrator-main-migration.md](spec-058-layer-orchestrator-main-migration.md) | ✅ 已实现 |
| SPEC-059 | 统计分析架构归置 + Token 统计真数据 | **P15** | [spec-059-enhance-token-stats-verification.md](spec-059-enhance-token-stats-verification.md) | ✅ 已实现 |
| SPEC-060 | Dashboard trend 接入 + 路径/字段修复 | **P15** | [spec-060-dashboard-trend-integration.md](spec-060-dashboard-trend-integration.md) | ✅ 已实现 |
| SPEC-061 | 配置统一缓存到 Redis 并支持热更新（Cache-Aside + 消除预加载） | **P15** | [spec-061-config-redis-cache-hotreload.md](spec-061-config-redis-cache-hotreload.md) | ✅ 已实现 |
| SPEC-062 | 多模型配置与 Session 绑定模型（per-model Runtime 注册表 + 模型选择器） | **P15** | [spec-062-multi-model-session-binding.md](spec-062-multi-model-session-binding.md) | ✅ 已实现 |
| SPEC-063 | 异步/定时 Agent 任务执行器实现（RFC worker→AgentExecutor→Runtime.Run） | **P15** | [spec-063-async-scheduled-agent-executor.md](spec-063-async-scheduled-agent-executor.md) | 📐 设计中 |
| SPEC-064 | RBAC 角色权限管理系统（角色层级、权限管控、用户-角色关联、侧边栏权限化） | **P20** | [spec-064-rbac-implementation.md](spec-064-rbac-implementation.md) | 📐 设计中 |
| SPEC-065 | API 注册与 MCP 工具集成（OpenAPI 上传 → 注册外部 API → external_api_search/summary/method/call 工具） | **P20** | [spec-065-api-collection-mcp-tools.md](spec-065-api-collection-mcp-tools.md) | ✅ 已实现 |
| SPEC-066 | 配置存储拆分（system_configs 去 namespace；模型配置每模型一条文档+DB分页；model_defaults 独立集合；skill 独立集合） | **P15** | [spec-066-config-storage-split.md](spec-066-config-storage-split.md) | 📐 设计中 |
| SPEC-067 | 用户意图识别与 LLM 输出相关性检查（Guard：intent/relevance use case + Redis 计数重试 + compaction 角色/范围调整） | **P15** | [spec-067-intent-relevance-guard.md](spec-067-intent-relevance-guard.md) | 📐 设计中 |
| SPEC-068 | 知识库文本 PII 脱敏（Presidio pii-redaction：spacy+纯规则；后端封装 + KB 上传纯文本脱敏落库 + 模型输入/输出审计 + 内置 LLM 审计接入 + 输入 token 校验） | **P15** | [spec-068-pii-redaction.md](spec-068-pii-redaction.md) | ✅ 已实现 |
| SPEC-069 | compaction 机制缺陷修复 + summary 语义拆分 + raw_events 存储重构（token 估算补全 / tool 链配对保护 / summary 与提示分流 / raw_events 独立 collection） | **P15** | [spec-069-compaction-trigger-fixes.md](spec-069-compaction-trigger-fixes.md) | 📐 设计中 |
| SPEC-070 | KB 切片图数据库索引（Neo4j）+ 图访问共用组件 + 图谱搜索 Skill（GraphRepository 接口 + knowledge_graph_search tool + seed） | **P15** | [spec-070-kb-graph-index.md](spec-070-kb-graph-index.md) | 📐 详细设计 |
| SPEC-071 | agent 调用子 agent（sub agent tool + interface 解耦、能力提示词单独组装、独立 session 父绑定返回即硬删、最终返回同 tool response、model 与主 agent 一致、并行委派） | **P15** | [spec-071-agent-invoke-subagent.md](spec-071-agent-invoke-subagent.md) | 📐 调研完成（方案已定，待实现） |
| SPEC-072 | 领域内聚重构（业务领域 logic/service/db_model 垂直切片，替换水平分层） | **P15** | [spec-072-domain-cohesion-refactor.md](spec-072-domain-cohesion-refactor.md) | 📐 立项（不展开） |

## Phase 对应与依赖

| Phase | Spec | 标题 | 前置依赖 |
|:-----:|------|------|:---------:|
| 前置 | SPEC-002 | CI/CD 环境与工具链 | — |
| **P1** | SPEC-003 | 基础设施与认证授权 | SPEC-002 |
| **P2** | SPEC-004 | Agent 核心引擎（含安全审计） | SPEC-003 |
| **P2** | SPEC-005 | Artifact 存储与工作区 | SPEC-003, 004 |
| **P3** | SPEC-006 | 知识库系统 | SPEC-004 |
| **P3** | SPEC-007 | 数据分析 Logic 层 | SPEC-003, 004, 006 |
| **P4** | SPEC-008 | Skill 实现层 | SPEC-004, 005, 006, 007 |
| **P4** | SPEC-009 | 任务队列与调度基础设施 | SPEC-003, 004 |
| **P4** | SPEC-010 | 系统统计监控 | SPEC-004, 008, 009 |
| **P4** | SPEC-011 | IM 集成（飞书） | SPEC-004 |
| **P5** | SPEC-012 | Hermes 自由探索 | 独立 |
| **P5** | SPEC-013 | 管理后台 | SPEC-004, 010, 011, 009 |
| **P6** | SPEC-014 | 测试体系 | 全部 |
| **P7** | SPEC-015 | 代码审核修复 | SPEC-001 ~ SPEC-014 |
| **P7** | SPEC-016 | Docker Compose 配置修复 | SPEC-002 |
| **P8 前置** | SPEC-043 | Mock Model Service | SPEC-004 (LLMProvider 接口) |
| **P8** | SPEC-017 ~ SPEC-042 | UI E2E 测试设计 | SPEC-043 (Mock Model Service), SPEC-001 ~ SPEC-016 (全部已实现) |
| **P9** | SPEC-044 | 邀请注册系统 | SPEC-003 (用户模型 + JWT), SPEC-023 (User Mgmt) |
| **P10** | SPEC-045 | Go Service 单元测试全覆盖 | SPEC-002 (CI), SPEC-014 (原测试体系), SPEC-003~013 (待测服务) |
| **P11** | SPEC-048 | 引擎层迁移 Google ADK | SPEC-004, SPEC-006, SPEC-008, SPEC-043 |
| **P12** | SPEC-049 | 统一模型配置与多模型能力体系 | SPEC-003, SPEC-006, SPEC-025, **SPEC-048** |
| **P12** | SPEC-050 | Go 1.26 升级与 adk-go-memory 迁移 | **SPEC-048, SPEC-049** |
| **P12** | SPEC-051 | LLM 全链路 Token 统计与 Redis 缓存 | SPEC-009, SPEC-010, **SPEC-048, SPEC-049** |
| **P11** | SPEC-046 | UI E2E 真实集成验证 | **SPEC-048, SPEC-049, SPEC-050, SPEC-051**, SPEC-022, SPEC-028, SPEC-043 |
| **P11** | SPEC-047 | UI 截图审查与布局修复 | SPEC-017~042, SPEC-046 (联动) |
| **P13** | SPEC-052 | 多模型路由与用途关联 | SPEC-003, SPEC-025, SPEC-048, SPEC-049 |

> **实施顺序（2026-07-18 晓军确认）**: SPEC-048 → **SPEC-049 → SPEC-050 → SPEC-051** → SPEC-046 → SPEC-047。049/050/051 在 046 之前，因为 046 的 E2E 用例（KB embedding 索引、Mem0、Dashboard 真实数据、token 统计）依赖这三个 spec 的能力就绪。

### 依赖流向（简化）

```
[前置] SPEC-002 (CI)
         │
         ▼
[P1]  SPEC-003 ─── Infrastructure
         │
    ┌────┴──────────┐
    ▼                 ▼
[P2] SPEC-004    [P2] SPEC-005
    Agent Core        Artifact
    (+Security)       │
    │  │               │
    ▼  │               │
[P3]   │               │
SPEC-006│               │
   KB   │               │
    │   │               │
    ├───┘               │
    │                   │
    ▼                   │
[P3] SPEC-007 ─── 数据分析 Logic
    │
    ├───────────────────┤
    ▼                   ▼
[P4] SPEC-008      [P4] SPEC-009
    Skills              Task Queue
    │                   + Scheduler
    │                   │
    ├───────────────────┤
    ▼                   │
[P4] SPEC-010 ─── 统计监控  │
    │                   │
    ├───────────────────┤
    ▼                   │
[P4] SPEC-011 (IM)      │
    │                   │
    ├───────────────────┘
    │
    ├──────► [P5] SPEC-013 (Admin)
    │
[P5] SPEC-012 (Hermes, 独立)
    │
    └───────┬───────┐
            │       │
            ▼       ▼
[P6]   SPEC-014 ─── 测试体系
    │
    ▼
[P7]   SPEC-015 ─── 审核修复（基于 SPEC-001~014 一致性审计）
    │
    ▼
[P10]  SPEC-045 ─── Go UT 全覆盖（98% 底线，CI 门禁）

    │
    ▼
[P11] SPEC-048 ─── 引擎层迁移 Google ADK
    │               (ReAct loop / Session 压缩 / 模型路由)
    │
    ├─────────────────┐
    ▼                 ▼
[P11] SPEC-046 ─── UI E2E 真实集成验证
    │               (KB 索引 / 工具调用链 / Mem0 / Dashboard 真实数据)
    │
    └──────┬──────────┘
           ▼
[P11] SPEC-047 ─── UI 截图审查与布局修复
                   (与 SPEC-046 联动，修复后验证布局)

[P13] SPEC-053 ─── 会话存储/记忆压缩/KB 索引逻辑对齐
                   (Chat/Hermes 双轨梳理、删除恢复策略)

[P13] SPEC-052 ─── 多模型路由与用途关联
                   (UseCase-based routing: chat/compaction/enhance/embedding)

[P13] SPEC-054 ─── Sysconfig RBAC 权限修复
                   (admin 用户 403 → permission 枚举对齐)

[P14] SPEC-055 ─── 分层架构重构（Controller→Service→Repository→Infra）
                   (main.go 减至 300 行、UT 无需 MongoDB 连接、接口化)

[P15] SPEC-056 ─── 分层语义纠正（一）: domain ID 解耦 / service SDK 清理 /
                   middleware 解耦 / IMBind 补全（低-中风险快赢）
                   │
                   ├──► [P15] SPEC-057 ─── domain model 全量去 bson tag + infra 转换全量改造
                   │
                   └──► [P15] SPEC-058 ─── 分层语义纠正（二）: logic 编排层 / chat 解耦 gin /
                                        service 扁平化 / main.go 迁移 1053→300 / 覆盖率 98%
                                        (依赖 SPEC-056 完成)

[P15] SPEC-061 ─── 配置统一缓存到 Redis 并支持热更新
                   (Cache-Aside: 写DB→刷缓存 / 删DB→删缓存 / 读缓存优先;
                    消除 LLM/Instruction/Embedding 启动预加载;
                    依赖 SPEC-003/049/051/055/058 已实现)
                   │
                   ▼
[P15] SPEC-062 ─── 多模型配置与 Session 绑定模型
                   (per-model Runtime 注册表懒创建; session 绑定 modelId 不可换;
                    默认模型 IsDefault 逻辑; 模型列表分页;
                    chat/agent/定时/imbind 模型选择; 前端 ModelSelector;
                    系统级 LLM map[UseCase]*Runtime; ModelEntry 加 ID 字段;
                    指纹比对热更新无需Pub/Sub;
                    依赖 SPEC-061 cache 装饰器就位)
                   │
                   ▼
[P15] SPEC-063 ─── 异步/定时 Agent 任务执行器实现
                   (AgentExecutor 实现 worker.TaskExecutor; 复用 Runtime.RunAndCollect
                    同实时执行范式; 修复 pool.go 三缺陷: DB加载task/回写结果/通知;
                    RFC §16 processTask 映射; 派生user message from Task.Params;
                    依赖 SPEC-062 Registry 按 task.ModelID 选 Runtime)

[P20] SPEC-064 ─── RBAC 角色权限管理系统
                   (角色层级 L0→L1→L2，父角色拥有子角色权限;
                    所有 API/UI 走 RBAC permission 检查;
                    用户关联多个 RBAC 角色(≤10)，权限三步 Go 查询;
                    侧边栏权限化; 旧 role 体系删除;
                    RequirePermission 闭包注入 rbac.Service)

[P20] SPEC-065 ─── API 注册与 MCP 工具集成
                   (OpenAPI 上传注册外部 HTTP API 为工具函数;
                    external_api_search/summary/method/call 四工具;
                    审核流 pending/approved/rejected; 依赖 SPEC-064 RBAC 权限)

[P15] SPEC-066 ─── 配置存储拆分
                   (system_configs 去 namespace 仅存系统配置;
                    skill → skill_configs; 模型配置 → model_configs;
                    SysConfigRepository 接口去 namespace; ModelConfigRepository 落地;
                    迁移脚本幂等 + _bak 备份回滚;
                    依赖 SPEC-061/062 已实现)

[P15] SPEC-067 ─── 用户意图识别与 LLM 输出相关性检查 (Guard)
                   (chat/feishu 意图判断 is_task; chat/feishu/agent task 相关性检查
                    is_relevant; system 事件写 events; Redis 计数有限重试;
                    intent_check/relevance_check 两个 use case;
                    compaction 角色 model→system、只压缩 tool/user;
                    依赖 SPEC-061/062/063 已实现)

[P15] SPEC-068 ─── 知识库文本 PII 脱敏 (Presidio pii-redaction) [✅ 已实现]
                   (官方 docker 部署 presidio-analyzer/anonymizer，
                    spacy 引擎 + 纯规则 recognizer（禁用 NER）;
                    后端封装 pii-redaction 服务;
                    KB 上传纯文本脱敏后落库 kb_chunks/Qdrant;
                    模型输入/输出审计 + 内置 LLM(compaction/enhance/
                    intent/relevance/kb) 经 AuditedLLM 统一接入审计;
                    输入 token 长度校验;
                    无硬依赖)

[P15] SPEC-069 ─── compaction 机制缺陷修复 + summary 语义拆分 + raw_events 存储重构
                   (1) estimateEventTokens 补全 FunctionCall.Args/FunctionResponse.Response;
                   2) 压缩边界保护 tool 链配对(方案C:悬空 call 保护);
                   3) summary 只进 events、压缩提示只进 raw_events;
                   4) raw_events 拆独立 collection 一条 event 一个 document;
                    依赖 SPEC-066/067 已实现)

[P15] SPEC-070 ─── KB 切片图数据库索引 (Neo4j) + 图访问共用组件 + 图谱搜索 Skill (详细设计)
                   (GraphRepository 接口 + neo4j infra 适配器，KB 与 Skill 共用;
                    仅 Chunk 节点 + RELATED_TO 边(最小图模型);
                    AddChunks 复用向量检索找同 creator 切片，topN=5 写图;
                    Chunk 节点含 creator_id/is_public，查询按 system_admin 策略过滤;
                    seed 仅 EnsureSchema DDL + skill 配置(无存量回填);
                    前置修复: Qdrant Client.Search 支持 filter(现存权限过滤失效 bug);
                    依赖 SPEC-006/068 已实现)

[P15] SPEC-071 ─── agent 调用子 agent (调研完成，方案已定，待实现)
                   (sub agent tool + interface 解耦，无 import 循环;
                    能力提示词单独组装; 独立 session 父绑定 + 返回即硬删;
                    最终返回同 tool response 写回主 session;
                    model 与主 agent 一致; 并行委派;
                    ctx 继承 + 取消销毁 session/runtime/DB;
                    依赖 SPEC-066/067/069)

[P15] SPEC-072 ─── 领域内聚重构 (立项，不展开，放最后实现)
                   (业务领域 logic/service/db_model 垂直切片;
                    替换水平分层 domain/service/logic/infra;
                    依赖 SPEC-066/067/069/071 落地后再展开)
```
