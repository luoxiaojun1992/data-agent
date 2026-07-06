# Skill 实现层

> **SPEC-007** | Status: 设计中 | 依赖: SPEC-004（Skill 接口/注册中心）, SPEC-005（知识库基础设施）, SPEC-006（Artifact 存储 + 工作区管理）

## 目标

实现所有 Skill 的具体逻辑。本 spec 遵循 **Logic ↔ Skill 分层架构**：

```
┌─────────────────────────────────────┐
│  Skill 层（skills/）                │
│  - 参数校验 / 权限检查              │
│  - 调用 Logic 层                    │
│  - 格式化输出 / 错误处理            │
├─────────────────────────────────────┤
│  Logic 层（internal/logic/）        │
│  - 数据分析核心算法                 │
│  - SQL 构建与安全校验               │
│  - 混合搜索与重排序                 │
│  - 报告格式校验                     │
│  - OpenAPI 解析                     │
└─────────────────────────────────────┘
```

- **Logic 层**：可复用、可独立测试的数据分析核心。Skill 间可共享 Logic 组件（如 `sql_validator` 同时被 `sql_executor` 和 `save_analysis_report` 使用）。
- **Skill 层**：Agent 调用的接口适配。只做参数校验、权限检查、调用 Logic、格式化输出。不包含业务算法。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 就绪 |
| SPEC-003 | ✅/❌ | 基础设施可用 |
| SPEC-004 | ✅/❌ | Skill 接口 + 注册中心可用 |
| SPEC-005 | ✅/❌ | GridFS + Milvus Collection 可用（knowledge_search 依赖） |
| SPEC-006 | ✅/❌ | Artifact 存储 + 工作区管理可用（save_artifact / workspace_* 依赖） |

## 背景

Skill 是 Agent 执行任务的核心单元。SPEC-004 建立了 Skill 接口和注册机制，本 spec 实现所有 Skill 及其依赖的 Logic 层。基础 Skill 在 Phase 2 实现，高级 Skill（聚类/PCA/OpenAPI）在 Phase 4 实现。

---

## Part A: Logic 层（`internal/logic/`）

### A1. SQL Logic（`internal/logic/sql/`）

- SQL 安全校验：基于 pingcap/tidb/parser AST 解析，仅允许 `SELECT | DESCRIBE | SHOW | EXPLAIN`
- SQL 生成：根据自然语言描述 → 参数化 SQL
- 结果缓存：Redis `sql:cache:{hash}`，相同查询 5min 内直接返回
- 错误友好提示：表不存在 → 返回可用表名列表；字段错误 → 返回表结构

### A2. Stats Logic（`internal/logic/stats/`）

- **基础统计**（Phase 2）：回归分析（线性/多元，gonum）、时间序列分析（趋势/季节分解）
- **高级统计**（Phase 4）：聚类分析（K-Means）、主成分分析（PCA）、财务分析（比率计算 + 趋势对比）
- 结果统一输出：结构化 JSON `{method, parameters, result, summary}`
- 数值安全：NaN / Inf 检测 + 替换为友好提示

### A3. Knowledge Search Logic（`internal/logic/knowledge/`）

- 混合搜索：Milvus 向量相似度 + MongoDB 全文搜索 + RRF 融合排序
- 权限过滤：按用户角色裁剪搜索结果
- doc_id 隔离：跨文档不串数据
- 上下文截断：top-5 × 800 tokens（由 SPEC-004 上下文窗口管理调用）

### A4. Report Validation Logic（`internal/logic/report/`）

- Markdown AST 解析提取标题层级
- 验证章节存在性：摘要 / 数据来源 / 分析方法 / 关键指标 / 结论
- 校验失败 → 生成修正 feedback → 返回 Skill 层触发 Agent 重试循环（最多 3 次）
- 校验记录写入 `report_validation_logs`

### A5. OpenAPI Parser Logic（`internal/logic/openapi/`）

- OpenAPI 3.0 规范解析（路径/参数/响应 Schema）
- 自动生成 MCP Tool 定义（name / description / inputSchema）
- 域名白名单 + 管理员审批 → 上线
- 调用频率限制

---

## Part B: Skill 层（`skills/`）

每个 Skill 实现 SPEC-004 定义的 `Skill` 接口，遵循固定模式：

```
1. 参数校验 → 2. 权限检查 → 3. 调用 Logic 层 → 4. 格式化输出
```

### B1. SQL Executor

- 参数校验：`query` 字段必填且为 string
- 权限检查：检查用户数据库连接权限
- 调用 Logic：`sql.ASTValidate()` → `sql.Execute()` / `sql.CacheGet()`
- 输出：表格数据 + 行数统计

### B2. Stats Engine

