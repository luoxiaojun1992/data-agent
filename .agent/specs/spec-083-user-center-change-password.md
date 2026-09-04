# 用户中心与修改密码

> **SPEC-083** | Status: 设计中

## 1. 目标

在侧边栏新增「用户中心」菜单入口，跳转用户中心页面，页内提供「修改密码」卡片，点击弹出修改密码弹窗。将改密接口从 `admin` 专用路径迁移到用户自身分组（`/api/v1/auth/change-password`），并明确「每个用户只能改自己的密码」（通过 JWT token 获取 user_id，不接收外部目标用户）。为「用户中心」新增对应 RBAC 权限（默认分配给普通用户级别），并同步原始 seed（保证存量库升级与全新部署默认数据均不丢失新权限）。UI 样式遵循 SPEC-078（弹窗玻璃样式）与 SPEC-076（主题变量）。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-064 RBAC 角色权限管理系统 | ✅ | 权限枚举 `domain/model/rbac.go` + seed `rbac_seed.go` + `RequirePermission` middleware 已就绪 |
| SPEC-078 前端列表页 UI 规范统一 | ✅ | 弹窗玻璃样式 + 主按钮渐变规范 |
| SPEC-076 前端主题切换 | ✅ | CSS 变量主题（本 spec 复用变量，不依赖其落地） |
| SPEC-003 基础设施与认证授权 | ✅ | JWT AuthMiddleware 已就绪；改密逻辑现挂 `ConfigHandler`，本 spec 迁到 auth 域 |

> 无阻塞项，可立即开始。

## 2. 背景（现状与动机）

**改密能力代码已存在，但两处缺陷导致不可用**：

1. **后端**：`ConfigHandler.ChangePassword` 已实现改密逻辑（旧密码校验 + 复杂度 + bcrypt 更新），但挂在 `POST /api/v1/admin/change-password` —— **admin 专用路径**，语义错误（改自己密码不是管理操作），且与用户自身分组（`/api/v1/auth/*`）割裂。
2. **前端**：`/change-password` 页面已存在，但其 `apiFetch('/change-password')` 实际请求 `/api/v1/change-password`，与后端 `/api/v1/admin/change-password` **路径不匹配**，且侧边栏无入口、登录后无跳转 —— 实为不可用的孤儿页面。

**本次改动**：① 改密接口迁到 `/api/v1/auth/change-password`（用户自身分组），② 明确「只能改自己」（token 取 user_id），③ 侧边栏加「用户中心」入口 + 弹窗化交互，④ 纳入 RBAC 权限 + seed。

## 3. 权限设计

### 3.1 新增权限

| 权限 Key | 常量名 | 名称 | 模块 | 默认角色 |
|----------|--------|------|------|:--------:|
| `sidebar:profile` | `PermSidebarProfile` | 用户中心菜单 | sidebar | user_role（普通用户） |

- 仅新增 **1 个** 权限（用户中心菜单可见性），遵循现有 `sidebar:*` 命名规范。
- **不新增**「改密操作」独立权限：改密是「修改自己密码」，人人可做，无需角色区分。改密接口 `POST /api/v1/auth/change-password` 只做 JWT 认证（**无 RBAC middleware**），因为「改自己密码」是每个登录用户的基本能力，不随角色变化。
- admin/system_admin 通过 RBAC 角色层级（父角色拥有子角色权限）自动继承 user_role 的 `sidebar:profile`。

### 3.2 安全语义：只能改自己的密码

- 改密接口**不接收**请求体中的 `user_id` / `username` / `target` 等目标字段，**只从 JWT token 的 `user_id` claim**（`c.Get("user_id")`）确定操作对象。
- JWT 校验已有（`JWTManager.AuthMiddleware()`），本 spec 复用之。保证 A 用户无法通过改密接口修改 B 用户的密码。

### 3.3 seed 同步（关键：增量补齐）

现有 `seedPermissions` 采用「全有或全无」逻辑——`CountDocuments > 0` 即 `return`，**导致存量库升级时新增权限永远不会补齐**。本 spec 升级为**逐权限增量补齐**（参考 `SeedSkills` 的 `existMap + Upsert` 模式）：

```go
// seedPermissions 升级：
// 1. 查出现有权限 key 集合（existingKeys map[string]bool）
// 2. 遍历 perms 数组，缺失的 key → InsertOne 权限文档 + 逐 roleID 插入 RBACRolePermission
// 3. 同步更新各角色 permission_count（对新增的角色-权限关联累加）
```

