# DataAgent - 系统架构

## 系统概述

DataAgent 是企业级智能数据分析平台，采用 **前后端分离的 B/S 架构**，后端使用 Go 语言单二进制部署。

### 核心服务

```
┌────────────────────────────────────────────────────────────┐
│                    Client Layer                             │
│  ┌──────────────┐  ┌──────────────────┐                    │
│  │ Web Frontend │  │  Admin Dashboard │  ┌─────────────┐  │
│  │ (React/Next) │  │  (React/Next)    │  │ 飞书机器人   │  │
│  └──────┬───────┘  └────────┬─────────┘  └──────┬──────┘  │
└─────────┼───────────────────┼──────────────────┼──────────┘
          │                   │                   │
┌─────────┼───────────────────┼──────────────────┼──────────┐
│         │          API Gateway Layer            │          │
│  ┌──────┴──────────────────────────────────────┴────────┐  │
│  │          HTTP/WebSocket Gateway + Auth               │  │
│  │    (JWT Auth, Rate Limit, Request Validation)        │  │
│  └──────────────────────────┬───────────────────────────┘  │
└─────────────────────────────┼──────────────────────────────┘
                              │
┌─────────────────────────────┼──────────────────────────────┐
│                Service Layer (Go — 单二进制)                 │
│  ┌──────────────────────────┴───────────────────────────┐  │
│  │                    Router & Middleware                 │  │
│  │  Auth/RBAC | Audit | Security Filter                  │  │
│  └──────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────┐  ┌──────────────┐  ┌───────────────┐  │
│  │ Agent Service │  │ Scheduler    │  │ Admin Service │  │
│  │ ★中央协调器   │  │ (Cron触发)   │  │ (管理后台API) │  │
│  │ · Chat 同步   │  │              │  │               │  │
│  │ · Agent 同步  │  └──────┬───────┘  └───────────────┘  │
│  │ · Session     │         │                              │
│  │ · Progress    │  ┌──────┴────────┐                     │
│  └───────┬───────┘  │ Worker Pool   │                     │
│          │          │ (goroutine)   │                     │
│  ┌───────┴──────────┤ Redis Stream  │                     │
│  │ Agent Engine     │ Consumer      │                     │
│  │ (ADK)            └───────────────┘                     │
│  │ · LLM Router                                          │
│  │ · Skill Registry                                       │
│  │ · Security Audit                                       │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌───────────────────────────────────────────────────────┐  │
│  │              Plugin / MCP Registry                    │  │
│  │  SQL Exec | Stats Engine | Email | OpenAPI→MCP        │  │
│  │  Knowledge Search | Save Report | Save Artifact       │  │
│  └───────────────────────────────────────────────────────┘  │
│                                                             │
│  ┌──────────────┐  ┌───────────────────────────────────┐  │
│  │ Hermes Svc   │  │ IM 模块 (internal/service/im/)     │  │
│  │ (独立服务)   │  │ 飞书 Webhook + 消息路由 (集成在主二进制)│  │
│  └──────────────┘  └───────────────────────────────────┘  │
└─────────────────────────────┬───────────────────────────────┘
                              │
┌─────────────────────────────┴───────────────────────────────┐
│                   Data / Storage Layer                       │
│  MongoDB | Milvus | SeaweedFS | Redis | Mem0 | Vault        │
└─────────────────────────────────────────────────────────────┘
```

## 模块分解

### Agent Service
- **路径**: `internal/service/agent/`, `internal/service/chat/`
- **职责**: 系统中央协调器。Chat 同步交互、Agent 同步/异步任务、Session 生命周期管理、任务进度上报、任务取消
- **关键文件**:
  - `internal/service/chat/chat_service.go`: Chat SSE 流式响应
  - `internal/service/agent/agent_service.go`: Agent 任务创建与执行
  - `internal/service/agent/session.go`: Session 管理器
- **依赖**: Agent Engine, Worker Pool, Redis Stream, MongoDB

