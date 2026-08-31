# 可搜索下拉选择器统一设计（模型 / 角色 / 父角色 / 权限）

> **SPEC-074** | Status: 设计中

## 1. 目标

为 4 处下拉选择器（chat/agent task 选模型、用户关联角色、角色关联父角色、角色关联权限）提供统一的可搜索下拉能力：默认展示 topN（默认项排最前），支持关键词搜索（返回 topN）。核心红线：**过滤、排序、截取全部下沉到 DB 层（MongoDB `$match` + `$sort` + `$limit`），禁止在 Go 内存层排序/截取、禁止前端排序/截取**。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-062 多模型配置与 Session 绑定模型 | ✅ | 模型列表 `/models/list`、`IsDefaultFor` 字段已就绪 |
| SPEC-064 RBAC 角色权限管理系统 | ✅ | 角色/权限/用户-角色关联模型与 API 已就绪 |
| SPEC-066 配置存储拆分 | ✅ | model_configs / model_defaults / rbac_roles / rbac_permissions 独立集合已就绪 |
| — | — | 无阻塞项，可立即实现 |

## 2. 背景（现状不足）

现有 4 处列表接口均为「分页列表」形态，无搜索、无默认项优先排序，不满足下拉选择器的交互需求：

| 场景 | 现有接口 | 分页 | 搜索 q | 默认项排最前 |
|------|---------|:---:|:---:|:---:|
| chat/agent task 选模型 | `GET /models/list`（`ListLLM`） | ✅ page/page_size | ❌ | ❌ |
| 用户关联角色 | `GET /roles`（`ListRoles`） | ✅ + parent_id | ❌ | 部分（level 升序） |
| 角色关联父角色 | `GET /roles/:id/available-parents`（`AvailableParents`） | ❌（全量） | ❌ | 部分 |
| 角色关联权限 | `GET /permissions`（`ListPermissions`） | ✅ | ❌ | 部分（module 升序） |

现状技术债：`ListRoles`/`ListPermissions` 已在 DB 层 `$sort`+`$skip`+`$limit`（✅ 合规），但缺搜索；`AvailableParents` 返回全量（⚠️ 未截取）；模型列表的「是否默认」信息在 `model_defaults` 独立集合，`model_configs` 无冗余字段，当前 `attachDefaults` 是**内存组装**，无法纯 DB 排序实现「默认项排最前」。

## 3. 架构概述

统一「下拉选择器数据源」约定，4 处复用同一套后端查询模式 + 前端组件：

```
┌─ 前端 ─────────────────────────────────────────────┐
│  SearchableSelect（统一组件，4 处复用）              │
│   - 默认展示：GET /xxx?limit=20                    │
│   - 输入搜索：GET /xxx?q=<kw>&limit=20（防抖）       │
└──────────────────────┬─────────────────────────────┘
                       │ q, limit
┌─ 后端 handler ────────▼─────────────────────────────┐
│  统一 query 解析：q(可选) + limit(topN, 默认20, ≤100) │
└──────────────────────┬─────────────────────────────┘
                       │
┌─ repository (DB 层，红线) ─▼─────────────────────────┐
│  $match(类型/父级 + q 模糊) → $sort(默认项优先→次级)   │
│  → $skip → $limit   （禁止内存/前端排序截取）          │
└──────────────────────────────────────────────────────┘
```

## 4. API 设计

### 4.1 统一查询参数

| 参数 | 类型 | 说明 |
|------|------|------|
| `q` | string，可选 | 搜索关键词，对 `name` + `display_name`（模型含 `description`）做模糊匹配 |
| `limit` | int，可选 | topN 截取，默认 `20`，上限 `100`；下拉场景不传 page/page_size |

> 保留原有 `page`/`page_size` 分页参数不影响（管理列表页仍用分页）；新增 `q`/`limit` 为下拉场景的轻量入口。

### 4.2 接口清单

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/models/list` | LLM 模型下拉数据源（新增 `q`/`limit`，默认项排最前） |
| GET | `/api/v1/admin/models/embedding` | embedding 模型下拉（同逻辑，可选） |
| GET | `/api/v1/roles` | 角色下拉（新增 `q`/`limit`，level 升序） |
| GET | `/api/v1/roles/:id/available-parents` | 父角色下拉（改造为 `q`/`limit` + level 升序 + 排除自身与后代） |
| GET | `/api/v1/permissions` | 权限下拉（新增 `q`/`limit`，module 升序） |
| GET | `/api/v1/users` | 用户列表（关联角色下拉复用 `/roles`，本接口不动） |

## 5. 详细设计

### 5.1 各实体排序规则（默认项排最前）

| 实体 | 默认优先规则 | 次级排序 | DB 实现 |
|------|------------|---------|---------|
| 模型（llm/embedding） | `IsDefaultFor` 非空的排最前 | `name` 升序 | aggregation `$addFields` 计算 sortKey + `$sort` |
| 角色 | 内置低 level 自然靠前 | `level` 升序 → `display_name` 升序 | `$sort: {level:1, display_name:1}` |
| 父角色 | 同上 | `level` 升序 | 同角色 |
| 权限 | 常用模块靠前 | `module` 升序 → `name` 升序 | `$sort: {module:1, name:1}` |

### 5.2 模型列表「默认项排最前」的 DB 实现（核心难点）

「是否默认」在 `model_defaults` 独立集合，`model_configs` 无冗余字段。采用 **aggregation pipeline 两段**：

```
1. provider 先查 model_defaults → 得 defaultIDs []string（1 次轻量查询，已缓存）
2. aggregation:
   [
     { $match: { type: "llm",
                 ...(q 非空 ? { $or: [ {name:{$regex:q,$options:"i"}},
                                          {display_name:{$regex:q,$options:"i"}} ] } : {}) } },
     { $addFields: { _defaultOrder: { $cond: [ { $in: ["$_id", defaultIDs] }, 0, 1 ] } } },
     { $sort:  { _defaultOrder: 1, name: 1 } },
     { $skip:  skip },
     { $limit: limit },
   ]
