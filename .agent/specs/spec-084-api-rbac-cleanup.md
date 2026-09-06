# API 权限整理与废弃接口清理

> **SPEC-084** | Status: 已实现

## 1. 目标

全面整理 data-agent 的 HTTP API 权限体系：① 删除所有不再使用的 API（无前端页面/无前端调用/重构后遗留，飞书回调除外）；② 清理主动注册（保留邀请注册，走临时 token）；③ 补齐用户管理/通知/知识库/artifact/task/统计分析等接口的 RBAC 权限与数据隔离；④ 前端通知 UI 区分「定向发送」与「广播」。

**权限总原则**（晓军确认）：除「token refresh / 查看 me 信息 / 查看 me 权限 / 修改密码 / 登录 / 被邀请注册」外，**其余所有 API 都必须有对应的 RBAC 权限限制与校验**。

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-064 RBAC 角色权限管理系统 | ✅ | 权限枚举 `domain/model/rbac.go` + `RequirePermission` middleware + `rbac_seed.go` 已就绪 |
| SPEC-044 邀请注册系统 | ✅ | HMAC 邀请 token 已实现，`IsInviteEnabled()` 就绪 |
| SPEC-072 Dashboard 统计重构 | ✅ | `/dashboard` + `/dashboard/trends` 已实现（当前 JWT-only） |
| SPEC-082 Chat/Task 取消 | ✅ | 已定稿删除 `pause/resume`（本 spec 落地其删除动作） |
| SPEC-083 用户中心与改密 | ✅ | 改密迁 `/auth/change-password`，登录即可（无 RBAC） |

> 无阻塞项。本 spec 复用已有权限常量，仅新增 2 个通知权限。

## 3. 背景（现状与问题）

### 3.1 现状权限分布（已排查）

| 类别 | 路由 | 问题 |
|------|------|------|
| 无 auth | `/health` `/api/v1/health` `/im/feishu/webhook` | 合理（健康检查/飞书回调） |
| 无 auth | `/auth/login` `/auth/register`(POST) `/auth/register`(GET verify) `/auth/complete-registration` | **`/auth/register`(POST) 主动注册已死**（IsInviteEnabled 后返回 410） |
| 无 auth | `/api/v1/system/stats` | **无前端调用 + 泄露内存/goroutine**，SPEC-079 已有 `/health` 取代 |
| JWT-only | `/auth/refresh` `/auth/profile` `/rbac/me/permissions` | 合理（白名单） |
| JWT-only | `/admin/change-password` | 合理（SPEC-083 迁 `/auth/change-password`） |
| JWT-only | `/users/*` | ⚠️ **无 RBAC**（普通 user 能增删改查用户/改角色/停用） |
| JWT-only | `/notifications/*` | ⚠️ **无 RBAC**（含 POST 定向 + `/broadcast` 广播） |
| JWT-only | `/knowledge/*` | ⚠️ 权限常量 `kb:*` 已定义但路由未挂 |
| JWT-only | `/artifacts/*` | ⚠️ 权限常量 `artifact:*` 已定义但路由未挂 |
| JWT-only | `/workspace/:session_id/files` | ⚠️ **前端无调用**（agent 走 `fsops` 本地目录，不走 HTTP） |
| JWT-only | `/tasks/*` `/runs/:id` | ⚠️ **无 RBAC**（task 管理未挂 `agent:*`） |
| JWT-only | `/im/feishu/configs/*` | ⚠️ 权限常量 `im:*` 已定义但路由未挂 |
| JWT-only | `/dashboard` `/dashboard/trends` | ⚠️ `stats:view` 常量存在但 `RequirePermission` 是**死代码**（见 3.2） |
| RBAC | `/chat/*` `/agent/*` `/sessions/*` `/tools/api/*` `/admin/*` | `/agent/*` 疑似遗留（见 3.3） |

### 3.2 dashboard 死代码

`routes.go` 中 `dashRoutes` group 挂了 `RequirePermission(stats:view)`，但 `RegisterDashboardRoutes(router, ...)` 传入的是全局 `router`（而非 `dashRoutes`），实际只走 JWT。需二选一：删除死代码、或真正挂上 `stats:view`（本 spec 选择后者）。

