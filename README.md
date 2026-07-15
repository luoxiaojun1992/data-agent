# DataAgent — 企业级数据分析平台

企业级智能数据分析平台，通过自然语言与数据对话。普通员工可以快速查询和简单分析，专业分析师可以发起复杂的批量数据处理任务。系统结合企业知识库和行业数据对结果进行深度解读，让数据真正驱动决策。

## Feature Overview

- **Chat 模式 (即时查询)**: 自然语言对话式数据查询、快捷提示词一键分析、消息美化渲染（工具调用卡片/SQL高亮/数据表格/图表内嵌）
- **Agent 模式 (批量任务)**: 异步批量分析任务（回归/聚类/时间序列/财务分析/多维聚合）、任务进度追踪、邮件+IM通知
- **共享知识库**: 多格式文档上传（PDF/Word/Excel/Markdown）、LLM 智能分片索引、向量语义搜索+全文搜索、分析结果自动解读
- **飞书 IM 集成**: 飞书机器人@消息查询和分析、卡片消息呈现结果、快捷指令（/分析 /查询 /周报）、异步任务通知
- **管理后台**: 可视化数据看板、用户与权限管理 (RBAC)、模型配置、审计日志、知识库管理
- **安全合规**: 输入输出安全审查、操作审计日志、SQL AST 安全校验（拒绝写入操作）、密钥 Vault 加密管理

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend Language | Go 1.22+ |
| Agent Framework | google.golang.org/adk |
| Business Database | MongoDB 7.0+ |
| Vector Database | Qdrant 2.4+ |
| Object Storage | SeaweedFS |
| Cache & Message Queue | Redis 7.2+ |
| Secrets Management | HashiCorp Vault 1.18+ |
| Memory System | Mem0 |
| Frontend | React 18 / Next.js 14 |
| IM SDK | go-lark/lark (飞书) |

## Quick Start

```bash
# 1. Clone the repository
git clone git@github.com:luoxiaojun1992/data-agent.git
cd data-agent

# 2. Start development environment
docker-compose up -d

# 3. Build and run
make build
./bin/server

# 4. Visit http://localhost:3000
```

## Key Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `MONGO_URI` | `mongodb://localhost:27017` | MongoDB connection string |
| `REDIS_ADDR` | `localhost:6379` | Redis address |
| `QDRANT_URL` | `localhost:6334` | Qdrant address |
| `SEAWEEDFS_MASTER` | `localhost:9333` | SeaweedFS master address |
| `VAULT_ADDR` | `http://localhost:8200` | Vault address |
| `JWT_SECRET` | (required) | JWT signing secret |
| `LLM_BASE_URL` | `https://api.openai.com/v1` | OpenAI-compatible API base URL |
| `LLM_API_KEY` | (required) | API key for LLM service |

## Project Structure

```
data-agent/
├── cmd/server/           # Main server entry (single binary)
├── internal/
│   ├── api/handler/      # HTTP handlers
│   ├── api/middleware/   # Auth/RBAC/Audit middleware
│   ├── logic/            # Shared business logic
│   │   ├── sql/          # SQL generation + AST validation
│   │   ├── stats/        # Statistics (regression, PCA, clustering)
│   │   ├── knowledge/    # Hybrid search + RRF ranking
│   │   ├── report/       # Markdown AST report validation
│   │   └── openapi/      # OpenAPI 3.0 parser
│   ├── service/          # Business services (chat/agent/admin/im)
│   ├── domain/           # Domain models + Agent engine + Skill registry + Security
│   ├── worker/           # Worker pool + task runner
│   ├── queue/            # Redis Stream task queue + dead letter
│   ├── scheduler/        # Cron scheduler (robfig/cron)
│   ├── infra/            # Infrastructure (MongoDB/Qdrant/Redis/SeaweedFS)
│   └── config/           # Configuration management
├── skills/               # Skill definitions (SQL/Stats/Email/Knowledge/Workspace)
├── frontend/             # React/Next.js frontend
├── tests/ui/             # Playwright E2E tests
├── configs/              # Configuration files
├── docs/                 # Public documentation
├── .agent/               # AI agent instructions (SSOT)
│   ├── specs/            # Design specifications (14 specs)
│   ├── skills/           # AI agent skills (11 skills)
│   └── memory/           # Agent memory files
├── docker-compose.yml
└── Makefile
```

## Roadmap

