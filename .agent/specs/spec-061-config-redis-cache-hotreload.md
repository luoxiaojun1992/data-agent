# 配置统一缓存到 Redis 并支持热更新

> **SPEC-061** | Status: 设计中

## 1. 目标

为所有动态配置（`system_configs` 集合，含模型配置 `model/models`、`model/embedding`）建立统一的 Redis 缓存层与 Cache-Aside 读写规则，并消除"配置值被预加载到应用内存（闭包/map）"的旧模式，保证**配置值**变更无需重启即可在所有读取端生效（热更新）。

> **范围边界（晓军 2026-07-22 确认）**：本 spec 仅处理**配置数据**的缓存与热更新（读写删规则 + 消除配置值预加载）。基于配置构建的**LLM 实例/Runtime 生命周期**（per-model Runtime 注册表、实例缓存、配置变更触发的 Runtime 重建）移交 **SPEC-062** 处理，不在本 spec 范围内。

核心规则（晓军 2026-07-22 确认）：

1. **所有配置缓存到 Redis**
2. **写：先创建/更新 DB 记录，再更新缓存**
3. **删：先删除 DB 记录，再删除缓存**
4. **读：优先读缓存，miss 回源 DB 并回填缓存**
5. **统一从 缓存→DB 读，禁止提前把配置加载到应用内存（结构体/map/闭包），保证热更新**

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（Redis client 已集成于 `internal/infra/redis/client.go`） |
| SPEC-049 | ✅ | 统一模型配置体系（`modelcfg.Provider` + `system_configs` namespace `model`） |
| SPEC-051 | ✅ | Redis 缓存使用先例（`llmcache`），证明 Redis 可选降级模式可行 |
| SPEC-055 | ✅ | 分层架构（repository=interfaces / infra=impl），缓存装饰器归 infra 层 |
| SPEC-058 | ✅ | logic 编排层 + main.go 已瘦身，`wire.go` 为唯一布线入口 |
| SPEC-054 | 📐 设计中 | Sysconfig RBAC 权限修复（与本 spec 写路径独立，不阻塞，但建议先合） |

> 本 spec 不依赖 SPEC-054，二者可并行。SPEC-054 修权限中间件，本 spec 修读写链路。

## 2. 背景

现状（代码核对）：

**两套配置并存，读取方式割裂：**

- 静态启动配置 `internal/config/config.go`：Viper 加载 YAML+env，`main.go:113` 一次性 `config.Load`，含 infra 连接信息。**本 spec 不涉及**（连接配置无热更新需求）。
- 动态配置 MongoDB `system_configs`（`namespace+key+value`）：核心消费者是 `modelcfg.Provider`（`internal/adk/modelcfg/provider.go`）。

**配置值预加载缺口（违反规则 5）：**

| 预加载点 | 文件:行 | 现状 | 后果 |
|---------|--------|------|------|
| Embedding 配置 baked 进闭包 | `wire.go:113-122` `buildEmbedFn`、`wire.go:203-214` `initKnowledgeBase` | 启动时 `modelCfg.EmbeddingConfig()` 取 `EmbeddingEntry` 值，构造 embedding client 并 baked 进 `func(ctx,text)` 闭包 | embedding 模型/地址改了不生效（memory + KB）直到重启 |

> **LLM 实例 baked 进 Runtime**（`wire.go:131,159-167`）属于 LLM 实例生命周期问题，由 **SPEC-062** 的 per-model Runtime 注册表解决，不在本 spec 范围。

**对比：enhance service 是正确范式** — `service/enhance/service.go:65` 每次请求 `s.modelCfg.BuildLLM(ctx, UseCaseEnhance)`，按需读配置（走 cache→DB），天然热更新。本 spec 将此"配置值按需读"范式推广到 embedding 等配置消费端。

**其它现状：**

