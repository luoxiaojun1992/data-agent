# 多模型配置与 Session 绑定模型

> **SPEC-062** | Status: 已实现

## 1. 目标

在 SPEC-061 配置缓存层基础上，实现多模型配置管理与"session 绑定模型"的完整能力：

1. **默认模型**：模型列表初始默认第一个（`IsDefault`），可增加模型；未指定模型时用默认模型。
2. **模型列表分页**：admin 模型配置页支持结构化多模型列表管理 + 分页展示。
3. **session 绑定模型**：新建 chat / agent(实时/异步/定时) / imbind 时可选模型；未选用默认；选定后绑定 session **不可更换**；新 session 可重选；**只能选 LLM 模型**（`Type==llm`，embedding 不可选）。
4. **LLM 实例生命周期**（晓军 2026-07-22 纠正）：
   - **不启动时一次性 new 所有 LLM**，而是**新建 session 时按绑定模型懒创建**（per-model Runtime 注册表 + 复用）。
   - **系统级 LLM 实例**：可创建多个 LLM Runtime，放 `map[UseCase]*Runtime`（key 为 use case），用于提示词增强、KB 索引切片、memory 管理、session 压缩摘要等与 user 无关且不支持任意切换的场景。
5. **ModelEntry 加独立 ID 字段**（晓军 2026-07-22 确认）：不强依赖 `Name` 作标识，新增 `ID string` 作为模型唯一标识。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-049 | ✅ | 统一模型配置体系（`ModelEntry` + `UseCase` + `modelcfg.Provider`） |
| SPEC-055 | ✅ | 分层架构（domain/repo/infra 分离），本 spec 改 domain model 走分层 |
| SPEC-058 | ✅ | logic 编排层（`Orchestrator` 已就位，本 spec 扩展其 modelId 透传） |
| SPEC-061 | 📐 设计中 | 配置统一缓存（模型配置读走 cache→DB，热更新基础）。**本 spec 依赖 SPEC-061 的 cache 装饰器就位**——模型列表查询经缓存，避免每次 session 创建打 Mongo |

> **阻塞项**：SPEC-061 必须先实现（cache 装饰器 + 配置值热更新）。否则模型配置读取仍直打 Mongo，per-session Runtime 创建时每次触发 DB 读，性能不可接受。

## 2. 背景

现状（代码核对，含 ADK v1.5.0 源码确认）：

### 2.1 模型选择链路全线缺失

| 层 | 现状 | 缺口 |
|----|------|------|
| `Session` domain（`domain/chat/model.go:8-18`） | 无 `ModelID` 字段 | 需新增 |
| `SessionRecord`（`repository/notification.go:46-56`） | 无 `model_id` bson | 需新增 |
| `chat.Manager.Create`（`service/chat/session.go:27`） | 签名 `Create(userID, sessionType)` 无 modelId | 需扩展 |
| `ChatRequest.Model`（`domain/chat/contract.go:25`） | **字段已存在但被忽略** | 需接线 |
| `chat.Service.prepareRun`（`chat_service.go:56-116`） | 不读 `req.Model`，创建 session 不传 modelId | 需改造 |
| `Task` domain（`domain/task/model.go:23-40`） | 无 `ModelID` | 需新增 |
| `QueueMessage`（`task/model.go:92-101`） | 无 `ModelID`（worker 无法得知用哪个模型） | 需新增 |
| `ScheduledTask`/`Schedule`（`task/model.go:78`、`scheduler.go:15`） | 无 `ModelID` | 需新增 |
| `IMBind`（`repository/config.go:52`） | schema-less `map[string]any`，无 modelId | 需约定字段 |

### 2.2 LLM 实例生命周期现状（晓军纠正点）

**现状（错误范式）：** `wire.go:131,159-171` 启动时 `BuildLLM("")` 构造**单个** LLM，baked 进**单例** `adkruntime.Runtime`，所有 chat 请求共享。模型配置改了不生效（重启才更新）。

**晓军纠正：** LLM 实例应 per-session 懒创建（新建 session 时按绑定模型解析对应 Runtime），而非启动时 new 好。系统级场景（enhance/compaction/memoryx）用 `map[UseCase]*Runtime` 管理多个系统级 Runtime。

### 2.3 Google ADK 约束（决定性）

ADK v1.5.0 **构造时绑定 model**，`runner.Run` 无 model 参数：

```go
// adk@v1.5.0/agent/llmagent/llmagent.go:177-178
type Config struct { Model model.LLM ... }  // 仅 llmagent.New 时设置，无 SetModel

// adk@v1.5.0/runner/runner.go:131
func (r *Runner) Run(ctx, userID, sessionID, msg, cfg, opts...)  // opts 仅 WithStateDelta，无 model
```

**结论：** per-session LLM 无法通过"给 `Runtime.Run` 传 model"实现，必须**按 modelKey 维护多个 Runtime 实例**（注册表 + 懒创建），session 创建时按绑定 modelId 选对应 Runtime。

### 2.4 系统级 LLM 现状（部分分离）

