# 可搜索下拉选择器统一设计（模型 / 角色 / 父角色 / 权限）

> **SPEC-074** | Status: 设计中

## 1. 目标

为 4 处下拉选择器（chat/agent task 选模型、用户关联角色、角色关联父角色、角色关联权限）提供统一的可搜索下拉能力：默认展示 topN，支持关键词搜索（返回 topN）。核心红线：**过滤、搜索过滤、排序、截取全部下沉到 DB 层（MongoDB `$match` + `$sort` + `$limit`）** —— 禁止在 Go 内存层排序/截取、禁止前端排序/截取、**禁止前端本地过滤（搜索必须走后端 DB 层 `$match`，不允许前端对已加载列表做 `filter()`）**。

> 补充说明（晓军确认）：
> 1. **搜索过滤同样必须在后端 DB 层完成**：`q` 关键词的匹配在 MongoDB `$match`（`$regex`）中执行，前端只负责把 `q` 传给后端，绝不本地过滤。
> 2. **角色、权限无「默认项」概念**：默认排序无需「默认优先」，保持稳定排序（角色 level、权限 module）即可，允许随机，不做默认项加权。

> 范围补充（晓军 2026-09-01）：
> 1. **用户三级主角色下拉（定死枚举，非搜索）**：`User.Role`（`user`/`admin`/`system_admin`）写死 3 个 option、不走搜索，见 5.6。
> 2. **用户管理 RBAC 角色关联（DB 搜索）**：用户管理页给用户关联 RBAC 角色（`user_rbac_roles`）的场景**必须走 DB 搜索**（`GET /admin/rbac/roles?q=&limit=`），现状为「全量拉取 + 前端本地 filter」违反红线，见 5.7。
> 两者是**不同维度**（三级主角色 ≠ RBAC 角色），必须同时改、且互不混淆。

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
| 用户关联角色 | `GET /admin/rbac/roles`（`ListRoles`） | ✅ + parent_id | ❌ | 部分（level 升序） |
| 角色关联父角色 | `GET /admin/rbac/roles/:id/available-parents`（`AvailableParents`） | ❌（全量） | ❌ | 部分 |
| 角色关联权限 | `GET /admin/rbac/permissions`（`ListPermissions`） | ✅ | ❌ | 部分（module 升序） |

> ⚠️ 用户关联角色场景的现状问题（5.7 详述）：前端 `[id]/rbac-roles` 页「添加角色」弹窗调用 `apiFetch('/admin/admin/rbac/roles?page_size=200')` —— path 错误（`apiFetch` 自动补 `/api/v1` 前缀后变成 `/api/v1/admin/admin/rbac/roles` → 404），且即使修正 path，「`page_size=200` 全量拉取 + 前端 `filter()`/`includes()` 本地搜索」也违反本 spec 红线（搜索/过滤必须 DB 层）。

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
| `exclude_user_id` | string，可选 | **仅角色列表**：排除该用户已关联的角色（DB 层 `$nin`，供「用户关联角色」下拉用） |

> 保留原有 `page`/`page_size` 分页参数不影响（管理列表页仍用分页）；新增 `q`/`limit` 为下拉场景的轻量入口。

