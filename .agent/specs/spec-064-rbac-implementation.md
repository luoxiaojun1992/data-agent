# RBAC 角色权限管理系统

> **SPEC-064** | Status: 已实现

## 1. 目标

替换现有 `/admin/roles` 为 `/admin/rbac`，实现完整的 RBAC（Role-Based Access Control）系统：

1. **角色层级**：L0(系统管理员)→L1(管理员)→L2(普通用户)，最大 3 层，父角色拥有子角色所有权限
2. **权限管控**：所有 API/UI 功能走 RBAC permission 检查，system_admin 无特权
3. **用户-角色关联**：用户可关联多个 RBAC 角色（≤10），用户属性角色(system_admin/admin/user)仅用于 RBAC 入口和数据隔离
4. **数据隔离**：非 system_admin 按 `user_id` OR `is_public` 筛选，system_admin 看全部
5. **侧边栏权限化**：各菜单项由 RBAC permission 控制可见性

## 2. 核心规则

| 规则 | 说明 |
|------|------|
| 权限方向 | **父角色拥有子角色所有权限**（向上递归：admin 继承 user 的权限） |
| 层级 | L0 → L1 → L2，创建后 Level 不可改 |
| 计数上限 | Role.ChildCount ≤ 10, Role.PermissionCount ≤ 10, User.RBACRoleCount ≤ 10 |
| 内置保护 | type=builtin 不可删除/修改；内置 role 可作为自定义 role 的父 |
| RBAC 入口 | `/admin/rbac` 仅 system_admin 可访问（通过用户角色属性判断，非 RBAC） |
| 功能权限 | **所有非 RBAC 功能的 API/UI 走 RBAC permission 检查，system_admin 无特权** |
| 数据隔离 | **非 system_admin 按 `user_id + is_public` 筛选**（`user_id` / `is_system_admin` 注入 SkillContext，非 LLM 传参） |
| 侧边栏 | 改用 RBAC permission 控制可见性 |
| 初始化 | 默认 system_admin 用户(ID: `6a64aba51214fe22b2cb917d`) 关联 system_admin_role |

## 3. 数据模型

### 新增集合

**`rbac_roles`** — 角色表，唯一索引 `{name: 1}`
```
ID, Name, DisplayName, Description, ParentID(空=根), Level(0/1/2),
Type(builtin|custom), ChildCount, PermissionCount, CreatedAt, UpdatedAt
```

**`rbac_permissions`** — 权限表，唯一索引 `{key: 1}`
```
ID, Key("dashboard:view"), Name, Description, Module, Type(builtin|custom), CreatedAt
```

**`rbac_role_permissions`** — 角色-权限关联表，唯一索引 `{role_id, permission_id}`
```
ID, RoleID, PermissionID, CreatedAt
```

**`user_rbac_roles`** — 用户-角色关联表，唯一索引 `{user_id, role_id}`
```
ID, UserID, RoleID, CreatedAt
```

### 修改集合

**`users`** — 增加 `rbac_role_count: int` 字段（`$inc` 原子增减，不存在的字段从 0 开始）

### 计数字段联动规则

所有计数字段随关联记录的增删**原子联动**，不依赖定时任务或手动修正：

| 操作 | 计数字段变动 | MongoDB 操作 |
|------|-------------|-------------|
| 创建子角色 | 父 role `ChildCount` +1 | `$inc: {child_count: 1}` |
| 删除子角色 | 父 role `ChildCount` -1 | `$inc: {child_count: -1}` |
| 添加角色-权限关联 | role `PermissionCount` +1 | `$inc: {permission_count: 1}` |
| 删除角色-权限关联 | role `PermissionCount` -1 | `$inc: {permission_count: -1}` |
| 添加用户-角色关联 | user `rbac_role_count` +1 | `$inc: {rbac_role_count: 1}` |
| 删除用户-角色关联 | user `rbac_role_count` -1 | `$inc: {rbac_role_count: -1}` |

> 关联操作之前已做上限检查（≤10），`$inc` 保证计数原子准确。

### 删除安全检查（403）

| 操作 | 检查项 | 集合 |
|------|--------|------|
| 删除角色 | 子角色 | `rbac_roles.parent_id` |
| 删除角色 | 权限关联 | `rbac_role_permissions.role_id` |
| 删除角色 | 用户关联 | `user_rbac_roles.role_id` |
| 删除权限 | 角色关联 | `rbac_role_permissions.permission_id` |
| 删除用户 | RBAC 角色关联 | `user_rbac_roles.user_id` |