- `modelcfg.Provider.modelsFromDB()`（`provider.go:93-106`）与 `embeddingFromDB()`（`provider.go:255-268`）每次调用都直接打 MongoDB（`p.repo.Get`），**无缓存**。高频路径（enhance 每请求一次）产生不必要的 DB 读。
- `SysConfigRepository`（`repository/config.go:24-28`）**无 `Delete` 方法**，仅有 `Get/GetAll/Upsert`。规则 3（删 DB 再删缓存）需要新增删除能力。
- `CacheRepository` 接口（`repository/config.go:61-65`：`Get/Set(带ttl)/Delete`）已定义、mock 已生成（`repository/mocks/CacheRepository.go`），但**无实现、无使用**。
- `redis.Client`（`infra/redis/client.go`）的 `Set` 签名是 `Set(ctx, key, value)` **无 TTL 参数**，而 `CacheRepository.Set` 要求 `ttlSeconds`，需新实现而非复用。
- Redis 是**可选**的（`wire.go:228-232` 连接失败则 `deps.redisClient=nil`，服务器照常启动）。缓存层必须支持降级。
- 写路径无任何缓存失效逻辑（`config.Service.Upsert`、`provider.SetModels/SetEmbedding` 直通 mongo）。

## 3. 架构概述

### 模块关系图

```
                              ┌─────────────────────────────────────┐
   消费端                      │  Cache-Aside 装饰器 (infra/cache)     │
 ┌──────────────┐             │  SysConfigCacheRepository            │
 │ modelcfg     │  Get/Upsert  │  ┌─────────┐    miss    ┌────────┐  │
 │  .Provider   │────────────▶│  │ Redis   │───────────▶│ Mongo  │  │
 │ (models/emb) │             │  │ (hit?)  │◀──回填──── │  repo  │  │
 ├──────────────┤             │  └─────────┘            └────────┘  │
 │ config       │             │     │写:DB→Set缓存          │          │
 │  .Service    │             │     │删:DB→Del缓存          │          │
 │ (sysconfig)  │             └─────┼──────────────────────┼──────────┘
 ├──────────────┤                   │                      │
 │ enhance svc  │  (已按需读,天然热更)  │ CacheRepository     │ SysConfigRepository
 │ .BuildLLM    │                   │ (Redis impl)        │ (Mongo impl)
 ├──────────────┤                   ▼                      ▼
 │ buildEmbedFn │  闭包改每次读 EmbeddingConfig()          infra/mongo
 │ initKB       │  (走 cache→DB, 消除启动 baked)            system_config_repo.go
 └──────────────┘
   所有消费端统一走
   SysConfigRepository 接口       infra/redis
   (透明获得缓存,无需感知)         cache_repo.go
```

### 与现有模式对比

| 维度 | 现状 | SPEC-061 后 |
|------|------|------------|
| 配置读路径 | Provider 直读 Mongo（每调用一次 DB IO） | Provider 走 cache 装饰器 → Redis hit / Mongo miss |
| 配置写路径 | Service.Upsert 直通 Mongo，无缓存动作 | DB Upsert 成功 → Redis Set（更新缓存） |
| 配置删路径 | 不支持（无 Delete） | DB Delete 成功 → Redis Del |
| Embedding 配置预加载 | 启动 baked 进闭包 | 消除，闭包内按需读 cache→DB |
| Redis 故障 | 配置读直打 Mongo（本就如此） | 降级直读 Mongo，不阻断服务 |
| 缓存接口 | `CacheRepository` 定义未实现 | Redis 实现就位，复用于 sysconfig |

### 设计原则

1. **装饰器而非侵入**：缓存逻辑封装在 infra 层装饰器，实现 `SysConfigRepository` 接口，包装 mongo 实现。消费端（Provider/Service）无感知，无需改签名。
2. **职责分离**：本 spec 保证**配置数据**的缓存与热更新一致性。基于配置构建的**LLM 实例/Runtime 生命周期**由 SPEC-062 处理。
3. **降级优先**：Redis 不可用时透传 Mongo，不阻断启动与读写。

## 4. API 设计

新增配置删除端点（支持规则 3），并对齐 RBAC（与 SPEC-054 解耦，本 spec 先加 `RequirePermission`）。

| Method | Path | Description | Permission |
|--------|------|-------------|------------|
| GET | `/api/v1/sysconfig/:namespace` | 读配置（现有，走缓存） | `PermSystemConfig` |
| PUT | `/api/v1/sysconfig/:namespace` | upsert 配置（现有，写DB+刷缓存） | `PermSystemConfig` |
| **DELETE** | `/api/v1/sysconfig/:namespace` | **删除配置**（新增，删DB+删缓存） | `PermSystemConfig` |
| GET | `/api/v1/models` | 模型配置读（现有，走缓存） | `PermModelConfig` |
| PUT | `/api/v1/models` | 模型配置写（现有，写DB+刷缓存） | `PermModelConfig` |