### 3.3 已确认删除的遗留 API

> 晓军拍板确认（2026-09-04）：确认前端无调用 + 确认 tool 是后端程序内调用后，直接删除。

- **`/api/v1/agent/*`**（`/agent/tasks` `/agent/tasks/:id` `/agent/skills` `/agent/skills/search`）：✅ **前端已确认无调用**——前端 agent 页面全部走 `apiFetch('/tasks')` / `/tasks/:id` / `/tasks/:id/run` / `/tasks/:id/cancel`（TaskHandler），`router.push('/agent/tasks/...')` 是 Next.js 页面路由跳转，非 API 调用。AgentHandler 这 4 个 HTTP 端点无任何前端调用 → **删除**。
- **`/api/v1/tools/api/*`**（`/tools/api/search|summary|method|call`）：✅ **tool 已确认是后端程序内调用**——agent 的 `external_api_*` 走 ADK functiontool（`internal/adk/tools/tools.go`）直调 `apicollectionsvc.Service`（`SearchApproved/GetAPISummary/GetAPIMethod/CallAPI`），不经 HTTP。HTTP 层的 `APIToolsHandler`（`api_tools.go`）+ `registerAPIToolsRoutes` 是冗余的 → **删除 HTTP 路由**（保留 `tools.go` 的 functiontool，不动）。
- **前端坏调用**：`/vault/decrypt`（`admin/models/page.tsx`）指向不存在后端路由；`/change-password`（SPEC-083 已改 `/auth/change-password`）。

## 4. 权限全景与目标

### 4.1 角色级别约定

| 级别 | RBAC 角色 | 说明 |
|------|----------|------|
| 普通用户 | `user_role` | 基本查看/使用 |
| 普通管理员 | `admin_role` | 管理用户/审计/系统配置等 |
| 系统管理员 | `system_admin_role` | 全部权限（继承 admin） |

### 4.2 权限白名单（规则 5，仅 JWT 无需 RBAC）

| 路由 | 说明 |
|------|------|
| `POST /auth/refresh` | token 刷新 |
| `GET /auth/profile` | 查看 me 信息 |
| `GET /rbac/me/permissions` | 查看 me 权限 |
| `POST /auth/change-password` | 修改自己密码（SPEC-083） |
| `POST /auth/login` | 登录（无 auth） |
| `GET /auth/register?token` + `POST /auth/complete-registration` | 被邀请注册（无 auth，临时 invite token） |
| `GET /health` `/api/v1/health` | 健康检查（无 auth） |
| `POST /im/feishu/webhook` | 飞书回调（无 auth，规则 1 明确除外） |

### 4.3 RBAC 映射（目标态）

| 路由 | 权限 | 级别 |
|------|------|:---:|
| `GET /users` | `user:view` | admin |
| `POST /users` / `PUT /users/:id` / `PATCH /users/:id/status` | `user:create` / `user:edit` | admin |
| `DELETE /users/:id` | `user:delete` | admin |
| `POST /notifications`（定向） | `notification:send`（**新增**） | user |
| `POST /notifications/broadcast`（广播） | `notification:broadcast`（**新增**） | admin |
| `GET /knowledge/docs` `/docs/:id` `/search` | `kb:view` | user |
| `POST /knowledge/docs` | `kb:upload` | user |
| `PUT /knowledge/docs/:id/public` | `kb:upload`（发布） | user |
| `DELETE /knowledge/docs/:id` | `kb:delete` | user |
| `GET /artifacts/*` `/artifacts/:id/download*` | `artifact:view` | user |
| `POST /artifacts/upload` | `artifact:view`（上传=使用） | user |
| `DELETE /artifacts/:id` | `artifact:delete` | user |
| `GET /tasks` `/tasks/:id` `/tasks/:id/runs*` `/runs/:id` `/tasks/:id/artifacts/download` | `agent:view` | user |
| `POST /tasks` | `agent:create` | user |
| `POST /tasks/:id/run` | `agent:edit` | user |
| `PUT /tasks/:id/cancel` | `agent:edit` | user |
| `DELETE /tasks/:id` | `agent:delete` | user |
| `GET /dashboard` `/dashboard/trends` | `stats:view` | user |
| `GET/POST/PUT/DELETE /im/feishu/configs/*` | `im:view` / `im:edit` / `im:delete` | user |
| `POST /chat` + `/chat/enhance` | `chat:view`（已有） | user |