### 用户状态校验

`AuthMiddleware` 每次请求查 DB 验证 `user.Status == "enabled"`，`_id` 主键索引 <1ms。不做 JWT claims 缓存（token 未过期时，禁用无法立即生效）。

## 4. Seed 数据

### 内置角色（3 个）
| ID | Name | Level | ParentID | Type |
|----|------|-------|----------|------|
| rbac_role_system_admin | system_admin_role | 0 | "" | builtin |
| rbac_role_admin | admin_role | 1 | rbac_role_system_admin | builtin |
| rbac_role_user | user_role | 2 | rbac_role_admin | builtin |

### 内置权限（34 个）
| 归属角色 | Module | Key |
|----------|--------|-----|
| admin_role | dashboard | `dashboard:view` |
| user_role | chat | `chat:send`, `chat:view` |
| admin_role | chat | `chat:delete` |
| admin_role | agent | `agent:view`, `agent:create`, `agent:delete` |
| user_role | knowledge | `kb:view`, `kb:upload` |
| admin_role | knowledge | `kb:delete` |
| user_role | hermes | `hermes:view` |
| user_role | artifact | `artifact:view` |
| admin_role | artifact | `artifact:delete` |
| admin_role | im | `im:view` |
| admin_role | user | `user:view`, `user:create`, `user:edit`, `user:delete` |
| admin_role | model | `model:view`, `model:edit` |
| admin_role | task | `task:view`, `task:create`, `task:edit`, `task:delete` |
| admin_role | system | `system:view`, `system:edit` |
| admin_role | audit | `audit:view` |
| admin_role | invite | `invite:view`, `invite:create` |
| admin_role | apireview | `apireview:view` |
| admin_role | skills | `skills:view`, `skills:edit` |
| admin_role | stats | `stats:view` |
| admin_role | memory | `memory:search` |
| system_admin_role | rbac | `rbac:manage` |

> system_admin_role 无直接 permission，通过向上递归获得 admin_role → user_role 所有权限。
> 初始化时自动为默认 admin 用户关联 system_admin_role。

### 数据隔离模式（参考 knowledge search skill）

部分权限（如 `memory:search`）需要在 skill 执行时进行**数据范围隔离**，而非仅判断有无权限：

- `memory:search` — system_admin 可搜索所有用户 memory，非 system_admin 仅搜索自己的 memory
- `kb:view` — system_admin 可查看所有知识库文档，非 system_admin 仅查看自己的 + 公开文档
- 会话/任务/产出物等所有用户关联数据同理

**实现方式**: `user_id` 和 `is_system_admin`（由 `user_role` 属性推导）**注入到 SkillContext**，skill 内部根据这两个值构建 MongoDB/Qdrant filter，**不作为 LLM 生成参数传入**，避免 prompt injection 绕过数据隔离。

## 5. API 设计

### 角色 CRUD
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/rbac/roles` | 分页列表(含 count 字段) |
| GET | `/api/v1/rbac/roles/:id` | 详情 |
| POST | `/api/v1/rbac/roles` | 创建(Level=parent.Level+1) |
| PUT | `/api/v1/rbac/roles/:id` | 更新(parent_id 可改, Level 不可改) |
| DELETE | `/api/v1/rbac/roles/:id` | 删除(仅 custom，有关联则 403) |

### 权限
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/rbac/permissions` | 分页列表 |
| GET | `/api/v1/rbac/permissions/:id` | 详情 |
| DELETE | `/api/v1/rbac/permissions/:id` | 删除(仅 custom，有关联则 403) |

