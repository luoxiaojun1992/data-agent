# Phase 4 — Skill 实现层

> **SPEC-008** | Status: 已实现 | 依赖: SPEC-004（Skill 接口/注册中心）, **SPEC-007（数据分析 Logic 层）**, SPEC-006（知识库基础设施）, SPEC-005（Artifact 存储 + 工作区管理）

## 目标

实现所有 **Skill 包装层**的具体代码。每个 Skill 只做：参数校验 → 权限检查 → 调用 Logic 层 / 基础设施 → 格式化输出。

> 数据分析核心 Logic（SQL/Stats/Knowledge/Report 校验）见 **SPEC-007**，必须先于本 spec 完成。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 就绪 |
| SPEC-003 | ✅/❌ | 基础设施可用 |
| SPEC-004 | ✅/❌ | Skill 接口 + 注册中心可用 |
| SPEC-007 | ✅/❌ | 数据分析 Logic 层可用 |
| SPEC-006 | ✅/❌ | GridFS + Milvus Collection 可用（knowledge_search 依赖） |
| SPEC-005 | ✅/❌ | Artifact 存储 + 工作区管理可用（save_artifact / workspace_* 依赖） |

## 背景

SPEC-004 定义了 Skill 接口和注册机制，SPEC-007 实现了数据分析核心 Logic，本 spec 负责把 Logic 封装成 Agent 可调用的 Skill。Skill 层代码应足够薄：不包含业务算法，只处理输入输出适配。

---

## 详细设计

每个 Skill 遵循统一四步模式：

```
1. 参数校验 → 2. 权限检查 → 3. 调用 Logic / 基础设施 → 4. 格式化输出
```

### 1. SQL Executor

- 参数校验：`query` 字段必填且为 string
- 权限检查：检查用户数据库连接权限
- 调用 Logic：`sql.ASTValidate()` → `sql.Execute()` / `sql.CacheGet()`
- 输出：表格数据 + 行数统计

### 2. Stats Engine

- 参数校验：`method` ∈ {linear_regression, time_series, kmeans, pca, financial_ratio}
- 调用 Logic：`stats.{Method}()` → 返回 JSON
- 输出：方法名称 + 参数 + 结果摘要 + 可选的图表数据

### 3. Knowledge Search

- 参数校验：`query` 必填，`top_k` 可选（默认 5）
- 调用 Logic：`knowledge.HybridSearch()` + 权限过滤
- 输出：文档片段列表 + 相似度分数 + 来源 doc_id

### 4. Save Analysis Report

- 参数校验：`title` / `content` 必填
- 调用 Logic：`report.Validate()` → 失败则返回 feedback（触发 Agent 修正）
- 数据落库：MongoDB `analysis_reports` 集合
- 输出：`{report_id, status: "passed" | "needs_fix", feedback}`

### 5. Save Artifact

- 参数校验：`name` / `mime_type` / `data` 必填
- 文件上传 SeaweedFS + MongoDB `artifacts` 集合写元数据
- 输出：`{artifact_id, url, size_bytes}`

### 6. Workspace Manager

- `workspace_read(session_id)`: 读取工作区文件列表和内容，按 session 隔离
- `workspace_write(session_id, filename, data)`: 写入文件到 SeaweedFS session workspace
- `workspace_exec(session_id, script)`: 在沙箱环境中执行脚本，输出 stdout/stderr
- Session 隔离保证：不同 session 的 workspace 互不可见

### 7. Prompt Enhancement

- 无状态：不创建 Session，不记录 user_id
- 调用 LLM：自然语言输入 → 生成 3 个增强版本建议
- 输出：`[{suggestion_text, score}]`
- 审计日志写入 `prompt_enhancements`（仅记录，不关联用户）

### 8. Email Sender

- 参数校验：`to` / `subject` / `body` 必填
- 域名白名单校验：仅允许发送到企业域名
- 异步发送：goroutine 发送 → 立即返回 `{status: "queued"}`
- 发送结果异步写入 `email_logs`

### 9. OpenAPI → MCP 转换器（远程代理 Skill，Phase 4）