| 系统级消费者 | LLM 来源 | 绑定时机 | 现状 |
|-------------|---------|---------|------|
| enhance（`enhance/service.go:65`） | `Provider.BuildLLM(UseCaseEnhance)` | 每次调用 | ✅ 已分离，天然热更 |
| session 压缩摘要（`adk/session/summarizer.go:22`） | `BuildLLM(UseCaseCompaction)` baked 进 `LLMSummarizer` | 启动 | 固定实例 |
| memoryx Kit（`memoryx/kit.go:22`） | `compactionLLM` baked | 启动 | 固定实例 |
| KB embedding 切片（`wire.go:205`） | `EmbeddingConfig()`（embedding client，非 LLM） | 启动 baked | SPEC-061 改为按需读 |

> 晓军确认：系统级 LLM 可创建多个放 `map[UseCase]*Runtime`，比现有"各自 BuildLLM"更统一。本 spec 将系统级 LLM 纳入 Registry 统一管理。

### 2.5 异步任务执行器是 stub（前置缺口）

`simpleExecutor.Execute`（`main.go:279-287`）是 **no-op**，异步/定时 agent 任务当前**不真正执行**。本 spec 聚焦"模型绑定数据贯通到 QueueMessage"，执行器实现见 **SPEC-063**。

### 2.6 前端模型选择器不存在

- 无 `ModelSelector` 组件（`frontend/app/components/` 仅有 IdleTimer/NotificationBell/Sidebar）
- chat/agent/imbind 三处创建页无模型选择 UI
- admin models 页（`admin/models/page.tsx`）用 legacy flat 单模型表单 + 调错路由（`/model-config` 而非 `/models`）

### 2.7 默认模型逻辑现状

`IsDefault` 字段（`provider.go:52`）**仅用于 instruction 选择**（`DefaultInstruction` line 237-244），**不用于模型选择**。`BuildLLM("")` 按 `FallbackOrder` 取第一个，非 `IsDefault`。需补"默认模型选择"逻辑。

### 2.8 ModelEntry 无独立 ID 字段

当前 `ModelEntry`（`provider.go:41-54`）无 `ID` 字段，`Name` 兼作标识。晓军要求加独立 `ID`，不强依赖 `Name`。

## 3. 架构概述

### 模块关系图

```
┌──────────────────────────────────────────────────────────────────────┐
│            模型配置 (system_configs:model/models, 经 SPEC-061 cache)             │
│   []ModelEntry { ID(唯一标识), Name(显示名), Type(llm|embedding),               │
│                  IsDefault, UseCases, Instruction, ... }                      │
└──────────────┬───────────────────────────────────────────────────────┬────────┘
               │ Provider 读取 (走 cache→DB)                            │
               ▼                                                        ▼
   ┌───────────────────────┐                          ┌─────────────────────────────┐
   │  默认模型选择逻辑       │                          │  模型列表查询 (分页, 仅 llm)   │
   │  DefaultModel()        │                          │  ListLLMModels(page,size)    │
   │  IsDefault 优先         │                          │  过滤 Type==llm               │
   │  无则 Type==llm 第一   │                          └─────────────────────────────┘
   └───────────┬───────────┘
               │ 未指定 modelId 时用默认
               ▼
   ┌───────────────────────────────────────────────────────────────────────────────┐
   │                    Session 创建 (绑定 modelId=ModelEntry.ID, 不可换)              │
   │   chat.Session.ModelID / Task.ModelID / Schedule.ModelID / IMBind.model_id      │
   └───────────────┬───────────────────────────────────────────────────────────────┘
                   │
                   ▼
   ┌───────────────────────────────────────────────────────────────────────────────┐
   │                Runtime 注册表 (internal/adk/runtime/registry.go)                 │
   │                                                                                  │
   │   sessionRuntimes map[string]*Runtime   (key=modelID, per-session 模型)             │
   │   sysRuntimes     map[UseCase]*Runtime  (key=use case, 系统级模型)                 │
   │                                                                                  │
   │   GetOrCreate(modelID)         → session 路径: 懒创建 + 指纹比对热更新              │
   │   GetOrCreateByUseCase(useCase) → 系统路径: 懒创建 + 指纹比对热更新                │
   │                                                                                  │
   │   每个缓存项存 {Runtime, configHash}; 配置变更→指纹变化→下次请求重建               │
   │   Tools/Auditor/SessionService/MemoryService 共享 (仅 Model/Instruction 不同)      │
   └───────────────┬──────────────────────────────────┬──────────────────────────────┘
                   │ chat.Service 按 session.ModelID     │ 系统级消费者按 UseCase
                   ▼                                     ▼
   ┌──────────────────────────────┐         ┌──────────────────────────────────────┐
   │  chat.Service                 │         │  系统级 LLM (map[UseCase]*Runtime)     │
   │  registry.GetOrCreate(modelID)│         │  enhance: GetOrCreateByUseCase(Enhance)│
   │  prepareRun 读 session.ModelID│         │  compaction: GetOrCreateByUseCase(...)  │
   │  → 选对应 Runtime → Run       │         │  memoryx: 同 compaction use case       │
   └──────────────────────────────┘         └──────────────────────────────────────┘
```