### 关联与计算
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/rbac/roles/:roleId/permissions` | 已关联权限(分页) |
| POST | `/api/v1/rbac/roles/:roleId/permissions` | 添加(max=10) |
| DELETE | `/api/v1/rbac/roles/:roleId/permissions/:permId` | 删除 |
| GET | `/api/v1/rbac/roles/:id/effective-permissions` | 递归子角色权限 |
| GET | `/api/v1/rbac/me/permissions` | 当前用户权限列表(Sidebar 使用) |
| GET | `/api/v1/rbac/roles/:id/available-parents` | 可选父角色(层级 ≤ 2, ChildCount < 10) |

### 用户-角色关联
| 方法 | 路径 | 说明 |
|------|------|------|
| GET | `/api/v1/admin/users/:userId/rbac-roles` | 已关联角色(分页) |
| POST | `/api/v1/admin/users/:userId/rbac-roles` | 添加(max=10) |
| DELETE | `/api/v1/admin/users/:userId/rbac-roles/:roleId` | 删除 |

> 所有 RBAC API 受 `RequirePermission("rbac:manage")` 保护

## 6. 中间件改造

### RequirePermission 改造

**旧签名**: `func RequirePermission(permission string) gin.HandlerFunc`

**新签名**: `func RequirePermission(svc rbac.Service, permission string) gin.HandlerFunc`

逻辑：
```
1. targetPerm == "rbac:manage" → 仅 system_admin 通过
2. 其他 → 查 user_rbac_roles → 递归子角色 → 查 permissions → 判断
```

**权限查询**: 三步 Go 查询替代 aggregation pipeline（角色 ≤100/用户，<10ms）

**数据隔离**: 提供 `CanSeeAllData(userRole string) bool` 辅助函数，各 handler 内联判断 filter

### 旧权限常量迁移（按路由）

| 路由 | 旧常量 | 新 key |
|------|--------|--------|
| skill-config CRUD | `PermModelConfig` | `model:edit` |
| admin tasks list/retry/cancel | `PermUserManage` | `task:view`/`task:edit` |
| admin invites CRUD | `PermUserManage` | `invite:view`/`invite:create` |
| admin hmac-secret | `PermSystemConfig` | `system:edit` |
| admin knowledge docs | `PermUserManage` | `kb:delete` |
| audit logs | `PermAuditLogView` | `audit:view` |
| api-reviews | `PermAPIConvert` | `apireview:view` |
| stats | `PermUserManage` | `stats:view` |
| memory search | `PermUserManage` | `memory:search` |

## 7. 前端页面

### `/admin/rbac` (替换 `/admin/roles`)
- Tab 角色管理: 分页卡片列表 + 新建/编辑 Modal + [管理权限] 跳转
- Tab 权限列表: 分页表格(只读)

### `/admin/rbac/roles/:roleId/permissions`
- 已关联权限列表(分页，可删除) + [添加权限] Modal

### `/admin/users/:userId/rbac-roles`
- 已关联 RBAC 角色列表(分页) + [添加角色] Modal

### `/admin/users` 改造
- 每行加「RBAC 管理」按钮(仅 system_admin)
- 启用/禁用调 `PATCH /users/:id/status` 同步后端
- system_admin 行不显示启用/禁用按钮

### Sidebar 改造
- 新增 `rbac` 菜单项在 `users` 前
- 所有菜单项权限驱动 visibility

## 8. 文件变更

### 后端新增
`internal/domain/model/rbac.go`, `internal/infra/mongo/rbac_repository.go`, `internal/service/rbac/service.go`, `internal/api/handler/rbac.go`, `cmd/server/migration/rbac_seed.go`

### 后端修改
`model.go`(User+RBACRoleCount), `user_repository.go`(Add/RemoveRBACRole), `user.go`(路由), `routes.go`(RBAC路由+删除旧路由), `middleware/rbac.go`(闭包注入), `wire.go`(DI)

### 后端删除
`model.Role`/`FixedRoles()`/`PermXxx`, `role_repository.go`, `service/role/`, `/api/v1/roles`

### 前端新增
`admin/rbac/page.tsx`, `admin/rbac/roles/[id]/permissions/page.tsx`, `admin/users/[id]/rbac-roles/page.tsx`

### 前端修改
`admin/users/page.tsx`(RBAC按钮), `Sidebar.tsx`(入口+权限化), `admin/roles/page.tsx`(删除)

## 9. 实施顺序

| 阶段 | 内容 | 风险 |
|------|------|------|
| 1. Model + Seed | 模型定义，migration 插入数据 | seed 幂等 |
| 2. Repository | MongoDB 仓库 + 计数原子操作 | 不影响现有 |
| 3. Service | 权限判断 + 约束校验 | 不影响现有 |
| 4. Handler + Routes | RBAC API 注册 | 新路由 |
| 5. 前端 RBAC 页面 | 角色+权限 Tab + 关联页 | 新页面 |
| 6. **中间件改造** | 闭包注入，所有路由加权限 | **确保 admin 有所有权限** |
| 7. 用户-角色关联 | API + User Repository | 新增功能 |
| 8. Sidebar 改造 | RBAC 入口 + 权限化菜单 | 最后改 |
| 9. 删除旧代码 | 清理旧 role 体系 | RBAC 稳定后 |