> DELETE 请求体：`{"key":"<key>"}`，按 `(namespace, key)` 删除单条。

## 5. 详细设计

### 5.1 配置缓存基础设施

#### 5.1.1 `CacheRepository` 的 Redis 实现

新增 `internal/infra/redis/cache_repository.go`，实现 `repository.CacheRepository`（接口已存在于 `repository/config.go:61-65`）：

```go
package redis

// CacheRepo implements repository.CacheRepository over *redis.Client.
type CacheRepo struct {
    client *redis.Client
}

func NewCacheRepo(client *redis.Client) *CacheRepo { return &CacheRepo{client: client} }

func (c *CacheRepo) Get(ctx context.Context, key string) (string, error) {
    v, err := c.client.Get(ctx, key).Result()
    if err == redis.Nil {
        return "", nil  // miss 返回 ("", nil)，调用方据空串判断 miss
    }
    return v, err
}

func (c *CacheRepo) Set(ctx context.Context, key, value string, ttlSeconds int) error {
    ttl := 0 * time.Second
    if ttlSeconds > 0 {
        ttl = time.Duration(ttlSeconds) * time.Second
    }
    return c.client.Set(ctx, key, value, ttl).Err()
}

func (c *CacheRepo) Delete(ctx context.Context, keys ...string) error {
    return c.client.Del(ctx, keys...).Err()
}
```

> 不复用现有 `redis.Client.Set(ctx,key,value)`（无 TTL），新实现直接操作 `*redis.Client` 支持 TTL。

#### 5.1.2 `SysConfigRepository` 接口扩展（新增 Delete）

`internal/repository/config.go`：

```go
type SysConfigRepository interface {
    Get(ctx context.Context, namespace, key string) (*model.SystemConfig, error)
    GetAll(ctx context.Context, namespace string) ([]model.SystemConfig, error)
    Upsert(ctx context.Context, namespace, key, value string) error
    Delete(ctx context.Context, namespace, key string) error   // 新增
}
```

同步在 `infra/mongo/system_config_repository.go` 实现 `Delete`（`DeleteOne` filter `namespace+key`），并在 mock（`mocks/SysConfigRepository.go`）重新生成（`go generate`）。

> 删除语义遵循项目惯例：不存在不返回错误（幂等），返回 `nil`。

#### 5.1.3 Cache-Aside 装饰器

新增 `internal/infra/cache/sysconfig_cache.go`，实现 `SysConfigRepository` 接口，包装 mongo repo + `CacheRepository`：

```go
package cache

type SysConfigCacheRepo struct {
    mongo repository.SysConfigRepository   // 被装饰的 mongo 实现
    cache repository.CacheRepository        // Redis 实现（nil 时降级透传）
    ttl   int                               // 兜底 TTL 秒，默认 600
}

func NewSysConfigCacheRepo(mongo repository.SysConfigRepository, cache repository.CacheRepository, ttlSec int) *SysConfigCacheRepo {
    if ttlSec <= 0 { ttlSec = 600 }
    return &SysConfigCacheRepo{mongo: mongo, cache: cache, ttl: ttlSec}
}
```

**缓存 Key 规则：** `syscfg:{namespace}:{key}`（单条）；`syscfg:ns:{namespace}:all`（GetAll 聚合）。

**序列化：** 缓存 value 存 JSON。单条存 `{"id","namespace","key","value","updated_at"}`；GetAll 存 JSON 数组。

### 5.2 Cache-Aside 读写删规则（映射规则 1-4）

```
                  规则4: 优先读缓存
   Get(ns,key) ──────────────────────────────────────┐
     │ cache != nil ?                                  │
     ├─是─▶ cache.Get(syscfg:{ns}:{key})               │
     │        ├─ hit (非空) ─▶ 反序列化 ─▶ return       │ (不触 DB)
     │        └─ miss/err  ─▶ mongo.Get ─▶ 回填 cache.Set(key, val, ttl) ─▶ return
     └─否(cache nil) ─▶ mongo.Get ─▶ return             │ (降级直读)
                                                       │
                  规则2: 先写DB再更新缓存               │
   Upsert(ns,key,val) ──▶ mongo.Upsert ──成功──▶ cache.Set(syscfg:{ns}:{key}, val, ttl)
                                  │                  │ + cache.Del(syscfg:ns:{ns}:all)  // 失效聚合缓存
                                  └─err ─▶ return err (不碰缓存, 保持旧值一致性)
                                                       │
                  规则3: 先删DB再删缓存                  │
   Delete(ns,key) ──▶ mongo.Delete ──成功──▶ cache.Del(syscfg:{ns}:{key}, syscfg:ns:{ns}:all)
                                  │
                                  └─err ─▶ return err (不碰缓存)
```