### Agent Engine (ADK)
- **路径**: `internal/domain/agent/`
- **职责**: LLM 推理编排、Tool/Skill 调用、Agent System Prompt 管理
- **关键文件**:
  - `internal/domain/agent/engine.go`: Agent 引擎核心
  - `internal/domain/agent/llm_router.go`: LLM 模型路由
  - `internal/domain/agent/system_prompt.go`: System Prompt 模板
- **依赖**: Skill Registry, LLM Router, Security Audit Layer

### Skill Registry
- **路径**: `internal/domain/skill/`, `skills/`
- **职责**: Skill 注册、发现、调用。自动扫描 `skills/` 目录加载 Skill
- **关键文件**:
  - `internal/domain/skill/registry.go`: Skill 注册中心
  - `internal/domain/skill/context.go`: SkillContext 注入机制
- **依赖**: 各 Skill 实现

### Worker Pool
- **路径**: `internal/worker/`
- **职责**: 异步任务消费。从 Redis Stream 消费 Agent 任务和定时任务，回调 Agent Service 执行
- **关键文件**:
  - `internal/worker/pool.go`: Worker 池管理
  - `internal/worker/consumer.go`: Redis Stream 消费
  - `internal/worker/executor.go`: 任务执行调度
- **依赖**: Redis, Agent Service

### Scheduler Service
- **路径**: `internal/scheduler/`
- **职责**: Cron 定时触发，创建定时任务记录，写入 Redis Stream
- **关键文件**:
  - `internal/scheduler/cron.go`: Cron 调度器
  - `internal/scheduler/manager.go`: 调度任务 CRUD
- **依赖**: robfig/cron, Redis Stream, MongoDB

### Hermes Service
- **路径**: 独立服务（与 Agent Service 解耦）
- **职责**: 自由探索模式。转发用户请求到 Hermes API，SSE 流式透传，保存 hermes_sessions 快照
- **依赖**: Hermes API

### IM Gateway Service
- **路径**: 独立服务（与 Agent Service 低耦合）
- **职责**: 飞书 Webhook 接收、签名验证、用户绑定（open_id ↔ user_id）、消息路由到 Agent Service Chat API、结果卡片格式化、快捷指令解析、异步通知
- **关键文件**: `internal/service/im_gateway/`
- **依赖**: go-lark/lark SDK, Agent Service Chat API, MongoDB

### Admin Service
- **路径**: `internal/service/admin/`
- **职责**: 管理后台 API（Dashboard/用户（三��角色：system_admin/admin/user）/模型配置+系统配置（仅system_admin）/任务/知识库/审计日志/站内信/密码管理）
- **依赖**: MongoDB（notifications 集合）

### Logic 层
- **路径**: `internal/logic/`
- **职责**: Skill/Handler/Service 共用业务逻辑。无状态函数，幂等性内置
- **关键文件**:
  - `internal/logic/artifact.go`: Artifact 创建/删除（save_artifact & save_report 共用）
  - `internal/logic/report.go`: Report CRUD
  - `internal/logic/session.go`: Session 清理
  - `internal/logic/idempotent.go`: 通用幂等操作 Helper

## 数据流

### Chat 模式（同步）
```
User → API Gateway → Auth/RBAC → Security Filter
  → Agent Service → Agent Engine → [LLM Router → Tool Calls → KB Search]
  → Security Filter (Output) → Audit Log → SSE Stream → User
```

### Agent 异步模式
```
User → API Gateway → Auth/RBAC → Agent Service
  → Enqueue Redis Stream → 返回 task_id
  → Worker 消费 → 回调 Agent Service → Agent Engine → Execute
  → 结果存储 → Email/飞书通知 → User
```

### 定时任务
```
Cron 触发 → Scheduler 创建 ScheduledTask → XADD agent:tasks {type:scheduled}
  → Worker 消费 → 转换为 AgentTask → XADD agent:tasks {type:agent} (重入队)
  → Worker 再次消费 → 回调 Agent Service → 复用 Agent 异步管道
```

### 飞书 IM
```
飞书 @机器人 → Webhook → IM Gateway → 用户识别 (open_id→user_id)
  → Agent Service Chat API → Agent Engine → 结果
  → IM Gateway 格式化卡片 JSON → 飞书消息 API → User
```

## 目录结构