```

- `defaultIDs` 查询由 provider 完成（读取 model_defaults），**排序仍在 DB 层完成**，不违反红线。
- 搜索命中 `name`/`display_name` 双字段（模型可扩展 `description`）。
- embedding 模型同理（`$match type:"embedding"`）。

### 5.3 角色 / 权限搜索过滤

在现有 `ListRoles`/`ListPermissions` 的 `$match` 上追加 `q` 模糊匹配（name + display_name），保持已有 `$sort`+`$skip`+`$limit` 结构不变：

```go
filter := bson.M{}
if parentID != "" { filter["parent_id"] = parentID }
if q != "" {
    filter["$or"] = []bson.M{
        {"name": bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}},
        {"display_name": bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}},
    }
}
```

> 搜索词需 `regexp.QuoteMeta` 转义，防止正则注入。

### 5.4 父角色下拉（available-parents）改造

现有 `AvailableParents(level)` 返回全量，改为：
- 接受 `q`/`limit`
- DB 层过滤：`level < 当前角色 level`（排除自身及后代，父角色必须层级更低）
- `$sort {level:1, display_name:1}` + `$limit`，**禁止全量返回后内存截取**

### 5.5 前端统一组件

新建 `SearchableSelect` 组件，4 处复用：

- props：`fetch(q, limit)` 数据源回调、`defaultValue`、`onChange`、`labelKey`（默认 `display_name`）
- 交互：打开即请求 `limit=20`；输入触发防抖（250ms）请求 `q=...&limit=20`
- 渲染：展示 `display_name`（模型/角色/权限均含该字段），选中回传 `id`
- 接入点：chat 页模型选择器、agent task 表单模型选择器、用户管理角色多选、角色管理父角色单选、角色管理权限多选

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 model_configs / model_defaults / rbac_roles / rbac_permissions） |
| 是否影响现有 API | Yes（`/models/list`、`/roles`、`/roles/:id/available-parents`、`/permissions` 增参数，向后兼容：不传 `q`/`limit` 时行为不变） |
| 性能影响 | 正向：搜索走 `$regex`（name/display_name 建议加索引），`$limit` 截断降低传输；`defaultIDs` 已缓存，无额外全表扫 |
| 是否需要新增 Skill | No |
| 是否需要索引 | 建议：`rbac_roles.name`、`rbac_roles.display_name`、`rbac_permissions.name`、`model_configs.name/display_name`（模糊查询走正则索引收益有限，量小可接受全扫） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/mongo/model_config_repo.go` | 模型列表改 aggregation（sortKey + q + limit） | Medium |
| `internal/infra/mongo/rbac_repository.go` | ListRoles/ListPermissions/AvailableParents 加 q + limit | Medium |
| `internal/adk/modelcfg/provider.go` | ListLLMModels/ListEmbeddingModels 透传 q/limit + defaultIDs | Medium |
| `internal/service/rbac/service.go` | ListRoles/ListPermissions/AvailableParents 加 q/limit | Medium |
| `internal/api/handler/modelconfig.go` | ListLLM/ListEmbedding 解析 q/limit | Small |
| `internal/api/handler/rbac.go` | ListRoles/ListPermissions/AvailableParents 解析 q/limit | Small |
| `frontend/components/SearchableSelect.tsx` | 统一下拉组件（新） | New |
| `frontend/app/...`（chat/agent task/用户/角色页） | 接入 SearchableSelect | Medium |

## 8. 测试策略

1. **Unit tests（Go）**：repository 层验证 `$match(q)` + `$sort` + `$limit` 的 bson 结构（gomonkey mock mongo collection）；provider 验证 defaultIDs 组装 sortKey；handler 验证 q/limit 解析与参数透传。
2. **Integration tests**：Docker Compose 环境验证真实 Mongo 查询（正则转义、默认项排最前、topN 截断）。
3. **E2E tests**：`UI-xxx` 覆盖 4 处下拉的「默认展示 + 搜索过滤 + 选中回传」。
4. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5 Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体 | **100%** | 排序规则、sortKey 组装 |
| L2 | 依赖接口，可 mock | **100%** | service 透传 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `service/*`, `api/handler/*` |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc` 验证 handler→service 参数传递正确性
- [ ] **必须** Service 测试的写操作验证写入内容字段和值
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证实际结果

## 10. 验证标准

1. 4 处下拉均支持默认 topN（20）展示，默认项排最前。
2. 输入关键词后返回 topN（≤20）匹配项，匹配 `name`/`display_name`，大小写不敏感。
3. 搜索词含正则元字符（如 `.` `*` `(`）不报错、不误匹配。
4. **红线验证**：代码 review 确认无内存 `sort.Slice`、无前端 `sort()/slice()` 参与最终截取；排序截取全部在 MongoDB 查询语句中。
5. 不传 `q`/`limit` 时，原分页列表行为不变（向后兼容）。