- 全新部署（空库）：逻辑自然插入全部权限（含新增的 `sidebar:profile`）。
- 存量库升级：缺失的 `sidebar:profile` 被增量补入，并关联到 `rbac_role_user`，用户通过已有 `user_rbac_roles` 关联立即生效，无需重建库。
- `seedRoles` / `seedDefaultUserRole` 保持不变（本 spec 不加角色、不改默认用户绑定）。

## 4. API 设计

### 4.1 改密接口（迁移 + 复用）

| Method | Path | Description | 变更 |
|--------|------|-------------|------|
| POST | `/api/v1/auth/change-password` | 修改当前登录用户密码 | **迁移**：由 `/api/v1/admin/change-password` 迁来 |
| GET | `/api/v1/rbac/me/permissions` | 返回当前用户权限列表 | 无（自动包含新增 `sidebar:profile`） |

请求/响应（逻辑不变，仅路径迁移）：

```json
// 请求（只含旧/新密码，不含目标用户）
{ "old_password": "OldPass1", "new_password": "NewPass1" }
// 响应（成功）
{ "message": "密码修改成功" }
// 响应（失败）
{ "error": "旧密码不正确" | "密码至少 8 位，需包含大小写字母和数字" | "用户不存在" | ... }
```

### 4.2 迁移后遗留

- 旧路径 `POST /api/v1/admin/change-password` **删除**（迁到 `/api/v1/auth/change-password`），不保留兼容别名。
- 前端 `apiFetch` 调用路径同步改为 `/auth/change-password`（修复原路径不匹配缺陷）。

## 5. 详细设计

### 5.1 后端

**`internal/domain/model/rbac.go`**：Sidebar 权限常量区新增：

```go
PermSidebarProfile = "sidebar:profile"
```

**`internal/service/auth/interface.go`**：`AuthService` 接口新增：

```go
ChangePassword(ctx context.Context, userID, oldPassword, newPassword string) error
```

**`internal/service/auth/service.go`**：`Service` 实现 `ChangePassword`——复用已有 `pwd.Check`（旧密码校验）、`pwd.Hash`（新密码 bcrypt）、`userRepo.FindByID` / `userRepo.UpdatePassword`，并内置密码复杂度校验（原 `validatePasswordComplexity` 逻辑迁入 service，便于单测）：

```go
func (s *Service) ChangePassword(ctx, userID, oldPassword, newPassword string) error {
    // 1. 复杂度校验（≥8 位 + 大小写字母 + 数字）
    // 2. FindByID(userID) → 不存在返回 ErrUserNotFound
    // 3. pwd.Check(user.PasswordHash, oldPassword) → 失败返回 ErrWrongOldPassword
    // 4. pwd.Hash(newPassword) → UpdatePassword(userID, newHash)
}
```

**`internal/service/auth/mocks/AuthService.go`**：`mockery` 重新生成（新增 `ChangePassword` mock）。

**`internal/api/handler/auth.go`**：`AuthHandler` 新增 `ChangePassword` 方法：

```go
// POST /api/v1/auth/change-password
func (h *AuthHandler) ChangePassword(c *gin.Context) {
    userID, _ := c.Get("user_id") // 只从 token 取，不接收请求体目标字段
    // bind JSON { old_password, new_password }
    // 调 h.authService.ChangePassword(ctx, userID.(string), old, new)
    // 按错误类型返回 400/404/500
}
```

**`internal/api/handler/config.go`**：删除 `ChangePassword` 方法 + `validatePasswordComplexity` + `ConfigHandler.userRepo` 字段 + `NewConfigHandler` 的 userRepo 参数 + `RegisterSysConfigRoutes` 中的 `admin.POST("/change-password", ...)`。

**`internal/api/handler/routes.go`**：`registerAuthProtected` 新增：

```go
api.POST("/auth/change-password", authHandler.ChangePassword)
```

**`cmd/server/wire.go`（依赖注入）**：`NewConfigHandler(cfgSvc)` 移除 userRepo 实参。

**`cmd/server/migration/rbac_seed.go`**：
1. `perms` 数组新增：`{perm: RBACPerm("rbac_perm_sidebar_profile", model.PermSidebarProfile, "用户中心菜单", "sidebar"), roleIDs: []string{user}}`
2. `seedPermissions` 从「全有或全无」升级为「逐权限增量补齐」（见 §3.3）。

