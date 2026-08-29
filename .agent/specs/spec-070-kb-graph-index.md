# KB 切片图数据库索引（Cayley）+ 图访问共用组件 + 图谱搜索 Skill

> **SPEC-070** | Status: 详细设计（2026-08-29 晓军拍板：Cayley 替代 Neo4j，其余设计不变）

## 目标

1. 在 KB 切片索引进 Qdrant 之后，**同时把切片索引进 Cayley 图数据库**：仅 `Chunk` 节点 + `RELATED_TO` 相似边（最小化图模型）。
2. 图数据库访问**抽象成共用组件**（`GraphRepository` 接口 + Cayley infra 适配器），供 KB 索引写入与图谱搜索 Skill 共同复用。
3. **新增 `knowledge_graph_search` Skill**：Agent 通过该工具查询图数据库，返回 topN 相关节点；查询按 `system_admin` 策略过滤（与 KB 可见性一致）。
4. Cayley 以 **Go 库嵌入** data-agent 进程（bbolt 单文件持久化），无需独立图数据库服务；seed 仅 **skill 配置（predefinedSkills）**（Cayley 无 schema），无存量数据回填。

## 背景 / 动机

- 现状：KB 切片索引 `AddChunks(docID, texts)` 只做「PII 脱敏 → embedding → MongoDB `kb_chunks` + Qdrant 向量」（`internal/service/knowledge/service.go:139-193`）。
- 缺口：向量检索只能表达语义相似度，无法表达切片间的显式关系。图数据库用 `Chunk` 节点 + `RELATED_TO` 边显式存储"相关"关系，支撑图遍历与后续 GraphRAG。
- 找相关节点最简机制：**复用现有 embedding + Qdrant 向量检索**——每个 chunk 索引时已算出向量 `vec`，用它在 Qdrant `Search(topN)` 检索最相似的已索引 chunk 即"相关节点"，零新增相似度算法。
- **为何 Cayley 替代 Neo4j**：Cayley 是 Go 原生、**Apache-2.0**（无 Neo4j CE 的 GPLv3 copyleft 顾虑）、可嵌入进程（无需 JVM/独立服务），与 data-agent 单二进制部署哲学一致。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 知识库系统 | ✅ 已实现 | `AddChunks` 索引流程已就绪 |
| SPEC-068 知识库 PII 脱敏 | ✅ 已实现 | 切片脱敏后落库，图索引接在脱敏之后 |
| SPEC-065 external_api_* tools | ✅ 已实现 | Skill 三步注册范式（Seed + specs() + wire）可复用 |
| ⚠️ Qdrant Search filter 扩展（本 spec 前置修复项） | ✅ 已完成（commit 9911090） | `Client.Search` 已支持 filter + `VectorStore.Search` 透传，`vectorSearch` 权限过滤已回归验证生效。剩余前置：Qdrant 按 doc_id 删点能力（`DeletePoints`）随本 spec 实现 |

## 架构概述

图数据库访问抽象为**独立共用组件**（对齐 Qdrant 的 `internal/infra/qdrant` + `VectorRepository` 组织）。Cayley 作为 Go 库**嵌入进程**，不引入独立图数据库容器：

```
internal/repository/graph.go        ← GraphRepository 接口（共用层，与 VectorRepository 平级）
internal/infra/cayley/              ← Cayley 实现（client.go 封装 quad store + graph_store.go）
internal/service/knowledge/         ← KB 索引写入调用 GraphRepository（AddChunks hook）
internal/adk/tools/tools.go         ← knowledge_graph_search tool（调用 GraphRepository 查询）
```

| 组件 | 角色 |
|------|------|
| `GraphRepository` 接口 | 图库访问契约（写入/删除/查询），KB 与 Skill 共用，与实现解耦 |
| `internal/infra/cayley` | Cayley quad store（bbolt 后端）实现，wire.go 注入 |
| KB `AddChunks` | 索引时写入 chunk quad + RELATED_TO 关系 |
| `knowledge_graph_search` Skill | 查询图库返回 topN（按可见性过滤） |

## 详细设计

### 图模型（逻辑：仅 Chunk 节点 + RELATED_TO 边；物理：quad 三元组）

逻辑模型不变：

| 元素 | 属性 | 说明 |
|------|------|------|
| 节点 `Chunk` | `chunk_id`(唯一), `doc_id`, `chunk_idx`, `creator_id`, `is_public`, `char_count` | 切片节点；**含 creator_id/is_public**，供查询过滤 |
| 边 `RELATED_TO` | `score` | Chunk → Chunk，向量 topN 相似，**唯一边类型** |