**规则 1（全部缓存到 Redis）：** 所有 `system_configs` 读写在装饰器内自动维护缓存，消费端无需显式操作缓存。**不存在任何绕过装饰器的直读 Mongo 配置路径**（见 §5.3 消除预加载）。

**一致性要点：**
- 写/删先操作 DB，成功后才操作缓存 —— DB 是 SSOT，缓存是副本。DB 失败绝不动缓存，避免缓存写入比 DB 新的脏数据。
- 兜底 TTL（10 min）应对极端情况（如写 DB 成功但进程崩溃未刷缓存）—— TTL 到期自动失效，最终一致。
- GetAll 命中后聚合缓存 `syscfg:ns:{ns}:all`；任一 Upsert/Delete 都 Del 该聚合 key（失效，下次 GetAll 回源重建）。

### 5.3 消除配置值预加载（规则 5 — 配置值热更新）

消除"启动时读配置值 baked 进闭包"的模式，改为运行时按需读（走 cache→DB）。

> **范围说明**：本节仅处理**配置值读取**的预加载消除（embedding 配置闭包）。LLM 实例/Instruction 的物化与 Runtime 注入属于实例生命周期，由 **SPEC-062** 的 Runtime 注册表处理。

#### 5.3.1 Embedding 配置闭包改造

**`wire.go` `buildEmbedFn`（当前 line 113-122）：**

```go
// 现状（违反规则5）：启动时读 EmbeddingConfig()，baked 进闭包
func buildEmbedFn(deps *serverDependencies) func(ctx, text) ([]float32, error) {
    cfg := deps.modelCfg.EmbeddingConfig()        // ← 启动快照
    e := adkmemory.NewOpenAIEmbedding({...cfg})
    return func(ctx, text) { return e(ctx, text) }  // ← 闭包持有旧 cfg 的 client
}
```

**改造后：** 闭包内每次调用读最新配置（走 cache→DB），embedding client 按配置内容缓存实例（相同配置复用，配置变化时重建）：

```go
func buildEmbedFn(deps *serverDependencies) func(ctx, text) ([]float32, error) {
    var (
        lastCfg  modelcfg.EmbeddingEntry
        embedder adkmemory.EmbeddingFunc
    )
    return func(ctx context.Context, text string) ([]float32, error) {
        cfg := deps.modelCfg.EmbeddingConfig()   // 每次走 cache→DB，热更新
        if cfg.BaseURL == "" { return nil, nil }
        // 配置未变则复用 embedder 实例；变化时重建
        if embedder == nil || cfg != lastCfg {
            e := adkmemory.NewOpenAIEmbedding(adkmemory.OpenAIEmbeddingConfig{
                BaseURL: cfg.BaseURL, Model: cfg.Model, APIKey: cfg.APIKey,
            })
            embedder = func(ctx context.Context, text string) ([]float32, error) { return e(ctx, text) }
            lastCfg = cfg
        }
        return embedder(ctx, text)
    }
}
```

> embedding client 实例缓存是"对象缓存"非"配置缓存"，不违反规则 5（规则 5 禁止的是配置值预加载，不是禁止缓存构建产物）。配置变化（`cfg != lastCfg`）即重建 embedder，保证配置热更新传导。

**`wire.go` `initKnowledgeBase`（当前 line 203-214）：** 同样改造，移除启动时 `embCfg := deps.modelCfg.EmbeddingConfig()` 快照，闭包内按需读。

#### 5.3.2 消费端统一入口

所有读取 `system_configs` 的代码必须经 `SysConfigRepository`（cache 装饰后的实例）。**禁止**新增任何"启动加载到 struct/map/全局变量"的配置读取。审查清单：

| 消费端 | 文件 | 读法 | 改造 |
|--------|------|------|------|
| `modelcfg.Provider` | `provider.go:97,259` | `p.repo.Get` | repo 替换为 cache 装饰器即可（无签名改动） |
| `config.Service` | `service/config/service.go` | `s.sysConfig.*` | repo 替换为 cache 装饰器即可 |
| `buildEmbedFn` | `wire.go:113-122` | 启动快照 baked | 改闭包内按需读（§5.3.1） |
| `initKnowledgeBase` | `wire.go:203-214` | 启动快照 baked | 改闭包内按需读（§5.3.1） |

