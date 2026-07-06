# Spec Index

所有设计规格文档的中央注册表，统一编号管理。

## 编号规则

- 格式: `SPEC-XXX`，三位数字，从 `001` 递增
- 每个独立设计文档分配一个编号，终身不可变
- Sub-directory 下的 spec 共享同一功能域

---

## 索引表

| 编号 | 标题 | 文件路径 | 状态 |
|------|------|----------|------|
| SPEC-001 | 项目初始化与文档架构 | [spec-001-project-init.md](spec-001-project-init.md) | ✅ 已实现 |
| SPEC-002 | CI/CD 环境与工具链 | [spec-002-ci-environment.md](spec-002-ci-environment.md) | 🚧 设计中 |
| SPEC-003 | Phase 1 — 基础设施与认证授权 | [spec-003-infrastructure.md](spec-003-infrastructure.md) | 🚧 设计中 |
| SPEC-004 | Phase 2 — Agent 核心引擎与服务 | [spec-004-agent-engine.md](spec-004-agent-engine.md) | 🚧 设计中 |
| SPEC-005 | Phase 3 — 知识库系统 | [spec-005-knowledge-base.md](spec-005-knowledge-base.md) | 🚧 设计中 |
| SPEC-006 | Phase 2 — Artifact 存储与工作区 | [spec-006-artifact-storage.md](spec-006-artifact-storage.md) | 🚧 设计中 |
| SPEC-007 | Skill 实现层 | [spec-007-skill-implementations.md](spec-007-skill-implementations.md) | 🚧 设计中 |
| SPEC-008 | Phase 4 — 安全审计与报告校验 | [spec-008-security-report-validation.md](spec-008-security-report-validation.md) | 🚧 设计中 |
| SPEC-009 | Phase 4 — 系统统计监控 | [spec-009-stats-monitoring.md](spec-009-stats-monitoring.md) | 🚧 设计中 |
| SPEC-010 | Phase 4 — IM 集成（飞书） | [spec-010-im-integration.md](spec-010-im-integration.md) | 🚧 设计中 |
| SPEC-011 | Phase 5 — Hermes 自由探索 | [spec-011-hermes-explore.md](spec-011-hermes-explore.md) | 🚧 设计中 |
| SPEC-012 | Phase 5 — 管理后台 | [spec-012-admin-dashboard.md](spec-012-admin-dashboard.md) | 🚧 设计中 |
| SPEC-013 | Phase 6 — 测试体系与生产部署 | [spec-013-testing-deploy.md](spec-013-testing-deploy.md) | 🚧 设计中 |

## 依赖关系

```
SPEC-002 (CI)
  ↓
SPEC-003 (Infra)
  ↓
SPEC-004 (Agent Core) ← depends: 003 ───┐
  ↓                                      │
SPEC-005 (KB) ← depends: 004 ────────────┤
SPEC-006 (Artifact) ← depends: 003,004 ──┤ ← 平行
  ↓                                      │
SPEC-007 (Skills) ← depends: 004,005,006 ┤
  ↓                                      │
SPEC-008 (Stats+Security) ← deps: 004,005,006,007──┤
  ↓                                      │
SPEC-009 (Monitoring) ← depends: 004,007 ┤
  ↓                                      │
SPEC-010 (IM) ← depends: 004 ────────────┤
  ↓                                      │
SPEC-011 (Hermes) ←─ 独立，无依赖
  ↓
SPEC-012 (Admin) ────────────────────┤ depends: 004,008,009,010
  ↓                                  │
SPEC-013 (Test + Deploy) ←── all ────┘
```

---

## 状态说明

| 状态 | 含义 |
|--------|------|
| 📋 待开始 | 设计未启动 |
| 🚧 设计中 | 正在编写设计文档 |
| 👀 评审中 | 设计完成，等待 Review |
| ✅ 已实现 | 设计已实现并验证 |
| 🗄️ 已废弃 | 不再适用的历史规格 |

---

## 添加新 Spec

1. 从索引表中分配下一个可用编号
2. 创建 `.md` 文件，头部格式：

```markdown
# {标题}

> **SPEC-XXX** | Status: 设计中

## 目标
...
```

3. 更新本索引表
