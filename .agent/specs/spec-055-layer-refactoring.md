# 分层架构重构：Controller → Service → Repository → Infra

> **SPEC-055** | Status: 设计中

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-048 | ✅ | ADK 引擎已迁移，Agent 调用链路稳定 |
| SPEC-049 | ✅ | 模型配置统一，路由逻辑独立 |
| SPEC-053 | ✅ | Memory/KB embedding 已统一缓存 |
| — | — | 无阻塞前置依赖，可立即开始 |

## 1. 目标

将 data-agent 项目从当前的"main.go 大杂烩 + service 直接依赖 infra"重构为严格分层架构：**Controller → Service → Repository → Infra**。每一层仅依赖下一层的接口，单元测试不再需要 MongoDB/Redis/Qdrant/SeaweedFS 等真实网络连接。

## 2. 背景

### 2.1 当前架构问题

```
当前状态（混乱）:
  cmd/server/main.go (1875行)
    ├── 40+ 个 inline gin HandlerFunc 闭包
    ├── 直接调用 mongoClient.DB().Collection(...).FindOne()
    ├── 组装所有 infra 依赖（MongoDB, Redis, Qdrant, SeaweedFS, Vault）
    └── 注册路由

  internal/service/
    ├── 直接 import *mongo.Collection, *qdrant.Client, *redis.Client
    ├── 在 service 方法中直接写 bson.M{}, FindOne, UpdateOne
    └── 无 Repository 接口抽象 → UT 需要 gomonkey（Linux+race 下不可用）

  internal/logic/
    └── 仅有验证/解析/统计纯函数（1/5 的业务逻辑在此层）
```

### 2.2 导致的直接后果

| 问题 | 严重度 |
|------|:---:|
| `go test` 本地无法运行——需要 MongoDB 连接，hang 超时 | 🔴 P0 |
| UT 覆盖率 97% → 96.5% —— 新代码无法 UT（Redis/MongoDB 依赖） | 🔴 P0 |
| coverpkg 导入 `service/` 包即触发 MongoDB init() | 🔴 P0 |
| main.go 1875 行，Sonar 认知复杂度 350+ | 🟡 P1 |
| 40+ inline HandlerFunc，修改一个 API 要改 main.go | 🟡 P1 |
| 无 Repository 接口，mock 只能靠 gomonkey（不可靠） | 🟡 P1 |

## 3. 架构概述

### 3.1 目标架构

```
┌─────────────────────────────────────────────────────────────┐
│                     cmd/server/main.go                      │
│                 只做初始化组装（~300行）                       │
│  config.Load() → infra.NewXxx() → svc.New(repo) → handler.New(svc) → router  │
└─────────────────────────────────────────────────────────────┘
                              │
    ┌─────────────────────────┼─────────────────────────┐
    ▼                         ▼                         ▼
┌──────────┐           ┌──────────┐           ┌──────────────┐
│ handler/  │           │ handler/ │           │ handler/     │
│ auth.go   │           │ task.go  │           │ knowledge.go │  ... etc
│ depends:  │           │ depends: │           │ depends:     │
│ AuthSvc   │           │ TaskSvc  │           │ KBSvc        │
│ interface │           │ interface│           │ interface    │
└──────────┘           └──────────┘           └──────────────┘
     │                       │                       │
     ▼                       ▼                       ▼
┌──────────┐           ┌──────────┐           ┌──────────────┐
│ service/  │           │ service/ │           │ service/     │
│ auth/     │           │ task/    │           │ knowledge/   │
│ depends:  │           │ depends: │           │ depends:     │
│ UserRepo  │           │ TaskRepo │           │ KBRepo,      │
│ interface │           │ interface│           │ VectorRepo   │
└──────────┘           └──────────┘           └──────────────┘
     │                       │                       │
     ▼                       ▼                       ▼
┌──────────────────────────────────────────────────────────────┐
│                      repository/                             │
│  UserRepository interface ─── mongo.UserRepo (impl)          │
│  TaskRepository interface ─── mongo.TaskRepo (impl)          │
│  KBRepository   interface ─── mongo.KBRepo (impl)            │
│  VectorRepository interface ─── qdrant.VectorStore (impl)    │
│  FileRepository  interface ─── seaweedfs.FileStore (impl)    │
│  CacheRepository interface ─── redis.CacheStore (impl)       │
│  ... 所有数据访问抽象                                         │
└──────────────────────────────────────────────────────────────┘
     │
     ▼
┌─────────────┐  ┌──────────┐  ┌───────────┐  ┌──────────┐
│ infra/mongo/ │  │ infra/   │  │ infra/    │  │ infra/   │
│              │  │ qdrant/  │  │ seaweedfs │  │ redis/   │
│ (纯实现层)    │  │ (纯实现)  │  │ (纯实现)   │  │ (纯实现)  │
└─────────────┘  └──────────┘  └───────────┘  └──────────┘
```