### 与现有模式对比

| 维度 | 现状 | SPEC-062 后 |
|------|------|------------|
| 模型标识 | 无独立 ID，Name 兼作标识 | `ModelEntry.ID` 独立唯一标识 |
| LLM 实例 | 启动 new 1 个单例，baked 进单 Runtime | per-model 懒创建（注册表），session 创建时选 |
| 系统级 LLM | 各自 BuildLLM（分散） | 统一 `map[UseCase]*Runtime`，Registry 管理 |
| Runtime | 单例 `deps.adkRuntime` | 注册表 `Registry`，chat.Service 持有 |
| session 模型 | 无 ModelID，全共享单 Runtime | session.ModelID 绑定，不可换，按 ID 选 Runtime |
| 默认模型 | 无（BuildLLM 按 FallbackOrder） | `DefaultModel()`：IsDefault 优先，无则 llm 第一个 |
| 模型列表 | 无分页，admin 用 legacy flat | 分页查询，结构化 ModelEntry |
| 前端选择器 | 不存在 | ModelSelector 组件，chat/agent/imbind 接入 |
| 热更新 | 无（重启才更新） | 指纹比对：配置变更→下次请求重建 Runtime |

## 4. API 设计

### 4.1 模型配置管理（admin）

| Method | Path | Description | Permission |
|--------|------|-------------|------------|
| GET | `/api/v1/models` | 模型配置读（现有，改造为返回结构化 `[]ModelEntry` + 分页） | `PermModelConfig` |
| PUT | `/api/v1/models` | 模型配置 upsert（现有，改为整体列表 upsert） | `PermModelConfig` |
| **GET** | `/api/v1/models/list` | **新增**：可选模型列表（仅 `Type==llm`，分页，供选择器） | auth |
| **POST** | `/api/v1/models` | **新增**：增加单个模型条目（自动生成 ID） | `PermModelConfig` |
| **DELETE** | `/api/v1/models/:id` | **新增**：删除单个模型（SPEC-061 Delete 端点复用） | `PermModelConfig` |
| **PATCH** | `/api/v1/models/:id/default` | **新增**：设为默认模型（更新 IsDefault） | `PermModelConfig` |

> `GET /models` 查询参数：`page`（默认1）、`page_size`（默认20）、`type`（可选过滤 `llm`/`embedding`）。返回 `{models: [...], total: N, page, page_size}`。

### 4.2 session / task 创建（模型选择）

| Method | Path | 新增字段 | 说明 |
|--------|------|---------|------|
| POST | `/api/v1/sessions` | body `model_id`（可选，= ModelEntry.ID） | 未传用默认模型；绑定后不可换 |
| POST | `/api/v1/chat` | body `model`（现有字段，接线，传 modelId） | 仅新建 session 时生效；已有 session 忽略此字段用 session.ModelID |
| POST | `/api/v1/agent/tasks` | body `model`（现有字段，接线） | 实时 agent 创建 session 时绑定 |
| POST | `/api/v1/tasks` | body `model_id`（新增） | 异步任务，透传到 QueueMessage |
| PUT | `/api/v1/im/bind` | body `model_id`（可选） | imbind 选模型，仅 llm |

> **不可换约束**：`PUT /sessions/:id`（若存在）不暴露 `model_id` 字段；chat 在已有 session 时**忽略** `req.Model`，强制用 `session.ModelID`。

## 5. 详细设计

### 5.1 ModelEntry ID 字段与默认模型逻辑

#### 5.1.1 ModelEntry 加 ID 字段

`internal/adk/modelcfg/provider.go` `ModelEntry` 新增 `ID` 字段：

```go
type ModelEntry struct {
    ID            string    `json:"id"`           // 新增：模型唯一标识（UUID 或 slug）
    Name          string    `json:"name"`         // 显示名（可重复，如"GPT-4o 生产""GPT-4o 测试"）
    BaseURL       string    `json:"base_url"`
    APIKey        string    `json:"-"`
    Type          ModelType `json:"type"`
    Instruction   string    `json:"instruction"`
    Capability    string    `json:"capability"`
    UseCases      []string  `json:"use_cases"`
    TokenMultiplier float64 `json:"token_multiplier"`
    Temperature   float64   `json:"temperature"`
    MaxTokens     int       `json:"max_tokens"`
    IsDefault     bool      `json:"is_default"`
    FallbackOrder int       `json:"fallback_order"`
}
```

**ID 生成策略：** `POST /models` 新增模型时由后端自动生成（`uuid.NewString()` 或 `model_` + short hash）。前端不传 ID。ID 不可变（模型创建后不可改 ID，`Name` 可改）。

**约束：** `ID` 在模型列表内唯一（Provider `SetModels` 时校验重复 ID 拒绝）。`Name` 不要求唯一（仅显示用）。

**向后兼容：** 旧配置无 `ID` 字段的 `ModelEntry`，读取时 ID 为空 → Provider 补全逻辑：空 ID 的模型自动用 `Name` 作 ID（兼容期），admin 编辑后补正式 ID。

#### 5.1.2 模型标识贯通

