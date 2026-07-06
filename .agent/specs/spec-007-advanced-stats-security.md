# Phase 4 — 高级统计与安全审计

> **SPEC-007** | Status: 设计中 | 依赖: SPEC-004, SPEC-005, SPEC-006

## 目标

实现高级统计分析（聚类/PCA/财务）、安全审计层、OpenAPI→MCP 转换器、报告格式校验（Markdown AST）。

## 背景

Roadmap Phase 4 Week 7-8 前半，P4-05 ~ P4-15，涵盖高级统计 Skill 扩展、安全层、OpenAPI 转换、报告校验、子 Agent 编排、熔断器。

## 详细设计

### 1. 高级统计分析
- 聚类分析（K-Means / gonum）
- 主成分分析（PCA / gonum）
- 财务分析 Skill：比率计算 + 趋势对比
- 复用 Stats Engine Skill 接口（SPEC-004 已建立）

### 2. 子 Agent 编排
- Sub-Agent 编排基础（A2A 协议）
- 子 Agent 会话生命周期管理

### 3. 安全审计层（Security Audit Layer）
- Input Sanitization（Regex + Keyword Filter）
- Output Sanitization
- Tool Call 审计
- 熔断器 Circuit Breaker
- Hot-reload 安全规则（Redis pub/sub）

### 4. OpenAPI → MCP 转换器
- OpenAPI 3.0 规范解析
- 自动生成 MCP Tool 定义
- 双重审核机制（管理员审批 + 域名白名单）
- 调用频率限制

### 5. 报告格式校验
- Markdown AST 解析提取标题层级
- 验证章节存在性（摘要/数据来源/分析方法/关键指标/结论）
- Agent 修正循环：校验失败 → 返回 feedback → LLM 修正 → 重试（最多 3 次）

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（security_alerts, pending_api_conversions, skill_configs, report_rules, report_validation_logs） |
| 是否影响现有 API | No |
| 性能影响 | 熔断器引入轻微延迟（< 5ms） |
| 是否需要新增 Skill | Yes（openapi_converter, stats_engine 扩展聚类/PCA/财务） |
| 是否需要 E2E 测试 | OpenAPI 审核页面 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `skills/openapi_converter/` | OpenAPI→MCP | 新建 |
| `internal/logic/report_validator.go` | 报告校验 | 新建 |
| `internal/domain/security/` | 安全审计层 | 新建 |
| `internal/logic/stats/` | 高级统计扩展 | 修改 |

## UI Test / E2E 验收规则

- [ ] OpenAPI 规范上传→解析→审批 流程 UI 用例
- [ ] 报告校验失败→Agent 修正→重新校验 循环 UI 用例

## 验证标准

1. 聚类/PCA 分析返回正确统计结果
2. 报告校验：缺失章节被检测 → Agent 修正 → 通过
3. OpenAPI 规范上传 → 解析为 MCP Tools → 管理员审批 → 上线
4. 熔断器在连续失败 N 次后打开，恢复正常后自动关闭
5. 安全过滤层拦截 SQL 注入 / XSS / 敏感词
