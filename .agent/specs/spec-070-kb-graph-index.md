# KB 切片图数据库索引（ArcadeDB）+ 图访问共用组件 + 图谱搜索 Skill

> **SPEC-070** | Status: 详细设计（2026-08-29 晓军拍板：ArcadeDB 替代 Neo4j，其余设计不变）

## 目标

1. 在 KB 切片索引进 Qdrant 之后，**同时把切片索引进 ArcadeDB 图数据库**：仅 `Chunk` 节点 + `RELATED_TO` 相似边（最小化图模型）。
2. 图数据库访问**抽象成共用组件**（`GraphRepository` 接口 + ArcadeDB infra 适配器），供 KB 索引写入与图谱搜索 Skill 共同复用。
3. **新增 `knowledge_graph_search` Skill**：Agent 通过该工具查询图数据库，返回 topN 相关节点；查询按 `system_admin` 策略过滤（与 KB 可见性一致）。
4. ArcadeDB **独立部署**（docker 服务），`docker-compose` 新增 `arcadedb` 服务；seed 仅 **schema（约束/索引 DDL）+ skill 配置（predefinedSkills）**，无存量数据回填。

## 背景 / 动机

- 现状：KB 切片索引 `AddChunks(docID, texts)` 只做「PII 脱敏 → embedding → MongoDB `kb_chunks` + Qdrant 向量」（`internal/service/knowledge/service.go:139-193`）。
- 缺口：向量检索只能表达语义相似度，无法表达切片间的显式关系。图数据库用 `Chunk` 节点 + `RELATED_TO` 边显式存储"相关"关系，支撑图遍历与后续 GraphRAG。
- 找相关节点最简机制：**复用现有 embedding + Qdrant 向量检索**——每个 chunk 索引时已算出向量 `vec`，用它在 Qdrant `Search(topN)` 检索最相似的已索引 chunk 即"相关节点"，零新增相似度算法。
- **为何 ArcadeDB 替代 Neo4j**：ArcadeDB 是 **Apache-2.0**（无 Neo4j CE 的 GPLv3 copyleft 顾虑），**原生属性图模型（节点+边）**，且**完整实现 Neo4j Bolt 协议**（官方认证 Go driver 兼容），Cypher 查询与 Neo4j 设计零改动平滑迁移。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 知识库系统 | ✅ 已实现 | `AddChunks` 索引流程已就绪 |
| SPEC-068 知识库 PII 脱敏 | ✅ 已实现 | 切片脱敏后落库，图索引接在脱敏之后 |
| SPEC-065 external_api_* tools | ✅ 已实现 | Skill 三步注册范式（Seed + specs() + wire）可复用 |
| ⚠️ Qdrant Search filter 扩展（本 spec 前置修复项） | ✅ 已完成（commit 9911090） | `Client.Search` 已支持 filter + `VectorStore.Search` 透传，`vectorSearch` 权限过滤已回归验证生效。剩余前置：Qdrant 按 doc_id 删点能力（`DeletePoints`）随本 spec 实现 |

## 架构概述

图数据库访问抽象为**独立共用组件**（对齐 Qdrant 的 `internal/infra/qdrant` + `VectorRepository` 组织）。ArcadeDB **独立部署**为 docker 服务，data-agent 通过 Bolt 协议访问：

```
docker-compose:
  arcadedb:  arcadedata/arcadedb 独立服务（Bolt 7687 / HTTP 2480）

代码:
internal/repository/graph.go        ← GraphRepository 接口（共用层，与 VectorRepository 平级）
internal/infra/arcadedb/            ← ArcadeDB 实现（client.go + graph_store.go）
internal/service/knowledge/         ← KB 索引写入调用 GraphRepository（AddChunks hook）
internal/adk/tools/tools.go         ← knowledge_graph_search tool（调用 GraphRepository 查询）
```

| 组件 | 角色 |
|------|------|
| `GraphRepository` 接口 | 图库访问契约（写入/删除/查询），KB 与 Skill 共用，与实现解耦 |
| `internal/infra/arcadedb` | ArcadeDB Bolt 客户端（neo4j-go-driver）+ 实现，wire.go 注入 |
| `arcadedb` 独立服务 | 属性图数据库，Cypher 查询 + Bolt 协议 |
| KB `AddChunks` | 索引时写入 chunk 节点 + RELATED_TO 关系 |
| `knowledge_graph_search` Skill | 查询图库返回 topN（按可见性过滤） |

