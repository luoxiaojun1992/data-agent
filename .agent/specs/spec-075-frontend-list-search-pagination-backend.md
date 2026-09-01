# 前端列表搜索/分页后端化重构（统一 DB 层筛选分页）

> **SPEC-075** | Status: 设计已定稿（2026-09-01 过时引用修正：tasks/roles/sysconfig 页面已删除）

## 1. 目标

排查前端所有列表页的搜索、过滤、分页实现，凡是**前端内存 `filter()` / `slice()` 过滤分页**的，一律重构为**后端 DB 层筛选、排序、分页**（MongoDB `$match` + `$sort` + `$skip` + `$limit`），并返回准确的 `total`。核心红线：**搜索、筛选、分页全部在后端 DB 层完成，前端只传参数、只渲染后端返回结果**。

## 1.5 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-074 可搜索下拉选择器 | ✅ 设计已定稿 | 模型/角色/权限的 `q` 搜索后端能力可复用（模型 `q` 搜索、父角色过滤、`exclude_user_id`） |
| SPEC-062 / 064 / 066 | ✅ | 模型/角色/权限/任务/会话等后端列表接口已存在，仅缺 `q`/过滤参数 |
| — | — | 无阻塞项 |

## 2. 背景（现状排查结论）

### 2.1 违规清单（前端本地过滤/分页，需重构）

| # | 页面 | 文件 | 违规点 | 整改方向 |
|---|------|------|--------|---------|
| 1 | 知识库 `/knowledge` | `app/knowledge/page.tsx` | 搜索 `docs.filter(...)`、标签 `filter(tagFilter)`、分页 `ceil(filtered.length/PAGE_SIZE)` 全部前端本地 | 后端 `/knowledge/docs` 加 `q` + `tag` + 分页返回 total |
| 2 | 模型管理 `/admin/models` | `app/admin/models/page.tsx:369` | `filteredLLM = llmList.filter(matches)`、`filteredEmbedding` 前端本地搜索 | 后端 `/models/list`、`/admin/models/embedding` 加 `q`（复用 SPEC-074） |
| 3 | ~~任务管理 `/admin/tasks`~~ | 页面已删除（2026-09-01，commit 28b34b3） | ~~`tasks.filter(t.status === filter)`~~ | ✅ 违规随页面删除消除；任务入口 `/agent` 页经 grep 验证无本地 filter/slice/search，无需整改 |
| 4 | 会话列表 `/chat` | `app/chat/page.tsx:738` | `sessions.filter(s => title/id includes search)` 前端本地搜索 | 后端 `/sessions` 加 `q` 搜索 |
| 5 | RBAC 角色管理 `/admin/rbac` | `app/admin/rbac/page.tsx:220,247` | 父角色下拉 `roles.filter(level<2 && child_count<10)`、`roles.filter(level===...-1 ...)` 前端过滤 | 复用 SPEC-074 5.4 available-parents 改造（`q`/`limit` + level 过滤下沉 DB） |
| 6 | 角色权限分配 `/admin/rbac/roles/[id]/permissions` | `.../permissions/page.tsx:47` | `allPerms.filter(p => !perms.find(...))` 前端计算可用权限 | 后端提供「已分配/可用」权限查询（`assigned` 参数） |
| 7 | 用户角色分配 `/admin/users/[id]/rbac-roles` | `.../rbac-roles/page.tsx:36` | `allRoles.filter(r => !roles.find(...))` 前端计算可用角色 | 复用 SPEC-074 5.7（`/admin/rbac/roles?q&limit&exclude_user_id`，DB `$nin`） |
| 8 | ~~旧角色管理 `/admin/roles`~~ | 页面已删除（2026-09-01，commit 28b34b3） | ~~前端分组 filter~~ | ✅ 已确认废弃并删除，违规随之消除 |

### 2.2 合规清单（后端 DB 分页/搜索，无需改，作样板）