> 拍板：不做 Document 节点、不做 BELONGS_TO/NEXT_CHUNK 边。图只表达"切片相似关系"，归属/顺序仍以 `doc_id`/`chunk_idx` 属性承载。

**物理存储（Cayley quad 模型映射）**：Cayley 是 RDF-like quad store，`(subject, predicate, object, label)` 四元组，无 schema。映射如下：

| 逻辑概念 | Cayley quad | 说明 |
|---------|-------------|------|
| Chunk 节点 | subject = `chunk:<chunk_id>`（IRI） | 切片主体 |
| 节点属性 | `<chunk:id> <doc_id> "xxx"` / `<creator_id> "u1"` / `<is_public> "true"` / `<chunk_idx> "0"` / `<char_count> "123"` | predicate-object 字面量 |
| RELATED_TO 边 | `<chunk:A> <related_to> <chunk:B> "0.85"` | label 存 score（字符串） |

### 图访问共用组件接口

```go
// internal/repository/graph.go
type GraphRepository interface {
    // EnsureSchema 确保 quad store 就绪（bbolt 打开/初始化，幂等）。Cayley 无 schema，无 DDL。
    EnsureSchema(ctx context.Context) error
    // UpsertChunk 写入/更新切片节点（幂等：先删旧 quad 再写，或按 subject 覆盖）
    UpsertChunk(ctx context.Context, c GraphChunk) error
    // LinkRelated 建立切片与 topN 相关切片的 RELATED_TO 边（label 存 score）
    LinkRelated(ctx context.Context, chunkID string, refs []RelatedRef) error
    // DeleteChunk 删除切片节点及其边（幂等）
    DeleteChunk(ctx context.Context, chunkID string) error
    // DeleteByDocID 删除文档下所有切片节点及其 RELATED_TO 边（幂等）
    DeleteByDocID(ctx context.Context, docID string) error
    // QueryTopN 图查询：从锚点切片出发返回 topN 相关节点（按可见性过滤）
    QueryTopN(ctx context.Context, anchorID string, topN int, filter GraphFilter) ([]GraphNode, error)
}

type GraphChunk struct {
    ChunkID   string
    DocID     string
    ChunkIdx  int
    CreatorID string
    IsPublic  bool
    CharCount int
}
type RelatedRef struct {
    ChunkID string
    Score   float64
}
// GraphFilter 可见性过滤，与 KB 可见性规则一致
type GraphFilter struct {
    CreatorID     string
    IsSystemAdmin bool
}
type GraphNode struct {
    ChunkID string
    DocID   string
    Score   float64
}
```

### 前置修复：Qdrant `Client.Search` 支持 filter

**已完成**（commit `9911090`，2026-08-28 回归验证通过）：

- `internal/infra/qdrant/client.go` `Search` 增加 `filter` 参数写入 payload；
- `internal/infra/qdrant/vector_store.go` `Search` 透传 filter；
- `vectorSearch`（`service.go:206`）普通用户的 `creator_id`/`is_public` 权限过滤已生效（回归验证：普通用户仅见 public 文档）。

本 spec 剩余前置：Qdrant 按 doc_id 删点能力（`DeletePoints`）随删除清理章节一并实现。

### 索引时找相关节点（时序）

`AddChunks` 在「embed → Qdrant」基础上，图索引步骤**先搜后写**（天然排除自己）：

```
对每个 chunk:
  redacted = maybeRedact(text)
  vec = embed(redacted)
  ① hits = vector.Search(collection, vec, topN, filter{creator_id: chunk.CreatorID})  ← 新增：仅检索同 creator 的相似切片
  ② 取前 topN 条作为 RelatedRef（score 存 quad label）
  vectors.append(vec)                                                               ← 现有
  kb.AddChunks(chunks) → vector.Upsert(vectors)                                     ← 现有
  ③ graph.UpsertChunk(chunk) + graph.LinkRelated(chunkID, topN refs)                ← 新增：Upsert 之后写图
```

- **拍板约束**：找相关节点**只关联同 `creator_id` 的切片**（Qdrant filter 精确匹配），`RELATED_TO` 边两端必然同 creator。
- **topN**：默认 **5**（system_configs 可配，上限 20）。
- 图写入失败与 Qdrant 一致：`log.Printf` 降级不阻断（`AddChunks` 主链路不因图库故障失败）。