- 参数校验：`method` ∈ {linear_regression, time_series, kmeans, pca, financial_ratio}
- 调用 Logic：`stats.{Method}()` → 返回 JSON
- 输出：方法名称 + 参数 + 结果摘要 + 可选的图表数据

### B3. Knowledge Search

- 参数校验：`query` 必填，`top_k` 可选（默认 5）
- 调用 Logic：`knowledge.HybridSearch()` + 权限过滤
- 输出：文档片段列表 + 相似度分数 + 来源 doc_id

### B4. Save Analysis Report

- 参数校验：`title` / `content` 必填
- 调用 Logic：`report.Validate()` → 失败则返回 feedback（触发 Agent 修正）
- 数据落库：MongoDB `analysis_reports` 集合
- 输出：`{report_id, status: "passed" | "needs_fix", feedback}`

### B5. Save Artifact

- 参数校验：`name` / `mime_type` / `data` 必填
- 调用 Logic：文件上传 SeaweedFS + MongoDB `artifacts` 集合写元数据
- 输出：`{artifact_id, url, size_bytes}`

### B6. Workspace Manager

- `workspace_read(session_id)`: 读取工作区文件列表和内容，按 session 隔离
- `workspace_write(session_id, filename, data)`: 写入文件到 SeaweedFS session workspace
- `workspace_exec(session_id, script)`: 在沙箱环境中执行脚本，输出 stdout/stderr
- Session 隔离保证：不同 session 的 workspace 互不可见

### B7. Prompt Enhancement

- 无状态：不创建 Session，不记录 user_id
- 调用 Logic：自然语言输入 → LLM 生成 3 个增强版本建议
- 输出：`[{suggestion_text, score}]`
- 审计日志写入 `prompt_enhancements`（仅记录，不关联用户）

### B8. Email Sender

- 参数校验：`to` / `subject` / `body` 必填
- 域名白名单校验：仅允许发送到企业域名
- 异步发送：goroutine 发送 → 立即返回 `{status: "queued"}`
- 发送结果异步写入 `email_logs`

### B9. OpenAPI → MCP 转换器

- 参数校验：`spec_url` 必填，检查 OpenAPI 版本 ≥ 3.0
- 调用 Logic：`openapi.Parse()` → `openapi.ToMCPTools()`
- 双重审核：管理员审批 + 域名白名单 → 上线
- 上线后 Skill Registry 自动注册生成的 MCP Tools

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
| 是否需要新 DB 集合 | Yes（analysis_reports, report_validation_logs, email_logs, openapi_tools, prompt_enhancements） |
| 是否影响现有 API | No（Skill 内部实现，API 由 SPEC-004 Agent Service 暴露） |
| 性能影响 | SQL Executor 缓存命中 < 5ms，KB Search < 200ms，Stats 计算 < 1s |
| 是否需要新增 Skill | Yes（全部 9 个 Skill） |
| 是否需要 E2E 测试 | Chat → Skill 调用链 + 报告校验失败→修正 UI 用例 |

## 相关文件

### Logic 层

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `internal/logic/sql/validator.go` | SQL AST 安全校验 | 新建 |
| `internal/logic/sql/executor.go` | SQL 执行与缓存 | 新建 |
| `internal/logic/stats/regression.go` | 回归分析 | 新建 |
| `internal/logic/stats/timeseries.go` | 时间序列 | 新建 |
| `internal/logic/stats/clustering.go` | 聚类分析（Phase 4） | 新建 |
| `internal/logic/stats/pca.go` | PCA（Phase 4） | 新建 |
| `internal/logic/stats/financial.go` | 财务分析（Phase 4） | 新建 |
| `internal/logic/knowledge/search.go` | 混合搜索 | 新建 |
| `internal/logic/report/validator.go` | 报告格式校验 | 新建 |
| `internal/logic/openapi/parser.go` | OpenAPI 解析 | 新建（Phase 4） |

### Skill 层

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

## 验证标准

1. SQL Executor 注入 `DROP TABLE` → AST 拦截拒绝（Logic 层校验，非 Skill 层）
2. Stats Engine 数值含 NaN → Logic 层替换为友好提示，Skill 层正常返回
3. 知识库搜索返回 top-5 chunk，按 doc_id 隔离，无跨文档数据泄漏
4. 报告缺失「结论」章节 → Report Logic 校验失败 → Skill 返回 feedback → Agent 修正 → 重新校验通过
5. 邮件发送域名不在白名单 → Skill 层拒绝
6. SkillContext 注入 `SessionID`，Skill 代码内尝试覆盖 → 编译失败（字段不可导出）
7. Logic 层独立单元测试：无需启动完整 Agent，直接测试算法正确性
8. OpenAPI 规范上传 → 解析为 MCP Tools → 管理员审批 → 自动注册上线