| 页面 | 实现 | 结论 |
|------|------|------|
| 记忆 `/memory` | `/memory/list?q=&page=&page_size=`（防抖搜索走后端） | ✅ 样板 |
| 产出物 `/artifacts` | `/artifacts?session_id=&page=` | ✅ |
| 用户管理 `/admin/users` | `/users?skip=&limit=&sort=` | ✅ |
| API 集合 `/admin/api-collections` | `/admin/api-collections?page=&page_size=` | ✅ |
| Skill `/admin/skills` | `/admin/skills?page=&page_size=` | ✅ |
| 审计 `/admin/audit`、邀请 `/admin/invites` | 后端分页 | ✅ |

## 3. 架构概述

统一「列表页 = 后端筛选 + 分页」约定，前端列表组件只做「传参 + 渲染 + 翻页/搜索触发重新请求」：

```
前端列表页
  ├─ 搜索框 onChange → 防抖 → 请求 ?q=<kw>&page=1（重置页码）
  ├─ 筛选器（状态/标签/类型）→ 请求 ?<field>=<value>&page=1
  ├─ 分页器 → 请求 ?page=N
  └─ 渲染后端返回的 rows + total
          │
          ▼
后端 handler：解析 q / 筛选字段 / page / page_size（统一约定）
          │
          ▼
repository（红线）：$match(类型 + q 模糊 + 筛选字段) → $sort → $skip → $limit
                     + CountDocuments（返回 total）
```

## 4. API 设计

### 4.1 统一参数约定

| 参数 | 类型 | 说明 |
|------|------|------|
| `q` | string，可选 | 关键词搜索，`$regex`（`regexp.QuoteMeta` 转义），匹配 name/display_name/title 等 |
| `page` / `page_size` | int | 分页（列表页沿用），下拉用 `limit`（SPEC-074） |
| 业务筛选字段 | — | 如 `status`、`tag`、`session_id`、`parent_id`、`assigned`、`exclude_assigned_to` |

### 4.2 接口改造清单

| Method | Path | 新增/变更 | 说明 |
|--------|------|----------|------|
| GET | `/api/v1/knowledge/docs` | `q` + `tag` | 知识库搜索/标签筛选（DB 层 `$match`） |
| GET | `/api/v1/models/list` | `q` | 模型列表搜索（复用 SPEC-074） |
| GET | `/api/v1/admin/models/embedding` | `q` | embedding 模型搜索 |
| GET | `/api/v1/sessions` | `q` | 会话搜索（title/id） |
| GET | `/api/v1/admin/rbac/roles/:id/available-parents` | `q` + `limit` | 父角色候选（复用 SPEC-074 5.4 改造） |
| GET | `/api/v1/admin/rbac/roles/:id/permissions` | `assigned`(bool) | 已分配 vs 可用权限 |
| GET | `/api/v1/admin/rbac/roles` | `exclude_user_id`（+`q`/`limit`） | 用户可选角色（复用 SPEC-074 5.7） |

> path 一律以 `routes.go` 实际注册为准（`/api/v1/admin/rbac/*`），参数命名与 SPEC-074 保持一致（`exclude_user_id`，非 `exclude_assigned_to`）。

## 5. 详细设计

### 5.1 DB 层搜索过滤（统一实现模式）

```go
// repository 层（以 ListDocs 为例）
filter := bson.M{}
if q != "" {
    filter["$or"] = []bson.M{
        {"title": bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}},
        {"file_name": bson.M{"$regex": regexp.QuoteMeta(q), "$options": "i"}},
    }
}
if tag != "" {
    filter["tags"] = tag // 数组包含匹配
}
total, _ := coll.CountDocuments(ctx, filter)
cur, _ := coll.Find(ctx, filter, options.Find().
    SetSkip(skip).SetLimit(limit).SetSort(bson.M{"created_at": -1}))
```

> 红线：`$match` 过滤、`$sort` 排序、`$skip`/`$limit` 分页、`CountDocuments` 计 total，全部在 DB 层；禁止 Go 内存 `sort.Slice`/切片截取、禁止前端 `filter`/`slice` 参与最终结果。

### 5.2 前端改造模式

以知识库页为改造样例：

```tsx
// 改造后：搜索/标签/分页全部走后端
const res = await apiFetch(`/knowledge/docs?page=${page}&page_size=${PAGE_SIZE}&q=${q}&tag=${tag}`);
// data: { docs, total } — 前端只渲染，不再 filter/slice
```

