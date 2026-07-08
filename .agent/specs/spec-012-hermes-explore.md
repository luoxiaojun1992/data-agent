# Phase 5 — Hermes 自由探索模式

> **SPEC-012** | Status: 已实现 | 依赖: 无（独立服务，不经 Agent Service）

## 目标

实现 Hermes 自由探索模式独立服务，允许用户绕过 Agent Service 进行无约束的 LLM 对话，适用于探索性分析场景。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| — | — | 无前置依赖（独立服务），可立即开始 |

## 背景

Roadmap Phase 5 Week 10 附加，P5-17 ~ P5-20，总计 ~9h。Hermes 是独立 Go 转发层，不经过 Agent Service，无 Tool/Skill 调用权限。

## 详细设计

### 1. Hermes Service

- Go 轻量转发层（`cmd/hermes/main.go`）
- 请求直接转发 Hermes LLM API（不经 Agent Service）
- SSE 流式透传
- 与 Agent Service 无耦合，独立部署

### 2. Session 管理

- MongoDB `hermes_sessions` 集合（上下文快照）
- 仅保存日志快照：session_id, user_input, hermes_output, tool_calls
- Session 生命周期由 Hermes 端管理

### 3. 前端探索模式 UI

- 模式切换（mode toggle: Chat / Agent / Explore）
- 输入转发到 Hermes 端点
- 状态指示（在线/离线）

### 4. Docker Compose 集成

- Hermes 容器 + 健康检查

### 5. 约束

- Hermes Session 与 Data Agent Session 完全隔离
- 探索模式下无法访问 Data Agent 的工具/知识库/MCP
- 仅做自由探索，不做数据分析任务

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（hermes_sessions） |
| 是否影响现有 API | No（独立服务） |
| 性能影响 | SSE 透传，无额外延迟 |
| 是否需要新增 Skill | No |
| 是否需要 E2E 测试 | 探索模式 UI 切换 + 对话用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `cmd/hermes/main.go` | Hermes 服务入口 | 新建 |
| `internal/service/hermes/` | Hermes 转发层 | 新建 |
| `frontend/src/components/ExploreMode/` | 探索模式 UI | 新建 |

## 验证标准

1. Hermes 服务独立启动，Health Check 通过
2. 探索模式下 SSE 流式返回 LLM 对话
3. hermes_sessions 记录上下文快照
4. 探索模式无法调用 Agent Skill
5. 前端 mode toggle 可切换 Chat / Agent / Explore