- 选择器返回的 modelId = `ModelEntry.ID`
- session/task/schedule/imbind 存的 `model_id` = `ModelEntry.ID`
- Runtime 注册表 key = `ModelEntry.ID`
- API 路由 `:id` 参数 = `ModelEntry.ID`

#### 5.1.3 默认模型选择

`modelcfg.Provider` 新增方法：

```go
// DefaultModel 返回默认 LLM 模型（IsDefault 优先；无则 Type==llm 第一个；皆无则 nil）。
func (p *Provider) DefaultModel(ctx context.Context) (*ModelEntry, error)
```

逻辑：
1. 遍历 `models()`，找 `IsDefault==true && Type==llm` 的第一个 → 返回
2. 无 → 找 `Type==llm` 列表第一个（按当前排序） → 返回
3. 无 LLM 模型 → 返回 error

"初始默认第一个"：新增模型时若列表无 `IsDefault`，admin API 自动将第一个 LLM 模型标记 `IsDefault`（`POST /models` / `PUT /models` 时维护不变量：**有且仅有一个 LLM 模型 IsDefault=true**）。

#### 5.1.4 模型列表查询（分页 + 仅 LLM）

`modelcfg.Provider` 新增：

```go
// ListLLMModels 返回 Type==llm 的模型列表（分页）。
func (p *Provider) ListLLMModels(ctx context.Context, page, pageSize int) ([]ModelEntry, int, error)
// 返回 (models, total, err)
```

> 整个模型列表存在单个 config value（JSON 数组），数据量小，分页在内存切片上做（`slice[offset:limit]`）。total 为 LLM 模型总数。

### 5.2 数据模型扩展（ModelID 贯通）

#### 5.2.1 Session

`internal/domain/chat/model.go`：
```go
type Session struct {
    ID            string     `json:"id"`
    UserID        string     `json:"user_id"`
    Type          string     `json:"type"`
    ModelID       string     `json:"model_id"`   // 新增：绑定的模型 ID（ModelEntry.ID），空=用默认
    Status        string     `json:"status"`
    ...
}
```

`internal/repository/notification.go` `SessionRecord`：
```go
type SessionRecord struct {
    ...
    ModelID    string `bson:"model_id"`   // 新增
    ...
}
```

`internal/service/chat/session.go`：
- `Manager.Create` 签名扩展：`Create(userID, sessionType, modelID string)`
- `sessionToRecord` / `recordToSession` 双向映射 `ModelID`
- modelID 为空时由调用方（`prepareRun`）先用 `Provider.DefaultModel()` 解析为具体 ID 再传入，**保证 DB 存的始终是具体 modelId**（而非空串），避免后续默认模型变更影响已绑定 session

#### 5.2.2 Task / QueueMessage

`internal/domain/task/model.go`：
```go
type Task struct {
    ...
    ModelID   string `json:"model_id"`   // 新增（ModelEntry.ID）
    ...
}
type QueueMessage struct {
    ...
    ModelID   string `json:"model_id"`   // 新增（worker 据此选 Runtime）
    ...
}
```

`NewTask` / `TaskService.CreateTask` 加 `modelID` 参数。`QueueMessage` 构造时透传 `task.ModelID`。

#### 5.2.3 ScheduledTask / Schedule

`internal/domain/task/model.go` `ScheduledTask` 加 `ModelID`。`internal/scheduler/scheduler.go` `Schedule` 加 `ModelID`，`TaskCreator.CreateTask` 接口加 `modelID` 参数，`executeJob` 透传 `sch.ModelID`。

#### 5.2.4 IMBind

IMBind 是 schema-less `map[string]any`。约定 `model_id` key：
- `IMBindHandler.Update` body 可含 `model_id`（string，= ModelEntry.ID），透传存入 map
- 读取时 `data["model_id"]` 取出
- 不改 repository 签名（保持 map 灵活性），仅约定 key

### 5.3 Runtime 注册表（双 map + 指纹热更新）

新增 `internal/adk/runtime/registry.go`：

```go
package runtime

// cachedRuntime 缓存一个 Runtime 实例 + 创建时的配置指纹。
// 指纹变化（配置变更）时触发重建，实现热更新。
type cachedRuntime struct {
    rt       *Runtime
    hash     string   // 创建时的模型配置指纹（sha256(json)）
}

// Registry 维护两类 Runtime：session 级（按 modelID）和系统级（按 UseCase）。
// 两者都懒创建 + 指纹比对热更新。Tools/Auditor/Session/Memory 共享。
type Registry struct {
    provider    *modelcfg.Provider
    sessionSvc  session.Service
    memorySvc   memory.Service
    tools       []tool.Tool
    auditor     Auditor
    appName     string
    mu          sync.RWMutex
    sessions    map[string]*cachedRuntime   // key = modelID (ModelEntry.ID)
    sys         map[UseCase]*cachedRuntime   // key = UseCase（系统级）
}

// GetOrCreate 按 modelID 查/建 session 级 Runtime。modelID 空则用默认模型。
func (r *Registry) GetOrCreate(ctx context.Context, modelID string) (*Runtime, error)

// GetOrCreateByUseCase 按 UseCase 查/建系统级 Runtime。
// 用于 enhance/compaction/memoryx 等不随 session 切的场景。
func (r *Registry) GetOrCreateByUseCase(ctx context.Context, useCase modelcfg.UseCase) (*Runtime, error)
```

