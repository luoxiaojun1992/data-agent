# Phase 2 — Agent 核心引擎与服务

> **SPEC-004** | Status: 设计中 | 依赖: SPEC-003

## 目标

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 就绪 |
| SPEC-003 | ✅/❌ | 基础设施（MongoDB/Redis/Milvus/SeaweedFS）可用 |

实现 Agent Engine（ADK）、LLM Router、Skill Registry、Chat Service（SSE 流式）、Agent Service（同步/异步）、Worker Pool、Scheduler、上下文窗口管理、Vault 密钥管理。

## 背景

Roadmap Phase 2 (Week 3-4)，P2-01 ~ P2-16 + P2-11a ~ P2-11c，总计 ~76h。这是系统核心——所有后续功能依赖此模块。

## 详细设计

### 1. Agent Engine (ADK)
- ADK Agent Engine 初始化与配置
- LLM Router：多模型切换（GPT-4o/Claude/Gemini）、参数配置
- Skill 接口定义：`Name()`, `Description()`, `Parameters()`, `Execute()`, `Permissions()`, `RateLimit()`
- Skill 自动加载器：扫描 `skills/` 目录注册

### 2. SQL Executor Skill
- MCP 对接数据库
- SQL AST 安全校验（pingcap/tidb/parser）— 仅允许 SELECT/DESCRIBE/SHOW/EXPLAIN
- 结果缓存（Redis，相同查询 5min 内直接返回）

### 3. Stats Engine Skill
- 回归分析（gonum）
- 时间序列、PCA、聚类、财务分析
- 复用 Skill 接口

### 4. Artifact 存储
- SeaweedFS 存储 workspace 文件
- `save_artifact` Skill：字段校验 + 文件落盘 + MongoDB 元数据

### 5. Chat Service
- SSE 流式响应
- Session Manager：创建、续期、过期清理
- 快捷提示词 CRUD + 按角色展示
- Prompt Enhancement Service：无状态，LLM 增强 → 填充输入框

### 6. Agent Service
- 统一入口：Chat 实时 + Agent 同步 + Agent 异步入队
- 异步任务：入队 Redis Stream → 立即返回 task_id
- Worker Pool：goroutine 消费 Stream → 回调 Agent Service
- 任务取消（Context Cancel）、进度上报（WebSocket）、完成通知（Email + In-app）

### 7. Scheduler
- robfig/cron 集成
- Cron 触发 → 创建 AgentTask → 入队 Redis Stream

### 8. 上下文窗口管理
- tiktoken-go token 计数
- 阈值 = max_context_tokens × 50%（从 model_configs 读取）
- LLM 摘要压缩（轻量模型，~100 tokens/次）
- KB 结果截断 top-5 × 800 tokens
- 长报告分段生成合并

### 9. Vault 密钥管理
- AES-256-GCM 加密
- 密钥轮转

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（sessions, agent_tasks, artifacts, model_configs, vault_secrets, prompts, prompt_enhancements, token_consumptions） |
| 是否影响现有 API | No |
| 性能影响 | Worker Pool 并发数需压测 |
| 是否需要新增 Skill | Yes（sql_executor, stats_engine, save_artifact, save_analysis_report, prompt_enhance, workspace_read/write/exec） |
| 是否需要 E2E 测试 | Chat 模式需登录→输入→SSE 流式返回 UI 用例 |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `internal/domain/agent/engine.go` | Agent Engine | 新建 |
| `internal/domain/skill/registry.go` | Skill Registry | 新建 |
| `internal/domain/skill/context.go` | SkillContext | 新建 |
| `internal/service/chat/` | Chat Service | 新建 |
| `internal/service/agent/` | Agent Service | 新建 |
| `internal/worker/pool.go` | Worker Pool | 新建 |
| `skills/{sql_executor,stats_engine,...}/` | Skill 实现 | 新建 |

## 验证标准

1. Chat 模式：发送消息 → SSE 流式返回 → 包含 tool_call 调用链
2. Agent 异步：创建任务 → 入队 → Worker 消费 → 结果持久化 → 通知
3. SQL Executor：注入 `DROP TABLE` → AST 拦截拒绝
4. Skill 自动加载：新增 Skill 目录 → 重启后自动注册
5. 上下文窗口：超阈值自动触发摘要压缩
