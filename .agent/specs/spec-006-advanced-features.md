# Phase 4 — 高级功能（统计/安全/IM）

> **SPEC-006** | Status: 设计中 | 依赖: SPEC-004, SPEC-005

## 目标

实现高级统计分析（聚类/PCA/财务）、安全审计层、OpenAPI→MCP 转换器、IM 集成（飞书机器人，仅 Chat 模式）、报告格式校验（Markdown AST）、Redis Stats 统计、MongoDB TTL 日志清理。

## 背景

Roadmap Phase 4 (Week 7-8) + Week 8 附加，P4-01 ~ P4-26, P4-16 ~ P4-24（IM），总计 ~90h。

## 详细设计

### 1. 高级统计分析
- 聚类分析（K-Means / gonum）
- 主成分分析（PCA / gonum）
- 财务分析 Skill：比率计算 + 趋势对比

### 2. 安全审计层（Security Audit Layer）
- Input Sanitization（Regex + Keyword Filter）
- Output Sanitization
- Tool Call 审计
- 熔断器 Circuit Breaker

### 3. OpenAPI → MCP 转换器
- OpenAPI 3.0 规范解析
- 自动生成 MCP Tool 定义
- 双重审核机制（管理员审批 + 域名白名单）
- 调用频率限制

### 4. IM 集成 — 飞书机器人
- 仅限轻量办公 Chat 模式，不接入 Agent/Hermes
- IM 模块集成在主二进制（`internal/service/im/`）
- go-lark SDK：Webhook 接收 + 签名验证 + 消息解密
- 用户绑定：飞书 open_id ↔ 系统 user_id（`im_bindings` 集合）
- 消息路由：IM → Agent Service Chat API（内部调用）
- 卡片消息：分析结果格式化为飞书卡片 JSON
- 快捷指令：/分析 /查询 /周报 /帮助

### 5. 报告格式校验
- Markdown AST 解析提取标题层级
- 验证章节存在性（摘要/数据来源/分析方法/关键指标/结论）
- Agent 修正循环：校验失败 → 返回 feedback → LLM 修正 → 重试（最多 3 次）

### 6. Redis Stats 统计
- Scheduler 定时直接写入 Redis（AOF+RDB 持久化）
- 指标：Agent调用/模型调用/Session/Task/Token消耗
- Dashboard ROI 计算：等效节省人时 / AI 总成本

### 7. MongoDB TTL 自动清理
- 日志类集合：审计日志 90 天 / 请求日志 30 天 / 通知 7 天 / Token消耗 90 天

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（security_alerts, pending_api_conversions, skill_configs, report_rules, report_validation_logs, im_bindings, im_bind_tokens, output_stats, roi_metrics） |
| 是否影响现有 API | Yes（新增 IM webhook + 飞书消息端点） |
| 性能影响 | 熔断器引入轻微延迟（< 5ms） |
| 是否需要新增 Skill | Yes（email_sender, openapi_converter, stats_engine 扩展） |
| 是否需要 E2E 测试 | IM 绑定流程 + 卡片消息渲染 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `internal/service/im/` | IM 模块 | 新建 |
| `skills/openapi_converter/` | OpenAPI→MCP | 新建 |
| `internal/logic/report_validator.go` | 报告校验 | 新建 |
| `internal/domain/security/` | 安全审计层 | 新建 |

## 验证标准

1. 聚类/PCA 分析返回正确统计结果
2. 报告校验：缺失章节被检测 → Agent 修正 → 通过
3. OpenAPI 规范上传 → 解析为 MCP Tools → 管理员审批 → 上线
4. 飞书 @机器人 "分析本月销售" → SSE 流式返回 → 卡片消息
5. Redis Stats 计数器正确记录 → Dashboard ROI 展示
6. 审计日志 90 天后自动过期删除
