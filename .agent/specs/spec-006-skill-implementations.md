# Skill 实现层

> **SPEC-006** | Status: 设计中 | 依赖: SPEC-004（Skill 接口）, SPEC-005（知识库基础设施）

## 目标

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 就绪 |
| SPEC-003 | ✅/❌ | 基础设施可用 |
| SPEC-004 | ✅/❌ | Skill 接口 + 注册中心可用 |
| SPEC-005 | ✅/❌ | GridFS + Milvus Collection 可用（knowledge_search 依赖） |

实现所有自定义 Skill 的完整逻辑，覆盖数据查询、统计分析、知识库搜索、结果保存、邮件通知、工作区管理、提示词增强等核心能力。本 spec 承接 SPEC-004 定义的 Skill 接口和注册机制，提供具体实现。

## 背景

Skill 是 Agent 执行任务的核心单元。SPEC-004 建立了 Skill 接口和注册中心，各 Skill 的具体实现分散在后续 Phase 中，但共享共同的 Logic 层和 SkillContext 注入机制。本 spec 集中覆盖所有基础 Skill 实现。

## 详细设计

### 1. SQL Executor

- MCP 对接企业数据库
- SQL AST 安全校验（pingcap/tidb/parser）：仅允许 SELECT/DESCRIBE/SHOW/EXPLAIN
- 结果缓存（Redis，相同查询 5min 内直接返回）
- 错误友好提示：表不存在/字段错误 → 返回可用表和字段列表

### 2. Stats Engine

- 回归分析（线性/多元，gonum）
- 时间序列分析（趋势/季节分解）
- 结果结构化 JSON 输出

### 3. Knowledge Search

- 混合搜索（Milvus 向量 + MongoDB 全文 + RRF 融合排序）
- 权限过滤：按用户角色裁剪搜索结果
- doc_id 隔离：跨文档不串数据
- 上下文截断：top-5 × 800 tokens

### 4. Save Analysis Report

- 报告格式校验（Markdown AST 提取标题层级）
- 验证章节存在性：摘要/数据来源/分析方法/关键指标/结论
- Agent 修正循环：校验失败 → 返回 feedback → LLM 修正 → 重试（最多 3 次）
- MongoDB `analysis_reports` 落库

### 5. Save Artifact

- 文件上传 SeaweedFS
- MongoDB `artifacts` 元数据记录
- 字段校验：name, mime_type, size_bytes

### 6. Workspace Manager

- `workspace_read`: 按 session_id 读取工作区文件列表和内容
- `workspace_write`: 写入文件到 SeaweedFS session workspace
- `workspace_exec`: 在 session workspace 中执行脚本（沙箱环境）
- Session 隔离：不同 session 的 workspace 互不可见

### 7. Prompt Enhancement（无状态）

- 用户输入 → LLM 生成 3 个增强版本建议
- 不创建 Session，不记录 user_id
- `prompt_enhancements` 集合记录增强日志（仅审计用）

### 8. Email Sender

- SMTP 邮件发送（gomail / go-mail）
- 域名白名单：仅允许发送到企业域名
- 异步发送（避免阻塞 Agent 流程）

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（analysis_reports, artifacts, prompt_enhancements） |
| 是否影响现有 API | No |
| 性能影响 | SQL Executor 缓存命中 < 5ms, KB Search < 200ms |
| 是否需要新增 Skill | Yes（全部以上 Skill） |
| 是否需要 E2E 测试 | Chat 模式 skill 调用链 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `skills/sql_executor/` | SQL 执行 | 新建 |
| `skills/stats_engine/` | 统计分析 | 新建 |
| `skills/knowledge_search/` | 混合搜索 | 新建 |
| `skills/save_analysis_report/` | 报告保存 | 新建 |
| `skills/save_artifact/` | Artifact 保存 | 新建 |
| `skills/workspace_read/` | 文件读取 | 新建 |
| `skills/workspace_write/` | 文件写入 | 新建 |
| `skills/workspace_exec/` | 脚本执行 | 新建 |
| `skills/prompt_enhance/` | 提示词增强 | 新建 |
| `skills/email_sender/` | 邮件发送 | 新建 |
| `internal/logic/` | 共用 Logic 层 | 新建 |

## 验证标准

1. SQL Executor 注入 `DROP TABLE` → AST 拦截拒绝
2. 知识库搜索返回 top-5 相关 chunk，按 doc_id 隔离
3. 报告校验：缺失章节被检测 → Agent 修正 → 通过（最多 3 次）
4. 邮件发送仅允许企业域名白名单
5. SkillContext 自动注入 SessionID/UserID/TaskID，Skill 无法覆盖
6. 全部 Skill 通过 Skill 自动加载器注册