```
data-agent/
├── cmd/server/main.go        # 主入口（启动所有 goroutine）
├── internal/
│   ├── api/
│   │   ├── handler/          # HTTP Handler（参数校验+响应格式化）
│   │   ├── middleware/       # Auth/RBAC/Audit/Security/CORS
│   │   └── router/           # 路由注册
│   ├── logic/                # ★ Logic 层（Skill/Handler/Service 共用）
│   │   ├── sql/              # SQL 生成 + AST 安全校验
│   │   ├── stats/            # 统计分析（回归/PCA/聚类）
│   │   ├── knowledge/        # 混合搜索 + RRF 排序
│   │   ├── report/           # Markdown AST 报告校验
│   │   └── openapi/          # OpenAPI 3.0 解析器
│   ├── service/
│   │   ├── chat/             # Chat 模式业务逻辑
│   │   ├── agent/            # Agent 同步/异步任务入口
│   │   ├── admin/            # 管理后台业务逻辑
│   │   └── im/               # IM 消息收发
│   ├── domain/
│   │   ├── model/            # MongoDB 数据模型
│   │   ├── agent/            # Agent 引擎 (ADK)
│   │   ├── skill/            # Skill 注册中心 + SkillContext
│   │   ├── audit/            # 审计系统
│   │   └── security/         # 安全审查层（Input/Output/Tool Call 过滤 + 熔断器）
│   ├── worker/               # Worker Pool (goroutine)
│   ├── queue/                # Redis Stream 任务队列 + 死信队列
│   ├── scheduler/            # Cron 调度器 (robfig/cron)
│   ├── infra/                # 基础设施
│   │   ├── mongo/            # MongoDB Repository
│   │   ├── milvus/           # Milvus Client
│   │   ├── seaweedfs/        # SeaweedFS Client (S3)
│   │   ├── redis/            # Redis Client (Cache + Stream)
│   │   ├── mem0/             # Mem0 Client
│   │   └── email/            # 邮件客户端
│   └── config/               # 配置管理 (Viper)
├── skills/                   # Skill 实现目录
│   ├── sql-executor/
│   ├── stats-engine/
│   ├── email-sender/
│   ├── openapi-converter/
│   ├── knowledge-search/
│   ├── save-analysis-report/
│   ├── save-artifact/
│   └── workspace-read/write/exec/
├── frontend/                 # React/Next.js 前端
├── configs/                  # 配置文件 (YAML)
├── docs/                     # 公开文档
├── .agent/                   # AI Agent 指令 (SSOT)
├── docker-compose.yml
└── Makefile
```

## 数据库

| Collection | 用途 |
|-----------|------|
| `users` | 用户信息 + 角色 + 状态 |
| `roles` | 角色定义 + 权限列表 |
| `sessions` | 用户会话 (Chat/Agent) |
| `chat_messages` | 对话消息记录 |
| `agent_tasks` | Agent 批量任务 |
| `scheduled_tasks` | 定时任务定义 |
| `analysis_reports` | 分析报告（引用 artifact_id） |
| `knowledge_docs` | 知识库文档元数据 |
| `kb_chunks` | 知识库文档分片 + Embedding |
| `kb_index_tasks` | 知识库索引任务状态 |
| `artifacts` | Artifact 元数据 |
| `audit_logs` | 操作审计日志 |
| `im_bindings` | IM 用户绑定 (open_id ↔ user_id) |
| `model_configs` | LLM 模型配置 |
| `report_rules` | 报告格式校验规则 |

## Docker Compose 服务

| Service | Image | Port | 用途 |
|---------|-------|------|------|
| `mongodb` | mongo:7.0 | 27017 | 业务数据库 |
| `milvus` | milvusdb/milvus:2.4 | 19530 | 向量数据库 |
| `seaweedfs` | chrislusf/seaweedfs | 9333/8080 | 对象存储 |
| `redis` | redis:7.2 | 6379 | 缓存+消息队列 |
| `vault` | vault:1.18 | 8200 | 密钥管理 |
| `hermes-service` | - | - | 自由探索模式（转发层） |
| `im-gateway` | - | - | IM 消息网关（飞书） |