> 通知的读取侧（`GET /notifications` `/unread-count` `/read` `/read-all`）是「查自己的通知」，纳入白名单，**不加 RBAC**（人人可查自己的通知）。

> KB 的写侧归属校验（`GET /knowledge/docs/:id` / `DELETE /knowledge/docs/:id` / `PUT /knowledge/docs/:id/public` 按 owner + is_public + system_admin 豁免校验，IDOR 防护）见 **§6.7**，与 SPEC-091 §3 数据隔离语义一致。

## 5. API 设计

### 5.1 删除清单（规则 1 / 2）

| 路由/方法 | 理由 |
|-----------|------|
| `POST /api/v1/auth/register` | 主动注册已废弃（邀请制） |
| `GET /api/v1/system/stats` | 无前端调用 + 信息泄露；`/health` 已取代 |
| `GET/PUT /api/v1/workspace/:session_id/files*` | 前端无调用；agent 走 `fsops` 本地目录 |
| `PUT /api/v1/tasks/:task_id/pause` | SPEC-082 已决定删除（太复杂、无需求） |
| `PUT /api/v1/tasks/:task_id/resume` | SPEC-082 已决定删除 |
| `GET/POST /api/v1/agent/tasks*` `/agent/skills*` | 遗留，前端已改用 `/tasks`（已确认无前端调用，删除） |
| `GET/POST /api/v1/tools/api/*` | 遗留，agent 走 ADK functiontool 直调 service（已确认程序内调用，删除 HTTP 路由） |

前端坏调用清理：

| 文件 | 清理 |
|------|------|
| `frontend/app/admin/models/page.tsx` | 删除 `/vault/decrypt` 调用（后端无此路由，mask 由视觉层做） |
| `frontend/app/change-password/page.tsx` | 路径修正 `/change-password` → `/auth/change-password`（SPEC-083） |

### 5.2 新增权限

| 权限 Key | 常量名 | 名称 | 模块 | 默认角色 |
|----------|--------|------|------|:--------:|
| `notification:send` | `PermNotificationSend` | 定向发送通知 | notification | user_role |
| `notification:broadcast` | `PermNotificationBroadcast` | 广播通知 | notification | admin_role |

### 5.3 权限变更同步铁律（三处同步，防重新部署遗漏）

> ⚠️ **任何 RBAC 权限变更（新增/改名/删除/调整默认角色）必须三处同步，缺一不可**，否则全新部署（空库）会与存量库产生权限漂移：

| 序号 | 同步位置 | 内容 |
|:---:|---------|------|
| 1 | `internal/domain/model/rbac.go` | 权限常量定义（`PermXxx` + key 字符串） |
| 2 | `cmd/server/migration/rbac_seed.go` `perms` 数组 | **原始 seed 数据源**（全新部署空库时唯一的数据来源） |
| 3 | 路由层 `RequirePermission` + 前端 `api.ts` 权限常量 | 实际挂载与 UI 可见性 |

- **`perms` 数组是 single source of truth**：每次新增权限，若只改 `rbac.go` 常量、漏了 seed 数组，全新部署（`docker compose down -v` + up）后该权限**不会存在**，路由挂的 `RequirePermission` 会因权限不存在而对所有用户返回 403，造成功能不可用。
- 本次新增 `notification:send` / `notification:broadcast` 时，必须同步写入 `perms` 数组（含对应 roleIDs），保证 seed 完整。
- **本次同时有「调整默认角色」变更**：`agent:view/create/edit/delete` 与 `sidebar:agent` 的 seed `roleIDs` 从 `admin` 改为 `user`（task 权限降级，见 §6.6），也必须同步改 `perms` 数组，否则全新部署后这些权限仍是 admin 级、与存量库漂移。
- 后续任何 SPEC 若再动 RBAC 权限，**必须在 spec 文档中显式标注「三处同步」**，并列入验证标准。

## 6. 详细设计

### 6.1 用户管理数据隔离（规则 3）

