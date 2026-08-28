# KB 切片图数据库索引（Neo4j）+ 图访问共用组件 + 图谱搜索 Skill

> **SPEC-070** | Status: 详细设计（2026-08-28 晓军拍板定稿）

## 目标

1. 在 KB 切片索引进 Qdrant 之后，**同时把切片索引进 Neo4j 图数据库**：仅 `Chunk` 节点 + `RELATED_TO` 相似边（最小化图模型）。
2. 图数据库访问**抽象成共用组件**（`GraphRepository` 接口 + Neo4j infra 适配器），供 KB 索引写入与图谱搜索 Skill 共同复用。
3. **新增 `knowledge_graph_search` Skill**：Agent 通过该工具查询图数据库，返回 topN 相关节点；查询按 `system_admin` 策略过滤（与 KB 可见性一致）。
4. `docker-compose` 新增 `neo4j` 服务；seed 仅两项：**Neo4j schema（约束/索引 DDL）+ skill 配置（predefinedSkills）**，无存量数据回填。

## 背景 / 动机

- 现状：KB 切片索引 `AddChunks(docID, texts)` 只做「PII 脱敏 → embedding → MongoDB `kb_chunks` + Qdrant 向量」（`internal/service/knowledge/service.go:139-193`）。
- 缺口：向量检索只能表达语义相似度，无法表达切片间的显式关系。图数据库用 `Chunk` 节点 + `RELATED_TO` 边显式存储"相关"关系，支撑图遍历与后续 GraphRAG。
- 找相关节点最简机制：**复用现有 embedding + Qdrant 向量检索**——每个 chunk 索引时已算出向量 `vec`，用它在 Qdrant `Search(topN)` 检索最相似的已索引 chunk 即"相关节点"，零新增相似度算法。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 知识库系统 | ✅ 已实现 | `AddChunks` 索引流程已就绪 |
| SPEC-068 知识库 PII 脱敏 | ✅ 已实现 | 切片脱敏后落库，图索引接在脱敏之后 |
| SPEC-065 external_api_* tools | ✅ 已实现 | Skill 三步注册范式（Seed + specs() + wire）可复用 |
| ⚠️ Qdrant Search filter 扩展（本 spec 前置修复项） | ✅ 已完成（commit 9911090） | `Client.Search` 已支持 filter + `VectorStore.Search` 透传，`vectorSearch` 权限过滤已回归验证生效。剩余前置：Qdrant 按 doc_id 删点能力（`DeletePoints`）随本 spec 实现 |

## 架构概述

图数据库访问抽象为**独立共用组件**（对齐 Qdrant 的 `internal/infra/qdrant` + `VectorRepository` 组织）：

```
internal/repository/graph.go        ← GraphRepository 接口（共用层，与 VectorRepository 平级）
internal/infra/neo4j/               ← Neo4j 实现（client.go + graph_store.go）
internal/service/knowledge/         ← KB 索引写入调用 GraphRepository（AddChunks hook）
internal/adk/tools/tools.go         ← knowledge_graph_search tool（调用 GraphRepository 查询）
```

## 详细设计

### 图模型（最小化：仅 Chunk 节点 + RELATED_TO 边）

| 元素 | 属性 | 说明 |
|------|------|------|
| 节点 `Chunk` | `chunk_id`(唯一), `doc_id`, `chunk_idx`, `creator_id`, `is_public`, `char_count` | 切片节点；**含 creator_id/is_public**，供查询过滤 |
| 边 `RELATED_TO` | `score` | Chunk → Chunk，向量 topN 相似，**唯一边类型** |

> 拍板：不做 Document 节点、不做 BELONGS_TO/NEXT_CHUNK 边。图只表达"切片相似关系"，归属/顺序仍以 `doc_id`/`chunk_idx` 属性承载。

### 图访问共用组件接口

