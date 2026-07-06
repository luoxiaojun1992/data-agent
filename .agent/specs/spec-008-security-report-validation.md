# Phase 4 — 安全审计层

> **SPEC-008** | Status: 设计中 | 依赖: SPEC-004

## 目标

实现 Agent 统一安全审计层：Input/Output/Tool Call 过滤 + 熔断器。这是覆盖所有 Agent 流量的基础设施横切层，非业务功能。

> 子 Agent 编排（A2A）已移入 SPEC-004（Agent 核心引擎），报告格式校验已移入 SPEC-007（Skill 实现层 — save_analysis_report）。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 就绪 |
| SPEC-003 | ✅/❌ | 安全过滤中间件框架已就绪（P1-13） |
| SPEC-004 | ✅/❌ | Agent Engine 可用（安全层嵌入 Agent 请求链路） |

## 背景

Roadmap Phase 4 Week 8，P4-10 ~ P4-11（安全审计层完整实现 + 熔断器）。P1-13 已建立安全过滤中间件框架，本 spec 实现完整的规则引擎和运行时保护。

## 详细设计

### 1. 安全审计层（Security Audit Layer）

- **Input Sanitization**：Regex + Keyword Filter + SQL 注入检测
- **Output Sanitization**：敏感信息脱敏（手机号/身份证/API Key）
- **Tool Call 审计**：拦截高风险 Skill 调用（如 workspace_exec 写敏感路径）
- **审计日志**：所有安全事件写入 `security_alerts` 集合
- **Hot-reload 安全规则**：Redis pub/sub 推送规则更新，无需重启服务

### 2. 熔断器（Circuit Breaker）

- 连续失败 N 次 → 打开（Open），拒绝后续请求
- 冷却时间后 → 半开（Half-Open），允许探测请求
- 探测成功 → 关闭（Closed），恢复正常
- 按 Skill 粒度独立熔断，避免单个 Skill 故障拖垮整个 Agent

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（security_alerts） |
| 是否影响现有 API | No（中间件层透明注入） |
| 性能影响 | Input/Output 过滤 < 2ms，熔断器判断 O(1) |
| 是否需要新增 Skill | No（安全层是基础设施，非 Skill） |
| 是否需要 E2E 测试 | 熔断器打开→拒绝请求→恢复 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `internal/domain/security/audit.go` | 安全审计引擎 | 新建 |
| `internal/domain/security/circuit_breaker.go` | 熔断器 | 新建 |
| `internal/domain/security/rules.yaml` | 默认安全规则 | 新建 |

## 验证标准

1. SQL 注入输入 → Input Sanitization 拦截并记录 alert
2. 输出含手机号 → Output Sanitization 脱敏为 `138****1234`
3. 连续 5 次 Skill 执行失败 → 熔断器打开，返回 503
4. 冷却 30s 后 → 探测请求成功 → 熔断器关闭
5. Redis pub/sub 推送新规则 → 无需重启，即时生效