- 权限：`/users/*` 挂 `user:view/create/edit/delete`（均 admin 级，seed 已配）。
- **数据隔离**：`List` 按当前角色过滤——
  - `system_admin`：返回全部用户；
  - `admin`：仅返回 `role == "user"` 的普通用户（**看不到其他 admin / system_admin**）。
- 现有 handler 内 `denyAdminManagingAdmin`（admin 只能管 user 角色）保留，作为写操作的第二道防线。
- `UserHandler.List` 增加 `c.GetString("role")` 读取，透传 role 过滤给 `service.List`。

### 6.2 通知权限 + 前端 UI 区分（规则 4）

- 后端：
  - `POST /notifications`（定向 `SendNotification`，带 `target_ids`）→ `RequirePermission(notification:send)`（user 级）。
  - `POST /notifications/broadcast`（`BroadcastNotification`）→ `RequirePermission(notification:broadcast)`（admin 级）。
  - 读取侧（`GET /notifications` 等）保持 JWT-only（查自己的通知）。
- 前端：
  - 通知发送 UI 根据 `canAccess('notification:send')` 显示「定向发送」，根据 `canAccess('notification:broadcast')` 显示「广播」入口。
  - `SIDEBAR_PERMS` / 权限常量同步新增 `notification:send` / `notification:broadcast`。

### 6.3 原始 seed 同步 + 增量补齐（关键）

> **前提**：新增权限的原始 seed 数据（`perms` 数组）必须按 §5.3 铁律同步写入（全新部署数据源完整）。
>
> 现有 `seedPermissions` 是「全有或全无」——`CountDocuments > 0` 即 `return`，**存量库升级不会补新增权限**。本 spec 新增 2 个权限，必须升级为**逐权限增量补齐**（参考 `SeedSkills` 的 `existMap + Upsert` 模式）：

```go
// seedPermissions 升级：
// 1. 查出现有权限 key 集合（existingKeys map[string]bool）
// 2. 遍历 perms 数组，缺失的 key → InsertOne 权限文档 + 逐 roleID 插入 RBACRolePermission
// 3. 同步更新各角色 permission_count（对新增的角色-权限关联累加）
```

- 全新部署（空库）：自然插入全部权限（含 2 个新通知权限）。
- 存量库升级：缺失的 `notification:send` / `notification:broadcast` 被增量补入并关联到 `user_role` / `admin_role`，无需重建库。

### 6.4 dashboard 死代码修复（规则 10）

- `RegisterDashboardRoutes` 改为挂 `RequirePermission(stats:view)`（user 级，seed 已配 `rbac_perm_stats_view`）。
- 删除 `dashRoutes` 死代码 group，统一在 `RegisterDashboardRoutes` 内挂 auth + RBAC。

### 6.5 权限常量与路由落位

- `internal/domain/model/rbac.go`：新增 `PermNotificationSend` / `PermNotificationBroadcast` 常量。
- `internal/api/handler/routes.go` 各 `register*Routes`：按 §4.3 映射表补 `RequirePermission`。
- `internal/api/handler/notification.go`：`SendNotification` / `BroadcastNotification` 两个 handler 不动（权限在路由层挂）。

### 6.6 task 权限降级 + 归属校验（IDOR 防护，关键）

> 晓军拍板：task 权限改成**普通用户级别**（`agent:*` 从 admin 降到 user）。因 RBAC 有祖先继承（`GetAllRoleIDsWithAncestors`），降到 user 后 admin / system_admin 自动继承，**无权限回退风险**。

**副作用排查结论**：

