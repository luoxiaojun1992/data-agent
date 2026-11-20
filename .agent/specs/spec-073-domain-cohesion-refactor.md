# 领域内聚重构：业务领域 logic/service/db_model 垂直切片

> **SPEC-073** | Status: 设计中（细化，2026-11-20 对齐最新 main）

## 目标

将当前「水平分层」（domain / service / logic / repository / infra 分散）的架构，重构为「**一个业务领域的 entity / service / repository 接口内聚在同一个 domain 包内**」的垂直切片组织，消除单一业务领域被拆散导致的混乱。infra 技术适配器（mongo / arcadedb / qdrant / seaweedfs …）保留在技术层，通过 repository 接口注入。

## 背景 / 动机

- 现状：`ModelEntry` 在 `internal/domain/modelconfig`、`SystemConfig`/`User`/`Role` 在 `internal/domain/model`、模型 repo 在 `internal/repository/config.go` + `internal/infra/mongo/model_config_repository.go`、service 在 `internal/service/config`——**同一个「模型配置」领域被拆散在 4 个包**，跨层追踪困难。
- 正确方向：按业务领域垂直组织，每个领域包内聚自己的 entity + service + repository 接口，infra 实现靠接口注入。
- 触发点：SPEC-066 已把 `ModelEntry` 等归位到 `internal/domain/modelconfig`，SPEC-057 已把 domain 实体去 bson tag（infra converter 转换）。本 spec 在此基础上做「service + repository 接口」的领域内聚，**不回退 SPEC-057 的 domain 纯净性**。

## 前置依赖检查（全部已实现，可展开）

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-066 配置存储拆分 | ✅ | modelconfig 领域实体/db 结构已落地 |
| SPEC-067 意图识别 + 相关性检查 | ✅ | guard 归属包受本重构影响（归入 chat 领域）|
| SPEC-069 compaction 机制重构 | ✅ | 已完成，不再阻塞 |
| SPEC-071 agent 调用子 agent | ✅ | sub agent 归属 runtime，本 spec 不触碰 |

## 现状盘点（2026-11-20 最新 main，commit `abee83d`）

### 各层包清单

| 层 | 包 | 性质 |
|---|---|---|
| `domain/`（12） | agent, artifact, chat, consts, feishu, knowledge, model, modelconfig, notification, security, skill, task | 纯实体（部分已含接口）|
| `service/`（21） | apicollection, artifact, audit, auth, chat, config, enhance, feishu, guard, hermes, humanchannel, im, knowledge, monitor, notification, pii, rbac, skill, task, user, voice | 业务逻辑 |
| `logic/`（15） | agent, fsops, invite, knowledge, openapi, pptx, report, sql, stats, webfetch, webimport, websearch, workspace | 编排 + 工具 |
| `repository/`（11 文件）| artifact, config, feishu_config, graph, invite, knowledge, notification, session, skill_config, task, user | repo 接口（扁平文件）|
| `infra/`（11） | arcadedb, cache, llmcache, llmstats, metrics, mongo, mysql, qdrant, redis, seaweedfs, vault | 技术适配器 |
| `adk/`（8） | — | ADK 引擎层（不动）|
| `api/handler/` | 40+ 文件（按领域命名 `*.go`）| HTTP 适配器 |
| `cmd/server/wire.go` | serverDependencies + 60+ init 函数 | 依赖注入 |

### 关键痛点（实测）

1. **`internal/domain/model` 是杂项包**：混合了 `User`（user 领域）、`Role`/`PermissionInfo`/`RBACRole`（rbac 领域）、`APICollection`（apicollection 领域）。
2. **`internal/repository/config.go` 混合 5 个接口**：`SysConfigRepository`（config）、`ModelConfigRepository`/`ModelDefaultRepository`（modelconfig）、`RoleRepository`（rbac）、`IMBindRepository`（im）。
3. **「知识库」拆散 6 处**：`domain/knowledge` + `service/knowledge` + `logic/knowledge` + `repository/knowledge.go` + `repository/graph.go` + `infra/mongo/kb_repository.go` + `infra/arcadedb`。
4. **部分领域已开始内聚**（利好）：`domain/chat` 已含 `ChatService` 接口、`domain/task` 已含 `TaskService`/`TaskRunService` 接口、`domain/security` 已含 `Redactor`/`AlertLogger`/`CircuitBreaker`——垂直切片方向已在 chat/task/security 领域萌芽。

## 目标架构（垂直切片）