### 5.2 前端

**`frontend/app/components/Sidebar.tsx`**：`navItems` 新增「用户中心」菜单项（置于「管理后台」之前）：

```tsx
{ perm: 'sidebar:profile', href: '/profile', label: '用户中心', icon: '👤', testid: 'nav-profile' },
```

**`frontend/lib/api.ts`**：`SIDEBAR_PERMS` 新增 `profile: 'sidebar:profile'`。

**`frontend/app/profile/page.tsx`**（新建，用户中心页面）：
- 复用 `AppLayout` 包裹，页面结构：用户信息卡片（头像 + 用户名 + 角色，`glass` 样式）+「修改密码」卡片（`glass` 样式，点击打开弹窗）。
- 标题 `data-testid="profile-page"`。

**`frontend/app/components/ChangePasswordModal.tsx`**（新建，改密弹窗）：
- 遵循 SPEC-078 弹窗玻璃样式：遮罩 `backdrop-blur` + 变量面板 `var(--glass-bg)` / `var(--border-glass)`。
- 表单字段：旧密码 / 新密码 / 确认新密码（复用现有 `/change-password` 页面校验：两次一致 + 复杂度 + 旧密码校验）。
- 提交 `apiFetch('/auth/change-password', { method: 'POST', ... })`（**路径修正**）。
- 成功后：提示「密码修改成功，请重新登录」→ 延时 `logout()` + `router.push('/login')`。
- 颜色全部走 CSS 变量，主按钮渐变 `linear-gradient(135deg, #5c7cfa, #7c3aed)`（SPEC-078），兼容 SPEC-076 主题。
- `data-testid`：`pwd-modal` / `pwd-modal-old-input` / `pwd-modal-new-input` / `pwd-modal-confirm-input` / `pwd-modal-submit-btn`。

**现有 `frontend/app/change-password/page.tsx`**：**保留不动**（不改、不删，避免「修改其他功能」；它承载 `need_change_pw` 初始密码 banner 场景，且可能被书签/直链访问），但其内部 `apiFetch('/change-password')` 路径需同步修正为 `/auth/change-password`（否则该页改密仍不可用）。

### 5.3 数据流

```
用户点击侧边栏「用户中心」 → /profile 页面
  └─ 点击「修改密码」卡片 → 打开 ChangePasswordModal
       └─ 填旧/新/确认密码 → POST /api/v1/auth/change-password
            ├─ JWT 校验（已实现）→ token 取 user_id
            ├─ authService.ChangePassword：复杂度校验 + 旧密码校验 + bcrypt 更新
            ├─ 成功 → 前端提示 → logout → 跳 /login
            └─ 失败 → 弹窗内展示 error
```