## 详细设计

### 图模型（最小化：仅 Chunk 节点 + RELATED_TO 边）

ArcadeDB 是**原生属性图模型**（节点 + 边 + 属性），与最初 Neo4j 设计一致：

| 元素 | 属性 | 说明 |
|------|------|------|
| 节点 `Chunk` | `chunk_id`(唯一), `doc_id`, `chunk_idx`, **`creator_id`(=userId)**, **`is_public`**, `char_count` | 切片节点；`creator_id`/`is_public` 为自定义字段，供查询权限过滤 |
| 边 `RELATED_TO` | `score` | Chunk → Chunk，向量 topN 相似，**唯一边类型** |

> 拍板：不做 Document 节点、不做 BELONGS_TO/NEXT_CHUNK 边。图只表达"切片相似关系"，归属/顺序仍以 `doc_id`/`chunk_idx` 属性承载。
> **自定义字段（指 `creator_id`(=userId) 与 `is_public`）**：Chunk 节点需支持这两个字段，供查询时按可见性过滤（system_admin 全局 / 普通用户按 creator_id 或 is_public）。暂不含标签、分类等元数据字段。

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
    CreatorID string // = userId，自定义字段，供权限过滤
    IsPublic  bool   // 自定义字段，供可见性过滤
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
    Content string  // 查询后由 skill 从 Qdrant 反查填充（图本身不存内容）
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
  ② 取前 topN 条作为 RelatedRef（score 存边属性）
  vectors.append(vec)                                                               ← 现有
  kb.AddChunks(chunks) → vector.Upsert(vectors)                                     ← 现有
  ③ graph.UpsertChunk(chunk) + graph.LinkRelated(chunkID, topN refs)                ← 新增：Upsert 之后写图
