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
```