#### 5.3.1 session 级 Runtime（懒创建 + 指纹热更新）

`GetOrCreate(ctx, modelID)` 流程：
1. 解析 modelID（空串 → `Provider.DefaultModel(ctx).ID`，走 cache→DB）
2. 读该模型配置（走 SPEC-061 cache），计算配置指纹 `hash = sha256(json(ModelEntry))`
3. `r.mu.RLock` 查 `sessions[modelID]`：
   - 命中且 `hash` 一致 → 返回缓存 Runtime（**复用，不重建**）
   - 命中但 `hash` 不一致 → 配置已变更，需重建
   - 未命中 → 首次创建
4. 需创建/重建时 → `r.mu.Lock` 升级，double-check：
   - `Provider.BuildLLM(ctx, modelID)` 构造 LLM
   - 取该模型 `Instruction`（或 `DefaultInstruction`）
   - `adkruntime.New(Config{Model, Instruction, Tools, SessionService, MemoryService, Auditor})`
   - 存入 `sessions[modelID] = {rt, hash}`
5. 返回 Runtime

> **复用语义**：同一 modelID 的所有 session 共享一个 Runtime 实例（Runtime 内部 ADK runner 无状态，按 sessionID 隔离会话）。不是 per-session 一个 Runtime，而是 **per-model 一个 Runtime**。晓军说的"新建 session 时才 new"语义是"按需解析对应模型的 Runtime"，实例在注册表内复用。

> **热更新（无需 Pub/Sub）**：配置变更后，下次 `GetOrCreate` 读到新配置 → 指纹变化 → 自动重建 Runtime。无需 Redis Pub/Sub 通知，纯靠指纹比对。简单可靠，无额外基础设施。

#### 5.3.2 系统级 Runtime（map[UseCase]*Runtime）

`GetOrCreateByUseCase(ctx, useCase)` 流程：与 §5.3.1 同理，key 改为 `UseCase`，配置来源改为该 use case 对应的模型列表（`Provider.BuildLLM(ctx, useCase)`）。

系统级消费者改造为经 Registry：

| 系统级场景 | 改造后调用 | 现状 |
|-----------|-----------|------|
| 提示词增强（enhance） | `registry.GetOrCreateByUseCase(ctx, UseCaseEnhance)` | 现状每次 `BuildLLM`，改为 Registry 复用 |
| session 压缩摘要 | `registry.GetOrCreateByUseCase(ctx, UseCaseCompaction)` | 现状启动 baked，改为 Registry 懒创建 + 热更新 |
| memoryx Kit | 同 compaction use case | 现状 baked，改为 Registry |
| KB embedding 切片 | `EmbeddingConfig()`（非 LLM，不进 Registry） | SPEC-061 按需读 |

> 晓军确认："可以创建多个 LLM runtime，放 map（key 为 use case）"。系统级 Runtime 按 UseCase 索引，各 use case 独立实例（如 enhance 用便宜模型、compaction 用强模型）。指纹比对保证配置变更自动热更新。**比现有"各自 BuildLLM/baked"更统一**：所有 Runtime（session 级 + 系统级）经 Registry 统一管理生命周期与热更新。

### 5.4 chat.Service 改造

`internal/service/chat/chat_service.go`：

```go
type Service struct {
    registry    *adkruntime.Registry   // 替换原 rt *adkruntime.Runtime
    adkSessions session.Service
    sessions    *Manager
    provider    *modelcfg.Provider      // 新增：解析默认模型
    cbReg       *security.CircuitBreakerRegistry
    memoryWrite func(ctx context.Context, sess session.Session)
}
```

`prepareRun` 改造（`chat_service.go:56-116`）：

```go
func (s *Service) prepareRun(...) {
    ...
    if req.SessionID == "" {
        // 新建 session：解析 modelId
        modelID := req.Model
        if modelID == "" {
            if dm, dErr := s.provider.DefaultModel(ctx); dErr == nil && dm != nil {
                modelID = dm.ID   // 用 ModelEntry.ID
            }
        }
        sess, cErr := s.sessions.Create(userID, "chat", modelID)  // 绑定，不可换
        ...
        sessionID = sess.ID
    } else {
        // 已有 session：用 session.ModelID（忽略 req.Model，强制不可换）
        sess, _ := s.sessions.Get(req.SessionID)
        // rt 从 registry 按 sess.ModelID 解析
    }
    ...
    rt, rErr := s.registry.GetOrCreate(ctx, sess.ModelID)
    ...
    // s.rt.Run → rt.Run
}
```

> `Process`/`Stream`/`runAndCollect` 内的 `s.rt.Run(...)` 改为解析出的 `rt.Run(...)`。`s.rt.AppName()` 改为 `rt.AppName()`（所有 Runtime 同 appName）。`scheduleMemoryWrite` 内的 `s.rt.AppName()` 同理。