```

- **拍板约束**：找相关节点**只关联同 `creator_id` 的切片**（Qdrant filter 精确匹配），`RELATED_TO` 边两端必然同 creator。
- **topN**：默认 **5**（system_configs 可配，上限 20）。
- 图写入失败与 Qdrant 一致：`log.Printf` 降级不阻断（`AddChunks` 主链路不因图库故障失败）。

### ArcadeDB 初始化 + seed（schema DDL + skill 配置）

`wire.go` 的 `initKnowledgeBase` 中，与 Qdrant `EnsureCollection` 同层：

```go
if deps.arcadeDBClient != nil {
    graphStore := arcadedbinfra.NewGraphStore(deps.arcadeDBClient)
    deps.kbService.WithGraphIndex(graphStore)
    if err := graphStore.EnsureSchema(context.Background()); err != nil {
        log.Printf("[kb] WARNING: failed to ensure ArcadeDB schema: %v", err)
    }
}
```

`EnsureSchema`（约束/索引 DDL，幂等，**唯一数据侧 seed**）：

```cypher
CREATE CONSTRAINT chunk_id_unique IF NOT EXISTS FOR (c:Chunk) REQUIRE c.chunk_id IS UNIQUE;
CREATE INDEX chunk_creator_id IF NOT EXISTS FOR (c:Chunk) ON (c.creator_id);
```

> 拍板：**不做存量 chunks 回填**（当前都是测试数据）。skill 配置走现有 `predefinedSkills()` seed（见下）。
> 注：ArcadeDB 约束/索引语法与 openCypher 基本兼容，具体 DDL 语句实现时按 ArcadeDB 文档核对。

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
    Nodes []GraphNode `json:"nodes"`   // 含反查到的 content
    Count int         `json:"count"`
}
// handler（三步，含内容反查）：
//   1. 文本 → embed → Qdrant Search 定位锚点 chunk_id（走修复后的 filter 能力，Search 自带锚点 content）
//   2. chunk_id → graph.QueryTopN(anchorID, topN, GraphFilter{CreatorID, IsSystemAdmin}) → 相关 chunk_id 列表
//   3. 反查内容：chunk_id 哈希 → Qdrant RetrievePoints 批量取 payload.content（不查 MongoDB）
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

### 技术选型与 License 检查（2026-08-29 调研确认）

| 组件 | 选型 | License | 结论 |
|------|------|---------|------|
| 图数据库 | ArcadeDB（`arcadedata/arcadedb`，Java 21+） | **Apache-2.0** | ✅ 无 copyleft 顾虑（相比 Neo4j CE 的 GPLv3） |
| Go client | `github.com/neo4j/neo4j-go-driver/v5`（Bolt 协议） | **Apache-2.0** | ✅ ArcadeDB 官方对 Go driver 做了 Bolt 符合性认证（26.7.2，39 场景） |
| 查询语言 | Cypher（openCypher） | — | 与最初 Neo4j 设计一致，零改动 |

**ArcadeDB 合规结论**：本身 + Go client 均 Apache-2.0，**无需** Neo4j GPLv3 那种「待法务确认」，直接可用。

**选型优势**：
- 原生属性图（节点+边），非 quad/RDF，符合本项目语义；
- 完整 Neo4j Bolt 协议兼容 → 复用 `neo4j-go-driver/v5`，Cypher 查询零改动；
- 活跃维护（GDB-Engines #21，最近 3 个月有更新，商业支持可用）；
- 多数据库隔离（database 概念，连接时指定）。

**driver 使用要点**（沿用 neo4j-go-driver 惯例）：
- `neo4j.NewDriverWithContext("bolt://arcadedb:7687", neo4j.BasicAuth("root", pass, ""))` 全局单例
- session 指定 database：`neo4j.SessionConfig{DatabaseName: "kbgraph"}`
- 写操作用 `ExecuteWrite`（managed transaction，自动重试瞬时错误）
- driver 线程安全；session/transaction 非线程安全

### 文档删除关联清理（MongoDB + GridFS + Qdrant + ArcadeDB 四处级联）

**现存缺口**（本 spec 一并修复）：`DeleteDoc`（`service.go:95-101`）目前**只删 MongoDB doc + chunks**：
- Qdrant 向量**从未清理**（`VectorStore.DeleteCollection` 是 no-op，Client 无按 doc_id 删点能力）→ 文档删除后仍能被搜索命中（孤儿向量）
- ArcadeDB 图数据同理需级联清理
- **GridFS 原始文件从未清理**（`KBRepository` 无 `DeleteFile` 能力，`GridFSFileID` 指向的文件残留）→ 孤儿文件占用存储

**删除时序（拍板：先删索引 Qdrant + ArcadeDB，再删明细 chunks + GridFS，最后删主记录 doc）**：

```
DeleteDoc(docID):
  ① vector.DeletePoints(ctx, collection, filter{doc_id})  ← 先删 Qdrant 向量（索引）
  ② graph.DeleteByDocID(ctx, docID)                      ← 再删 ArcadeDB 图（索引）
  ③ kb.DeleteChunks(docID)                               ← 删 MongoDB chunks（明细）
  ④ kb.DeleteFile(doc.GridFSFileID)                      ← 删 GridFS 原始文件（明细，需先 GetDoc 取 fileID）
  ⑤ kb.DeleteDoc(docID)                                  ← 最后删 MongoDB doc（主记录 = 最终提交点）