| Spec | Phase | Feature | Status |
|------|:-----:|---------|--------|
| SPEC-001 | — | Project Initialization & Doc Architecture | ✅ Done |
| SPEC-002 | Pre | CI/CD Environment & Toolchain | ✅ Done |
| SPEC-003 | **P1** | Infrastructure & Auth | ✅ Done |
| SPEC-004 | **P2** | Agent Core Engine (incl. Security) | ✅ Done |
| SPEC-005 | **P2** | Artifact Storage & Workspace | ✅ Done |
| SPEC-006 | **P3** | Knowledge Base System | ✅ Done |
| SPEC-007 | **P3** | Data Analysis Logic Layer | ✅ Done |
| SPEC-008 | **P4** | Skill Implementations | ✅ Done |
| SPEC-009 | **P4** | Task Queue & Scheduler Infrastructure | ✅ Done |
| SPEC-010 | **P4** | System Monitoring (Redis Stats) | ✅ Done |
| SPEC-011 | **P4** | IM Integration (Feishu Bot) | ✅ Done |
| SPEC-012 | **P5** | Hermes Explore Mode | ✅ Done |
| SPEC-013 | **P5** | Admin Dashboard | ✅ Done |
| SPEC-014 | **P6** | Testing | ✅ Done |
| SPEC-015 | **P7** | Code Audit Fix | ✅ Done |
| SPEC-016 | **P7** | Docker Compose Fix | ✅ Done |
| SPEC-017 | **P8** | UI E2E — Auth (AUTH) | ✅ Done |
| SPEC-018 | **P8** | UI E2E — Layout & Nav (LAYOUT) | ✅ Done |
| SPEC-019 | **P8** | UI E2E — Chat (CHAT) | ✅ Done |
| SPEC-020 | **P8** | UI E2E — Agent (AGENT) | ✅ Done |
| SPEC-021 | **P8** | UI E2E — Hermes (HERMES) | ✅ Done |
| SPEC-022 | **P8** | UI E2E — Dashboard (DASH) | ✅ Done |
| SPEC-023 | **P8** | UI E2E — User Mgmt (USER) | ✅ Done |
| SPEC-024 | **P8** | UI E2E — Role Mgmt (ROLE) | ✅ Done |
| SPEC-025 | **P8** | UI E2E — Model Config (MODEL) | ✅ Done |
| SPEC-026 | **P8** | UI E2E — System Config (SYSCONFIG) | ✅ Done |
| SPEC-027 | **P8** | UI E2E — Task Mgmt (TASK) | ✅ Done |
| SPEC-028 | **P8** | UI E2E — KB (KB) | ✅ Done |
| SPEC-029 | **P8** | UI E2E — Audit Log (AUDIT) | ✅ Done |
| SPEC-030 | **P8** | UI E2E — API Review (API) | ✅ Done |
| SPEC-031 | **P8** | UI E2E — Notifications (NOTIF) | ✅ Done |
| SPEC-032 | **P8** | UI E2E — Password (PWD) | ✅ Done |
| SPEC-033 | **P8** | UI E2E — Prompt Enhance (PROMPT) | ✅ Done |
| SPEC-034 | **P8** | UI E2E — IM Feishu (IM) | ✅ Done |
| SPEC-035 | **P8** | UI E2E — List Mgmt (LIST) | ✅ Done |
| SPEC-036 | **P8** | UI E2E — File Upload (UPLOAD) | ✅ Done |
| SPEC-037 | **P8** | UI E2E — Session Mgmt (SESSION) | ✅ Done |
| SPEC-038 | **P8** | UI E2E — Security (SEC) | ✅ Done |
| SPEC-039 | **P8** | UI E2E — RBAC (RBAC) | ✅ Done |
| SPEC-040 | **P8** | UI E2E — Responsive (RESP) | ✅ Done |
| SPEC-041 | **P8** | UI E2E — Error States (ERR) | ✅ Done |
| SPEC-042 | **P8** | UI E2E — E2E Scenarios (E2E) | 🚧 Designing |
| SPEC-043 | **P8 前置** | Mock Model Service | ✅ Done |
| SPEC-044 | **P9** | Invite-Only Registration | 🚧 Designing |

> Full roadmap details: [Roadmap-企业数据分析Agent-MVP](docs/Roadmap-企业数据分析Agent-MVP.md)

## Architecture Documentation

See [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) (English) or [docs/ARCHITECTURE.zh-CN.md](docs/ARCHITECTURE.zh-CN.md) (中文) for detailed architecture.

## Related Documents

| Document | Description |
|----------|-------------|
| [PRD-企业数据分析Agent-MVP](docs/PRD-企业数据分析Agent-MVP.md) | Product Requirements Document |
| [RFC-企业数据分析Agent-技术方案](docs/RFC-企业数据分析Agent-技术方案.md) | Technical RFC |
| [Roadmap-企业数据分析Agent-MVP](docs/Roadmap-企业数据分析Agent-MVP.md) | Development roadmap with task breakdown |
| [UI原型设计文档](docs/UI原型设计文档.md) | UI prototype design specification |

## Design Specs

All feature design specifications are maintained in [.agent/specs/](.agent/specs/INDEX.md).