> LLM 实例构造（`BuildLLM`）、Runtime/Instruction 注入的预加载消除由 **SPEC-062** 负责。本 spec 确保 `modelcfg.Provider` 读取配置走 cache→DB（repo 替换为装饰器），为 SPEC-062 的 Runtime 注册表提供热更新配置源。

### 5.4 Redis 降级策略

- `wire.go` 初始化：`initTaskQueue` 内 Redis 连接成功后，额外构造 `CacheRepo`（`redis.NewCacheRepo(redisClient.Client())`）并注入 `SysConfigCacheRepo`。
- Redis 连接失败（`deps.redisClient == nil`）：`SysConfigCacheRepo.cache` 字段为 `nil`，所有方法检查 `c.cache == nil` 时透传 mongo，等价于无缓存直读 DB（与现状一致，不阻断服务）。
- 运行时 Redis 瞬断：`cache.Get/Set/Del` 返回 error 时，装饰器**降级透传 mongo**（读）或**忽略缓存错误继续**（写后刷缓存失败只记日志，不回滚 DB——DB 已是 SSOT，缓存靠兜底 TTL 自愈）。
- 绝不因 Redis 故障阻断业务读写。

### 5.5 数据模型

无新 DB 集合。`system_configs` 文档结构不变（`_id/namespace/key/value/updated_at`）。

**建议新增索引**（当前 `system_configs` 无索引，见 `infra/mongo/client.go:79-111` EnsureIndexes 未覆盖）：

```go
// client.go EnsureIndexes 追加
colls[model.CollSystemConfigs] = []mongo.IndexModel{
    {Keys: bson.D{{Key: "namespace", Value: 1}, {Key: "key", Value: 1}}, Options: options.Index().SetUnique(true)},
}
```

**Redis Key 总览：**

