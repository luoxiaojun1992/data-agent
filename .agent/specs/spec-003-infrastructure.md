# Phase 1 — 基础设施与认证授权

> **SPEC-003** | Status: 设计中 | 依赖: SPEC-002

## 目标

搭建项目脚手架（Go Module、目录结构、Makefile）、Docker Compose 开发环境、中间件（JWT/RBAC/审计/安全过滤）、数据库连接层（MongoDB/Redis/Milvus/SeaweedFS）、Mem0 记忆系统、Vault 密钥管理。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-002 | ✅/❌ | CI Pipeline 必须就绪 |

## 背景

Roadmap Phase 1 (Week 1-2)，P1-01 ~ P1-16，总计 ~50h。

## 详细设计

### 1. 项目脚手架
- Go Module 初始化（`go mod init github.com/luoxiaojun1992/data-agent`）
- 标准目录结构：`cmd/`, `internal/`, `skills/`, `configs/`, `tests/`
- Makefile：`build`, `test`, `lint`, `docker-up`, `docker-down`

### 2. Docker Compose 开发环境
- MongoDB 7.0, Milvus 2.4, SeaweedFS, Redis 7.2, Vault 1.18, Mem0
- 健康检查 + 依赖顺序

### 3. 配置管理
- Viper + YAML (`configs/config.yaml`) + 环境变量覆盖
- `.env.example` 模板

### 4. 日志系统
- `uber-go/zap` 结构化日志：Debug/Info/Warn/Error 级别
- 请求级 trace_id 注入

### 5. 数据库连接层
- MongoDB: Repository Pattern 封装，幂等 upsert/delete
- Redis: 缓存 + Stream 操作封装
- Milvus: Collection 管理 + 向量搜索
- SeaweedFS: Bucket CRUD + 文件操作

### 6. 认证与授权
- JWT 中间件：签发、验证、刷新
- RBAC 引擎：角色定义、权限校验
- 审计日志中间件（MongoDB TTL 90 天）
- 安全过滤中间件框架
- CORS / Rate Limit 中间件

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（users, roles, permissions, audit_logs） |
| 是否影响现有 API | No（新建项目） |
| 性能影响 | 无 |
| 是否需要新增 Skill | No |
| 是否需要 E2E 测试 | 占位即可（纯后端基础设施） |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `cmd/server/main.go` | 入口 | 新建 |
| `internal/infra/` | 数据库连接层 | 新建 |
| `internal/api/middleware/` | 中间件 | 新建 |
| `internal/config/` | 配置管理 | 新建 |
| `docker-compose.yml` | 开发环境 | 新建 |
| `Makefile` | 构建脚本 | 新建 |

## 验证标准

1. `make docker-up` 启动全部 6 个服务，健康检查通过
2. JWT 签发/验证/刷新流程正常
3. RBAC 角色权限校验阻断未授权请求
4. 审计日志自动写入 MongoDB
5. `make build` 编译通过