```go
// internal/repository/graph.go
type GraphRepository interface {
    // EnsureSchema 幂等创建约束/索引（启动 seed，对齐 Qdrant EnsureCollection）
    EnsureSchema(ctx context.Context) error
    // UpsertChunk 写入/更新切片节点（MERGE 幂等）
    UpsertChunk(ctx context.Context, c GraphChunk) error
    // LinkRelated 建立切片与 topN 相关切片的 RELATED_TO 边（边两端同 creator）
    LinkRelated(ctx context.Context, chunkID string, refs []RelatedRef) error
    // DeleteChunk 删除切片节点及其边（幂等）
    DeleteChunk(ctx context.Context, chunkID string) error
    // DeleteByDocID 删除文档下所有切片节点及其边（幂等，DETACH DELETE）
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

**现存 bug**（本 spec 开发前必须修复）：

- `internal/infra/qdrant/client.go:122` 的 `Search` payload 只有 `vector`/`limit`/`with_payload`，**无 `filter` 字段**；
- `internal/infra/qdrant/vector_store.go:47` 的 `Search` 签收了 `filter` 参数但**从未传递**给 client；
- 结果：`vectorSearch`（`service.go:206`）里普通用户的 `creator_id`/`is_public` 权限过滤**从未生效**，向量检索可能返回他人私有切片（权限隐患）。

修复：`Client.Search` 增加 `filter` 参数并写入 payload（Qdrant REST `/points/search` 原生支持 `filter`），`VectorStore.Search` 透传。

### 索引时找相关节点（时序）

`AddChunks` 在「embed → Qdrant」基础上，图索引步骤**先搜后写**（天然排除自己）：

```
对每个 chunk:
  redacted = maybeRedact(text)
  vec = embed(redacted)
  ① hits = vector.Search(collection, vec, topN, filter{creator_id: chunk.CreatorID})  ← 新增：仅检索同 creator 的相似切片
  ② 取前 topN 条作为 RelatedRef（score 存边属性）
  vectors.append(vec)                                                               ← 现有
  kb.AddChunks(chunks) → vector.Upsert(vectors)                                     ← 现有
  ③ graph.UpsertChunk(chunk) + graph.LinkRelated(chunkID, topN refs)                ← 新增：Upsert 之后写图
```

- **拍板约束**：找相关节点**只关联同 `creator_id` 的切片**（Qdrant filter 精确匹配），`RELATED_TO` 边两端必然同 creator。
- **topN**：默认 **5**（system_configs 可配，上限 20）。
- 图写入失败与 Qdrant 一致：`log.Printf` 降级不阻断（`AddChunks` 主链路不因 Neo4j 故障失败）。

### Neo4j 初始化 + seed（仅 schema，无数据回填）

`wire.go` 的 `initKnowledgeBase` 中，与 Qdrant `EnsureCollection` 同层：

```go
if deps.neo4jClient != nil {
    graphStore := neo4jinfra.NewGraphStore(deps.neo4jClient)
    deps.kbService.WithGraphIndex(graphStore)
    if err := graphStore.EnsureSchema(context.Background()); err != nil {
        log.Printf("[kb] WARNING: failed to ensure Neo4j schema: %v", err)
    }
}
```

`EnsureSchema`（约束/索引 DDL，幂等，**唯一数据侧 seed**）：

```cypher
CREATE CONSTRAINT chunk_id_unique IF NOT EXISTS FOR (c:Chunk) REQUIRE c.chunk_id IS UNIQUE;
CREATE INDEX chunk_creator_id IF NOT EXISTS FOR (c:Chunk) ON (c.creator_id);
```

> 拍板：**不做存量 chunks 回填**（当前都是测试数据）。skill 配置走现有 `predefinedSkills()` seed（见下）。

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

**查询过滤语义（拍板）**：`QueryTopN` 内部 Cypher 过滤，与 KB 可见性一致：

```cypher
MATCH (a:Chunk {chunk_id: $anchorID})-[r:RELATED_TO]-(n:Chunk)
WHERE $isSystemAdmin OR n.creator_id = $creatorID OR n.is_public = true
RETURN n.chunk_id, n.doc_id, r.score
ORDER BY r.score DESC LIMIT $topN
```

- `system_admin`：全局不过滤；普通用户：仅返回自己切片或公开切片。

### 技术选型与 License 检查（2026-08-28 调研确认）

| 组件 | 选型 | License | 结论 |
|------|------|---------|------|
| Go 驱动 | `github.com/neo4j/neo4j-go-driver/v5`（官方，Bolt 协议） | **Apache-2.0** | ✅ 无传染风险。项目 Go 1.26 满足 v5 要求（Go 1.18+）；驱动全局单例复用（内置连接池），勿每次请求新建 |
| Neo4j 服务端 | `neo4j:5` 官方镜像 = **Community Edition** | **GPLv3** | ⚠️ 需晓军确认可接受（见下） |

**Neo4j Community Edition（GPLv3）合规分析**：

- 项目红线只禁 **AGPL**；Neo4j CE 是 **GPLv3**（非 AGPL）。
- GPLv3 传染性仅在**分发软件**时触发。本方案中 Neo4j 作为独立 docker 容器运行、data-agent 通过 Bolt 协议（网络）调用 → **不构成分发/链接**，GPLv3 不传染 data-agent 代码。Go driver（Apache-2.0）与服务端相互独立。
- 若团队 FOSS policy 对 copyleft（GPLv3）有更严限制 → 备选：**ArcadeDB**（Apache-2.0，多模型，支持 Cypher + Bolt，生态较新）或 **Memgraph**（BSL）。MVP 建议先用 Neo4j CE，正式商用前由法务确认。
- CE 功能限制（可接受）：单机无集群、无在线备份、无企业级 RBAC —— MVP 够用。

**driver 使用要点**：
- `neo4j.NewDriverWithContext(uri, neo4j.BasicAuth(user, pass, ""))` 全局单例
- 写操作用 `ExecuteWrite`（managed transaction，自动重试瞬时错误）
- driver 线程安全；session/transaction 非线程安全

### 文档删除关联清理（MongoDB + Qdrant + Neo4j 三处级联）

**现存缺口**（本 spec 一并修复）：`DeleteDoc`（`service.go:95-101`）目前**只删 MongoDB doc + chunks**：
- Qdrant 向量**从未清理**（`VectorStore.DeleteCollection` 是 no-op，Client 无按 doc_id 删点能力）→ 文档删除后仍能被搜索命中（孤儿向量）
- Neo4j 图数据同理需级联清理

**删除时序（幂等，三处失败均 log 降级）**：

```
DeleteDoc(docID):
  ① kb.DeleteDoc(docID) + kb.DeleteChunks(docID)        ← 现有（MongoDB）
  ② vector.DeletePoints(ctx, collection, filter{doc_id}) ← 新增：Qdrant 按 doc_id 删点
  ③ graph.DeleteByDocID(ctx, docID)                     ← 新增：Neo4j 级联删节点+边