```
OpenAPI 3.0 Spec (JSON/YAML)
  → Parse via kin-openapi
  → Extract: endpoints, methods, parameters, schemas
  → Generate MCP Tool Definition:
    - Tool Name: operationId
    - Tool Description: summary / description
    - Parameters: JSON Schema from OpenAPI schema
  → Create Dynamic Skill（内存中）
  → Submit for Admin Review
  → [Approved] → Register to SkillRegistry + 域名白名单
  → [Rejected] → Delete from pending list
```

**运行时执行**：
- Agent → `skill.Execute()` 直接调用（Go 函数调用，无 HTTP）
- Skill 内部构造 HTTP 请求 → 调用外部 API（BaseURL 来自 `servers[0].url`）
- 域名白名单：仅允许调用白名单内域名的 API
- 调用频率限制（QPS 可配置）
- 审计日志记录每次外部 API 调用

**DB 集合**: `pending_api_conversions`

| 字段 | 说明 |
|------|------|
| `_id` | 主键 |
| `spec_content` | OpenAPI 规范原文 |
| `tools[]` | 生成的 MCP Tool 定义列表 |
| `submitted_by` | 提交人 |
| `reviewed_by` | 审批人 |
| `status` | pending / approved / rejected |
| `created_at` | 创建时间 |

---

## SkillContext 注入规则

SPEC-004 定义的 `SkillContext` 在每次 Skill 调用时由 Agent Engine 自动注入：

```go
type SkillContext struct {
    SessionID string // 只读，不可覆盖
    UserID    string // 只读，不可覆盖
    TaskID    string // 只读，不可覆盖
    Metadata  map[string]any // 可读写，Skill 间共享上下文
}
```

- `SessionID` / `UserID` / `TaskID` 由 Engine 注入，Skill 实现层不可修改
- `Metadata` 用于 Skill 间传递中间结果（如 SQL Executor 的查询结果 → Stats Engine）

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（analysis_reports, email_logs, openapi_tools, prompt_enhancements） |
| 是否影响现有 API | No（Skill 内部实现，API 由 SPEC-004 Agent Service 暴露） |
| 性能影响 | 每个 Skill 包装层延迟 < 1ms |
| 是否需要新增 Skill | Yes（全部 9 个 Skill） |
| 是否需要 E2E 测试 | Chat → Skill 调用链 + 报告校验失败→修正 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `skills/sql_executor/main.go` | SQL 执行 Skill | 新建 |
| `skills/stats_engine/main.go` | 统计分析 Skill | 新建 |
| `skills/knowledge_search/main.go` | 知识库搜索 Skill | 新建 |
| `skills/save_analysis_report/main.go` | 报告保存 Skill | 新建 |
| `skills/save_artifact/main.go` | Artifact 保存 Skill | 新建 |
| `skills/workspace_read/main.go` | 工作区读取 Skill | 新建 |
| `skills/workspace_write/main.go` | 工作区写入 Skill | 新建 |
| `skills/workspace_exec/main.go` | 工作区执行 Skill | 新建 |
| `skills/prompt_enhance/main.go` | 提示词增强 Skill | 新建 |
| `skills/email_sender/main.go` | 邮件发送 Skill | 新建 |
| `skills/openapi_converter/main.go` | OpenAPI→MCP Skill | 新建（Phase 4） |
| `internal/logic/` | 共用 Logic 层（见 SPEC-007） | 引用 |

## 验证标准

1. `sql_executor` 注入 `DROP TABLE` → 拒绝（调用 SPEC-007 Logic 层校验）
2. `stats_engine` 数值含 NaN → Logic 层替换为友好提示，Skill 层正常返回
3. `knowledge_search` 返回 top-5 chunk，按 doc_id 隔离，无跨文档数据泄漏
4. `save_analysis_report` 缺失「结论」章节 → 返回 feedback → Agent 修正 → 重新校验通过
5. `email_sender` 域名不在白名单 → Skill 层拒绝
6. SkillContext 注入 `SessionID`，Skill 代码内尝试覆盖 → 编译失败（字段不可导出）
7. 全部 Skill 通过 Skill 自动加载器注册
8. OpenAPI 规范上传 → 解析为 MCP Tools → 管理员审批 → 自动注册上线