### Cayley 初始化 + seed（无 schema，仅 skill 配置）

`wire.go` 的 `initKnowledgeBase` 中，与 Qdrant `EnsureCollection` 同层：

```go
graphStore, err := cayleyinfra.NewGraphStore(cfg.KBGraphPath) // bbolt 单文件路径
if err != nil {
    log.Printf("[kb] WARNING: failed to open Cayley graph store: %v", err)
} else {
    deps.kbService.WithGraphIndex(graphStore)
    // Cayley 无 schema，EnsureSchema 仅确保 store 就绪（幂等）
    if err := graphStore.EnsureSchema(context.Background()); err != nil {
        log.Printf("[kb] WARNING: failed to ensure Cayley store: %v", err)
    }
}
```

- **Cayley 无 schema/约束/索引 DDL**（bbolt 是无 schema KV），`EnsureSchema` 幂等、只做「打开/初始化 quad store」。
- 拍板：**不做存量 chunks 回填**（当前都是测试数据）。skill 配置走现有 `predefinedSkills()` seed（见下）。

### 图谱搜索 Skill：`knowledge_graph_search`

三步注册（对齐现有 `knowledge_search` 范式）：

**① Seed 配置**（`internal/service/skill/config.go` `predefinedSkills()`）：
```go
{
    Name:        "knowledge_graph_search",
    DisplayName: "知识图谱搜索",
    Description: "查询知识图谱中与某切片/概念最相关的 topN 节点及关系",
    Enabled:     true,
    ConfigJSON:  `{"top_n":5,"max_top_n":20}`,
}
```

**② ADK tool 注册**（`internal/adk/tools/tools.go`，仿 `knowledgeSearch`）：
```go
type GraphSearchArgs struct {
    Query string `json:"query" jsonschema:"切片 ID 或查询文本"`
    TopN  int    `json:"top_n,omitempty" jsonschema:"返回节点数（默认5，最大20）"`
}
type GraphSearchResult struct {
    Query string      `json:"query"`
    Nodes []GraphNode `json:"nodes"`
    Count int         `json:"count"`
}
// handler：
//   1. 文本 → embed → Qdrant Search 定位锚点 chunk_id（走修复后的 filter 能力）
//   2. chunk_id → graph.QueryTopN(anchorID, topN, GraphFilter{CreatorID, IsSystemAdmin})
// user_id / role / is_system_admin 从 session state 取（stateString），绝不由 LLM 传参。
```

**③ 依赖注入**（`wire.go`）：`tools.Deps` 增加 `GraphRepo repository.GraphRepository`，注册到 `specs()`。

**查询过滤语义（拍板）**：`QueryTopN` 分两步，图遍历用 Cayley 简单出边查询，**权限过滤 + score 排序 + topN 截断在 Go 层**（逻辑简单，不依赖 Gizmo 复杂过滤）：

1. Cayley 查锚点 `chunk:<anchorID>` 的所有 `related_to` 出边 → 得到相关 chunk 列表（含 label=score）；
2. Go 层过滤：`isSystemAdmin` 全局不过滤；普通用户仅保留 `creator_id == 自己 || is_public == true`；
3. 按 score 降序 + 截断 topN。

- `system_admin`：全局不过滤；普通用户：仅返回自己切片或公开切片。

### 技术选型与 License 检查（2026-08-29 调研确认）

| 组件 | 选型 | License | 结论 |
|------|------|---------|------|
| 图数据库 | `github.com/cayleygraph/cayley`（Google 开源，Go 原生） | **Apache-2.0** | ✅ 无 copyleft 顾虑（相比 Neo4j CE 的 GPLv3） |
| 存储后端 | **bbolt**（`go.etcd.io/bbolt`，单文件嵌入式 KV） | MIT | ✅ 嵌入进程，无独立服务 |
| 查询 | Cayley Go API（quad 遍历）+ Go 层过滤 | — | 不引入 Gizmo/JS 运行时（简化） |

**Cayley 合规结论**：Apache-2.0，**无需** Neo4j GPLv3 那种「待法务确认」，直接可用。

**⚠️ 选型风险（需知悉）**：Cayley 维护不活跃——最新 release v0.7.7（2019），GitHub 最后 push 约 5 个月前，stars ~15k。相比 Neo4j 生态弱，但：
- 本项目只用最简 quad 存储 + 单跳遍历（`related_to` 出边），不依赖高级图算法；
- 通过 `GraphRepository` 接口隔离，若 Cayley 后续不满足，可替换实现（接口不变）；
- bbolt 是 etcd 团队维护的成熟 KV，稳定可靠。