- 搜索框输入 → 防抖 250ms → `setQ` + `setPage(1)` → 重新请求
- 筛选器切换 → `setFilter` + `setPage(1)` → 重新请求
- 分页器 → `setPage` → 重新请求
- 删除/更新操作后 → 刷新当前页（若删到空页则回退一页）

### 5.3 疑似废弃页面确认（已闭环）

~~`app/admin/roles/page.tsx`（旧 role 体系）~~：✅ **已确认废弃并删除**（2026-09-01，commit 28b34b3「清理废弃 admin 页面」，与 `/admin/tasks`、`/admin/sysconfig` 一并删除）。前端本地 `filter(type)` 违规随删除消除，无需后端化改造。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | Yes（8 处接口加 `q`/筛选参数，向后兼容：不传参数行为不变） |
| 性能影响 | 正向：过滤/分页下沉 DB，减少传输；建议 name/title/status 加索引 |
| 是否需要新增 Skill | No |
| 是否需要索引 | 建议：`knowledge_docs.title/file_name`、`sessions.title`、`tasks.status`、`rbac_roles.name/display_name` |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/infra/mongo/kb_repository.go` | ListDocs 加 q/tag | Small |
| `internal/infra/mongo/model_config_repo.go` | 模型 List 加 q（复用 SPEC-074） | Medium |
| `internal/infra/mongo/task_repository.go` | ListTasks 加 status | Small |
| `internal/infra/mongo/session_repository.go` | ListSessions 加 q | Small |
| `internal/infra/mongo/rbac_repository.go` | ListRoles/ListPermissions 加过滤参数 | Medium |
| `internal/service/knowledge/service.go` | ListDocs 透传 q/tag | Small |
| `internal/service/rbac/service.go` | ListRoles/ListPermissions 透传过滤 | Medium |
| `internal/api/handler/*.go` | 各 handler 解析 q/筛选参数 | Small |
| `frontend/app/knowledge/page.tsx` | 搜索/标签/分页走后端 | Medium |
| `frontend/app/admin/models/page.tsx` | 搜索走后端 | Small |
| `frontend/app/chat/page.tsx` | 会话搜索走后端 | Small |
| `frontend/app/admin/rbac/page.tsx` | 父角色下拉走后端 | Small |
| `frontend/app/admin/rbac/roles/[id]/permissions/page.tsx` | 可用权限走后端 | Small |
| `frontend/app/admin/users/[id]/rbac-roles/page.tsx` | 可用角色走后端（复用 SPEC-074 5.7） | Small |
| ~~`frontend/app/admin/roles/page.tsx`~~ / ~~tasks~~ | ✅ 已删除（2026-09-01） | — |

## 8. 测试策略

1. **Unit tests（Go）**：repository 层验证 `$match(q/tag/status)` + `$sort` + `$skip/$limit` + CountDocuments 的 bson 结构；handler 验证参数解析与透传。
2. **Integration tests**：Docker Compose 验证真实查询（正则转义、分页 total 准确、筛选字段匹配）。
3. **E2E tests**：`UI-xxx` 覆盖知识库/模型/任务/会话搜索 + 分页、父角色/权限/角色分配下拉的后端化行为。
4. **审计**：`.agent/skills/go-ut-audit`。

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
| L1 | 纯函数/纯结构体 | **100%** | bson filter 组装 |
| L2 | 依赖接口，可 mock | **100%** | service 透传 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `service/*`, `api/handler/*` |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc` 验证 handler→service 参数传递正确性
- [ ] **必须** Service 测试的写操作验证写入内容字段和值
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证实际结果

## 10. 验证标准

1. 6 处整改页面（知识库/模型/会话/RBAC 角色/权限分配/用户角色）的搜索/筛选/分页全部走后端 DB 层，前端无 `filter()`/`slice()` 参与最终列表结果。
2. 分页 `total` 准确反映后端过滤后的总数。
3. 搜索词含正则元字符不报错、不误匹配。
4. 不传 `q`/筛选参数时行为不变（向后兼容）。
5. ✅ 废弃页面已闭环：`/admin/roles`、`/admin/tasks` 已删除（2026-09-01），违规随之消除；任务入口 `/agent` 页验证无本地过滤。
