# DataAgent - AI Agent Common Instructions

> 本文档是所有 AI Agent 指令的 **Single Source of Truth (SSOT)**。
> `.workbuddy/` 和 `.github/` 指令文件均为指向本文件的薄索引，禁止重复内容。

## 项目概述

DataAgent 是一个企业级智能数据分析平台，基于 Go 语言构建。用户通过 Chat 模式（实时对话）和 Agent 模式（批量任务）与数据交互，系统通过 LLM + MCP 工具链执行 SQL 查询、统计分析、知识库检索等操作，并将结果持久化存储。支持飞书 IM 集成，用户可在飞书中 @机器人 进行数据查询和分析。

## 技术栈

| 层级 | 技术 | 版本 |
|-------|-----------|-------|
| 后端语言 | Go | 1.22+ |
| Agent 框架 | google.golang.org/adk | latest |
| 业务数据库 | MongoDB | 7.0+ |
| 向量数据库 | Milvus | 2.4+ |
| 对象存储 | SeaweedFS | RELEASE.2024+ |
| 缓存/消息队列 | Redis | 7.2+ |
| 密钥管理 | HashiCorp Vault | 1.18+ |
| 记忆系统 | Mem0 | latest |
| 前端 | React / Next.js | 18 / 14 |
| 飞书 SDK | go-lark/lark | latest |
| 结构化日志 | uber-go/zap | latest |
| SQL AST 解析 | pingcap/tidb/parser | latest |
| 统计计算 | gonum | latest |

## 架构核心原则

### 1. 单二进制部署
- 整个 Service Layer 编译为单个 Go 二进制文件
- Worker 和 Scheduler 在同一进程中以独立 goroutine 运行
- 通过 Redis Stream 协调异步任务

### 2. Agent Service 是唯一入口
- Chat 和 Agent 两种模式共享同一 Agent Engine 实例
- Session 统一由 Agent Service 管理
- Hermes Service（自由探索模式）例外：请求直接转发 Hermes，不经过 Agent Service

### 3. 三层架构（Handler → Service → Repository）
- Handler：参数校验、权限前置检查、响应格式化
- Service：业务逻辑编排、跨模块协调
- Repository：数据持久化、缓存操作
- Logic 层（`internal/logic/`）：Skill/Handler/Service 共用逻辑抽取，无状态函数

### 4. SkillContext 自动注入
- 所有 Skill 通过 `SkillContext` 自动获取 `SessionID`/`UserID`/`TaskID`
- Skill 不得接受外部传入的 `session_id` 参数
- 数据归属（session_id/user_id）由 framework 强制写入，Skill 无法覆盖

### 5. 幂等性要求
- 创建操作使用 MongoDB `$setOnInsert` upsert，支持安全重试
- 删除操作幂等：资源不存在直接返回成功，绝不返回 404
- 跨资源操作失败时执行 best-effort 回滚

### 6. 关键文件位置

| 功能 | 文件路径 |
|----------|-----------|
| 主入口 | `cmd/server/main.go` |
| Agent 引擎 | `internal/domain/agent/engine.go` |
| Skill 注册中心 | `internal/domain/skill/registry.go` |
| Chat Service | `internal/service/chat/` |
| Agent Service | `internal/service/agent/` |
| Worker Pool | `internal/worker/pool.go` |
| Logic 层 | `internal/logic/` |
| Skill 定义 | `skills/` |

## 编码规范

### 行为准则

**1. 先思考，后行动**
- 不要假设，不要隐藏困惑——暴露假设和疑问
- 存在多种理解时，先列出再选择

**2. 简洁优先**
- 只写解决问题所需的代码
- 不为单次使用创建抽象
- 不引入未要求的功能

**3. 精确修改**
- 只改必须改的
- 匹配现有代码风格
- 清理孤儿代码（未使用的 import/变量/函数）

**4. 目标驱动执行**
- 将任务转化为可验证的目标
- 多步骤任务：先出简要计划，逐步验证

### Go 特定规则
- 使用 `internal/` 包组织私有代码
- Repository 模式封装数据访问
- 错误通过 `fmt.Errorf("context: %w", err)` 包装传递
- 日志使用 `uber-go/zap` 结构化日志
- 配置通过 Viper 管理，支持 YAML + 环境变量覆盖

## 修改约束

### ⚠️ 禁止修改

1. **SkillContext 注入机制**: `internal/domain/skill/context.go` — 确保 Session 归属不可篡改
2. **SQL AST 安全校验白名单**: `skills/sql-executor/` — 仅允许 SELECT/DESCRIBE/SHOW/EXPLAIN
3. **幂等性模式**: `internal/logic/idempotent.go` — 所有资源 CRUD 必须遵循