**driver/使用要点**：
- `cayley.NewGraph(store)` 或 `graph.NewQuadStore("bolt", dbPath, nil)`，进程内全局单例
- 测试用内存后端（`cayley.NewMemoryGraph()`），无需 docker
- quad 写入：`store.AddQuad(quad.Make(subject, predicate, object, label))`
- 出边遍历：`g.V(quad.IRI("chunk:xxx")).Out(quad.IRI("related_to"))`

### 文档删除关联清理（MongoDB + Qdrant + Cayley 三处级联）

**现存缺口**（本 spec 一并修复）：`DeleteDoc`（`service.go:95-101`）目前**只删 MongoDB doc + chunks**：
- Qdrant 向量**从未清理**（`VectorStore.DeleteCollection` 是 no-op，Client 无按 doc_id 删点能力）→ 文档删除后仍能被搜索命中（孤儿向量）
- 图数据同理需级联清理

**删除时序（幂等，三处失败均 log 降级）**：

```
DeleteDoc(docID):
  ① kb.DeleteDoc(docID) + kb.DeleteChunks(docID)        ← 现有（MongoDB）
  ② vector.DeletePoints(ctx, collection, filter{doc_id}) ← 新增：Qdrant 按 doc_id 删点
  ③ graph.DeleteByDocID(ctx, docID)                     ← 新增：Cayley 级联删 chunk quad + related_to 边
```

配套新增能力：

1. **Qdrant**：`Client.DeletePoints(collection, filter)`（REST `POST /points/delete`，body `{filter}`）；`VectorRepository` 增加 `DeletePoints(ctx, collection, filter)`，`VectorStore` 实现。
2. **Cayley**：`GraphRepository.DeleteByDocID` 实现：
   - 先查 `<chunk:*> <doc_id> "{docID}"` 反查该文档所有 chunk subject；
   - 逐个删除该 subject 的所有 quad（属性 + 入边 + 出边 `related_to`），无孤儿边残留；
   - `DeleteChunk(chunkID)` 同理删单个 subject 的全部 quad。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 服务 | No — Cayley 嵌入进程（bbolt 单文件），无需独立容器 |
| 是否需要持久化 | Yes — bbolt 文件挂 volume（`kb-graph-data:/data/kb-graph`） |
| 是否影响现有 API | No — 仅 `AddChunks` 新增图写入 + `DeleteDoc` 补级联清理 + 新增 Skill |
| 性能影响 | 每 chunk 多一次 Qdrant `Search(topN)` + 一次 bbolt 写入；可异步/批量，待压测 |
| 是否需要新增 Skill | Yes — `knowledge_graph_search` |
| License 合规 | Cayley Apache-2.0 ✅ + bbolt MIT ✅，无 copyleft 顾虑 |
| 复用现有能力 | 复用 embedding + Qdrant `Search` + skill 三步注册范式 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `docker-compose.yml` / `docker-compose.ui-test.yml` | data-agent 服务加 volume `kb-graph-data:/data/kb-graph`（bbolt 持久化） | Modify |
| `internal/infra/qdrant/client.go` + `vector_store.go` | **新增** `Client.DeletePoints` + `VectorStore.DeletePoints`（按 doc_id 删点） | Modify |
| `internal/repository/knowledge.go` | `VectorRepository` 接口增 `DeletePoints` | Modify |
| `internal/repository/graph.go` | `GraphRepository` 接口 + 类型 | New |
| `internal/infra/cayley/client.go` + `graph_store.go` | Cayley quad store（bbolt）+ 实现（quad 映射、删除、QueryTopN） | New |
| `internal/service/knowledge/service.go` | `AddChunks` hook 图写入 + `WithGraphIndex` + `DeleteDoc` 级联清理 Qdrant/Cayley | Modify |
| `internal/adk/tools/tools.go` | `knowledge_graph_search` tool | Modify |
| `internal/service/skill/config.go` | `predefinedSkills` 增 seed 配置 | Modify |
| `cmd/server/wire.go` | Cayley store 初始化（bbolt path）+ `Deps.GraphRepo` 注入 | Modify |
| `internal/config` | 新增 `KBGraphPath` 配置项（bbolt 路径，默认 `/data/kb-graph/kb.graph`） | Modify |
| `go.mod` / `go.sum` | 引入 `github.com/cayleygraph/cayley` + `github.com/cayleygraph/quad` | Modify |

