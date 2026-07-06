---
name: spec-writer
description: |
  data-agent 项目的标准化 spec 设计文档编写规范。当用户提出"设计一个新功能"、"写spec"、
  "设计规范文档"、或任何新功能的设计/架构决策时使用。提供标准化的章节模板、编号规则、
  INDEX.md 更新流程。触发词：设计、spec、规范、设计文档、设计方案。
agent_created: true
---

# Spec Writer

## Overview

为 data-agent 项目编写标准化设计规范文档（spec）。确保所有 spec 文档遵循统一的章节结构、编号规则和完整性要求。

## When to Use

- 设计新功能或架构变更
- 用户说"写spec"、"设计规范"、"设计方案"
- 任何涉及多文件改动、新组件、新 API 的设计任务

## Spec 编号规则

格式：`SPEC-XXX`，三位数字从 `001` 递增。查看 `.agent/specs/INDEX.md` 获取当前最新编号。

## Spec 文件位置

```
.agent/specs/<kebab-case-name>.md
```

## 通用章节模板

### 文档头部（必需）

```markdown
# <中文标题>

> **SPEC-XXX** | Status: 设计中 / 已实现
```

### 1. 目标（必需）

1-3 句话说明要解决什么问题、实现什么功能。

### 1.5. 前置依赖检查（必需）

在目标之后，列出此 spec 依赖的所有前置 spec 及其状态检查表：

```markdown
## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-XXX | ✅/❌ | xxx 必须就绪 |
| — | — | 无前置依赖，可立即开始 |
```

> 此表在开发前必须逐项确认为 ✅，任一项为 ❌ 则阻塞当前 spec 开发。

### 2. 背景（条件）

现有实现的不足或问题，设计动机。纯新增功能可省略。

### 3. 架构概述（条件）

涉及系统架构、多模块交互的 spec 需要：
- 模块关系图（ASCII 或引用架构图）
- 与现有模块的对比表

### 4. API 设计（条件）

涉及 REST API 或 Skill 接口的 spec：

```markdown
## API 设计

| Method | Path | Description |
|--------|------|-------------|
| POST | /api/v1/xxx | xxx |
```

### 5. 详细设计（条件）

```markdown
## 详细设计

### 数据流
### 模块设计
### 数据模型（Go struct）
```

### 6. 可行性分析（必需）

```markdown
| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes/No |
| 是否影响现有 API | Yes/No |
| 性能影响 | 评估 |
| 是否需要新增 Skill | Yes/No |
```

### 7. 相关文件（必需）

```markdown
| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/xxx/xxx.go` | New module | New |
```

### 8. 测试策略（条件）

涉及代码变更的 spec 需要：

```markdown
1. **Unit tests**: xxx
2. **Integration tests**: xxx
3. **E2E tests**（条件，前端/Skill 涉及时）: xxx
```

### 9. UI Test / E2E 验收规则（必需，暂占位）

```markdown
## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`
```

### 10. 验证标准（必需）

可验证、可度量的标准列表。

## 章节选择速查表

| Spec 类型 | 必需章节 | 条件章节 |
|-----------|---------|---------|
| 新 Go 模块/服务 | 1, 6, 7, 9 | 2, 3, 4, 5, 8 |
| 新 Skill | 1, 4, 6, 7, 9 | 2, 5, 8 |
| API 变更 | 1, 4, 6, 7, 9 | 2, 5, 8 |
| 架构重构 | 1, 2, 3, 5, 6, 7, 8, 9 | — |
| 基础设施变更 | 1, 2, 3, 6, 7, 9 | 5, 8 |

## Spec 编写流程

1. 读取 `.agent/specs/INDEX.md`，取下一个编号
2. 根据速查表选择章节
3. 编写内容：先写目标 → 可行性分析前置验证 → 详细设计
4. 更新 `.agent/specs/INDEX.md` 索引表
5. 提交

```bash
git add .agent/specs/<file>.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-XXX <简短描述>"
```

## 格式约定

- 标题、说明用中文；代码、文件名保持英文
- 路径用反引号包裹
- Commit: `docs: add SPEC-XXX <英文描述>`
