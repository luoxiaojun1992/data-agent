# DataAgent — 企业级数据分析平台

企业级智能数据分析平台，通过自然语言与数据对话。普通员工可以快速查询和简单分析，专业分析师可以发起复杂的批量数据处理任务。系统结合企业知识库和行业数据对结果进行深度解读，让数据真正驱动决策。

## 功能概览

- **Chat 模式 (即时查询)**: 自然语言对话式数据查询、快捷提示词一键分析、消息美化渲染（工具调用卡片/SQL高亮/数据表格/图表内嵌）
- **Agent 模式 (批量任务)**: 异步批量分析任务（回归/聚类/时间序列/财务分析/多维聚合）、任务进度追踪、邮件+IM通知
- **共享知识库**: 多格式文档上传（PDF/Word/Excel/Markdown）、LLM 智能分片索引、向量语义搜索+全文搜索、分析结果自动解读
- **飞书 IM 集成**: 飞书机器人@消息查询和分析、卡片消息呈现结果、快捷指令（/分析 /查询 /周报）、异步任务通知
- **管理后台**: 可视化数据看板、用户与权限管理 (RBAC)、模型配置、审计日志、知识库管理
- **安全合规**: 输入输出安全审查、操作审计日志、SQL AST 安全校验（拒绝写入操作）、密钥 Vault 加密管理

## 技术栈

| 层级 | 技术 |
|-------|-----------|
| 后端语言 | Go 1.22+ |
| Agent 框架 | google.golang.org/adk |
| 业务数据库 | MongoDB 7.0+ |
| 向量数据库 | Milvus 2.4+ |
| 对象存储 | SeaweedFS |
| 缓存与消息队列 | Redis 7.2+ |
| 密钥管理 | HashiCorp Vault 1.18+ |
| 记忆系统 | Mem0 |
| 前端 | React 18 / Next.js 14 |
| IM SDK | go-lark/lark (飞书) |

## 快速开始

```bash
# 1. 克隆仓库
git clone git@github.com:luoxiaojun1992/data-agent.git
cd data-agent

# 2. 启动开发环境
docker-compose up -d

# 3. 构建并运行
make build
./bin/server

# 4. 访问 http://localhost:3000
```

## 关键环境变量

| 变量 | 默认值 | 描述 |
|----------|---------|-------------|
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB 连接字符串 |
| `REDIS_ADDR` | `localhost:6379` | Redis 地址 |
| `MILVUS_ADDR` | `localhost:19530` | Milvus 地址 |
| `SEAWEEDFS_MASTER` | `localhost:9333` | SeaweedFS 主节点地址 |
| `VAULT_ADDR` | `http://localhost:8200` | Vault 地址 |
| `JWT_SECRET` | (必填) | JWT 签名密钥 |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | OpenAI 兼容 API 地址 |
| `LLM_API_KEY` | (必填) | LLM 服务 API Key |

## 项目结构

```
data-agent/
├── cmd/server/           # 主服务入口（单二进制部署）
├── internal/
│   ├── api/handler/      # HTTP 处理器
│   ├── api/middleware/   # Auth/RBAC/审计中间件
│   ├── logic/            # 共用业务逻辑（Skill + Service 共用）
│   ├── service/          # 业务服务（chat/agent/scheduler/admin）
│   ├── domain/           # 领域模型 + Agent 引擎 + Skill 注册中心
│   ├── worker/           # 异步任务 Worker Pool
│   ├── infra/            # 基础设施（MongoDB/Milvus/Redis/SeaweedFS）
│   └── config/           # 配置管理
├── skills/               # Skill 定义（SQL/统计/邮件/知识库/工作区）
├── frontend/             # React/Next.js 前端
├── tests/ui/             # Playwright E2E 测试
├── configs/              # 配置文件
├── docs/                 # 公开文档
├── .agent/               # AI Agent 指令（SSOT）
│   ├── specs/            # 设计规格（13 个 spec）
│   ├── skills/           # AI Agent 技能（9 个 skill）
│   └── memory/           # Agent 记忆文件
├── docker-compose.yml
└── Makefile
```

## 开发路线图

| 规格编号 | Phase | 功能 | 状态 |
|------|:-----:|---------|--------|
| SPEC-001 | — | 项目初始化与文档架构 | ✅ 已实现 |
| SPEC-002 | 前置 | CI/CD 环境与工具链 | 🚧 设计中 |
| SPEC-003 | **P1** | 基础设施与认证授权 | 🚧 设计中 |
| SPEC-004 | **P2** | Agent 核心引擎与服务 | 🚧 设计中 |
| SPEC-006 | **P2** | Artifact 存储与工作区管理 | 🚧 设计中 |
| SPEC-005 | **P3** | 知识库系统 | 🚧 设计中 |
| SPEC-007 | P2～4 | Skill 实现层 | 🚧 设计中 |
| SPEC-008 | **P4** | 安全审计与报告校验 | 🚧 设计中 |
| SPEC-009 | **P4** | 系统统计监控 | 🚧 设计中 |
| SPEC-010 | **P4** | IM 集成（飞书机器人） | 🚧 设计中 |
| SPEC-011 | **P5** | Hermes 自由探索模式 | 🚧 设计中 |
| SPEC-012 | **P5** | 管理后台 | 🚧 设计中 |
| SPEC-013 | **P6** | 测试体系与生产部署 | 🚧 设计中 |

> 完整路线图详见：[Roadmap-企业数据分析Agent-MVP](docs/Roadmap-企业数据分析Agent-MVP.md)

## 架构文档

详见 [docs/ARCHITECTURE.zh-CN.md](docs/ARCHITECTURE.zh-CN.md)（中文）或 [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md)（英文）。

## 相关文档

| 文档 | 描述 |
|----------|-------------|
| [PRD-企业数据分析Agent-MVP](docs/PRD-企业数据分析Agent-MVP.md) | 产品需求文档 |
| [RFC-企业数据分析Agent-技术方案](docs/RFC-企业数据分析Agent-技术方案.md) | 技术方案 RFC |
| [Roadmap-企业数据分析Agent-MVP](docs/Roadmap-企业数据分析Agent-MVP.md) | 开发计划与任务分解 |
| [UI原型设计文档](docs/UI原型设计文档.md) | UI 原型设计规范 |

## 设计规格

所有功能设计规格文件维护在 [.agent/specs/](.agent/specs/INDEX.md) 中。
