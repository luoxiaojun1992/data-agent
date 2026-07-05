# Spec Index

所有设计规格文档的中央注册表，统一编号管理。

## 编号规则

- 格式: `SPEC-XXX`，三位数字，从 `001` 递增
- 每个独立设计文档分配一个编号，终身不可变
- Sub-directory 下的 spec 共享同一功能域

---

## 索引表

| 编号 | 标题 | 文件路径 | 状态 |
|--------|-------|-----------|--------|
| SPEC-001 | 项目初始化与文档架构 | [spec-001-project-init.md](spec-001-project-init.md) | ✅ 已实现 |
| SPEC-002 | Phase 1: 基础设施与中间件 | [spec-002-infrastructure.md](spec-002-infrastructure.md) | 🚧 设计中 |
| SPEC-003 | Phase 2: 核心服务（Agent Engine + Chat） | [spec-003-core-services.md](spec-003-core-services.md) | 📋 待开始 |
| SPEC-004 | Phase 3: 知识库与解读 | [spec-004-knowledge-base.md](spec-004-knowledge-base.md) | 📋 待开始 |
| SPEC-005 | Phase 4: 高级功能 + 飞书 IM | [spec-005-advanced-features.md](spec-005-advanced-features.md) | 📋 待开始 |
| SPEC-006 | Phase 5: 管理后台 | [spec-006-admin-dashboard.md](spec-006-admin-dashboard.md) | 📋 待开始 |
| SPEC-007 | Phase 6: 测试与 Alpha 发布 | [spec-007-testing-release.md](spec-007-testing-release.md) | 📋 待开始 |

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
