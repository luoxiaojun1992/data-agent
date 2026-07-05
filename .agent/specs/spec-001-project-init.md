# 项目初始化与文档架构

> **SPEC-001** | Status: ✅ 已实现

## 目标

建立 DataAgent 项目的标准化文档架构，采用 doc-architect 标准的 Hub-and-Spoke 架构，以 `.agent/` 为 Single Source of Truth。确保 AI Agent 和人类开发者有一致的上下文来源。

## 背景

项目仓库从零创建，需要建立文档体系以支撑后续 12 周的 MVP 开发。已有完整的设计文档（PRD/RFC/Roadmap/UI 原型），需要规范化组织。

## 设计

### 文档架构

采用三层文档结构：

| 层级 | 受众 | 位置 | 用途 |
|-------|----------|----------|------|
| Public Entry | 全部用户 | `README.md`, `docs/` | 项目概述、快速开始、架构 |
| Agent Internal | AI Agent | `.agent/` | SSOT 指令、记忆、规格、Skill |
| Runtime Indexes | WorkBuddy | `.workbuddy/`, `.github/` | 指向 `.agent/` 的薄索引 |

### 核心原则

1. **SSOT**: `.agent/AI_AGENT_COMMON_INSTRUCTIONS.md` 是唯一权威
2. **Hub-and-Spoke**: 中央指令文件 → 专业子文档，无循环引用
3. **薄索引**: `.workbuddy/` 和 `.github/` 不得包含实质内容，仅指向 `.agent/`
4. **双语**: 公开文档中英双语（README.md + README.zh-CN.md）
5. **SPEC 驱动**: 所有功能以编号 Spec 跟踪，有生命周期状态

### 文件清单

| 文件 | 状态 |
|------|--------|
| `README.md` | ✅ 已创建 |
| `README.zh-CN.md` | ✅ 已创建 |
| `.agent/AI_AGENT_COMMON_INSTRUCTIONS.md` | ✅ 已创建 |
| `.agent/memory/INDEX.md` | ✅ 已创建 |
| `.agent/memory/ARCHITECTURE.md` | ✅ 已创建 |
| `.agent/memory/CONVENTIONS.md` | ✅ 已创建 |
| `.agent/memory/MEMORY.md` | ✅ 已创建 |
| `.agent/memory/REUSABLE_PATTERNS.md` | ✅ 已创建 |
| `.agent/specs/INDEX.md` | ✅ 已创建 |
| `docs/ARCHITECTURE.md` | ✅ 已创建 |
| `docs/ARCHITECTURE.zh-CN.md` | ✅ 已创建 |
| `docs/PRD-企业数据分析Agent-MVP.md` | ✅ 已迁移 |
| `docs/RFC-企业数据分析Agent-技术方案.md` | ✅ 已迁移 |
| `docs/Roadmap-企业数据分析Agent-MVP.md` | ✅ 已迁移 |
| `docs/UI原型设计文档.md` | ✅ 已迁移 |
| `.workbuddy/AI_AGENT_INSTRUCTIONS.md` | ✅ 已创建 |
| `.github/copilot-instructions.md` | ✅ 已创建 |

### 迁移说明

原有 `outputs/` 目录下的设计文档已迁移至 `docs/`：
- `PRD-企业数据分析Agent-MVP.md` → `docs/PRD-企业数据分析Agent-MVP.md`
- `RFC-企业数据分析Agent-技术方案.md` → `docs/RFC-企业数据分析Agent-技术方案.md`
- `Roadmap-企业数据分析Agent-MVP.md` → `docs/Roadmap-企业数据分析Agent-MVP.md`
- `UI原型设计文档.md` → `docs/UI原型设计文档.md`
- `diagrams/` → `docs/images/`（后续迁移）
- `screenshots/` → `docs/images/screenshots/`（后续迁移）

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|-----------------|
| `.agent/AI_AGENT_COMMON_INSTRUCTIONS.md` | SSOT 核心指令 | 新建 |
| `.agent/memory/ARCHITECTURE.md` | 详细架构文档 | 新建 |
| `.agent/memory/CONVENTIONS.md` | 编码规范与红线 | 新建 |
| `README.md` | 项目入口 | 新建 |

## 验证标准

- [x] SSOT 文件存在: `.agent/AI_AGENT_COMMON_INSTRUCTIONS.md`
- [x] README.md 包含所有标准章节
- [x] `.agent/memory/INDEX.md` 交叉引用所有记忆文件
- [x] `.agent/memory/MEMORY.md` 已创建并记录决策
- [x] `.agent/memory/ARCHITECTURE.md` 覆盖模块、数据流、关键文件
- [x] `.agent/specs/INDEX.md` 至少有 SPEC-001
- [x] `docs/ARCHITECTURE.md` 覆盖系统概述和技术栈
- [x] 薄索引指向 `.agent/`
- [x] 无断链的相对链接
- [x] README road map 与 spec INDEX 状态同步
