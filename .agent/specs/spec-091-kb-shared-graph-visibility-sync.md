# KB 设 shared 联动更新知识图谱 is_public（图索引可见性同步）

> **SPEC-091** | Status: 设计中

## 1. 目标

修复 KB 文档设为 shared（`is_public` 置 true）时，ArcadeDB 知识图谱中该文档所有 Chunk 节点的 `is_public` 未同步更新的缺陷，使图谱查询（`knowledge_graph_search`）的可见性过滤与 KB 文档列表/检索保持完全一致。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-070 KB 切片图数据库索引（ArcadeDB） | ✅ | 图谱节点 `is_public` 字段、`GraphRepository` 接口、`knowledge_graph_search` skill 均已就绪 |
| SPEC-068 KB 文本 PII 脱敏 | ✅ | 与本次改动无冲突 |
| SPEC-006 知识库系统 | ✅ | `SetPublicFlag` 现有链路已就绪 |

## 2. 背景

SPEC-070 引入 ArcadeDB 知识图谱：每个 KB 切片对应一个 `Chunk` 节点，节点携带 `creator_id`（归属）与 `is_public`（可见性）两个自定义字段。写入侧 `GraphStore.UpsertChunk` 在 `MERGE ... SET` 时写入 `c.is_public`，`knowledge_graph_search` 读取侧 `QueryTopN` 通过 `WHERE $isAdmin OR n.creator_id = $creatorID OR n.is_public = true` 做可见性过滤。

**缺陷**：KB 文档的 `is_public` 是可变的——用户在文档已创建、切片已写入图谱后，仍可通过 `SetPublicFlag` 切换 shared 状态。但当前 `Service.SetPublicFlag` 只更新了三处：

1. MongoDB `knowledge_docs`（`KBRepository.SetPublicFlag`）
2. MongoDB `kb_chunks`（`KBRepository.UpdateChunkVisibility`）
3. Qdrant vector payload（`VectorRepository.SetPayload`）

**唯独漏了 ArcadeDB 图谱**。且 `GraphRepository` 接口没有「按 docID 批量更新 is_public」的方法（只有单节点 `UpsertChunk` / `DeleteByDocID`）。

**后果**：文档先以私有状态创建（图谱节点 `is_public=false`），之后设为 shared（`is_public=true`）→ 图谱节点仍是 `false` → 其他用户执行 `knowledge_graph_search` 时，`n.is_public = true` 过滤会**漏掉这些已公开的切片**，造成「文档已共享但图谱搜不到」的不一致。

## 3. 可见性语义（晓军 2026-09-05 拍板，维持现状）

数据隔离与豁免为**两级**语义，与 KB 文档列表 `ListDocsByVisibility`、KB 检索 `SearchChunks` 完全一致，**本 spec 不修改查询隔离逻辑**：

| 角色 | 可见范围 | 是否豁免 |
|------|------|:---:|
| 系统管理员 `system_admin` | 全部数据 | ✅ 豁免 |
| 普通管理员 `admin` / 普通用户 `user` | 自己的 + 公开（`is_public=true`） | ❌ 无豁免 |
| 公开分享 | 所有用户可见 | — |

> 即「豁免」= 系统管理员有全部数据权限，或公开分享的切片大家都能看。普通管理员与普通用户同等隔离，无需单独豁免。此语义在现有 `QueryTopN` / `ListDocsByVisibility` / `SearchChunks` 中已正确实现，本次只补齐「shared 联动更新图谱 is_public」这一处遗漏。

## 4. 详细设计

### 4.1 接口新增方法

`internal/repository/graph.go` 的 `GraphRepository` 接口新增：

```go
// SetDocPublic updates the is_public flag on all chunk nodes of a document
// (idempotent). Keeps graph visibility in sync with the KB doc's shared state.
SetDocPublic(ctx context.Context, docID string, isPublic bool) error
```

### 4.2 ArcadeDB 实现

`internal/infra/arcadedb/graph_store.go` 新增：

```go
func (g *GraphStore) SetDocPublic(ctx context.Context, docID string, isPublic bool) error {
    g.mu.Lock()
    defer g.mu.Unlock()
    return g.run(ctx,
        `MATCH (c:Chunk {doc_id: $docID}) SET c.is_public = $isPublic`,
        map[string]any{"docID": docID, "isPublic": isPublic})
}
```

