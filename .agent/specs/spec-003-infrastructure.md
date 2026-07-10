# Phase 1 — 基础设施与认证授权

> **SPEC-003** | Status: 已实现 | 依赖: SPEC-002

## 目标

搭建项目脚手架（Go Module、目录结构）、Docker Compose 开发环境、中间件（JWT/RBAC/审计/安全过滤）、数据库连接层（MongoDB/Redis/Qdrant/SeaweedFS）、Mem0 记忆系统、Vault 密钥管理、系统管理员账号自动生成、模型配置优先级。

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
- MongoDB 7.0, Qdrant 2.4, SeaweedFS, Redis 7.2, Vault 1.18, Mem0
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
- Qdrant: Collection 管理 + 向量搜索
- SeaweedFS: Bucket CRUD + 文件操作

### 6. 认证与授权

#### 6.1 用户角色层级（三级）

```
┌─────────────────────────────────┐
│  system_admin（系统管理员）       │  ← 唯一，拥有全部权限
│  - 管理所有用户、所有知识库文档    │
│  - 修改模型配置、系统配置          │
│  - 查看所有审计日志               │
│  - 管理所有 API 转换审核           │
├─────────────────────────────────┤
│  admin（普通管理员）              │
│  - 管理所有普通用户               │
│  - 管理属于自己的知识库文档        │
│  - 不能修改模型配置/系统配置       │
├─────────────────────────────────┤
│  user（普通用户）                 │
│  - 只能管理自己的知识库文档        │
│  - 不能管理其他用户               │
│  - 不能访问管理后台的用户管理      │
└─────────────────────────────────┘
```

#### 6.2 权限矩阵

| 操作 | system_admin | admin | user |
|------|:--:|:--:|:--:|
| 修改模型配置 | ✅ | ❌ | ❌ |
| 修改系统配置 | ✅ | ❌ | ❌ |
| 管理所有用户 | ✅ | ❌ | ❌ |
| 管理普通用户 | ✅ | ✅ | ❌ |
| 管理所有知识库 | ✅ | ❌ | ❌ |
| 管理自己知识库 | ✅ | ✅ | ✅ |
| 查看审计日志 | ✅ | ❌ | ❌ |
| 修改自己密码 | ✅ | ✅ | ✅ |
| 创建 API 转换 | ✅ | ✅ | ❌ |
| 站内信全站发送 | ✅ | ❌ | ❌ |
| 站内信群发 | ✅ | ✅ | ❌ |

#### 6.3 系统管理员自动创建

- 系统启动时检测 `users` 集合是否存在 `system_admin` 角色用户
- 不存在 → 自动创建唯一 system_admin 账号
  - 用户名字段为系统管理员
  - 生成 16 位随机密码（大小写字母 + 数字 + 符号）
  - 明文密码通过 `zap` 日志输出（INFO 级别，仅启动时一次）
- 登录后后台头部显示横幅通知：**「您正在使用系统初始密码，请尽快修改」**，点击跳转修改密码页
- 修改密码后横幅消失（`users` 集合 `password_changed` 字段标记）

#### 6.4 JWT 中间件
- 签发、验证、刷新
- Token 过期时间可配置（默认 24h）

#### 6.5 RBAC 引擎
- 角色定义（MongoDB `roles` 集合）
- 权限校验中间件（基于 jwt claims 中的 role）

#### 6.6 审计日志中间件
- 所有 CUD 操作记录到 MongoDB `audit_logs`（TTL 90 天）

#### 6.7 安全过滤 + CORS
- 安全过滤中间件框架（实际过滤实现见 SPEC-004）
- CORS / Rate Limit 中间件

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes（users, roles, permissions, audit_logs, notifications） |
| 是否影响现有 API | No（新建项目） |
| 性能影响 | 无 |
| 是否需要新增 Skill | No |
| 是否需要 E2E 测试 | 占位即可（纯后端基础设施） |

## 相关文件

| 文件 | 角色 | 变更幅度 |
|------|------|---------|
| `cmd/server/main.go` | 入口 + admin 自动创建 | 新建 |
| `internal/infra/` | 数据库连接层 | 新建 |
| `internal/api/middleware/` | 中间件（JWT/RBAC/Audit） | 新建 |
| `internal/config/` | 配置管理 | 新建 |
| `internal/domain/notification/` | 站内信领域模型 | 新建 |
| `docker-compose.yml` | 开发环境 | 新建 |

## 验证标准

1. `docker compose up -d` 启动全部 6 个服务，健康检查通过
2. JWT 签发/验证/刷新流程正常
3. RBAC 三级角色权限校验阻断未授权请求
4. 系统启动时自动创建 system_admin，日志输出随机密码
5. system_admin 登录后头部通知提示修改密码
6. system_admin 可创建 admin 和 user 账号
7. admin 可创建 user，不可创建 system_admin
8. user 不可访问用户管理
9. 审计日志自动写入 MongoDB