```
internal/domain/            # 业务领域（每个领域一个包，内聚）
  modelconfig/
    entity.go               # ModelEntry / ModelDefault / ModelType（纯，无 bson，保持 SPEC-057）
    service.go              # config service（从 internal/service/config 迁入，拆出非 modelconfig 部分）
    repository.go           # ModelConfigRepository / ModelDefaultRepository 接口
  chat/
    entity.go               # Message / ChatEvent / ChatRequest / ChatResponse
    service.go              # chat service + ChatService 接口（已部分内聚）
    enhance.go              # enhance（从 service/enhance 迁入）
    guard.go                # intent/relevance guard（从 service/guard 迁入）
    voice.go                # 语音转写（从 service/voice 迁入）
    humanchannel.go         # 人机交互信道（从 service/humanchannel 迁入）
    repository.go           # SessionRepository 接口
  task/
    entity.go               # Task / TaskRun / TaskProgress（已含接口，补 service 实现）
    service.go
    repository.go           # TaskRepository / TaskRunRepository / QueueRepository
  knowledge/
    entity.go               # KnowledgeDoc / Chunk / SearchResult
    service.go              # 从 service/knowledge 迁入
    logic.go                # 索引编排（从 logic/knowledge 迁入）
    repository.go           # KBRepository / VectorRepository / FileRepository / GraphRepository
  rbac/
    entity.go               # RBACRole / RBACPermission / Role / PermissionInfo（从 domain/model 迁入）
    service.go              # 从 service/rbac 迁入
    repository.go           # RoleRepository
  user/
    entity.go               # User / UserRole / UserStatus（从 domain/model 迁入）
    service.go              # 从 service/user 迁入
    auth.go                 # 从 service/auth 迁入
    invite.go               # 从 logic/invite 迁入
    repository.go           # UserRepository / InviteRepository
  artifact/
    entity.go               # Artifact
    service.go              # 从 service/artifact 迁入
    repository.go           # ArtifactRepository
  skill/
    entity.go               # SkillConfig
    service.go              # 从 service/skill 迁入
    repository.go           # SkillConfigRepository
  feishu/
    entity.go               # Config
    service.go              # 从 service/feishu 迁入
    repository.go           # FeishuConfigRepository
  notification/
    entity.go
    service.go              # 从 service/notification 迁入
    repository.go           # NotificationRepository
  audit/
    entity.go
    service.go              # 从 service/audit 迁入
    repository.go           # AuditRepository
  apicollection/
    entity.go               # APICollection（从 domain/model 迁入）
    service.go              # 从 service/apicollection 迁入
    repository.go           # APICollection 接口
  im/
    entity.go
    service.go              # 从 service/im 迁入
    repository.go           # IMBindRepository
  agent/
    entity.go               # Manager / Secret
    service.go              # 从 service/hermes 迁入
    logic.go                # executor/orchestrator（从 logic/agent 迁入）
  config/
    entity.go               # SystemConfig
    service.go              # 从 service/config 迁入（仅 SysConfig 部分）
    repository.go           # SysConfigRepository
  security/                 # 保持现状（已内聚 Redactor/Auditor/CircuitBreaker）
  consts/                   # 保持现状（共享常量）
```

### 保留不动的层

| 层 | 说明 |
|---|---|
| `internal/infra/` | 技术适配器保留，按接口实现对应领域 repo。`mongo/converter.go` 的 bson 转换保留（SPEC-057 成果）。`infra/mongo/*_repository.go` 改名/归组为按领域，但物理位置可暂留 infra/mongo。 |
| `internal/adk/` | ADK 引擎层完全不动。 |
| `internal/logic/`（工具类）| fsops / sql / pptx / webfetch / webimport / websearch / openapi / report / workspace / stats 属「横切工具」，不是业务领域，保留 logic（或后续迁 `internal/tool/`，本 spec 不展开）。 |
| `internal/api/handler/` | handler 保持现状（按领域命名），仅改 import 路径 + 依赖注入来源。 |
| `internal/config/`、`internal/queue/`、`internal/scheduler/`、`internal/worker/` | 横切基础设施，不动。 |

## 依赖规则（调整后）

> 延续 DDD 铁律，垂直切片不改变分层语义，只改变「物理归属」。

1. **domain 包内**：`entity` 纯结构无 bson、无外部依赖；`service` 依赖 `repository` 接口（同包）与 `entity`；`repository` 接口只依赖 `entity`。包内禁止 service 依赖 infra 实现。
2. **跨领域**：只通过 repository/service **接口** 依赖，禁止 import 对方包的 service 实现。
3. **infra → domain**：infra 适配器实现 domain 包的 repository 接口，infra 可依赖 domain，domain 永不依赖 infra（与现状一致）。
4. **handler → domain**：handler 依赖 domain 包的 service，不再依赖 `internal/service/*`。
5. **禁止**：domain 包 import `internal/infra/*`；service 实现 import 其他领域的 service 实现（跨领域编排保留在 `logic` 或显式接口）。

## 迁移映射表