### 4.3 Service 联动调用

`internal/service/knowledge/service.go` 的 `SetPublicFlag` 在 Qdrant 更新后追加图谱同步（graph 为 nil 时跳过，与 `AddChunks` 图写入失败降级一致）：

```go
// 同步图谱节点可见性（SPEC-091）
if s.graph != nil {
    if err := s.graph.SetDocPublic(ctx, docID, isPublic); err != nil {
        log.Printf("[kb] sync graph is_public for doc=%s: %v", docID, err)
    }
}
```

### 4.4 mock 重新生成

`internal/repository/mocks/GraphRepository.go` 用 mockery 重新生成，补 `SetDocPublic` 桩方法。

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 ArcadeDB 现有 `Chunk` 节点属性） |
| 是否影响现有 API | No（无新增/变更 REST API，仅内部方法） |
| 性能影响 | 低：`MATCH (c:Chunk {doc_id})` 有 `doc_id` 属性，单文档切片数量有限；复用 `chunk_creator_id` 索引体系（可考虑补 `doc_id` 索引，见验证标准） |
| 是否需要新增 Skill | No |
| 是否引入传染性 license | No（沿用 neo4j-go-driver Apache-2.0） |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/repository/graph.go` | 接口新增 `SetDocPublic` | Small |
| `internal/infra/arcadedb/graph_store.go` | 实现 `SetDocPublic` | Small |
| `internal/repository/mocks/GraphRepository.go` | 重新生成 mock | Small（自动生成） |
| `internal/service/knowledge/service.go` | `SetPublicFlag` 追加图谱同步调用 | Small |
| `internal/service/knowledge/spec070_test.go` | 补 UT：SetPublicFlag 触发 SetDocPublic | Small |

## 7. 测试策略

1. **Unit tests**（Go）：补 `SetPublicFlag` 调用链测试，验证 graph 非 nil 时 `SetDocPublic` 被调用且参数正确（`docID` + `isPublic`），graph nil 时不 panic。
2. **ArcadeDB 集成**（条件）：验证 `SetDocPublic` cypher 正确更新同 doc 所有节点、不影响其他 doc。
3. **E2E**（条件）：无前端交互变更，不新增 UI 用例。

## 8. UI Test / E2E 验收规则

> 本 spec 纯后端内部修复，无前端交互变更。

- [ ] 无新增前端交互功能，无需新增 E2E 用例
- [ ] 改动不涉及 UI 组件，无需更新 `data-testid`

## 9. Go Unit Test 验收规则

> 开发完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L2 | 依赖接口，可 mock | **100%** | `service/knowledge` 的 SetPublicFlag |
| L3 | 依赖 ArcadeDB（Bolt） | **98%** | `infra/arcadedb` 的 SetDocPublic |

### 断言质量要求

- [ ] SetPublicFlag 测试用 `graph.AssertCalled(t, "SetDocPublic", ctx, docID, isPublic)` 验证参数传递
- [ ] graph=nil 分支测试验证不 panic、不影响主链路
- [ ] 严禁 `t.Skip()` 绕过、严禁只断言 `err == nil`

## 10. 验证标准

- [ ] `GraphRepository` 接口与 mock 均含 `SetDocPublic`，编译通过
- [ ] `SetPublicFlag` 在 graph 注入时同步更新图谱节点 `is_public`，与 MongoDB / Qdrant 三处状态一致
- [ ] `knowledge_graph_search` 在 doc 设为 shared 后能检索到已公开切片（可见性过滤正确放行）
- [ ] 单元测试全绿，覆盖率 ≥ 98%，`go vet` 无警告
- [ ] （可选）评估为 `Chunk.doc_id` 增加索引以支撑 `SetDocPublic` 的批量 MATCH 性能

## 11. 决策点

| 决策点 | 内容 | 推荐值 |
|--------|------|--------|
| D1 | `SetDocPublic` 图同步失败是否容错降级 | 容错 log、不阻断主链路（与 AddChunks 图写入失败降级一致） |