```

配套新增能力：

1. **Qdrant**：`Client.DeletePoints(collection, filter)`（REST `POST /points/delete`，body `{filter}`）；`VectorRepository` 增加 `DeletePoints(ctx, collection, filter)`，`VectorStore` 实现。
2. **Neo4j**：`GraphRepository.DeleteByDocID` 实现为单条 Cypher：

```cypher
MATCH (c:Chunk {doc_id: $docID}) DETACH DELETE c
```

`DETACH DELETE` 连带删除该节点所有 `RELATED_TO` 边，无孤儿边残留；`DeleteChunk(chunkID)` 同理单节点 `DETACH DELETE`。

### Neo4j 服务（docker-compose）

```yaml
neo4j:
  image: neo4j:5                                  # Community Edition (GPLv3)
  ports: ["7687:7687", "7474:7474"]
  environment:
    - NEO4J_AUTH=neo4j/<password>          # 凭据走 env / Vault
    - NEO4J_server_memory_heap_max__size=512m
  volumes: [neo4j-data:/data]
  healthcheck:
    test: ["CMD-SHELL", "cypher-shell -u neo4j -p <password> 'RETURN 1' || exit 1"]
```

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes — Neo4j 图数据库（新基础设施） |
| 是否影响现有 API | No — 仅 `AddChunks` 新增图写入 + `DeleteDoc` 补级联清理 + 新增 Skill；前置修复项不改 API 语义 |
| 性能影响 | 每 chunk 多一次 Qdrant `Search(topN)` + 一次 Neo4j 写入；可异步/批量，待压测 |
| 是否需要新增 Skill | Yes — `knowledge_graph_search` |
| License 合规 | Go driver Apache-2.0 ✅；Neo4j CE GPLv3（非 AGPL，服务端使用不传染）⚠️ 待最终确认 |
| 复用现有能力 | 复用 embedding + Qdrant `Search` + skill 三步注册范式 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `docker-compose.yml` / `docker-compose.ui-test.yml` | 新增 neo4j 服务 + volume | New |
| `internal/infra/qdrant/client.go` + `vector_store.go` | **前置修复**：`Client.Search` 支持 filter + `VectorStore.Search` 透传；**新增** `Client.DeletePoints` + `VectorStore.DeletePoints`（按 doc_id 删点） | Modify |
| `internal/repository/knowledge.go` | `VectorRepository` 接口增 `DeletePoints` | Modify |
| `internal/repository/graph.go` | `GraphRepository` 接口 + 类型 | New |
| `internal/infra/neo4j/client.go` + `graph_store.go` | neo4j-go-driver/v5 客户端 + 实现（含 `DeleteByDocID` DETACH DELETE） | New |
| `internal/service/knowledge/service.go` | `AddChunks` hook 图写入 + `WithGraphIndex` + `DeleteDoc` 级联清理 Qdrant/Neo4j | Modify |
| `internal/adk/tools/tools.go` | `knowledge_graph_search` tool | Modify |
| `internal/service/skill/config.go` | `predefinedSkills` 增 seed 配置 | Modify |
| `cmd/server/wire.go` | neo4j 客户端初始化 + `EnsureSchema` + `Deps.GraphRepo` 注入 | Modify |
| `go.mod` / `go.sum` | 引入 `github.com/neo4j/neo4j-go-driver/v5`（Apache-2.0） | Modify |

## 测试策略

1. **Unit tests（Go）**：L2 `GraphRepository` mock（`repository/mocks`）；L3 neo4j 适配器走真实 Neo4j（`go test -tags=integration`）。覆盖率 gate 见 SPEC-045。
2. **Integration**：Docker Compose 起 Neo4j，验证 `EnsureSchema` 幂等、`UpsertChunk`/`LinkRelated`/`QueryTopN`/`DeleteByDocID` 端到端。
3. **E2E**：`knowledge_graph_search` 工具走真实图库返回 topN（用例编号 `UI-XXX`）。
4. **前置修复回归**：Qdrant filter 修复后，补 UT 验证 `vectorSearch` 权限过滤生效（普通用户搜不到他人私有切片）。
5. **删除清理回归**：删除文档后验证 Qdrant 无残留 points（搜索不命中）+ Neo4j 无残留节点/边（`MATCH (c:Chunk {doc_id})` 为空）。

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
| L3 | neo4j 适配器 / KB service 图写入 / Qdrant filter | 98% |
| Overall | 全量 | ≥98% |

- [ ] 每个 Success 测试 ≥2 个行为验证断言
- [ ] `EnsureSchema` 幂等性验证（重复调用不报错不重复建）
- [ ] `QueryTopN` 验证 topN 截断 + score 排序 + system_admin/普通用户过滤分支
- [ ] Qdrant `Client.Search` filter 参数正确写入 payload
- [ ] **严禁** `t.Skip()` 绕过不可测场景

## 验证标准

- [ ] `docker compose up -d` 后 neo4j 容器 healthy（bolt/http 可达）
- [ ] 启动后 `EnsureSchema` 幂等执行，约束/索引存在
- [ ] KB 上传文档后，切片除 Qdrant 外，Neo4j 出现 Chunk 节点 + `RELATED_TO` 边
- [ ] 单 chunk 的 `RELATED_TO` 边数 ≤ topN，且边两端 `creator_id` 相同
- [ ] 普通用户 `knowledge_graph_search` 仅返回自己或公开切片；system_admin 返回全局
- [ ] Qdrant filter 修复后，`vectorSearch` 权限过滤生效（回归验证）
- [ ] **文档删除三处级联清理**：MongoDB doc/chunks 删除 → Qdrant 按 doc_id 删点（搜索不再命中）→ Neo4j `DETACH DELETE` 节点+边（`MATCH (c:Chunk {doc_id})` 为空）
- [ ] License 确认：neo4j-go-driver（Apache-2.0）✅；Neo4j CE（GPLv3）需晓军最终确认可接受

## 拍板记录（2026-08-28 晓军确认）

| # | 决策点 | 结论 |
|---|---|---|
| 1 | 图模型 | 仅 `Chunk` 节点 + `RELATED_TO` 边（无 Document 节点/归属边/顺序边） |
| 2 | 索引时找相关节点 | 只关联同 `creator_id` 的切片（Qdrant filter 精确匹配） |
| 3 | topN | 默认 5 |
| 4 | seed | 仅 Neo4j schema DDL + skill 配置；**无存量回填**（测试数据不迁移） |
| 5 | 节点属性与查询过滤 | Chunk 节点存 `creator_id`/`is_public`；查询时 system_admin 全局、普通用户按 creator_id 或 is_public 过滤（与 KB 可见性一致） |
| 6 | 附带发现 | Qdrant `Client.Search` 不支持 filter → 现有 `vectorSearch` 权限过滤失效，列为前置修复项（已完成 9911090） |
| 7 | Go 驱动选型 | `neo4j-go-driver/v5`（官方，Apache-2.0，全局单例 + managed transaction） |
| 8 | Neo4j license | CE = GPLv3（非 AGPL），docker 容器服务端使用不构成分发、不传染代码；待法务最终确认 |
| 9 | 文档删除清理 | 三处级联：MongoDB + Qdrant `DeletePoints`（按 doc_id）+ Neo4j `DeleteByDocID`（DETACH DELETE）；修复现有 Qdrant 孤儿向量缺口 |