| 检查项 | 结论 |
|--------|------|
| `agent:*` 权限 seed 降级 | `agent:view/create/edit/delete` 的 `roleIDs` 从 `admin` → `user`（§5.3 三处同步） |
| `sidebar:agent` 菜单权限 | **精确同步**从 `admin` → `user`（权限常量 + seed roleIDs + 前端 `SIDEBAR_PERMS` 三处对齐），否则普通用户有 `agent:*` 却看不到 Agent 侧边栏入口 |
| 数据隔离 | task 是**个人资源**（`Task.UserID` / `Run.UserID` 存在，`ListTasks`/`CreateTask` 已按 `user_id` 过滤）。**必须做隔离**，参考 `memory.go` / `knowledge.go` 范式：`isSystemAdmin := role == "system_admin"`，非 system_admin 强制只看自己的数据（task 无共享） |
| ⚠️ **IDOR 越权隐患** | `GetTask(id)` / `ListRuns(taskID)` / `GetRun(id)` / `CreateRun(taskID)` / `CancelTask(id)` / `DownloadArtifacts(taskID)` **按 ID 操作、不校验归属**。降级前是 admin 级（管理员看全部无越权问题）；降级后普通用户可凭 task_id/run_id 操作他人 task → **必须补归属校验** |
| `/admin/tasks/:id/scheduled-enabled` | 挂 `agent:edit`，降级后普通用户可切换「自己的」定时任务开关（个人资源，合理） |
| LLM 资源消耗 | 暂不考虑（晓军拍板） |

**归属校验方案（参考 `memory.go` / `knowledge.go` 数据隔离范式）**：

- 判定依据：`role := c.GetString("role")`（JWT 注入的 **user 角色属性** `system_admin` / `admin` / `user`，非 RBAC 角色），`isSystemAdmin := role == "system_admin"`。
- 隔离规则（user/admin 只看自己的；system_admin 看全部）：
  - `role != "system_admin"` → 强制校验 `task.UserID == userID`（`run.UserID == userID`），不一致返回 `ErrNotFound`（不泄露资源存在性）；
  - `system_admin` → 豁免，可访问全部 task/run。
- 落地：`GetTask/ListRuns/GetRun/CreateRun/CancelTask/DownloadArtifacts` 增加 `userID` + `isSystemAdmin` 参数，service 层校验归属，handler 层从 `c.GetString("user_id")` / `c.GetString("role")` 透传。`ListTasks` 同样按此范式（已按 user_id 过滤，补 `isSystemAdmin` 后 system_admin 可看全部）。

### 6.7 KB 写侧归属校验 + is_public 可操作语义（IDOR 防护）

> **数据隔离语义以 SPEC-091 §3 为 SSOT**。`is_public` 是**权限**（访问 + 操作），两级语义；本 spec 落地其**写侧**（单点读取 / 删除 / 改 shared）的归属校验，读侧（列表 / 检索 / 图谱）已由 `ListDocsByVisibility` / `Search` / `QueryTopN` 实现。两处语义**完全一致**——无论 SPEC-084 与 SPEC-091 谁先实现，隔离规则不变、互不依赖、无冲突（SPEC-091 只改 `SetPublicFlag` 内部副作用顺序 + 图同步 `SetDocPublic`，不碰归属校验；本 spec 只加归属校验，不碰图同步）。

**隔离规则（访问 + 操作，两级）**：

| 角色 | 权限范围（访问 + 操作） | 豁免 |
|------|------|:---:|
| 系统管理员 `system_admin` | 全部 doc | ✅ |
| 普通管理员 `admin` / 普通用户 `user` | 自己的（`doc.UserID == userID`）+ 公开（`doc.IsPublic == true`） | ❌ |
| 他人的私有 doc（`UserID != userID && IsPublic == false`） | 无权限（访问 + 操作均拒绝） | ❌ |

**现状缺口**：`GetDoc` / `DeleteDoc` / `SetPublicFlag` 均只按 `docID` 操作、不校验归属（仅 JWT），任何登录用户可读/删/改任意 doc（IDOR 隐患）。

**落地（handler 透传 → service 校验，参考 §6.6 task 范式）**：

| 端点 | 操作 | 校验规则（非 system_admin） |
|------|------|------|
| `GET /knowledge/docs/:id` | 读单点 | 须 `doc.UserID == userID` 或 `doc.IsPublic == true`，否则 `ErrNotFound`（不泄露存在性） |
| `DELETE /knowledge/docs/:id` | 删 | 同上（owner 或 public，否则 `ErrNotFound`） |
| `PUT /knowledge/docs/:id/public` | 改 shared | 同上（owner 或 public，否则 `ErrNotFound`） |

