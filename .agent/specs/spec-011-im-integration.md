# Phase 4 — IM 集成（飞书机器人）

> **SPEC-011** | Status: 设计中 | 依赖: SPEC-004（Chat API）

## 目标

实现飞书机器人 IM 集成，仅服务于轻量办公 Chat 模式。用户可在飞书中 @机器人 进行数据查询和分析。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 就绪 |
| SPEC-003 | ✅/❌ | 基础设施可用 |
| SPEC-004 | ✅/❌ | Agent Service Chat API 可用 |

## 背景

Roadmap Phase 4 Week 8 附加，P4-16 ~ P4-24，总计 ~22h。IM 模块集成在主二进制（`internal/service/im/`），不复用独立网关服务。

## 详细设计

### 1. IM 模块架构

- 集成在主二进制（`internal/service/im/`），非独立服务
- 飞书平台适配器实现 `PlatformAdapter` 接口
- 复用 Agent Service 的 MongoDB/Redis 连接池

### 2. 飞书适配器

- go-lark/lark SDK 集成
- Webhook 接收 + 签名验证 + 消息解密
- 消息收发：文本消息 + 卡片消息

### 3. 用户绑定

- `im_bindings` 集合：飞书 open_id ↔ 系统 user_id
- 未绑定用户 → 返回绑定引导卡片 → 跳转 Web 绑定页
- 一次性绑定 Token（5 分钟过期，TTL 自动清理）

### 4. 消息路由

- IM 消息 → Agent Service Chat API（内部调用，非 HTTP）
- 快捷指令支持：/分析 /查询 /周报 /帮助
- 分析结果 → 飞书卡片 JSON 格式化

### 5. 异步通知

- Agent 任务完成 → 飞书消息推送

### 6. 适用范围约束

> **仅限轻量办公 Chat 模式**：不接入 Agent 批量任务模式，不接入 Hermes 自由探索模式。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（im_bindings, im_bind_tokens, im_templates V1.1） |
| 是否影响现有 API | Yes（新增 GET/POST /api/v1/im/feishu/* 路由） |
| 性能影响 | Webhook 处理 < 50ms |
| 是否需要新增 Skill | No（复用 Chat API） |
| 是否需要 E2E 测试 | IM 绑定流程 + 卡片消息渲染 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `internal/service/im/gateway.go` | IM 模块入口 | 新建 |
| `internal/service/im/feishu/adapter.go` | 飞书适配器 | 新建 |
| `internal/service/im/feishu/webhook.go` | Webhook 处理 | 新建 |
| `internal/service/im/feishu/card.go` | 卡片消息模板 | 新建 |
| `internal/service/im/binding.go` | 用户绑定管理 | 新建 |
| `internal/service/im/command.go` | 快捷指令解析 | 新建 |
| `internal/service/im/formatter.go` | 消息格式化 | 新建 |

## 验证标准

1. 飞书 @机器人 "分析本月销售" → SSE 流式返回 → 卡片消息
2. 未绑定用户 → 收到绑定引导卡片 → 绑定成功后可正常使用
3. /分析 /查询 /周报 /帮助 四条快捷指令均可正确触发
4. Agent 任务完成后 → 飞书收到异步任务通知卡片
5. 消息来源 `"feishu_bot"` 写入审计日志
