# 用户中心与修改密码

> **SPEC-083** | Status: 设计中

## 1. 目标

在侧边栏新增「用户中心」菜单入口，跳转用户中心页面，页内提供「修改密码」卡片，点击弹出修改密码弹窗。将改密接口从 `admin` 专用路径迁移到用户自身分组（`/api/v1/auth/change-password`），并明确「每个用户只能改自己的密码」（通过 JWT token 获取 user_id，不接收外部目标用户）。UI 样式遵循 SPEC-078（弹窗玻璃样式）与 SPEC-076（主题变量）。

> **权限语义（2026-09-04 确认）**：改密**不配 RBAC**——只要登录的用户都可以改自己的密码。用户中心菜单同样**不加 `sidebar:*` 权限**，对所有登录用户无条件可见。本 spec 因此**不新增任何 RBAC 权限、不改动 seed**。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-078 前端列表页 UI 规范统一 | ✅ | 弹窗玻璃样式 + 主按钮渐变规范 |
| SPEC-076 前端主题切换 | ✅ | CSS 变量主题（本 spec 复用变量，不依赖其落地） |
| SPEC-003 基础设施与认证授权 | ✅ | JWT AuthMiddleware 已就绪；token 已注入 `user_id` claim |

> 无阻塞项，可立即开始。

## 2. 背景（现状与动机）

**改密能力代码已存在，但两处缺陷导致不可用**：

1. **后端**：`ConfigHandler.ChangePassword` 已实现改密逻辑（旧密码校验 + 复杂度 + bcrypt 更新），但挂在 `POST /api/v1/admin/change-password` —— **admin 专用路径**，语义错误（改自己密码不是管理操作），且与用户自身分组（`/api/v1/auth/*`）割裂。
2. **前端**：`/change-password` 页面已存在，但其 `apiFetch('/change-password')` 实际请求 `/api/v1/change-password`，与后端 `/api/v1/admin/change-password` **路径不匹配**，且侧边栏无入口、登录后无跳转 —— 实为不可用的孤儿页面。

**本次改动**：① 改密接口迁到 `/api/v1/auth/change-password`（用户自身分组），② 明确「只能改自己」（token 取 user_id），③ 侧边栏加「用户中心」入口 + 弹窗化交互。

## 3. 权限设计

- **改密操作**：`POST /api/v1/auth/change-password` 只做 JWT 认证（**无 RBAC middleware**）。改「自己密码」是每个登录用户的基本能力，不随角色变化。
- **用户中心菜单**：不加 `sidebar:profile` 权限，对所有登录用户无条件可见（Sidebar 渲染逻辑对无 perm 项做「始终可见」处理）。
- admin / system_admin 无需额外配置即可使用（本就是登录用户）。

### 安全语义：只能改自己的密码

- 改密接口**不接收**请求体中的 `user_id` / `username` / `target` 等目标字段，**只从 JWT token 的 `user_id` claim**（`c.Get("user_id")`）确定操作对象。
- JWT 校验已有（`JWTManager.AuthMiddleware()`），本 spec 复用之。保证 A 用户无法通过改密接口修改 B 用户的密码。

## 4. API 设计

### 4.1 改密接口（迁移 + 复用）

| Method | Path | Description | 变更 |
|--------|------|-------------|------|
| POST | `/api/v1/auth/change-password` | 修改当前登录用户密码 | **迁移**：由 `/api/v1/admin/change-password` 迁来 |

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

### 5.2 前端

**`frontend/app/components/Sidebar.tsx`**：`navItems` 新增「用户中心」菜单项（置于「管理后台」之前），**无 perm 字段**（对所有登录用户可见）：

```tsx
{ href: '/profile', label: '用户中心', icon: '👤', testid: 'nav-profile' },
```

同时调整可见性过滤逻辑，使**无 perm 的项始终可见**：

