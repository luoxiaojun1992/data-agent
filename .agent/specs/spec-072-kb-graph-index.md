# KB 切片图数据库索引（Neo4j）

> **SPEC-072** | Status: 立项（不展开）

## 目标

在 KB 切片索引进向量数据库（Qdrant）之后，**同时把切片索引进图数据库（Neo4j）**，用于表达切片之间的结构化关系（文档归属、切片顺序、潜在引用/语义关联），为后续基于图的检索与推理（如 GraphRAG）打基础。同时在 `docker-compose` 中新增 `neo4j` 服务。

## 背景 / 动机

- 现状：KB 切片索引流程 `AddChunks(docID, texts)` 只做「PII 脱敏 → embedding → MongoDB `kb_chunks` + Qdrant 向量」（`internal/service/knowledge/service.go:139-193`）。
- 缺口：向量检索只能表达**语义相似度**，无法表达切片之间的**显式结构化关系**——同一文档内切片的先后顺序、切片与文档的归属、跨文档的引用等，这些关系在纯向量存储里丢失。
- 图数据库（Neo4j）可用节点/边显式存储「切片-文档」归属边、「切片-切片」顺序边/引用边，支撑后续图遍历查询与 GraphRAG 增强检索。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 知识库系统 | ✅ 已实现 | KB 切片索引流程已就绪（`AddChunks` → Qdrant + kb_chunks） |
| SPEC-068 知识库 PII 脱敏 | ✅ 已实现 | 切片脱敏后落库，图索引接在脱敏之后 |
| SPEC-066 配置存储拆分 | 📐 设计中 | Neo4j 连接配置的归属待定（system_configs vs 独立） |

## 立项说明（不展开）

> 本 spec 仅**立项**，暂不展开详细设计。核心动作与基础设施范围先定，图模型 schema、检索 API、容错策略等留待后续展开。

### 核心动作（已明确）

1. `docker-compose.yml` / `docker-compose.ui-test.yml` 新增 `neo4j` 服务（官方镜像 `neo4j:5`，默认端口 bolt `7687` / http `7474`，数据卷持久化 + 认证配置 + healthcheck）。
2. `AddChunks` 在 `s.vector.Upsert(...)`（Qdrant 写入）**之后**，新增图索引写入步骤，把切片节点及关系写入 Neo4j。

### 待展开项（后续）

1. **图模型 schema**：`Document` / `Chunk` 节点 + `BELONGS_TO`（切片→文档）边 + `NEXT_CHUNK`（切片顺序）边，是否引入跨文档引用/实体关系边。
2. **Go 客户端选型**：`neo4j-go-driver` 及其在分层架构中的位置（`internal/infra/graph` repo 适配器）。
3. **容错策略**：图索引写入失败是降级（log 不阻断，与现有 Qdrant `log.Printf` 一致）还是 fail-closed，需与 Qdrant 语义对齐。
4. **图检索查询 API**：是否暴露图遍历/GraphRAG 检索接口，还是仅作存储留后续。
5. **幂等与生命周期对齐**：文档删除/重建时 Neo4j 图数据（节点+边）的清理，与 `kb_chunks`/Qdrant 的删除语义一致。
6. **Neo4j 运维**：认证凭据（Vault 还是 env）、数据卷持久化、内存配置（heap/off-heap）、healthcheck 与依赖顺序。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | Yes — Neo4j 图数据库（**新基础设施**，非 MongoDB collection） |
| 是否影响现有 API | No — 仅新增索引写入步骤，不改变现有 KB API 语义 |
| 性能影响 | 每切片多一次 Neo4j 写入，需评估延迟（后续可异步/批量） |
| 是否需要新增 Skill | 待定 — 图检索能力留后续，本 spec 仅做索引写入 |

## 相关文件（预判）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `docker-compose.yml` / `docker-compose.ui-test.yml` | 新增 neo4j 服务 | New |
| `internal/service/knowledge/service.go` | `AddChunks` 后 hook 图索引写入 | Modify |
| `internal/infra/graph`（或 `internal/repository/graph`） | Neo4j 客户端 / repository 适配器 | New |
| `internal/adk/modelcfg` 或 `internal/service/config` | Neo4j 连接配置（待 SPEC-066 定归属） | Modify |

## 验证标准（占位，后续展开）

- [ ] `docker compose up -d` 后 neo4j 容器 healthy（bolt/http 端口可达）
- [ ] KB 上传文档后，切片除写入 Qdrant 外，Neo4j 中出现对应 Chunk 节点 + BELONGS_TO / NEXT_CHUNK 边
- [ ] 文档删除/重建后，Neo4j 图数据正确清理（幂等对齐）

## 提交约定

```bash
git add .agent/specs/spec-072-kb-graph-index.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-072 kb graph index (立项)"
```