- 判定依据：`role := c.GetString("role")`，`isSystemAdmin := role == "system_admin"`；`userID := c.GetString("user_id")`。
- 变更签名：`GetDoc` / `DeleteDoc` / `SetPublicFlag` 增加 `userID string, isSystemAdmin bool` 参数；service 内先 `GetDoc` 取 `UserID` + `IsPublic`，再按上表判定；handler 从 gin context 透传。
- `UploadDoc` / `CreateDoc` 无需改（新建 doc 天然 `UserID = 当前用户`）。

> **is_public 可操作语义的边界（两处 spec 必须同步）**：`is_public=true` 意味着「所有人都有权限」——含**删除**与**改 shared**，即任何登录用户可删除/改 shared 他人的公开 doc（这是「公开 = 有权限」的直接推论，与读侧「公开可见」对称）。若后续改为「公开 doc 仅授予读权、写权仍归 owner + system_admin」，则必须在 SPEC-091 §3 与本 spec 同步调整，**两处语义保持一致，不能一处「公开可操作」另一处「公开只读」**。

## 7. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 rbac_permissions / rbac_role_permissions） |
| 是否影响现有 API | Yes（删除 7 组废弃路由含 `/agent/*` `/tools/api/*` + 为约 20 个端点补 RBAC + task 权限降级为 user 级；前端同步清理坏调用 + 通知 UI） |
| 是否需要新增 RBAC 权限 | Yes（`notification:send` + `notification:broadcast`，含 seed 增量补齐） |
| 是否需要调整现有权限级别 | Yes（`agent:*` + `sidebar:agent` 从 admin 降为 user，同步 seed `roleIDs`） |
| 性能影响 | 忽略（RBAC 权限为内存查询；seed 启动时一次 O(n) 增量补齐） |
| 是否需要新增 Skill | No |
| 是否修改其他功能 | 边界内（删废弃 + 补权限 + task 降级 + task/KB IDOR 归属校验 + 通知 UI 区分） |

## 8. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/model/rbac.go` | 新增 2 个通知权限常量 | 极小 |
| `internal/api/handler/routes.go` | 删废弃路由 + 各端点补 `RequirePermission` | 大 |
| `internal/api/handler/auth.go` | 删 `Register`（主动注册） | 小 |
| `internal/api/handler/task.go` | 删 `PauseTask`/`ResumeTask` + 按 ID 操作补 `userID` 归属校验（IDOR 防护） | 中 |
| `internal/api/handler/knowledge.go` | `GetDoc`/`DeleteDoc`/`SetPublicFlag` 补 `userID` + `isSystemAdmin` 归属校验透传（§6.7） | 小 |
| `internal/service/knowledge/service.go` | `GetDoc`/`DeleteDoc`/`SetPublicFlag` 增加归属校验参数 + 判定逻辑 | 中 |
| `internal/api/handler/user.go` | List 加 role 过滤（数据隔离） | 中 |
| `internal/api/handler/dashboard.go` | `RegisterDashboardRoutes` 挂 `stats:view` | 小 |
| `internal/api/handler/agent.go` | 删 `/agent/*` 路由（已确认无前端调用） | 小 |
| `internal/api/handler/api_tools.go` | 删 `/tools/api/*` 路由（已确认程序内调用，保留 functiontool） | 小 |
| `internal/api/handler/notification.go` | 无改动（权限在路由层） | — |
| `cmd/server/migration/rbac_seed.go` | 新增 2 权限 + `agent:*`/`sidebar:agent` roleIDs 降级 + `seedPermissions` 增量补齐 | 中 |
| `cmd/server/migration/rbac_seed_test.go` | seed 增量补齐 + roleIDs 降级 + 一致性校验单测 | 中 |
| `internal/service/user/*` | List 支持 role 过滤参数 | 中 |
| `internal/service/task/*` | 按 ID 操作增加 `userID` 归属校验参数 | 中 |
| `internal/service/auth/*` | 删 Register 相关（确认无引用后） | 小 |
| 各 `*_test.go` / `routes_error_test.go` | 路由断言更新 + 权限测试 | 中 |
| `frontend/lib/api.ts` | 权限常量 + `canAccess` 通知权限 | 小 |
| `frontend/app/admin/models/page.tsx` | 删 `/vault/decrypt` 坏调用 | 极小 |
| `frontend/app/change-password/page.tsx` | 路径修正（SPEC-083） | 极小 |
| 通知发送 UI 组件 | 区分定向/广播 | 中 |

