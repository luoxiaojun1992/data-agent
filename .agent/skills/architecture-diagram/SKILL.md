---
name: architecture-diagram
description: |
  data-agent 项目的 Graphviz .dot 架构图维护技能。当需要绘制、更新或修复系统架构图、
  转换 PNG 嵌入文档时使用。
  触发词：架构图、architecture diagram、dot 图、graphviz、更新架构图。
agent_created: true
---

# Architecture Diagram — Graphviz .dot

## Overview

维护 data-agent 项目的 Graphviz .dot 系统架构图（`docs/images/01_system_architecture.dot` 及对应 PNG）。
架构图为有向无环图，包含 Client Layer / API Gateway / Service Layer / Data Layer 等层级。

> **路径约定**：本文档中的脚本路径均为相对于 skill 安装目录的相对路径。

核心能力：
1. **代码浏览研究** — 读取 `.agent/` 文档和 `docker-compose.yml` 理解当前系统状态
2. **架构图绘制** — Graphviz .dot 语法编辑，含子图 (subgraph)、配色、节点形状、边标注
3. **编译渲染** — `dot -Tpng` 生成 PNG，嵌入架构文档

## When to Use

- 架构图需要更新（新增服务、模块、数据存储）
- 架构图排版问题（节点重叠、箭头混乱、文字溢出）
- .dot 编译失败需修复
- 架构文档中嵌入的 PNG 需要更新

## Workflow

### Phase 1: 浏览研究

先理解系统当前状态再动手：

```
1. 读取 .agent/AI_AGENT_COMMON_INSTRUCTIONS.md（项目全貌 + 技术栈）
2. 读取 .agent/memory/ARCHITECTURE.md（详细架构）
3. 读取 docker-compose.yml（服务列表、端口、依赖）
4. 读取 docs/ARCHITECTURE.zh-CN.md（对外架构描述）
```

### Phase 2: .dot 文件编辑

架构图源文件位于 `docs/images/01_system_architecture.dot`，遵循以下规则：

**布局规则**：
- 使用 `rankdir=TB`（上→下）布局
- 每个层级用 `subgraph cluster_xxx` 表示
- 层内节点使用 `shape=box`（服务）、`shape=cylinder`（数据存储）
- `compound=true` 支持跨子图连线

**配色规则**（当前项目主题）：
- Client Layer: `#E3F2FD` 底色，`#1976D2` 边框
- Service Layer: `#E8F5E9` 底色，`#388E3C` 边框
- Data Layer: `#F3E5F5` 底色，`#7B1FA2` 边框
- Plugin Layer: `#FFEBEE` 底色，`#D32F2F` 边框
- 节点填充: 浅色 `fillcolor="#90CAF9"`（蓝）/ `#66BB6A`（绿）/ `#CE93D8`（紫）/ `#FFCDD2`（红）
- 关键节点: 深色填充 + 白色文字 `fontcolor="white"`
- Redis Stream 等核心组件: 红色/橙色强调

**边（箭头）规则**：
- 主数据流: `penwidth=2-3`
- 辅助/缓存连接: `style=dashed`
- 不同方向用不同颜色: 蓝 `#1976D2`、绿 `#388E3C`、橙 `#BF360C`、紫 `#7B1FA2`
- 跨子图连线使用 `lhead`/`ltail` 属性

### Phase 3: 编译 & 验证

```bash
# 编译为 PNG
dot -Tpng docs/images/01_system_architecture.dot -o docs/images/01_system_architecture.png

# 检查编译错误
dot -Tpng docs/images/01_system_architecture.dot > /dev/null 2>&1 || echo "Dot compile error!"
```

> **依赖**: Graphviz (`brew install graphviz` on macOS)

### Phase 4: 增量更新时的检查清单

- [ ] 新服务/模块是否已在对应 cluster 中添加
- [ ] 新数据存储是否已添加
- [ ] 箭头连接是否完整（新模块的上游/下游）
- [ ] 节点 label 文字是否准确
- [ ] `penwidth`/`color` 是否符合模块重要程度
- [ ] PNG 已重新编译

### Phase 5: 提交

```bash
git add docs/images/01_system_architecture.dot docs/images/01_system_architecture.png docs/ARCHITECTURE.zh-CN.md
git commit -m "docs: update architecture diagram for <变更描述>"
git push
```

## 常见踩坑

| 问题 | 原因 | 解决 |
|------|------|------|
| `dot` command not found | Graphviz 未安装 | `brew install graphviz` |
| `compound=true` 后连线看不到 | 需要 `lhead`/`ltail` 指向 cluster 名 | 添加 `lhead=cluster_xxx` |
| 子图 label 不显示 | 缺少 `label=""` 或 `fontname` 不匹配 | 检查 `fontname="Helvetica"` |
| 中文乱码 | macOS 默认字体不支持中文 | 用英文 label，或指定中文字体 |
| HTML-like label 中 `<br/>` 换行 | 节点标签 `label=<line1<br/>line2>` 需用尖括号包裹 | 使用 `<...>` 代替 `"..."` |

## References

- `.agent/AI_AGENT_COMMON_INSTRUCTIONS.md` — 项目全貌
- `.agent/memory/ARCHITECTURE.md` — 详细架构
- `docs/ARCHITECTURE.zh-CN.md` — 对外架构文档
- `docker-compose.yml` — 服务端口与依赖
- Graphviz 官方文档: https://graphviz.org/documentation/