| Key | 用途 | TTL | 失效时机 |
|-----|------|-----|---------|
| `syscfg:{namespace}:{key}` | 单条配置缓存 | 600s | Upsert 后 Set 更新；Delete 后 Del |
| `syscfg:ns:{namespace}:all` | namespace 聚合缓存 | 600s | 该 ns 任一 Upsert/Delete 后 Del |

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 `system_configs`，仅建议加唯一索引） |
| 是否影响现有 API | Yes（新增 DELETE 端点；GET/PUT 链路透明加缓存；PUT 后多一步刷缓存） |
| 性能影响 | 正向：高频配置读（enhance 每请求）从 Mongo IO 降为 Redis 内存读，延迟显著下降。负向：首次 miss 多一次回填。整体收益明显 |
| 是否需要新增 Skill | No |
| 是否影响现有 UT | Yes（`SysConfigRepository` 加 `Delete` 方法，所有 mock/实现需更新；config/provider/enhance 测试需适配 cache 装饰器） |
| Redis 降级是否安全 | Yes（cache==nil 透传 mongo，与现状一致） |
| 缓存一致性风险 | 低：write-through（DB 先成功）+ 兜底 TTL（600s）双保险；写后刷缓存失败靠 TTL 自愈 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/repository/config.go` | `SysConfigRepository` 加 `Delete` 方法 | Small |
| `internal/repository/mocks/SysConfigRepository.go` | mock 重新生成 | Auto |
| `internal/infra/redis/cache_repository.go` | **新增** `CacheRepository` 的 Redis 实现 | New |
| `internal/infra/cache/sysconfig_cache.go` | **新增** Cache-Aside 装饰器 | New |
| `internal/infra/cache/sysconfig_cache_test.go` | **新增** 装饰器 UT | New |
| `internal/infra/mongo/system_config_repository.go` | 加 `Delete` 实现 | Small |
| `internal/infra/mongo/client.go` | `EnsureIndexes` 加 `system_configs` 唯一索引 | Small |
| `internal/adk/modelcfg/provider.go` | repo 注入改 cache 装饰器（无签名改动） | Small |
| `cmd/server/wire.go` | `buildEmbedFn`/`initKnowledgeBase` 改闭包按需读；构造 cache 装饰器注入 | Medium |
| `internal/service/config/service.go` | 加 `Delete` 方法（透传 repo） | Small |
| `internal/api/handler/config.go` | 加 `DELETE /sysconfig/:namespace` handler | Small |
| `internal/api/handler/routes.go` | 注册 DELETE 路由 + `RequirePermission` 对齐 | Small |
| `internal/service/config/service_test.go` | 适配 cache 装饰器 | Medium |

## 8. 测试策略

1. **Unit tests（Go）**：覆盖率基线见 SPEC-045。L1 纯逻辑 100%，L3 完整链路 98%。CI: `ut-workflow.yml`
   - `infra/cache/sysconfig_cache_test.go`（L2，mock `SysConfigRepository` + `CacheRepository`）：覆盖 hit/miss/写后刷/删后清/降级透传/GetAll 聚合失效
   - `infra/redis/cache_repository_test.go`（L3，需 Redis）：用 miniredis 或真 Redis
   - `wire.go` embedding 闭包：验证配置变化时 embedder 重建
2. **Integration tests**：条件使用 Docker Compose（Redis+Mongo），验证端到端 Cache-Aside（写→缓存可见、删→缓存清空）
3. **E2E tests**（前端涉及 DELETE 端点时）：用例编号 `UI-XXX`，CI: `ui-tests.yml`
4. **审计**：使用 `.agent/skills/go-ut-audit` 审查 UT 质量

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 若前端新增"删除配置"交互，同步编写 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试
- [ ] **配置缓存为后端行为**：若仅后端改动无前端交互，E2E 无需新增用例，但须有集成测试覆盖 Cache-Aside

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `config` hash 工具 |
| L2 | 依赖接口，可 mock | **100%** | `infra/cache` 装饰器（mock repo + cache） |
| L3 | 依赖 MongoDB/Redis | **98%** | `infra/redis/cache_repository.go`、`infra/mongo/system_config_repository.go` |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 2 个行为验证断言（除 `err == nil` 外验证缓存命中/回填/失效）
- [ ] **必须** 装饰器测试验证 cache.Set/Delete 被调用的 key 和 value 正确（mock 期望调用参数）
- [ ] **必须** 降级测试（cache==nil）验证透传 mongo 且不 panic
- [ ] **必须** 写后刷缓存失败的测试验证：DB 已写成功、缓存刷失败、返回 nil error（降级不阻断）
- [ ] **严禁** `t.Skip()` 绕过（Redis 依赖用 miniredis 内存模拟）
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证缓存副作用

### 测试模式

- 装饰器：`mockery` 生成的 `SysConfigRepository`/`CacheRepository` mock，table-driven 覆盖 hit/miss/写/删/降级
- `cache_repository`：用 `github.com/alicebob/miniredis/v2` 避免 Redis 真依赖

### CI 门禁

- [ ] `go test -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

参考:
- `.agent/specs/spec-045-go-service-ut.md`
- `.agent/skills/go-ut-audit/SKILL.md`
- `.github/workflows/ut-workflow.yml`

## 10. 验证标准

### 配置缓存 + 值热更新

1. `PUT /api/v1/sysconfig/model` 更新 `models` → Redis `syscfg:model:models` 立即更新（`redis-cli GET` 可见最新 JSON）
2. `DELETE /api/v1/sysconfig/model` 删 key → Redis 对应 key 立即消失
3. `GET /api/v1/sysconfig/model` 命中缓存（Mongo 无新增 FindOne，可通过日志/计数验证）
4. **热更新（embedding）**：改 `model/embedding` 配置后，新发起的 memory/KB embedding 请求使用新地址（无需重启）—— 闭包内按需读生效
5. **降级**：停 Redis，服务正常启动，配置读写直通 Mongo 无报错
6. **缓存一致性**：写 DB 成功但模拟刷缓存失败 → 兜底 TTL 600s 后缓存自愈为新值
7. Go UT 覆盖率 ≥ 98%，装饰器 L2 100%

> **LLM 实例热更新**（模型配置变更后 chat/agent 使用新 LLM 实例）由 **SPEC-062** 的 Runtime 注册表 + 配置变更失效机制处理，不在本 spec 验证范围。

### 反向验证（禁止项）

- [ ] 全局 grep 无"启动加载配置到 struct/map/全局变量"的新代码（仅允许静态 infra 配置 `config.Config`）
- [ ] 无任何消费端直接 `NewSystemConfigRepository(mongoClient.DB())` 绕过 cache 装饰器
- [ ] enhance service 保持每次 `BuildLLM`（现有正确范式不回退）