## 9. 测试策略

1. **Unit tests**（Go）：
   - `rbac_seed.go`：seed 增量补齐——① 空库全量插入（含 2 新权限）；② 存量库仅补齐缺失、不重复插入；③ role_permissions 关联 + permission_count 正确；④ 幂等（二次运行零新增）。
   - **seed 一致性校验**（§5.3 铁律）：断言 `rbac.go` 中所有 `PermXxx` 权限常量在 `perms` 数组中**一一对应**（无「常量定义但未 seed」的遗漏项，也无不存在的常量），保证重新部署不丢权限。
   - `user/service`：`List` 角色过滤（admin 只见 user / system_admin 见全部）。
   - `task/service`：**归属校验（IDOR 防护）**——`GetTask/ListRuns/GetRun/CreateRun/CancelTask/DownloadArtifacts` 传入非 owner `userID` 且非 system_admin 时返回 `ErrNotFound`（不泄露资源存在性）；owner 正常访问；`system_admin` 豁免可访问全部。
   - `knowledge/service`：**KB 归属校验（§6.7，与 SPEC-091 §3 语义一致）**——`GetDoc`/`DeleteDoc`/`SetPublicFlag` 传入非 owner 且 `is_public=false` 时返回 `ErrNotFound`；owner 可操作；`is_public=true` 的 doc 他人可读/删/改 shared；`system_admin` 豁免可操作全部。
   - `routes`：各端点挂权限后，无权限返回 403、有权限放行；`agent:*` 降为 user 后普通用户可访问 `/tasks/*`，无权限（未登录）返回 401。
   - 删除接口：`/auth/register`(POST)、`/system/stats`、`/workspace/*`、`/tasks/:id/pause|resume`、`/agent/tasks*`、`/agent/skills*`、`/tools/api/*` 返回 404。
   - 覆盖率底线见 SPEC-045：L1 100% / L3 98%。
2. **E2E tests**（前端）：
   - `tests/ui/`：通知发送 UI 按权限区分定向/广播；无 `notification:broadcast` 的用户看不到广播入口。
   - 用户管理数据隔离：admin 登录后列表只显示普通用户。
   - task 归属隔离：普通用户 A 无法通过 task_id/run_id 访问用户 B 的 task（前端不展示、后端 404）。
   - KB 归属隔离：普通用户 A 无法删除/改 shared 用户 B 的私有 doc（后端 404）；B 的公开 doc 可被 A 读/删/改 shared；system_admin 可操作全部。

## 10. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 通知 UI 权限区分 + 用户管理数据隔离 + task 归属隔离时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 11. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L2 | 依赖接口，可 mock | **100%** | `service/user` List 过滤 |
| L3 | 依赖 MongoDB | **98%** | `migration/rbac_seed.go`（增量补齐） |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** seed 增量补齐 Success 测试验证：插入条数、缺失补齐、role_permissions 关联、幂等（二次运行零新增）
- [ ] **必须** 数据隔离测试验证：admin 查询返回结果中不含 admin/system_admin 角色用户
- [ ] **必须** task 归属校验测试验证：非 owner 访问他人 task/run 返回 `ErrNotFound`（不返回资源内容、不泄露存在性）；system_admin 豁免可访问全部
- [ ] **必须** KB 归属校验测试验证（§6.7）：非 owner 且 `is_public=false` 访问/删/改 shared 返回 `ErrNotFound`；`is_public=true` 的 doc 他人可操作；system_admin 豁免可操作全部
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

## 12. 验证标准