```

**顺序理由**：索引（Qdrant/ArcadeDB）先删，明细（chunks/GridFS）其次，主记录（doc）最后删。若中途失败：
- 索引已删但 MongoDB 未删 → 文档仍存在，只是索引缺失，可**重新索引恢复**（自愈）；
- chunks/GridFS 已删但 doc 未删 → doc 主记录仍在，可**重新切片/重建 chunks**（自愈）；
- 反之（doc 已删但 chunks/GridFS/索引残留）→ 孤儿数据更难发现且不可恢复。
- 因此 **doc 删除是最终提交点**，放最后一步。步骤 ④ 需先 `GetDoc` 取 `GridFSFileID`（doc 未删前可取到）。

**重试幂等（必须保证）**：五处删除均幂等，部分成功后重试安全：

| 步骤 | 幂等机制 | 重试行为 |
|------|---------|---------|
| ① Qdrant `DeletePoints`（filter doc_id） | filter 匹配不到点时删除 0 条，正常返回 | 已删后重试 → no-op ✅ |
| ② ArcadeDB `DeleteByDocID`（`MATCH ... DETACH DELETE`） | 无匹配节点时语句正常执行（删 0 节点） | 已删后重试 → no-op ✅ |
| ③ MongoDB `DeleteChunks`（docID） | 项目约定：**删除永不返回 404**（`DeleteMany` 0 行也返回成功） | 已删后重试 → no-op ✅ |
| ④ GridFS `DeleteFile`（fileID） | gridfs `Delete` 返回 `ErrFileNotFound` 时**忽略并返回 nil**（`GridFSFileID` 为空直接跳过） | 已删后重试 → no-op ✅ |
| ⑤ MongoDB `DeleteDoc`（docID） | 项目约定：**删除永不返回 404**（`DeleteOne` 0 行也返回成功） | 已删后重试 → no-op ✅ |

- 五处失败均 `log.Printf` 降级不阻断（与 `AddChunks` 一致）；删除流程可安全整体重试。
- `DeleteChunk(chunkID)` 同理幂等：单节点 `DETACH DELETE`。

配套新增能力：

1. **Qdrant**：`Client.DeletePoints(collection, filter)`（REST `POST /points/delete`，body `{filter}`）；`VectorRepository` 增加 `DeletePoints(ctx, collection, filter)`，`VectorStore` 实现。
2. **GridFS**：`KBRepository` 增加 `DeleteFile(ctx, fileID)`（`gridfs.Bucket.Delete`，`ErrFileNotFound` 忽略为幂等）；`Repository` 接口（`internal/repository/knowledge.go`）同步增加。
3. **ArcadeDB**：`GraphRepository.DeleteByDocID` 实现为单条 Cypher：

```cypher
MATCH (c:Chunk {doc_id: $docID}) DETACH DELETE c
```

`DETACH DELETE` 连带删除该节点所有 `RELATED_TO` 边，无孤儿边残留。

### 图查询内容反查（ArcadeDB 只存 chunk_id，内容从 Qdrant 反查）

**拍板：ArcadeDB 图节点只存元数据（chunk_id/doc_id/creator_id/is_public 等），不存内容**。图查询返回 chunk_id 后，**通过 chunk_id 从 Qdrant 反查内容**。

**Qdrant 存储现状确认（2026-08-31 查代码确认）**：

| 确认项 | 结论 |
|--------|------|
| Qdrant 是否直接存了内容？ | ✅ **是** — `AddChunks` 写 VectorPoint 时 payload 含 `content`（脱敏后全文），`vectorSearch` 直接读 `Metadata["content"]` 返回，无需反查 |
| Qdrant 是否关联了 chunk id？ | ✅ **是** — point `ID` = `chunk.ID`（`stringToInt64` 哈希后作为 point ID），与 chunk_id 一一对应（哈希可逆算） |
| 是否不需要通过 chunk id 反查？ | ⚠️ **图查询场景需要反查** — Qdrant 的 payload 里有 content，但**图（ArcadeDB）不存内容**；`QueryTopN` 返回的 chunk_id 需反查 content，反查数据源即 Qdrant（无需回 MongoDB） |

**反查实现**：

```
skill handler:
  ① 文本 → embed → Qdrant Search 定位锚点 chunk_id（Search 自带 payload.content，锚点内容直接可用）
  ② graph.QueryTopN(anchorID, topN, filter) → 相关 chunk_id 列表（无内容）
  ③ 反查内容：对每个 chunk_id 计算 stringToInt64 哈希 → Qdrant RetrievePoints(ids) 批量取 payload.content
```

- 新增 Qdrant 能力：`Client.RetrievePoints(collection, ids []int64)`（REST `POST /points`，body `{ids: [...], with_payload: true}`），`VectorStore` 封装 `GetByIDs(ctx, collection, ids)` 返回 `map[pointID]payload`。
- 反查从 **Qdrant** 取 content（内容已冗余存储），**不回查 MongoDB kb_chunks**；若 Qdrant 无该点（索引已删），content 置空（降级）。
- 锚点内容（第 ① 步 Search 结果）与相关节点内容（第 ③ 步 RetrievePoints）合并返回给 LLM。

### ArcadeDB 服务（docker-compose）

```yaml
arcadedb:
  image: arcadedata/arcadedb:latest             # Apache-2.0，Java 21+
  ports: ["7687:7687", "2480:2480"]             # 7687=Bolt, 2480=HTTP/Studio
  environment:
    - JAVA_OPTS=-Darcadedb.server.rootPassword=<password>   # 凭据走 env / Vault
    - arcadedb.server.databaseDirectory=/data/databases
  volumes: [arcadedb-data:/data]
  healthcheck:
    test: ["CMD-SHELL", "curl -f http://localhost:2480/api/v1/server || exit 1"]