| 现有包 | 目标 | 说明 |
|---|---|---|
| `internal/domain/modelconfig` | `domain/modelconfig/entity.go` | 已归位，补 service/repository |
| `internal/domain/model/model.go`(User/UserRole/UserStatus) | `domain/user/entity.go` | 拆出 User |
| `internal/domain/model/rbac.go` + `model.go`(Role/PermissionInfo) | `domain/rbac/entity.go` | 拆出 RBAC |
| `internal/domain/model/api_collection.go` | `domain/apicollection/entity.go` | 拆出 APICollection |
| `internal/service/config` | `domain/modelconfig/service.go` + `domain/config/service.go` | 按接口拆分 |
| `internal/service/chat` + `enhance` + `guard` + `voice` + `humanchannel` | `domain/chat/` | 合并 |
| `internal/service/task` | `domain/task/service.go` | 内聚 |
| `internal/service/knowledge` | `domain/knowledge/service.go` | 内聚 |
| `internal/service/rbac` | `domain/rbac/service.go` | 内聚 |
| `internal/service/user` + `auth` | `domain/user/service.go` + `auth.go` | 合并 |
| `internal/service/artifact` | `domain/artifact/service.go` | 内聚 |
| `internal/service/skill` | `domain/skill/service.go` | 内聚 |
| `internal/service/feishu` | `domain/feishu/service.go` | 内聚 |
| `internal/service/notification` | `domain/notification/service.go` | 内聚 |
| `internal/service/audit` | `domain/audit/service.go` | 内聚 |
| `internal/service/apicollection` | `domain/apicollection/service.go` | 内聚 |
| `internal/service/im` | `domain/im/service.go` | 内聚 |
| `internal/service/hermes` | `domain/agent/service.go` | 内聚 |
| `internal/service/monitor` + `health` | 保留（横切监控）| 不动 |
| `internal/service/pii` | 保留或并入 `domain/security` | 二选一，倾向保留 service/pii |
| `internal/repository/*.go`（11 文件）| 拆到各 `domain/<领域>/repository.go` | 按接口归属 |
| `internal/logic/agent` | `domain/agent/logic.go` | 内聚 |
| `internal/logic/knowledge` | `domain/knowledge/logic.go` | 内聚 |
| `internal/logic/invite` | `domain/user/invite.go` | 内聚 |
| `internal/logic/工具类` | 保留 `logic/` | 不动 |
| `internal/infra/mongo/*` | 保留 infra/mongo（按领域改名可后续）| 只改实现的接口 import 路径 |

## 迁移顺序（增量，每阶段一个 PR，禁止大爆炸）

按「依赖最少 → 依赖最多」排序，每个阶段 CI 全绿再进下一阶段：

| 阶段 | 领域 | 前置 | 影响面 |
|---|---|---|---|
| S1 | `config`（SysConfig 独立）| 无 | 拆 `domain/model` 的 SystemConfig、`repository/config.go` 的 SysConfigRepository |
| S2 | `modelconfig` | S1 | 拆 config service 的模型部分 |
| S3 | `skill` | 无 | 领域独立 |
| S4 | `feishu` | 无 | 领域独立 |
| S5 | `apicollection` | 无 | 拆 `domain/model/api_collection.go` |
| S6 | `notification` | 无 | 领域独立 |
| S7 | `audit` | S6 | 依赖 notification 语义 |
| S8 | `artifact` | 无 | 领域独立 |
| S9 | `user`（含 auth/invite）| 无 | 拆 `domain/model` 的 User |
| S10 | `rbac` | S9 | 拆 `domain/model` 的 Role/RBAC |
| S11 | `im` | S9 | IMBind 依赖 user 语义 |
| S12 | `task` | S9（task 归属 user）| 内聚（已含接口，最顺）|
| S13 | `knowledge` | S9/S12 | 依赖面大，后置 |
| S14 | `chat`（含 enhance/guard/voice/humanchannel）| S2/S13 | 核心领域，最后 |
| S15 | `agent`（含 hermes）| S12/S13/S14 | 编排层，收尾 |

> 每阶段完成后 `internal/domain/model` 逐步清空，最终删除该杂项包。

## 验收标准

1. `internal/domain/model` 杂项包被清空并删除，所有实体归位到对应领域包。
2. `internal/repository/` 扁平文件清空，接口归入各 `domain/<领域>/repository.go`。
3. `internal/service/` 仅剩横切（monitor/pii/health），其余迁入 domain。
4. `internal/logic/` 仅剩工具类与横切编排（stats 等）。
5. `go build ./...`、`golangci-lint run ./...`、`go test -gcflags=all=-l ./...` 全绿，覆盖率 ≥ 90%（COVERPKG 不变）。
6. wire.go 依赖注入按领域重组（每阶段增量改，不一次性重写）。
7. CI 全绿（Lint / Go UT / UI Tests / License Scan），无功能回归。

## 风险与约束

- **禁止大爆炸**：一次迁移一个领域，每阶段独立 PR + CI 全绿。
- **禁止回退 SPEC-057**：entity 保持无 bson tag，bson 转换仍在 infra converter。
- **禁止改行为**：纯「移动 + import 调整」，不改任何业务逻辑、不改测试断言。
- **gomonkey**：测试用 `-gcflags=all=-l`，移动后 import 路径需同步。
- **wire.go**：serverDependencies 结构体字段与 init 函数需随领域迁移调整，是主要回归风险点。

## 提交约定

```bash
git add .agent/specs/spec-073-domain-cohesion-refactor.md .agent/specs/INDEX.md
git commit -m "docs: refine SPEC-073 domain cohesion refactor (align latest code)"
```