### 4.2 接口清单

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/models/list` | LLM 模型下拉数据源（新增 `q`/`limit`，默认项排最前） |
| GET | `/api/v1/admin/models/embedding` | embedding 模型下拉（同逻辑，可选） |
| GET | `/api/v1/admin/rbac/roles` | 角色下拉（新增 `q`/`limit`/`exclude_user_id`，level 升序） |
| GET | `/api/v1/admin/rbac/roles/:id/available-parents` | 父角色下拉（改造为 `q`/`limit` + level 升序 + 排除自身与后代） |
| GET | `/api/v1/admin/rbac/permissions` | 权限下拉（新增 `q`/`limit`，module 升序） |
| GET | `/api/v1/admin/users/:id/rbac-roles` | 用户已关联角色分页列表（本接口不动，仅作 `exclude_user_id` 的数据源参考） |

> path 以 `routes.go` 实际注册为准（`registerRBACRoutes`：`/api/v1/admin/rbac/*`）。前端一律走 `apiFetch('/admin/rbac/...')`（自动补 `/api/v1` 前缀），**禁止**手写 `/admin/admin/...` 之类重复前缀。

## 5. 详细设计

### 5.1 各实体排序规则

> 「默认项排最前」仅适用于**模型**（有 `IsDefaultFor` 语义）。**角色、权限无默认项**，不做默认加权，保持稳定排序即可（允许随机）。

| 实体 | 是否有默认项 | 排序规则 | DB 实现 |
|------|:---:|---------|---------|
| 模型（llm/embedding） | ✅ 有 | `IsDefaultFor` 非空的排最前 → `name` 升序 | aggregation `$addFields` 计算 sortKey + `$sort` |
| 角色 | ❌ 无 | `level` 升序 → `display_name` 升序（稳定排序，无默认加权） | `$sort: {level:1, display_name:1}` |
| 父角色 | ❌ 无 | `level` 升序 → `display_name` 升序（稳定排序） | 同角色 |
| 权限 | ❌ 无 | `module` 升序 → `name` 升序（稳定排序，无默认加权） | `$sort: {module:1, name:1}` |

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
- 接入点：chat 页模型选择器、agent task 表单模型选择器、用户管理 RBAC 角色关联（`/admin/users/[id]/rbac-roles`「添加角色」弹窗，见 5.7）、角色管理父角色单选、角色管理权限多选

> **红线**：搜索必须请求后端（`q` 参数走 DB 层 `$match`），**禁止前端对已加载列表做本地 `filter()`/`sort()`/`slice()`**。组件只渲染后端返回的结果。

### 5.6 用户三级主角色下拉（定死枚举，非搜索）—— 修复「编辑用户角色空下拉」bug

> **本小节与 5.5 的 SearchableSelect「RBAC 角色搜索下拉」是两回事，必须区分。** 这是晓军 2026-09-01 追加的 bug 修复，合并进本 spec。

#### 5.6.1 概念澄清（关键边界）

系统里存在**两个维度**的「角色」，前端必须分清：

| 维度 | 类型 | 值域 | 数据来源 | 下拉形态 |
|------|------|------|---------|---------|
| **三级主角色** `User.Role` | `model.UserRole` 枚举 | **定死**：`user` / `admin` / `system_admin` | 无（常量） | **定死枚举 `<select>`（本 bug 修这里）** |
| **RBAC 角色** | `model.Role`（rbac_roles 集合） | 可自定义：`role_system_admin` / `role_data_analyst` / `role_kb_admin` / `role_auditor` … | `GET /roles`（DB） | SearchableSelect 搜索下拉（5.5 场景） |

- 本 bug 修的是**三级主角色**：`app/admin/users/page.tsx` 的「添加用户」(`user-add-role`) 与「编辑用户角色」(`user-edit-role`) 两个 `<select>` 目前**空选项**（仅注释 `NOTE: both dropdowns...`，无任何 `<option>`）。
- 打开编辑弹窗时 `setFormRole(user.role)` 已正确回填（page.tsx:197），但因下拉无 option，用户**看不到当前角色、也无法切换**。

#### 5.6.2 修复方案（定死枚举，写死 3 个 option）

两个 `<select>` 都写死以下 3 个 option，**不走 SearchableSelect、不走后端搜索**（枚举定死，无需查询）：

```tsx
<option value="user">普通用户</option>
<option value="admin">管理员</option>
<option value="system_admin">系统管理员</option>
```

#### 5.6.3 前后端一致（红线，晓军强调）

| 层 | 值（必须完全一致） |
|----|------------------|
| 前端 option `value` | `"user"` / `"admin"` / `"system_admin"` |
| 后端枚举 `model.UserRole`（model.go:11-13） | `RoleUser="user"` / `RoleAdmin="admin"` / `RoleSystemAdmin="system_admin"` |
| 后端校验（user.go:143-144） | 仅接受上述 3 值，否则 400 `"invalid role"` |
| DB 存储（`users.role`） | 上述 3 个字符串，MongoDB 直接落库 |

- ⛔ 前端**必须**传与后端枚举一致的字符串：禁止中文字面量（如 `"管理员"`）、禁止大小写变体（如 `"Admin"` / `"SYSTEM_ADMIN"`）、禁止空串/`undefined`。
- 显示名（普通用户 / 管理员 / 系统管理员）**仅前端展示用，不入参、不落库**。
- 提交逻辑统一：`handleAdd` 与 `handleEdit` 均直接传 `formRole`（formRole 由定死 option 保证只可能为 3 个合法值）。现有 `handleAdd` 的三元归一化（page.tsx:92）属冗余可删除，`handleEdit` 直接 `role: formRole`（page.tsx:119）已是正确形态。
- 编辑弹窗 `formRole` 回填已正确（page.tsx:197 `setFormRole(user.role)`），修复后无需改动；但需新增 E2E 断言验证回填 + 切换。

### 5.7 用户管理 RBAC 角色关联（DB 搜索）—— 修复「添加角色弹窗本地过滤」违规

> **本小节与 5.6 是同一页面（用户管理）的两个不同维度，必须同时改且互不混淆**：5.6 修三级主角色（定死枚举），本小节修 RBAC 角色关联（DB 搜索）。

#### 5.7.1 现状与问题

位置：`frontend/app/admin/users/[id]/rbac-roles/page.tsx`「添加角色」弹窗。

现状代码两个问题（page.tsx:30-37）：

1. **path 错误 → 弹窗可用角色列表永远为空**：`fetchAll()` 调用 `apiFetch('/admin/admin/rbac/roles?page=1&page_size=200')`。`apiFetch` 自动补 `/api/v1` 前缀（lib/api.ts:144），实际请求 `/api/v1/admin/admin/rbac/roles` → 404（正确 path 为 `/api/v1/admin/rbac/roles`，见 routes.go:326）。
2. **本地过滤/搜索违反红线**：即使修正 path，`page_size=200` 一次拉全量，然后 `available = allRoles.filter(r => !roles.find(...) && (search === '' || r.name.includes(search) || r.display_name.includes(search)))`（page.tsx:36-37）—— 排除已关联角色、关键词搜索**全部在前端内存完成**，违反「过滤、搜索必须下沉 DB 层」红线。

后端现状：`ListRoles`（routes.go:326，`rbac:view` 权限）已有 DB 层 `$sort`+`$skip`+`$limit`（✅ 合规），但无 `q` 搜索、无「排除已关联角色」过滤。

> **权限边界（晓军 2026-09-01 确认：现状即设计，⛔ 不修改 seed）**：
> - `rbac:view` 分配给 admin（rbac_seed.go:156）是**刻意设计**——admin 需要在**用户管理**的 RBAC 角色关联场景（本 5.7 节「添加角色」弹窗）查看/搜索角色列表，但**不需要** RBAC 管理页面。
> - `admin:menu:rbac`（管理后台首页入口卡片）与 `rbac:manage`（管理操作）均仅 sysAdmin —— admin 看不到 RBAC 入口、也不能进 RBAC 管理页做增删改，这是预期行为，**非 bug**。
> - 实现 5.7 时**禁止**把 `admin:menu:rbac` 分配给 admin「对齐」、也**禁止**收紧 `rbac:view`。`/admin/rbac/roles` 接口同时服务「RBAC 管理页」与「用户管理角色关联」两个前端场景，`rbac:view` 同时覆盖两者是共享接口的正常结果。

#### 5.7.2 修复方案

**后端（`ListRoles` 加参数）**：

- 新增可选参数 `q` / `limit` / `exclude_user_id`（见 4.1）。
- `exclude_user_id` 非空时：先查 `user_rbac_roles` 集合该用户的 role_ids（1 次轻量查询），`$match` 追加 `_id: {$nin: roleIDs}` —— **排除过滤下沉 DB，禁止前端做差集**。
- `$match` 追加 `q` 模糊匹配（name + display_name，`regexp.QuoteMeta` 转义），保持既有 `$sort {level:1, display_name:1}` + `$skip` + `$limit`。

**前端（`[id]/rbac-roles` 页「添加角色」弹窗）**：

- 删除 `fetchAll` 全量拉取 + 本地 `filter`/`includes`。
- 打开弹窗请求 `GET /admin/rbac/roles?limit=20&exclude_user_id=<id>`；输入搜索（防抖 250ms）请求 `q=<kw>&limit=20&exclude_user_id=<id>`。数据源逻辑复用 SearchableSelect 的 fetch 能力（组件提供列表渲染模式或 `renderItem` 定制）。
- 渲染形态保留现状「列表 + 逐行添加按钮」（该场景是"逐个关联"交互，非单选回传）；保留「最多 10 个」上限与移除交互。
- ⚠️ 该页已关联列表的手写分页不属本 spec，由 SPEC-078（分页统一）接管，本 spec 不掺入。

#### 5.7.3 与 5.6 的边界（同一页面、两个维度）

| 维度 | 下拉形态 | 数据来源 | 后端改动 | 前端改动 |
|------|---------|---------|---------|---------|
| 三级主角色 `User.Role`（5.6） | 定死枚举 `<select>`，写死 3 option | 常量 | **无**（枚举已定死） | `users/page.tsx` 两处空下拉补 option |
| RBAC 角色关联（5.7） | 搜索下拉（SearchableSelect 数据源） | `GET /admin/rbac/roles?q&limit&exclude_user_id` | ListRoles 加 3 参数 | `users/[id]/rbac-roles/page.tsx` 弹窗改造 |

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
| `internal/infra/mongo/rbac_repository.go` | ListRoles/ListPermissions/AvailableParents 加 q + limit；ListRoles 加 exclude_user_id（$nin） | Medium |
| `internal/adk/modelcfg/provider.go` | ListLLMModels/ListEmbeddingModels 透传 q/limit + defaultIDs | Medium |
| `internal/service/rbac/service.go` | ListRoles/ListPermissions/AvailableParents 加 q/limit；ListRoles 加 exclude_user_id 透传 | Medium |
| `internal/api/handler/modelconfig.go` | ListLLM/ListEmbedding 解析 q/limit | Small |
| `internal/api/handler/rbac.go` | ListRoles/ListPermissions/AvailableParents 解析 q/limit；ListRoles 解析 exclude_user_id | Small |
| `frontend/components/SearchableSelect.tsx` | 统一下拉组件（新，支持列表渲染模式） | New |
| `frontend/app/...`（chat/agent task/角色页） | 接入 SearchableSelect | Medium |
| `frontend/app/admin/users/page.tsx` | 修复两个空下拉（写死 3 option）+ 提交 role 归一化（5.6） | Small |
| `frontend/app/admin/users/[id]/rbac-roles/page.tsx` | 「添加角色」弹窗改造：删全量拉取+本地过滤，改 q/limit/exclude_user_id DB 搜索（5.7） | Small |

## 8. 测试策略

1. **Unit tests（Go）**：repository 层验证 `$match(q)` + `$sort` + `$limit` 的 bson 结构（gomonkey mock mongo collection）；provider 验证 defaultIDs 组装 sortKey；handler 验证 q/limit 解析与参数透传；`ListRoles` 验证 `exclude_user_id` 查询 user_rbac_roles 后生成 `$nin` 过滤（含该用户无关联角色时 `$nin` 可省略的分支）。
2. **Integration tests**：Docker Compose 环境验证真实 Mongo 查询（正则转义、默认项排最前、topN 截断、exclude_user_id 排除正确）。
3. **E2E tests**：`UI-xxx` 覆盖 4 处搜索下拉的「默认展示 + 搜索过滤 + 选中回传」；用户管理覆盖 5.6（三级角色定死枚举回填/切换）与 5.7（添加角色弹窗默认 topN、搜索后 topN、不含已关联角色、添加成功）。
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

1. 4 处下拉均支持默认 topN（20）展示；**模型**默认项排最前，**角色/权限无默认项**（稳定排序即可，允许随机）。
2. 输入关键词后返回 topN（≤20）匹配项，匹配 `name`/`display_name`，大小写不敏感；**搜索过滤在后端 DB 层完成**。
3. 搜索词含正则元字符（如 `.` `*` `(`）不报错、不误匹配。
4. **红线验证**：代码 review 确认无内存 `sort.Slice`、无前端 `sort()/slice()/filter()` 参与最终过滤/排序/截取；过滤、搜索、排序、截取全部在 MongoDB 查询语句中完成。
5. 不传 `q`/`limit` 时，原分页列表行为不变（向后兼容）。
6. 用户管理「添加用户」「编辑用户角色」两个下拉显示定死 3 个选项（普通用户/管理员/系统管理员），选值回传为 `user`/`admin`/`system_admin`，与后端枚举/DB 存储一致；编辑弹窗正确回填当前用户主角色。
7. 用户管理 `[id]/rbac-roles` 页「添加角色」弹窗：打开即显示 topN（20）可用角色（**已排除该用户已关联角色**），搜索关键词后显示 DB 层匹配的 topN；代码 review 确认无 `page_size=200` 全量拉取、无前端 `filter()`/`includes()` 本地搜索/排除。
