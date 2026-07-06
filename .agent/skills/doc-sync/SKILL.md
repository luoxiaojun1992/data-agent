---
name: doc-sync
description: |
  data-agent 项目文档同步更新技能。在完成任何实质性代码变更后，
  强制执行全量文档遍历检查，确保 README、架构文档、spec、memory 等所有
  文档与最新代码一致。触发词：docs、文档、readme、更新文档、doc sync。
agent_created: true
---

# Doc Sync — 文档同步检查清单

## Overview

在所有实质性代码变更完成后，**强制执行**全量文档遍历检查。不依赖记忆，不"觉得应该更新了"——必须逐项对照代码变更核对每一份文档。

> **核心原则**：代码变 = 文档变。任何漏更新的文档都是 Bug。

## When to Use

**强制触发**（完成以下任一操作后必须运行）：
- 新增/修改 Go 服务模块
- 新增/修改 Skill
- 新增/修改 API 端点
- 新增/修改 DB 集合
- 新增/修改 Docker 服务/端口
- 新增/修改 spec 文档
- 合并大分支到 `main`
- 架构图更新后

## 文档遍历清单

### 区域 A：项目根目录

```
□ README.md
  - 功能概览段落：是否有新功能需要添加？
  - 技术栈表格：是否新增依赖？
  - 目录结构：是否有新目录？
  - Roadmap/Spec 状态：SPEC 状态是否与实际一致？
  - 快速开始/部署说明：新服务的 Docker 步骤？

□ README.zh-CN.md
  - 与 README.md 同步更新
```

### 区域 B：docs/ 目录

```
□ docs/ARCHITECTURE.md / ARCHITECTURE.zh-CN.md
  - 服务列表：是否有新服务/模块？
  - 技术栈表格：依赖是否准确？
  - 关键设计决策：是否有新增决策？
  - 目录结构：新目录是否添加？

□ docs/PRD / RFC / Roadmap
  - 开发阶段是否已更新？
  - 道路图任务状态是否同步？
```

### 区域 C：.agent/memory/

```
□ .agent/memory/INDEX.md
  - 快速参考内容是否一致？
  - 新增文件是否已索引？

□ .agent/memory/ARCHITECTURE.md
  - 模块详解：是否新增模块？
  - 数据流：新服务交互是否加入？
  - 目录结构树：新目录是否添加？

□ .agent/memory/CONVENTIONS.md
  - 是否有新约定需要写入？
  - 被纠正的错误做法：是否有新增条目？

□ .agent/memory/MEMORY.md
  - 工程决策记录：本次变更是否有值得记录的决策？

□ .agent/memory/REUSABLE_PATTERNS.md
  - 是否有新的可复用模式？
```

### 区域 D：.agent/specs/

```
□ .agent/specs/INDEX.md
  - 所有 spec 状态是否一致？（设计中/已实现）
  - 新 spec 是否已添加索引行？

□ .agent/specs/<affected-spec>.md
  - spec 状态是否已更新？
  - 实现细节是否与 spec 设计一致？
```

□ .agent/memory/E2E_TESTING.md
  - data-testid 对照表：新组件是否添加？
  - 测试矩阵：新增用例是否添加行？

### 区域 E：.agent/ 根目录

```
□ .agent/AI_AGENT_COMMON_INSTRUCTIONS.md
  - 关键文件位置：新文件是否列出？
  - Skill 列表：新增 Skill 是否添加？
  - 修改约束：新保护区域是否需要声明？
```

### 区域 F：工作区记忆

```
□ .workbuddy/memory/MEMORY.md
  - 关键工程决策：本次变更的决策要点

□ .workbuddy/memory/YYYY-MM-DD.md
  - 每日工作日志：本次变更摘要（append-only）
```

### 区域 G：架构图

```
□ docs/images/01_system_architecture.dot
  - 新服务/模块是否已添加？
  - 箭头连接是否完整？

□ docs/images/01_system_architecture.png
  - PNG 是否重新编译？（dot -Tpng）
```

### 区域 H：知识图谱

代码结构变更后，运行 `graphify update .` 重建知识图谱（AST only，无需 API key）。

```
□ graphify-out/graph.json
  - 是否已执行 graphify update . ？
  - 节点/边/社区计数是否合理？
```

## 变更影响矩阵

| 变更类型 | 必查区域 |
|---------|---------|
| 新 Go 模块 | A, B, C2-C3, D, E, G, H |
| 新 Skill | A, C1, E |
| 新 API 端点 | A, C2, E |
| 新 DB 集合 | A, C2 |
| 新 Docker 服务 | A, B, C2, G |
| 新 spec | D |
| 架构图变更 | G |
| 代码重构 | H |

## 工作流

### Step 1: 理解变更 → Step 2: 对照清单检查 → Step 3: 提交

```bash
git add -A
git commit -m "docs: <英文简短描述>"
git push
```

## 常见遗漏 Top 5

1. README.md spec 状态未更新
2. .agent/specs/INDEX.md spec 状态不一致
3. docs/ARCHITECTURE.zh-CN.md 设计决策列表未更新
4. .agent/memory/ARCHITECTURE.md 目录树未加新目录
5. .agent/AI_AGENT_COMMON_INSTRUCTIONS.md Skill 列表未更新

## 引用的外部 Skill

| Skill | 使用场景 |
|-------|---------|
| `graphify` | 知识图谱更新 |
| `architecture-diagram` | 架构图更新 |
| `spec-writer` | Spec 文档编写/更新 |