### ✅ 可安全修改

1. **新增 Skill**: 在 `skills/` 下创建新目录，实现 Skill 接口即可
2. **新增 API Handler**: 在 `internal/api/handler/` 添加，遵循 Handler → Service → Repository 三层
3. **配置项**: `configs/` 下的 YAML 文件 + 环境变量

### ⚠️ 红线（强制性）

- **禁止 workaround**: 每个修复必须做根因分析
- **禁止绕过测试**: 不允许通过放宽断言让测试通过
- **契约完整性**: Tool 参数契约必须匹配当前 schema
- **禁止直连数据库**: 所有数据访问必须通过 Repository 层

## 测试规范

### 单元测试
```bash
go test ./internal/... -v -count=1
```

### 集成测试
```bash
go test ./tests/integration/... -v -tags=integration
```

### E2E 测试
```bash
# Playwright 端到端测试
cd frontend && npx playwright test
```

### Mock LLM Service
- 路径: `POST /mock/llm/v1/chat/completions`
- 使用 Redis List 存储 per-model 的 mock 响应队列

## 常用任务

### 新增一个 Skill
1. 在 `skills/<skill-name>/` 下创建 `skill.go`
2. 实现 `Skill` 接口（Name/Description/Parameters/Execute/Permissions）
3. Skill 自动加载器会扫描 `skills/` 目录并注册
4. 如需共用逻辑，抽取到 `internal/logic/`

### 新增一个 API 端点
1. 在 `internal/api/handler/` 添加 Handler 函数
2. 在 `internal/api/router/` 注册路由
3. Handler 完成参数校验 → 调用 Service 层 → 格式化响应

### 调试 Chat 流程
1. 查看日志: `grep "trace_id" logs/server.log`
2. 追踪 tool calls: `grep "invoking skill" logs/server.log`
3. 检查 Session: 查询 MongoDB `sessions` 集合

## 故障排查

### Agent 任务卡在 queued 状态
- 检查 Redis Stream 是否有积压: `redis-cli XLEN agent:tasks`
- 检查 Worker 是否存活: 查看 Worker goroutine 日志
- 检查 MongoDB 连接: `docker-compose logs mongodb`

### Milvus 搜索返回空结果
- 检查 Collection 是否存在: Python 脚本 `scripts/check_milvus.py`
- 检查 embedding 模型是否配置正确
- 检查 `doc_id` 过滤条件是否正确

### Skill 注册失败
- 检查 `skills/<name>/config.yaml` 格式
- 检查 Skill 的 `Name()` 是否与已注册 Skill 冲突
- 查看启动日志: `grep "Skill loaded" logs/server.log`

## 提交规范

```bash
feat: add xxx feature        # 新功能
fix: resolve xxx issue       # 修复
docs: update xxx docs        # 文档更新
test: add xxx test case      # 测试
refactor: restructure xxx    # 重构
chore: update dependencies   # 杂项
```

## 分支命名

| 类型 | 格式 | 示例 |
|------|--------|--------|
| 功能 | `feat/<spec>-<desc>` | `feat/spec-002-auth-middleware` |
| 修复 | `fix/<description>` | `fix/sql-ast-parse-error` |
| 文档 | `docs/<description>` | `docs/api-update` |

## 记忆索引

> 所有路径相对于仓库根目录。

| 文档 | 内容 |
|----------|---------|
| `.agent/memory/INDEX.md` | 主经验索引 |
| `.agent/memory/ARCHITECTURE.md` | 详细架构 |
| `.agent/memory/CONVENTIONS.md` | 编码规范与 bug 记录 |
| `.agent/memory/MEMORY.md` | 工程决策日志 |
| `.agent/specs/INDEX.md` | 设计规格注册表 |

## 内置 Skill 列表

| Skill | 用途 |
|-------|---------|
| `sql_executor` | SQL 查询执行（含 AST 安全校验） |
| `stats_engine` | 统计计算（回归/聚类/PCA/时间序列/财务分析） |
| `knowledge_search` | 知识库搜索（向量+全文混合） |
| `email_sender` | 邮件发送（域名白名单） |
| `openapi_converter` | OpenAPI → MCP Tool 转换 |
| `save_analysis_report` | 分析报告校验与保存 |
| `save_artifact` | Artifact 产物保存 |
| `workspace_read` | Session 工作区文件读取 |
| `workspace_write` | Session 工作区文件写入 |
| `workspace_exec` | Session 工作区脚本执行 |
| `prompt_enhance` | 无状态提示词增强 |

> **重要**: 启动时，始终先阅读 `.agent/memory/` 文件以了解项目上下文，而非依赖模型内置知识。
