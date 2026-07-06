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
| SPEC-002 | CI/CD 环境与工具链 | 前置 | [spec-002-ci-environment.md](spec-002-ci-environment.md) | 🚧 设计中 |
| SPEC-003 | 基础设施与认证授权 | **P1** | [spec-003-infrastructure.md](spec-003-infrastructure.md) | 🚧 设计中 |
| SPEC-004 | Agent 核心引擎（含安全审计） | **P2** | [spec-004-agent-engine.md](spec-004-agent-engine.md) | 🚧 设计中 |
| SPEC-006 | Artifact 存储与工作区 | **P2** | [spec-006-artifact-storage.md](spec-006-artifact-storage.md) | 🚧 设计中 |
| SPEC-005 | 知识库系统 | **P3** | [spec-005-knowledge-base.md](spec-005-knowledge-base.md) | 🚧 设计中 |
| SPEC-015 | 数据分析 Logic 层 | P2～3 | [spec-015-data-analysis-logic.md](spec-015-data-analysis-logic.md) | 🚧 设计中 |
| SPEC-007 | Skill 实现层 | P2～4 | [spec-007-skill-implementations.md](spec-007-skill-implementations.md) | 🚧 设计中 |
| SPEC-014 | 任务队列与调度基础设施 | P2～4 | [spec-014-task-queue-scheduler.md](spec-014-task-queue-scheduler.md) | 🚧 设计中 |
| SPEC-009 | 系统统计监控 | **P4** | [spec-009-stats-monitoring.md](spec-009-stats-monitoring.md) | 🚧 设计中 |
| SPEC-010 | IM 集成（飞书） | **P4** | [spec-010-im-integration.md](spec-010-im-integration.md) | 🚧 设计中 |
| SPEC-011 | Hermes 自由探索 | **P5** | [spec-011-hermes-explore.md](spec-011-hermes-explore.md) | 🚧 设计中 |
| SPEC-012 | 管理后台 | **P5** | [spec-012-admin-dashboard.md](spec-012-admin-dashboard.md) | 🚧 设计中 |
| SPEC-013 | 测试体系 | **P6** | [spec-013-testing-deploy.md](spec-013-testing-deploy.md) | 🚧 设计中 |

## Phase 对应与依赖

| Phase | Spec | 标题 | 前置依赖 |
|:-----:|------|------|:---------:|
| 前置 | SPEC-002 | CI/CD 环境与工具链 | — |
| **P1** | SPEC-003 | 基础设施与认证授权 | SPEC-002 |
| **P2** | SPEC-004 | Agent 核心引擎（含安全审计） | SPEC-003 |
| **P2** | SPEC-006 | Artifact 存储与工作区 | SPEC-003, 004 |
| **P3** | SPEC-005 | 知识库系统 | SPEC-004 |
| P2～3 | SPEC-015 | 数据分析 Logic 层 | SPEC-003, 004, 005 |
| P2～4 | SPEC-007 | Skill 实现层 | SPEC-004, 005, 006, 015 |
| P2～4 | SPEC-014 | 任务队列与调度基础设施 | SPEC-003, 004 |
| **P4** | SPEC-009 | 系统统计监控 | SPEC-004, 007, 014 |
| **P4** | SPEC-010 | IM 集成（飞书） | SPEC-004 |
| **P5** | SPEC-011 | Hermes 自由探索 | 独立 |
| **P5** | SPEC-012 | 管理后台 | SPEC-004, 009, 010, 014 |
| **P6** | SPEC-013 | 测试体系 | 全部 |

### 依赖流向（简化）

```
[前置] SPEC-002 (CI)
         │
         ▼
[P1]  SPEC-003 ─── Infrastructure
         │
    ┌────┴──────────────┐
    ▼                   ▼
[P2] SPEC-004       [P2] SPEC-006
    Agent Core          Artifact
    (+Security)         │
    │  │                │
    ▼  │                │
[P3]   │                │
SPEC-005│                │
   KB   │                │
    │   │                │
    ├───┘                │
    │                    │
    ▼                    │
[P2~3] SPEC-015 ─── 数据分析 Logic
    │                    │
    ├────────────────────┤
    ▼                    ▼
[P2~4] SPEC-007   [P2~4] SPEC-014
    Skills                 Task Queue
    │                      + Scheduler
    │                      │
    ├──────────────────────┤
    ▼                      │
[P4] SPEC-009 ─── 统计监控  │
    │                      │
    ├──────────────────────┤
    ▼                      │
[P4] SPEC-010 (IM)         │
    │                      │
    ├──────────────────────┘
    │
    ├──────► [P5] SPEC-012 (Admin)
    │
[P5] SPEC-011 (Hermes, 独立)
    │
    └───────┬───────┐
            │       │
            ▼       ▼
[P6]   SPEC-013 ─── 测试体系
```
