# Phase 4 — 安全审计与报告校验

> **SPEC-008** | Status: 设计中 | 依赖: SPEC-004, SPEC-007

## 目标

实现安全审计层（Input/Output/Tool Call 过滤 + 熔断器）、子 Agent 编排（A2A）、报告格式校验（Markdown AST）。

> 高级统计分析（聚类/PCA/财务）和 OpenAPI→MCP 转换器已移至 SPEC-007（Skill 实现层）。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 就绪 |
| SPEC-003 | ✅/❌ | 基础设施可用 |
| SPEC-004 | ✅/❌ | Agent Engine 可用 |
| SPEC-007 | ✅/❌ | 全部 Skill 实现可用（安全层过滤 Skill 输入输出） |

## 背景

Roadmap Phase 4 Week 7-8，P4-05 ~ P4-11（安全/熔断/子Agent），P4-15（报告校验）。

## 详细设计

### 1. 安全审计层（Security Audit Layer）

- Input Sanitization（Regex + Keyword Filter）
- Output Sanitization
- Tool Call 审计
- 熔断器 Circuit Breaker
- Hot-reload 安全规则（Redis pub/sub）

### 2. 子 Agent 编排

- Sub-Agent 编排基础（A2A 协议）
- 子 Agent 会话生命周期管理

### 3. 报告格式校验

- Markdown AST 解析提取标题层级
- 验证章节存在性（摘要/数据来源/分析方法/关键指标/结论）
- Agent 修正循环：校验失败 → 返回 feedback → LLM 修正 → 重试（最多 3 次）

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（security_alerts, skill_configs, report_rules, report_validation_logs） |
| 是否影响现有 API | No |
| 性能影响 | 熔断器引入轻微延迟（< 5ms） |
| 是否需要新增 Skill | No（安全层是基础设施，非 Skill） |
| 是否需要 E2E 测试 | 报告校验失败→修正 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `internal/domain/security/` | 安全审计层 | 新建 |
| `internal/logic/report_validator.go` | 报告校验 | 新建 |

## 验证标准

1. 熔断器在连续失败 N 次后打开，恢复正常后自动关闭
2. 安全过滤层拦截 SQL 注入 / XSS / 敏感词
3. 报告校验：缺失章节被检测 → Agent 修正 → 通过（最多 3 次）
4. 安全规则 Hot-reload 生效（无需重启服务）