### 5.5 Agent / Orchestrator / 定时任务改造

#### 5.5.1 实时 agent

实时 agent 走 chat 路径（`ChatRequest`），模型选择同 §5.4。`CreateAgentTaskRequest.Model`（已存在，`orchestrator.go:34`）接线：orchestrator 创建 session 时传 modelId。

`Orchestrator.CreateAgentTask`（`orchestrator.go:32-83`）改造：
```go
func (o *Orchestrator) CreateAgentTask(ctx, userID, req) {
    modelID := req.Model  // 接线（现有字段，值为 ModelEntry.ID）
    if modelID == "" { modelID = o.defaultModel(ctx).ID }
    sess, _ := o.sessions.Create(userID, "agent", modelID)
    t, _ := o.tasks.CreateTask(sess.ID, userID, taskType, skillChain, req.Params, modelID)  // 透传
    ...
}
```

#### 5.5.2 异步任务

`TaskHandler.CreateTask`（`handler/task.go:21-72`）body 加 `model_id`，`TaskService.CreateTask` 透传到 `Task.ModelID`，入队时 `QueueMessage.ModelID` 携带。worker 执行时（SPEC-063 实现）从 `QueueMessage.ModelID` → `Registry.GetOrCreate` → Runtime.Run。

> **执行器实现见 SPEC-063**：`simpleExecutor`（`main.go:279`）当前 no-op。本 spec 确保 modelId 数据贯通到 QueueMessage；执行器（QueueMessage.ModelID → Registry.GetOrCreate → Runtime.Run）由 SPEC-063 实现。

#### 5.5.3 定时任务

`Schedule.ModelID`（新增）→ `executeJob` 透传 → `CreateTask(..., sch.ModelID)` → `QueueMessage.ModelID`。定时任务创建 API（若前端有）body 加 `model_id`，未传用默认。

### 5.6 IMBind 模型选择

`IMBindHandler.Update`（`handler/imbind.go:49`）body 约定 `model_id`（可选 string，= ModelEntry.ID）。前端 imbind 页加 ModelSelector。imbind 的模型用于：IM 消息触发 agent 时，按 user 的 imbind.model_id 解析 Runtime（若 imbind 无 model_id 用默认）。

### 5.7 Runtime 失效与热更新（指纹比对，无需 Pub/Sub）

模型配置变更后（admin 改模型列表），已缓存的 Runtime 实例通过**指纹比对**自动失效重建：

- 每次 `GetOrCreate` / `GetOrCreateByUseCase` 读最新配置（走 SPEC-061 cache→DB），计算 `sha256(json)` 指纹
- 与缓存项的 `hash` 比对：一致则复用，不一致则重建 Runtime 并更新缓存项
- **无需 Redis Pub/Sub**：纯靠下次请求时的指纹比对触发重建。配置变更后，下一个使用该模型的请求自动获得新 Runtime

> 对比 SPEC-061 原方案的 Pub/Sub 重建：指纹比对更简单（无额外基础设施），延迟最多一个请求（配置变更后第一个请求触发重建）。SPEC-061 已删除 Pub/Sub Runtime 重建内容，本 spec 自管热更新。

> **降级（无 Redis）**：SPEC-061 cache 不可用时，配置读直通 Mongo（仍能获取最新配置），指纹比对照常工作，Runtime 热更新不受影响。

### 5.8 前端

#### 5.8.1 ModelSelector 组件

新增 `frontend/app/components/ModelSelector.tsx`：
- props: `value`, `onChange`, `disabled`（已绑定 session 时禁用）
- 调 `GET /api/v1/models/list` 获取可选 LLM 模型列表
- 渲染下拉，value = `ModelEntry.ID`，显示 `Name`（或 Capability 友好名）
- disabled 时显示当前模型名 + 锁图标（不可换）

#### 5.8.2 chat 页

`frontend/app/chat/page.tsx`：
- 新建 session 时显示 ModelSelector（可选，默认选中默认模型）
- session 已存在时 ModelSelector disabled（显示绑定模型）
- chat body 加 `model` 字段（新建时，值为 modelId）

#### 5.8.3 agent 页

`frontend/app/agent/page.tsx`：
- 创建任务 modal 加 ModelSelector
- 任务 body 加 `model` 字段（值为 modelId）
- 异步/定时选项区均显示选择器

#### 5.8.4 imbind 页

`frontend/app/im/bind/page.tsx`：
- 表单加 ModelSelector（可选）
- body 加 `model_id`

#### 5.8.5 admin models 页重写