### 3.2 层级依赖规则

| 层 | 可以 import | 禁止 import |
|----|-----------|------------|
| **handler/** | service 接口, `gin`, `net/http` | infra, mongo-driver, redis, qdrant |
| **service/** | repository 接口, logic 纯函数, domain | infra, mongo-driver, redis, qdrant, seaweedfs, gin |
| **repository/** | infra, mongo-driver | gin, handler |
| **infra/** | 外部 SDK (mongo-driver, redis, qdrant-go) | 项目内部任何包 |
| **logic/** | 无外部依赖, domain | 所有 side-effect 包 |

### 3.3 与当前架构的对比

| 对比项 | 当前 | 目标 |
|--------|------|------|
| main.go 行数 | ~1875 | ~300 |
| Inline HandlerFunc | ~40 | 0 |
| Service→MongoDB 直接依赖 | 7/10 服务 | 0 |
| Repository 接口 | 0 | 15+ |
| UT 需要 MongoDB | 是 | 否 |
| coverpkg 覆盖率可测性 | P0 不可测 | 全覆盖 |

## 4. 详细设计

### 4.1 Repository 接口定义位置

Repository 接口定义在 `internal/repository/` 包，impl 在 `internal/infra/` 下各子包：

```
internal/repository/
├── user.go          # UserRepository interface
├── role.go          # RoleRepository interface
├── task.go          # TaskRepository interface
├── session.go       # SessionRepository interface
├── knowledge.go     # KBRepository interface
├── audit.go         # AuditRepository interface
├── artifact.go      # ArtifactRepository interface
├── notification.go  # NotificationRepository interface
├── modelconfig.go   # ModelConfigRepository interface
├── sysconfig.go     # SysConfigRepository interface
├── apireview.go     # APIReviewRepository interface
├── invite.go        # InviteRepository interface
├── imbind.go        # IMBindRepository interface
├── vector.go        # VectorRepository interface（Qdrant）
├── file.go          # FileRepository interface（SeaweedFS）
└── cache.go         # CacheRepository interface（Redis）
```

### 4.2 接口设计原则

```go
// ✅ 正确：接收接口，返回接口
type UserRepository interface {
    FindByID(ctx context.Context, id string) (*domain.User, error)
    FindByUsername(ctx context.Context, username string) (*domain.User, error)
    Create(ctx context.Context, user *domain.User) error
    Update(ctx context.Context, id string, update bson.M) error
    Delete(ctx context.Context, id string) error
    List(ctx context.Context, filter UserFilter) ([]*domain.User, error)
}

// ❌ 错误：暴露 infra 细节
type UserRepository interface {
    Collection() *mongo.Collection  // 禁止！暴露实现
}
```

所有 repository 接口的输入/输出使用 `domain/` 包或纯 Go 类型，不使用 `bson.M`/`primitive.ObjectID` 等 MongoDB 类型。如果 update 参数需要灵活性，使用 `map[string]interface{}`。

### 4.3 Service 层重构

每个 service 构造函数改为接收 repository 接口：

```go
// Before（当前）:
func NewService(db *mongo.Database) *Service { ... }
func (s *Service) DoSomething() { s.coll.FindOne(...) }  // 直接操作 MongoDB

// After（目标）:
func NewService(repo repository.UserRepository, ...) *Service { ... }
func (s *Service) DoSomething() { s.repo.FindByID(...) }  // 通过接口
```

受影响的 service 包（按 P0→P2 排序）：

| 优先级 | Service | 当前依赖 | 改为 |
|:---:|---------|---------|------|
| P0 | `service/auth` | `*mongo.UserRepository` (已有 repo) | `repository.UserRepository` |
| P0 | `service/chat/session` | `*mongo.Collection` (裸) | `repository.SessionRepository` |
| P0 | `service/task` | `*mongo.Collection` + `*queue.Stream` | `repository.TaskRepository` + `repository.QueueRepository` |
| P0 | `service/knowledge` | `*mongo.Database` + `*qdrant.Client` | `repository.KBRepository` + `repository.VectorRepository` |
| P1 | `service/artifact` | `*mongo.Collection` + `*seaweedfs.Client` | `repository.ArtifactRepository` + `repository.FileRepository` |
| P1 | `service/audit` | `*mongo.Collection` | `repository.AuditRepository` |
| P1 | `service/notification` | `*mongo.Collection` | `repository.NotificationRepository` |
| P1 | `service/apireview` | `*mongo.Collection` | `repository.APIReviewRepository` |
| P2 | `service/im` | `*mongo.Collection` (bind) | `repository.IMBindRepository` |

### 4.4 Handler 层迁移

所有 `cmd/server/main.go` 中的 inline HandlerFunc 迁移到 `internal/api/handler/`：

| 来源 main.go 函数 | 目标文件 | 路由组 |
|-------------------|---------|--------|
| `setupUserManagement` (7 handlers) | `handler/user.go` | `/api/v1/admin/users` |
| `setupRoleManagement` (5 handlers) | `handler/role.go` | `/api/v1/admin/roles` |
| `setupModelConfig` (3 handlers) | `handler/modelconfig.go` | `/api/v1/admin/models` |
| `setupSysConfig` (2 handlers) | `handler/sysconfig.go` | `/api/v1/admin/sysconfig` |
| `setupChangePassword` | `handler/password.go` | `/api/v1/change-password` |
| `setupSessions` (6 handlers) | `handler/session.go` | `/api/v1/sessions` |
| `setupChatEnhance` | `handler/enhance.go` | `/api/v1/enhance` |
| `setupIMBind` (2 handlers) | `handler/imbind.go` | `/api/v1/im/bind` |
| `setupIMWebhook` | `handler/im_webhook.go` | `/api/v1/im/webhook` |
| `setupMemorySearch` | `handler/memory.go` | `/api/v1/memory` |
| `setupDashboard` | `handler/dashboard.go` | `/api/v1/admin/dashboard` |
| `setupHermesProxy` | `handler/hermes.go` | `/api/v1/hermes/*` |

路由注册也移出 main.go：每个 handler 包提供 `RegisterXxxRoutes(router *gin.RouterGroup, deps *Dependencies)` 方法。

### 4.5 main.go 缩减

重构后 main.go 只保留：
1. 配置加载
2. 基础设施初始化（MongoDB, Redis, Qdrant, SeaweedFS, Vault）
3. Repository 实现创建
4. Service 创建（注入 repository 接口）
5. Handler 创建（注入 service 接口）
6. 路由注册调用
7. 启动 HTTP server

```go
func main() {
    cfg := config.Load()
    logger := zap.NewProduction()
    
    // Infrastructure
    mongoClient := infraMongo.Connect(cfg.MongoURI)
    redisClient := infraRedis.Connect(cfg.RedisAddr)
    qdrantClient := infraQdrant.Connect(cfg.QdrantURL)
    seaweedClient := infraSeaweed.Connect(cfg.SeaweedURL)
    
    // Repositories (implement interfaces)
    userRepo := mongo.NewUserRepository(mongoClient)
    taskRepo := mongo.NewTaskRepository(mongoClient)
    kbRepo := mongo.NewKBRepository(mongoClient)
    vectorStore := qdrant.NewVectorStore(qdrantClient)
    fileStore := seaweed.NewFileStore(seaweedClient)
    
    // Services
    authSvc := auth.NewService(userRepo, inviteRepo, ...)
    taskSvc := task.NewService(taskRepo, queueRepo, ...)
    kbSvc := knowledge.NewService(kbRepo, vectorStore, ...)
    
    // Handlers + routes
    r := gin.Default()
    handler.RegisterAuthRoutes(r, authSvc)
    handler.RegisterTaskRoutes(r, taskSvc)
    handler.RegisterKBRoutes(r, kbSvc)
    // ... 每层一个 RegisterXxxRoutes 调用
    
    r.Run(cfg.Port)
}
```

### 4.6 chat.Service 解耦 gin

`chat.Service.HandleChat(c *gin.Context)` 改为不依赖 gin：

```go
// Before:
func (s *Service) HandleChat(c *gin.Context) { ... c.Request ... c.JSON(...) }

// After:
func (s *Service) ProcessChat(ctx context.Context, req ChatRequest, userID string) (*ChatResponse, error) { ... }
```

Handler 负责 gin → service 参数转换 + 响应写入。

### 4.7 不受影响的模块

以下模块已经符合分层要求，**不需要改动**：

| 模块 | 原因 |
|------|------|
| `internal/logic/` | 纯函数，零外部依赖 |
| `internal/domain/` | 纯数据模型，零外部依赖 |
| `internal/config/` | 纯配置 struct |
| `internal/adk/` | 已使用 ADK 接口（memory.Service, model.LLM），非 Mongo/Redis |
| `internal/api/middleware/` | 依赖接口 `middleware.JWTManager`, `security.Auditor` |
| `internal/api/handler/` (已有) | 已遵循 handler→service 模式 |

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No — 仅重构现有访问方式 |
| 是否影响现有 API | No — handler 行为不变，仅迁移位置 |
| 是否影响现有 UI | No — API 契约不变 |
| 性能影响 | 无 — 接口调用无虚函数开销（Go 静态分发） |
| 是否需要新增 Skill | No |
| 是否影响 ADK 集成 | No |
| 预计改动文件数 | ~60 文件（新建 16 repository 接口 + 15 handler 文件 + 修改 ~20 service + main.go） |
| 风险等级 | 中 — 大规模重构但无功能变更，Git 逐个文件对比可验证 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `internal/repository/*.go` | Repository 接口定义 | **New** (16 files) |
| `internal/infra/mongo/*_repo.go` | Repository 实现（已有部分） | Modify (~10 files) |
| `internal/infra/qdrant/vector_store.go` | VectorRepository 实现 | **New** |
| `internal/infra/seaweedfs/file_store.go` | FileRepository 实现 | **New** |
| `internal/infra/redis/cache_store.go` | CacheRepository 实现 | **New** |
| `internal/service/auth/*.go` | 改为接口依赖 | Modify |
| `internal/service/task/*.go` | 改为接口依赖 | Modify |
| `internal/service/knowledge/*.go` | 改为接口依赖 | Modify |
| `internal/service/chat/session.go` | 抽 Repository 接口 | Modify |
| `internal/service/chat/chat_service.go` | 解耦 gin | Modify |
| `internal/service/artifact/*.go` | 改为接口依赖 | Modify |
| `internal/service/audit/*.go` | 改为接口依赖 | Modify |
| `internal/service/notification/*.go` | 改为接口依赖 | Modify |
| `internal/service/apireview/*.go` | 改为接口依赖 | Modify |
| `internal/service/im/*.go` | 改为接口依赖 | Modify |
| `internal/api/handler/*.go` | 迁移 inline handler + 新增 | Modify + **New** (~12 files) |
| `cmd/server/main.go` | 删除 inline handler → 纯组装 | **Major reduce** (-1500 lines) |

## 7. 测试策略

### Mock 工具选择

| 场景 | 工具 | 说明 |
|------|------|------|
| Repository/Service 接口 mock | **mockery** (首选) | 基于 Go interface 自动生成 mock struct，零依赖 |
| 无法接口化的遗留调用 | gomonkey | 仅用于 mockery 无法覆盖的场景（如 ADK runtime 内部） |
| HTTP handler 测试 | `httptest` | Go 标准库，不依赖任何 mock 工具 |

**mockery 使用示例**：

```go
//go:generate mockery --name UserRepository --output ./mocks --outpkg mocks

// 测试中:
mockRepo := mocks.NewUserRepository(t)
mockRepo.On("FindByID", ctx, "user-1").Return(&domain.User{...}, nil)
svc := auth.NewService(mockRepo, ...)
result, err := svc.Login(ctx, req)
assert.NoError(t, err)
mockRepo.AssertExpectations(t)
```

### 测试编写

1. **Unit tests**: 
   - 所有 repository/service 接口 mock 使用 **mockery** 自动生成
   - Service 层依赖 repository 接口 → mock 注入 → 无需 MongoDB/Redis
   - Handler 层用 `httptest.NewRecorder` + mock service → 无外部依赖
   - gomonkey 仅用于 mockery 无法 mock 的场景（如 ADK `model.LLM`、`memory.Service` 等第 3 方接口）
2. **Integration tests**: 条件使用 Docker Compose（`go test -tags=integration`）验证实际 MongoDB/Qdrant/Redis 交互
3. **E2E tests**: 现有 UI 测试不变，API 行为一致
4. **审计**: 使用 `.agent/skills/go-ut-audit` 审查 UT 质量

### UT 可测性对比

| 场景 | 重构前 | 重构后 |
|------|--------|--------|
| 测试 service/auth.Login | 需要 MongoDB + gomonkey | mock UserRepository |
| 测试 service/task.Create | 需要 MongoDB | mock TaskRepository |
| 测试 handler/user/listUsers | 需要 MongoDB | mock UserService |
| 测试 chat session 写入 | 需要 MongoDB + ADK runtime | mock SessionRepository |
| 覆盖率提升 | 无法提升（infra 代码无 mock） | 可达 98% |

## 8. 实现阶段

| Phase | 内容 | 文件数 |
|:---:|------|:---:|
| **Phase 1** | 创建 `internal/repository/` 接口定义 | ~16 |
| **Phase 2** | 修改 infra 层，让现有实现满足新接口 | ~10 |
| **Phase 3** | 修改 service 层，构造函数改为接收接口 | ~12 |
| **Phase 4** | 迁移 main.go inline handler 到 handler/ | ~12 new, -1500 lines |
| **Phase 5** | chat.Service 解耦 gin | 1 |
| **Phase 6** | main.go 缩减为纯组装 | 1 (-1500 lines) |
| **Phase 7** | 新增 UT，补到 98% | ~15 test files |
| **Phase 8** | CI 验证全绿 | — |

## 9. UI Test / E2E 验收规则

> 本次为纯架构重构，API 契约不变。

- [ ] **必须** 现有 E2E 用例全部通过（无 UI 变更）
- [ ] **必须** 新增 handler 文件配套 UT
- [ ] **必须** service 层每个方法的 mock 测试覆盖
- [ ] **严禁** 降级已有测试

## 10. Go Unit Test 验收规则

> 重构后 UT 必须全部可本地运行（无需 Docker/网络）。

### 覆盖率底线

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L1 | repository 接口、logic、domain | **100%** |
| L2 | service（依赖 repository 接口）| **100%** |
| L3 | handler（依赖 service 接口） | **98%** |
| Overall | 全量 | ≥98% |

### 关键验证

- [ ] `go test ./internal/...` 无需 MongoDB 即可全部通过
- [ ] `go test -coverpkg=./internal/... ./internal/...` 覆盖率 ≥ 98%
- [ ] 所有 service 测试使用 **mockery** 生成的 mock repository（非 gomonkey）
- [ ] gomonkey 仅用于 mockery 无法覆盖的场景（如 ADK `model.LLM`、`memory.Service` 接口）
- [ ] 所有 handler 测试使用 `httptest` + mock service
- [ ] 每个 repository 接口所在包包含 `//go:generate mockery ...` 指令

## 11. 验证标准

1. `go test ./internal/...` 全部通过（本地，无 Docker、无网络）
2. `go test -race -coverprofile=coverage.out -coverpkg=./internal/... ./internal/...` → 覆盖率 ≥ 98%
3. CI `ut-workflow.yml` + `sonarqube` + `golangci-lint` + `ui-tests` 全部通过
4. main.go 从 ~1875 行缩减至 ~300 行
5. 零个 inline HandlerFunc 留存 main.go
6. 所有 service 构造函数接收接口而非 `*mongo.Collection`
7. 所有 service 测试使用 mockery 生成的 mock（非 gomonkey）
8. 每个 repository 接口包含 `//go:generate mockery --name XxxRepository` 指令