## 测试策略

1. **Unit tests（Go）**：L2 `GraphRepository` mock（`repository/mocks`）；L3 Cayley 适配器走**内存 quad store**（`cayley.NewMemoryGraph()`，无需 docker）。覆盖率 gate 见 SPEC-045。
2. **Integration**：Cayley bbolt 后端，验证 `UpsertChunk`/`LinkRelated`/`QueryTopN`/`DeleteByDocID`/`DeleteChunk` 端到端（临时目录，测试后清理）。
3. **E2E**：`knowledge_graph_search` 工具走真实图库返回 topN（用例编号 `UI-XXX`）。
4. **前置修复回归**：Qdrant filter 修复后，补 UT 验证 `vectorSearch` 权限过滤生效（普通用户搜不到他人私有切片）。
5. **删除清理回归**：删除文档后验证 Qdrant 无残留 points（搜索不命中）+ Cayley 无残留 quad（`chunk:<*> <doc_id>` 为空）。

## UI Test / E2E 验收规则

> 开发完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增 `knowledge_graph_search` Skill 时同步编写 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

参考: `.agent/memory/E2E_TESTING.md`

## Go Unit Test 验收规则

| Tier | 特征 | 目标 |
|:---:|------|:---:|
| L2 | `GraphRepository` 接口 mock | 100% |
| L3 | Cayley 适配器 / KB service 图写入 / Qdrant filter | 98% |
| Overall | 全量 | ≥98% |

- [ ] 每个 Success 测试 ≥2 个行为验证断言
- [ ] `EnsureSchema` 幂等性验证（重复调用不报错）
- [ ] `QueryTopN` 验证 topN 截断 + score 排序 + system_admin/普通用户过滤分支
- [ ] Qdrant `Client.Search` filter 参数正确写入 payload
- [ ] **严禁** `t.Skip()` 绕过不可测场景

## 验证标准

- [ ] 启动后 Cayley store（bbolt）正常打开，`EnsureSchema` 幂等执行
- [ ] KB 上传文档后，切片除 Qdrant 外，Cayley 出现 chunk quad + `related_to` 边
- [ ] 单 chunk 的 `related_to` 出边数 ≤ topN，且边两端 `creator_id` 相同
- [ ] 普通用户 `knowledge_graph_search` 仅返回自己或公开切片；system_admin 返回全局
- [ ] Qdrant filter 修复后，`vectorSearch` 权限过滤生效（回归验证）
- [ ] **文档删除三处级联清理**：MongoDB doc/chunks 删除 → Qdrant 按 doc_id 删点（搜索不再命中）→ Cayley 删 chunk quad + 边（`chunk:<*> <doc_id>` 为空）
- [ ] License 确认：Cayley（Apache-2.0）✅ + bbolt（MIT）✅，无 copyleft 顾虑

## 拍板记录（2026-08-28/29 晓军确认）

| # | 决策点 | 结论 |
|---|---|---|
| 1 | 图模型 | 仅 `Chunk` 节点 + `RELATED_TO` 边（无 Document 节点/归属边/顺序边） |
| 2 | 索引时找相关节点 | 只关联同 `creator_id` 的切片（Qdrant filter 精确匹配） |
| 3 | topN | 默认 5 |
| 4 | seed | 仅 skill 配置（Cayley 无 schema）；**无存量回填**（测试数据不迁移） |
| 5 | 节点属性与查询过滤 | Chunk 存 `creator_id`/`is_public`；查询时 system_admin 全局、普通用户按 creator_id 或 is_public 过滤（与 KB 可见性一致） |
| 6 | 附带发现 | Qdrant `Client.Search` 不支持 filter → 现有 `vectorSearch` 权限过滤失效（已完成 9911090） |
| 7 | 图数据库选型 | **Cayley（Apache-2.0）替代 Neo4j（GPLv3）**；bbolt 后端嵌入进程，无独立服务 |
| 8 | 查询实现 | Cayley 单跳出边遍历 + Go 层过滤/排序/topN（不引入 Gizmo JS 运行时） |
| 9 | 文档删除清理 | 三处级联：MongoDB + Qdrant `DeletePoints`（按 doc_id）+ Cayley 删 quad；修复现有 Qdrant 孤儿向量缺口 |