`frontend/app/admin/models/page.tsx` 重写：
- 调 `GET /api/v1/models`（正确路由，修复现有 `/model-config` 错误）
- 结构化展示 `[]ModelEntry` 列表表格 + 分页
- 每行：ID / Name / Type / UseCases / IsDefault(单选) / 操作(编辑/删除)
- 新增模型按钮 → `POST /models`（后端自动生成 ID）
- 设默认 → `PATCH /models/:id/default`

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `system_configs`，Session/Task/Schedule 文档加 `model_id` 字段，mongo schema-less 自动兼容旧文档） |
| 是否影响现有 API | Yes（chat/agent/task session 创建接 modelId；models API 改结构化 + 分页 + ID；新增多个端点） |
| 性能影响 | 正向：per-model Runtime 复用（非 per-session），开销可控；模型配置读走 SPEC-061 cache。负向：首次某模型 Runtime 创建有 BuildLLM 开销（一次性）；每次 GetOrCreate 多一次指纹计算（sha256，极轻） |
| 是否需要新增 Skill | No |
| ADK 约束可行性 | Yes：per-model Runtime 注册表绕过"构造时绑定"约束，无需改 ADK |
| 旧 session 兼容 | Yes：旧 session 文档无 `model_id` 字段，读取时为零值，`GetOrCreate` 解析为默认模型（向后兼容） |
| 旧 ModelEntry 兼容 | Yes：旧配置无 `ID` 字段，Provider 自动用 `Name` 补全 ID（兼容期），admin 编辑后补正式 ID |
| 异步执行器缺口 | 前置缺口：`simpleExecutor` 是 stub。本 spec 贯通 modelId 数据到 QueueMessage，执行器实现见 **SPEC-063** |
| 前端工作量大 | Yes：新增 ModelSelector 组件 + 4 处页面接入 + admin 页重写 |
| 模型删除影响 | 删除已被 session 绑定的模型 → 该 session 的 Runtime 失效。需校验：删除模型时检查是否有活跃 session 绑定，拒绝或提示 |
| 热更新可靠性 | 指纹比对无需 Pub/Sub，纯请求时触发，无额外基础设施依赖 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/modelcfg/provider.go` | `ModelEntry` 加 `ID` 字段；新增 `DefaultModel`/`ListLLMModels`/ID 唯一校验/旧 ID 补全 | Medium |
| `internal/domain/chat/model.go` | `Session` 加 `ModelID` | Small |
| `internal/domain/chat/contract.go` | `SessionService.Create` 加 modelId 参数 | Small |
| `internal/domain/task/model.go` | `Task`/`QueueMessage`/`ScheduledTask` 加 `ModelID` | Small |
| `internal/repository/notification.go` | `SessionRecord` 加 `ModelID` bson | Small |
| `internal/service/chat/session.go` | `Manager.Create` 加 modelId；映射函数 | Small |
| `internal/service/chat/chat_service.go` | `Service.rt`→`registry`；`prepareRun` 接 modelId | Medium |
| `internal/adk/runtime/registry.go` | **新增** Runtime 注册表（双 map + 指纹热更新） | New |
| `internal/adk/runtime/registry_test.go` | **新增** 注册表 UT | New |
| `internal/logic/agent/orchestrator.go` | `CreateAgentTask` 接 `req.Model` | Small |
| `internal/service/task/service.go` | `CreateTask` 加 modelId 透传 | Small |
| `internal/scheduler/scheduler.go` | `Schedule`/`TaskCreator` 加 modelId | Small |
| `internal/service/enhance/service.go` | 改用 `registry.GetOrCreateByUseCase(Enhance)` | Small |
| `internal/adk/session/summarizer.go` | 改用 Registry（或保持接口，wire 注入时改） | Small |
| `internal/api/handler/session.go` | `Create` 接 `model_id` | Small |
| `internal/api/handler/chat.go` | 透传 `req.Model`（现有字段接线） | Small |
| `internal/api/handler/task.go` | body 加 `model_id` | Small |
| `internal/api/handler/agent.go` | 透传 `req.Model` | Small |
| `internal/api/handler/imbind.go` | body 约定 `model_id` | Small |
| `internal/api/handler/modelconfig.go` | 改结构化返回 + 分页 + ID + 新增 POST/DELETE/PATCH | Medium |
| `internal/api/handler/routes.go` | 注册新路由 | Small |
| `cmd/server/wire.go` | 构造 Registry 注入 chat.Service/enhance/compaction；移除单例 rt | Medium |
| `frontend/app/components/ModelSelector.tsx` | **新增** 选择器组件 | New |
| `frontend/app/chat/page.tsx` | 接入 ModelSelector + model 字段 | Medium |
| `frontend/app/agent/page.tsx` | 接入 ModelSelector | Medium |
| `frontend/app/im/bind/page.tsx` | 接入 ModelSelector | Small |
| `frontend/app/admin/models/page.tsx` | 重写结构化列表 + 分页 + ID + 修复路由 | Medium |
| 各 mock（`repository/mocks`、`service/config/mocks`） | 重新生成 | Auto |

## 8. 测试策略

1. **Unit tests（Go）**：覆盖率基线见 SPEC-045。L1 100%，L3 98%。CI: `ut-workflow.yml`
   - `runtime/registry_test.go`（L2，mock Provider）：覆盖 session 级命中/懒创建/并发安全/默认模型兜底/指纹变化重建；系统级 GetOrCreateByUseCase 各 use case 独立实例
   - `modelcfg/provider_test.go`：`DefaultModel`/`ListLLMModels`/ID 唯一校验/旧 ID 补全
   - `chat/session_test.go`：ModelID 映射
   - `chat/chat_service_test.go`：prepareRun 接 modelId、已有 session 忽略 req.Model
   - `handler/*_test.go`：各端点 modelId 透传
2. **Integration tests**：Docker Compose，验证 session 创建绑定模型 → chat 用对应 Runtime
3. **E2E tests**（前端）：用例编号 `UI-XXX`，CI: `ui-tests.yml`
   - chat 新建选模型 / 已有 session 锁定
   - agent 任务选模型
   - admin models 列表分页 / 新增 / 设默认 / 删除
   - imbind 选模型
4. **审计**：`.agent/skills/go-ut-audit`

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增 ModelSelector 组件时同步编写 E2E（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** chat 新建 session 选模型 + 已有 session 不可换的 E2E 用例
- [ ] **必须** admin models 列表分页 + 新增/删除/设默认的 E2E
- [ ] **必须** agent 任务选模型 E2E
- [ ] **必须** imbind 选模型 E2E
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试
- [ ] **Mock Model Service**：模型选择 UI 测试用 mock（不调真实 LLM），但模型列表 API 用真实后端（返回配置数据）

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体 | **100%** | model 映射、默认模型选择纯逻辑、指纹计算 |
| L2 | 依赖接口，可 mock | **100%** | `runtime/Registry`（mock Provider） |
| L3 | 依赖 MongoDB/Redis | **98%** | `session_repository`、`modelconfig handler` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少 2 个行为验证断言（如 Registry 测试验证返回的 Runtime 非空 + 内部 map 增长）
- [ ] **必须** Registry 并发测试（`go test -race`，多个 goroutine 同时 GetOrCreate 同一 modelID 验证只创建一次）
- [ ] **必须** 指纹热更新测试（mock Provider 返回不同配置 → 验证 Runtime 重建，hash 变化）
- [ ] **必须** 系统级 map[UseCase] 测试（不同 use case 返回不同 Runtime 实例）
- [ ] **必须** 默认模型兜底测试（无 IsDefault 时返回 llm 第一个）
- [ ] **必须** 已有 session 忽略 req.Model 的测试（不可换约束）
- [ ] **必须** 模型 ID 唯一性校验测试
- [ ] **严禁** `t.Skip()` 绕过
- [ ] **严禁** Success 测试只验证 `err == nil`

### 测试模式

- Registry：mock Provider 返回不同 ModelEntry，验证 Runtime 创建/复用/指纹重建/use case 独立
- chat.Service：mock Registry + Manager，验证 modelId 解析与 Runtime 选择
- Handler：`httptest` + mock service，验证 modelId 透传

### CI 门禁

- [ ] `go test -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考:
- `.agent/specs/spec-045-go-service-ut.md`
- `.agent/skills/go-ut-audit/SKILL.md`
- `.github/workflows/ut-workflow.yml`

## 10. 验证标准

### 功能验证

1. `GET /api/v1/models/list` 返回仅 LLM 模型，分页正确（total/page/page_size），每项含 ID
2. admin 新增模型 → 后端自动生成 ID → 列表出现；设默认 → `IsDefault` 唯一标记；删除未被 session 绑定的模型成功
3. **session 绑定**：新建 chat 不传 model → 用默认模型（DB 存具体 ID）；传 model → 绑定该模型；`GET /sessions/:id` 返回 `model_id`
4. **不可换**：已有 session 的 chat 传不同 `model` → 后端忽略，仍用 session.ModelID 对应的 Runtime
5. **per-model Runtime**：两个 session 绑不同模型 → 各自 Runtime 不同（日志/计数可验证 BuildLLM 调用对应模型）
6. **系统级 map[UseCase]**：enhance/compaction 经 Registry.GetOrCreateByUseCase 获取，不同 use case 独立 Runtime 实例
7. **指纹热更新**：admin 改某模型配置 → 下一个使用该模型的请求自动重建 Runtime（新配置生效，无需重启）
8. agent 任务创建传 model → Task.ModelID / QueueMessage.ModelID 携带
9. 定时任务 Schedule.ModelID 透传到 QueueMessage
10. imbind 保存 model_id → 读取可见
11. 旧 session（无 model_id 字段）兼容：读取为零值，解析为默认模型，不报错
12. 旧 ModelEntry（无 ID 字段）兼容：Provider 用 Name 补全 ID，不报错

### 性能验证

13. 1000 次 session 创建（同模型）→ BuildLLM 调用 ≤1 次（Runtime 复用，非 per-session new）
14. 模型配置读走 SPEC-061 cache（Mongo 无额外 FindOne）
15. 指纹计算开销可忽略（sha256 < 1μs，每次 GetOrCreate 一次）

### 反向验证

- [ ] 无启动时"new 所有 LLM"的代码（Registry 懒创建）
- [ ] chat.Service 不再持有单例 `*adkruntime.Runtime`
- [ ] 已有 session 的 chat 不接受 model 切换
- [ ] 模型选择器不展示 embedding 模型
- [ ] ModelEntry 有独立 ID 字段，modelId 全链路用 ID 而非 Name