权限链路：登录时 `GET /api/v1/rbac/me/permissions` 已返回权限列表 → 前端 `canAccess('sidebar:profile')` 决定菜单可见性。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 rbac_permissions / rbac_role_permissions） |
| 是否影响现有 API | Yes（改密接口路径 `/api/v1/admin/*` → `/api/v1/auth/*`，旧路径删除；需同步前端 + 路由断言测试） |
| 性能影响 | 忽略（seed 启动时一次增量补齐，O(n) 遍历权限数组） |
| 是否需要新增 Skill | No |
| 是否修改其他功能 | No（现有 `/change-password` 页面仅修正调用路径，其余不动） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/model/rbac.go` | 新增 `PermSidebarProfile` 常量 | 极小（1 行） |
| `internal/service/auth/interface.go` | `AuthService` 接口新增 `ChangePassword` | 极小 |
| `internal/service/auth/service.go` | 实现 `ChangePassword` + 迁入复杂度校验 | 中 |
| `internal/service/auth/mocks/AuthService.go` | mockery 重新生成 | 自动 |
| `internal/api/handler/auth.go` | 新增 `ChangePassword` handler | 小 |
| `internal/api/handler/config.go` | 删除 `ChangePassword` + userRepo 依赖 | 中 |
| `internal/api/handler/routes.go` | `registerAuthProtected` 加改密路由 | 极小 |
| `cmd/server/wire.go` | `NewConfigHandler` 移除 userRepo 实参 | 极小 |
| `cmd/server/migration/rbac_seed.go` | 权限矩阵加新权限 + `seedPermissions` 增量补齐 | 中 |
| `cmd/server/migration/rbac_seed_test.go` | seed 增量补齐单测（追加） | 中 |
| `internal/api/handler/config_test.go` | 删除 ChangePassword 测试（迁至 auth） | 中 |
| `internal/api/handler/config_error_test.go` | 删除 ChangePassword 测试 + 更新路由断言 | 中 |
| `internal/api/handler/auth_test.go` / `auth_error_test.go` | 新增 ChangePassword 测试 | 中 |
| `internal/service/auth/auth_test.go` | 新增 `ChangePassword` service 单测 | 中 |
| `internal/api/handler/routes_error_test.go` | 更新 `/api/v1/admin/change-password` → `/api/v1/auth/change-password` | 极小 |
| `frontend/app/components/Sidebar.tsx` | navItems 加「用户中心」 | 极小 |
| `frontend/lib/api.ts` | SIDEBAR_PERMS 加 `profile` | 极小 |
| `frontend/app/profile/page.tsx` | 新建用户中心页面 | New |
| `frontend/app/components/ChangePasswordModal.tsx` | 新建改密弹窗组件 | New |
| `frontend/app/change-password/page.tsx` | 修正 `apiFetch` 路径 | 极小 |

## 8. 测试策略

1. **Unit tests**（Go）：
   - `auth/service.go`：`ChangePassword` 成功 / 复杂度不足 / 旧密码错误 / 用户不存在 / 哈希失败 / 更新失败 各分支。
   - `auth/handler`：`ChangePassword` 成功 + 各错误码路径，用 `gomonkey.ApplyMethodFunc` 验证 handler→service 参数（userID 来自 token）。
   - `rbac_seed.go`：seed 增量补齐——① 空库全量插入（含新权限）；② 存量库仅补齐缺失，不重复插入；③ role_permissions 关联 + permission_count 正确；④ 幂等（二次运行零新增）。
   - 覆盖率底线见 SPEC-045：L1 100% / L3 98%。
2. **E2E tests**（前端）：
   - `tests/ui/` 新增：UI-XXX「用户中心菜单可见 + 弹窗修改密码」——普通用户登录后侧边栏可见「用户中心」，进入 `/profile`，点卡片开弹窗，错误旧密码报错、正确旧密码改密成功后跳登录页。
   - 权限回归：无 `sidebar:profile` 权限的角色登录后菜单不可见（可选，若造角色成本高可并入 RBAC 现有用例）。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增用户中心菜单 + 弹窗改密交互时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性（`nav-profile` / `pwd-modal-*`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L2 | 依赖接口，可 mock | **100%** | `service/auth` ChangePassword |
| L3 | 依赖 MongoDB | **98%** | `migration/rbac_seed.go`（增量补齐） |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** `ChangePassword` 每个 Success 测试至少验证：调用 `UpdatePassword` 的参数（新 hash 非空、非明文）、handler 传参 userID 正确
- [ ] **必须** seed 增量补齐 Success 测试验证：插入条数、缺失补齐、role_permissions 关联、幂等（二次运行零新增）
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

## 10. 验证标准

1. 普通用户（user_role）登录后，侧边栏可见「用户中心」，点击进入 `/profile`。
2. `/profile` 显示用户信息卡片 + 「修改密码」卡片。
3. 点击卡片弹出玻璃样式弹窗，颜色随主题变量切换（深色/浅色一致）。
4. 旧密码错误 → 弹窗内报「旧密码不正确」；新密码复杂度不足 → 报复杂度提示；两次不一致 → 报「两次输入的密码不一致」。
5. 改密成功 → 提示成功后跳转登录页，旧密码登录失败、新密码登录成功。
6. **只能改自己**：请求体携带他人 `user_id` 字段不生效，接口始终只改 token 对应的当前用户密码。
7. 旧路径 `/api/v1/admin/change-password` 返回 404（已删除），新路径 `/api/v1/auth/change-password` 正常。
8. 存量库升级后，`rbac_permissions` 新增 `sidebar:profile`，`rbac_role_permissions` 关联到 `rbac_role_user`，普通用户权限列表含 `sidebar:profile`（无需重建库）。
9. 全新部署（空库），seed 完整插入含 `sidebar:profile` 的全部权限。
10. CI（sonar-check + ui-tests + ut-workflow）全绿。