1. `POST /auth/register` 返回 404（已删）；`GET /auth/register?token` + `POST /auth/complete-registration` 正常（临时 token，无 RBAC）。
2. `GET /api/v1/system/stats`、`/workspace/*`、`/tasks/:id/pause|resume`、`/agent/tasks*`、`/agent/skills*`、`/tools/api/*` 返回 404。
3. 普通用户调用 `/users/*` 返回 403（无 `user:*` 权限）；admin 可访问但列表仅含普通用户（数据隔离）；system_admin 见全部。
4. 普通用户可定向发通知（`notification:send`），无广播权限则 `/notifications/broadcast` 返回 403；admin 可广播。
5. `/knowledge/*`、`/artifacts/*`、`/im/feishu/configs/*` 普通用户可访问（`kb:*`/`artifact:*`/`im:*`），无权限返回 403。
5b. **KB 写侧归属校验（§6.7，与 SPEC-091 §3 一致）**：普通用户 A 删除/改 shared 用户 B 的**私有** doc 返回 404（不泄露存在性）；A 可读/删/改 shared 用户 B 的**公开** doc；`system_admin` 可操作全部 doc（豁免）。
6. `/tasks/*` 普通用户**可访问**（`agent:*` 已降为 user 级），可创建/查看/运行/取消**自己的** task；未登录返回 401；访问他人 task/run 返回 404（IDOR 归属校验）；**system_admin 可访问全部 task/run**（豁免）。
7. `/dashboard` `/dashboard/trends` 普通用户可访问（`stats:view`），未登录/无权限返回 401/403。
8. 白名单（refresh/profile/me permissions/change-password）仅 JWT 可访问，不因角色变化受限。
9. 存量库升级后 `rbac_permissions` 新增 2 个通知权限，`rbac_role_permissions` 关联到 user_role / admin_role；`agent:*`/`sidebar:agent` 的 role 关联从 admin_role 迁到 user_role；全新部署空库 seed 完整插入。
10. **全新部署（空库）不遗漏**：`rbac.go` 权限常量与 `rbac_seed.go` `perms` 数组一致性校验通过，空库 seed 后所有路由挂载的权限均存在、无 403 漂移。
11. CI（sonar-check + ui-tests + ut-workflow）全绿。

## 13. 收尾修正（2026-09-06：邀请注册权限隔离补强）

实现后复盘中确认邀请注册存在权限/隔离缺口，一并修正：

| # | 项 | 修正前 | 修正后 |
|---|----|--------|--------|
| 1 | 邀请 system_admin | handler + service 双层禁止 | 维持「任何人都不能邀请 system_admin」；角色校验统一收敛到 handler 层（删除 service 层重复校验） |
| 2 | RevokeInvite 归属校验 | 无归属校验（IDOR）：admin 可撤任意 invite | 非 system_admin 只能撤自己创建的 invite，否则 `ErrInviteNotFound`（不泄露存在性）；system_admin 豁免 |
| 3 | invite 注册自动绑 RBAC 角色 | 不自动绑定 | **明确决策：不自动绑定**，新用户由管理员显式赋 `rbac_role_*` |
| 4 | 前端角色下拉 | users 创建/编辑下拉含 system_admin（后端 403） | 移除 system_admin 选项；invites 页面本已正确（admin 选项仅 system_admin 可见） |

### 13.1 关键决策

- **角色校验单一职责**：role 白名单/限制（禁 system_admin、admin 只能邀 user）统一由 `AuthHandler.CreateInvite` 负责，`service.CreateInvite` 只做默认值 + 业务落库。
- **RevokeInvite 归属**：与 `ListInvites` 的隔离语义对齐（admin 只见/只能撤自己的，system_admin 见/撤全部）。
- **invite 注册不自动绑 RBAC 角色**：保持现状，RBAC 角色由管理员显式分配（`POST /admin/users/:id/rbac-roles`）。

### 13.2 变更文件

- `internal/service/auth/invite.go`：删 service 层 role 校验；`RevokeInvite` 加归属校验 + `ErrInviteNotFound`
- `internal/service/auth/interface.go`：`RevokeInvite` 签名加 `actorUserID`/`actorIsSystemAdmin`
- `internal/api/handler/auth.go`：`RevokeInvite` 传归属参数 + 404 映射
- `internal/service/auth/mocks/AuthService.go`：同步 mock
- `frontend/app/admin/users/page.tsx`：创建/编辑弹窗移除 system_admin 选项
- 测试：`TestRevokeInvite_*`（owner / system_admin 豁免 / 非 owner 403 / not found / no repo）、`TestCreateInvite_SystemAdminBlocked`（handler 层 403 + 不调 service）