```

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 服务 | Yes — ArcadeDB 独立 docker 服务（Java 21+，内存占用需关注） |
| 是否影响现有 API | No — 仅 `AddChunks` 新增图写入 + `DeleteDoc` 补级联清理 + 新增 Skill |
| 性能影响 | 每 chunk 多一次 Qdrant `Search(topN)` + 一次 ArcadeDB 写入；可异步/批量，待压测 |
| 是否需要新增 Skill | Yes — `knowledge_graph_search` |
| License 合规 | ArcadeDB Apache-2.0 ✅ + neo4j-go-driver Apache-2.0 ✅，无 copyleft 顾虑 |
| 复用现有能力 | 复用 embedding + Qdrant `Search` + skill 三步注册范式 + Cypher/Bolt 零改动迁移 |

## 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `docker-compose.yml` / `docker-compose.ui-test.yml` | 新增 arcadedb 服务 + volume | New |
| `internal/infra/qdrant/client.go` + `vector_store.go` | **新增** `Client.DeletePoints`（按 doc_id 删点）+ `Client.RetrievePoints`（按 ids 批量取 payload，供内容反查）+ `VectorStore` 封装 | Modify |
| `internal/repository/knowledge.go` | `VectorRepository` 接口增 `DeletePoints` + `RetrievePoints`；`KBRepository` 接口增 `DeleteFile` | Modify |
| `internal/infra/mongo/kb_repository.go` | **新增** `DeleteFile`（gridfs Bucket.Delete，`ErrFileNotFound` 忽略幂等） | Modify |
| `internal/repository/graph.go` | `GraphRepository` 接口 + 类型（GraphNode 含 Content，由 skill 反查填充） | New |
| `internal/infra/arcadedb/client.go` + `graph_store.go` | ArcadeDB Bolt 客户端（neo4j-go-driver）+ 实现（含 `DeleteByDocID` DETACH DELETE） | New |
| `internal/service/knowledge/service.go` | `AddChunks` hook 图写入 + `WithGraphIndex` + `DeleteDoc` 五步级联清理（索引 → 明细 → 主记录） | Modify |
| `internal/adk/tools/tools.go` | `knowledge_graph_search` tool（三步：Search 锚点 → QueryTopN → Qdrant 反查内容） | Modify |
| `internal/service/skill/config.go` | `predefinedSkills` 增 seed 配置 | Modify |
| `cmd/server/wire.go` | ArcadeDB 客户端初始化 + `EnsureSchema` + `Deps.GraphRepo` 注入 | Modify |
| `internal/config` | 新增 ArcadeDB 连接配置（URI/username/password/database） | Modify |
| `go.mod` / `go.sum` | 引入 `github.com/neo4j/neo4j-go-driver/v5`（Apache-2.0） | Modify |

## 测试策略

1. **Unit tests（Go）**：L2 `GraphRepository` mock（`repository/mocks`）；L3 ArcadeDB 适配器走真实 ArcadeDB（`go test -tags=integration`）。覆盖率 gate 见 SPEC-045。
2. **Integration**：Docker Compose 起 ArcadeDB，验证 `EnsureSchema` 幂等、`UpsertChunk`/`LinkRelated`/`QueryTopN`/`DeleteByDocID` 端到端。
3. **E2E**：`knowledge_graph_search` 工具走真实图库返回 topN（用例编号 `UI-XXX`）。
4. **前置修复回归**：Qdrant filter 修复后，补 UT 验证 `vectorSearch` 权限过滤生效（普通用户搜不到他人私有切片）。
5. **删除清理回归**：删除文档后验证 Qdrant 无残留 points（搜索不命中）+ ArcadeDB 无残留节点/边（`MATCH (c:Chunk {doc_id})` 为空）。

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
| L3 | ArcadeDB 适配器 / KB service 图写入 / Qdrant filter | 98% |
| Overall | 全量 | ≥98% |

- [ ] 每个 Success 测试 ≥2 个行为验证断言
- [ ] `EnsureSchema` 幂等性验证（重复调用不报错不重复建）
- [ ] `QueryTopN` 验证 topN 截断 + score 排序 + system_admin/普通用户过滤分支
- [ ] Qdrant `Client.Search` filter 参数正确写入 payload
- [ ] **严禁** `t.Skip()` 绕过不可测场景

## 验证标准

- [ ] `docker compose up -d` 后 arcadedb 容器 healthy（Bolt 7687 / HTTP 2480 可达）
- [ ] 启动后 `EnsureSchema` 幂等执行，约束/索引存在
- [ ] KB 上传文档后，切片除 Qdrant 外，ArcadeDB 出现 Chunk 节点 + `RELATED_TO` 边
- [ ] 单 chunk 的 `RELATED_TO` 边数 ≤ topN，且边两端 `creator_id` 相同
- [ ] 普通用户 `knowledge_graph_search` 仅返回自己或公开切片；system_admin 返回全局
- [ ] **图查询内容反查**：`QueryTopN` 返回的 chunk_id 经 Qdrant `RetrievePoints` 反查到 payload.content，skill 返回结果含内容（不查 MongoDB）
- [ ] Qdrant filter 修复后，`vectorSearch` 权限过滤生效（回归验证）
- [ ] **文档删除五处级联（顺序：Qdrant → ArcadeDB → MongoDB chunks → GridFS 文件 → MongoDB doc）**：删除后 Qdrant 搜索不再命中、ArcadeDB 无残留节点/边、MongoDB chunks/doc 已删、GridFS 文件已删
- [ ] **删除重试幂等**：删除后再次调用 `DeleteDoc` 五处均 no-op 正常返回（不报错）
- [ ] License 确认：ArcadeDB（Apache-2.0）✅ + neo4j-go-driver（Apache-2.0）✅，无 copyleft 顾虑

## 拍板记录（2026-08-28/29/31 晓军确认）

| # | 决策点 | 结论 |
|---|---|---|
| 1 | 图模型 | 仅 `Chunk` 节点 + `RELATED_TO` 边（原生属性图，无 Document 节点/归属边/顺序边） |
| 2 | 索引时找相关节点 | 只关联同 `creator_id` 的切片（Qdrant filter 精确匹配） |
| 3 | topN | 默认 5 |
| 4 | seed | 仅 schema DDL（约束/索引）+ skill 配置；**无存量回填**（测试数据不迁移） |
| 5 | 节点属性与查询过滤 | Chunk 存 `creator_id`/`is_public`；查询时 system_admin 全局、普通用户按 creator_id 或 is_public 过滤（与 KB 可见性一致） |
| 6 | 附带发现 | Qdrant `Client.Search` 不支持 filter → 现有 `vectorSearch` 权限过滤失效（已完成 9911090） |
| 7 | 图数据库选型 | **ArcadeDB（Apache-2.0）替代 Neo4j（GPLv3）/ Cayley（quad 模型）**；独立部署 |
| 8 | Go client | `neo4j-go-driver/v5`（Apache-2.0），经 Bolt 协议连 ArcadeDB（官方符合性认证），Cypher 零改动 |
| 9 | 文档删除清理 | 四处级联：MongoDB doc/chunks + GridFS 原始文件 + Qdrant `DeletePoints`（按 doc_id）+ ArcadeDB `DeleteByDocID`（DETACH DELETE）；修复现有 Qdrant 孤儿向量 + GridFS 孤儿文件缺口 |
| 10 | 自定义字段 | Chunk 节点支持 `creator_id`(=userId) 与 `is_public` 两个自定义字段，供权限/可见性过滤；暂不含标签、分类等元数据 |
| 11 | 联动删除顺序 | **先删索引（Qdrant → ArcadeDB），再删明细（MongoDB chunks → GridFS 文件），最后删 MongoDB doc（主记录 = 最终提交点）**；中途失败可重试，五处均幂等（Qdrant filter 删 / ArcadeDB MATCH 删 / Mongo 永不 404 / GridFS not found 忽略） |
| 12 | 图查询内容反查 | ArcadeDB 图只存 chunk_id 元数据不存内容；查询后经 chunk_id 哈希 → Qdrant `RetrievePoints` 反查 payload.content（**Qdrant 已直接存内容 + 关联 chunk_id，无需回 MongoDB**） |
