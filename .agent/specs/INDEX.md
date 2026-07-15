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
| SPEC-040 | UI E2E — 响应式设计 (RESP) | **P8** | [spec-040-ui-responsive.md](spec-040-ui-responsive.md) | 📐 设计中 |
| SPEC-041 | UI E2E — 错误状态与边界条件 (ERR) | **P8** | [spec-041-ui-error.md](spec-041-ui-error.md) | 📐 设计中 |
| SPEC-042 | UI E2E — 端到端场景测试 (E2E) | **P8** | [spec-042-ui-e2e-scenarios.md](spec-042-ui-e2e-scenarios.md) | 📐 设计中 |
| SPEC-043 | Mock Model Service — 测试用模型模拟服务 | **P8 前置** | [spec-043-mock-model-service.md](spec-043-mock-model-service.md) | ✅ 已实现 |
| SPEC-044 | 邀请注册系统 — 移除自由注册，改为邀请制 | **P9** | [spec-044-invite-registration.md](spec-044-invite-registration.md) | 📐 设计中 |

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
```