```tsx
const visibleItems = navItems.filter(item => !item.perm || canAccess(item.perm));
```

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

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | Yes（改密接口路径 `/api/v1/admin/*` → `/api/v1/auth/*`，旧路径删除；需同步前端 + 路由断言测试） |
| 是否需要新增 RBAC 权限 / 改 seed | **No**（本 spec 不配 RBAC，登录即可改） |
| 性能影响 | 忽略 |
| 是否需要新增 Skill | No |
| 是否修改其他功能 | No（现有 `/change-password` 页面仅修正调用路径，其余不动） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/auth/interface.go` | `AuthService` 接口新增 `ChangePassword` | 极小 |
| `internal/service/auth/service.go` | 实现 `ChangePassword` + 迁入复杂度校验 | 中 |
| `internal/service/auth/mocks/AuthService.go` | mockery 重新生成 | 自动 |
| `internal/api/handler/auth.go` | 新增 `ChangePassword` handler | 小 |
| `internal/api/handler/config.go` | 删除 `ChangePassword` + userRepo 依赖 | 中 |
| `internal/api/handler/routes.go` | `registerAuthProtected` 加改密路由 | 极小 |
| `cmd/server/wire.go` | `NewConfigHandler` 移除 userRepo 实参 | 极小 |
| `internal/api/handler/config_test.go` | 删除 ChangePassword 测试（迁至 auth） | 中 |
| `internal/api/handler/config_error_test.go` | 删除 ChangePassword 测试 + 更新路由断言 | 中 |
| `internal/api/handler/auth_test.go` / `auth_error_test.go` | 新增 ChangePassword 测试 | 中 |
| `internal/service/auth/auth_test.go` | 新增 `ChangePassword` service 单测 | 中 |
| `internal/api/handler/routes_error_test.go` | 更新 `/api/v1/admin/change-password` → `/api/v1/auth/change-password` | 极小 |
| `frontend/app/components/Sidebar.tsx` | navItems 加「用户中心」（无 perm）+ 过滤逻辑支持无 perm 项 | 小 |
| `frontend/app/profile/page.tsx` | 新建用户中心页面 | New |
| `frontend/app/components/ChangePasswordModal.tsx` | 新建改密弹窗组件 | New |
| `frontend/app/change-password/page.tsx` | 修正 `apiFetch` 路径 | 极小 |

## 8. 测试策略

1. **Unit tests**（Go）：
   - `auth/service.go`：`ChangePassword` 成功 / 复杂度不足 / 旧密码错误 / 用户不存在 / 哈希失败 / 更新失败 各分支。
   - `auth/handler`：`ChangePassword` 成功 + 各错误码路径，用 `gomonkey.ApplyMethodFunc` 验证 handler→service 参数（userID 来自 token）。
   - 覆盖率底线见 SPEC-045：L1 100% / L3 98%。
2. **E2E tests**（前端）：
   - `tests/ui/` 新增：UI-XXX「用户中心菜单可见 + 弹窗修改密码」——普通用户登录后侧边栏可见「用户中心」，进入 `/profile`，点卡片开弹窗，错误旧密码报错、正确旧密码改密成功后跳登录页。
   - 权限回归：所有角色（user / admin / system_admin）登录后均可见「用户中心」菜单（本 spec 不加权限）。

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
| L3 | 依赖 MongoDB | **98%** | 全量 gate |

### 断言质量要求

- [ ] **必须** `ChangePassword` 每个 Success 测试至少验证：调用 `UpdatePassword` 的参数（新 hash 非空、非明文）、handler 传参 userID 正确
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%（`ut-workflow.yml` gate）
- [ ] `go vet` 无警告

## 10. 验证标准

1. 任意登录用户（user / admin / system_admin）侧边栏均可见「用户中心」，点击进入 `/profile`。
2. `/profile` 显示用户信息卡片 + 「修改密码」卡片。
3. 点击卡片弹出玻璃样式弹窗，颜色随主题变量切换（深色/浅色一致）。
4. 旧密码错误 → 弹窗内报「旧密码不正确」；新密码复杂度不足 → 报复杂度提示；两次不一致 → 报「两次输入的密码不一致」。
5. 改密成功 → 提示成功后跳转登录页，旧密码登录失败、新密码登录成功。
6. **只能改自己**：请求体携带他人 `user_id` 字段不生效，接口始终只改 token 对应的当前用户密码。
7. 旧路径 `/api/v1/admin/change-password` 返回 404（已删除），新路径 `/api/v1/auth/change-password` 正常。
8. CI（sonar-check + ui-tests + ut-workflow）全绿。
