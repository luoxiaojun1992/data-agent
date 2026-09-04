# 用户中心与修改密码

> **SPEC-083** | Status: 设计中

## 1. 目标

在侧边栏新增「用户中心」菜单入口，跳转用户中心页面，页内提供「修改密码」卡片，点击弹出修改密码弹窗（复用现有改密 API）。为「用户中心」新增对应 RBAC 权限（默认分配给普通用户级别），并同步原始 seed（保证存量库升级与全新部署默认数据均不丢失新权限）。UI 样式遵循 SPEC-078（弹窗玻璃样式）与 SPEC-076（主题变量）。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-064 RBAC 角色权限管理系统 | ✅ | 权限枚举 `domain/model/rbac.go` + seed `rbac_seed.go` + `RequirePermission` middleware 已就绪 |
| SPEC-078 前端列表页 UI 规范统一 | ✅ | 弹窗玻璃样式 + 主按钮渐变规范 |
| SPEC-076 前端主题切换 | ✅ | CSS 变量 `var(--text-primary)` 等主题变量（本 spec 复用变量，不依赖其落地） |
| SPEC-003 基础设施与认证授权 | ✅ | 改密 API `POST /api/v1/admin/change-password` 已存在 |

> 无阻塞项，可立即开始。

## 2. 背景（现状与动机）

**改密能力其实已存在，但入口缺失**：

1. **后端**：`ConfigHandler.ChangePassword` 已实现 `POST /api/v1/admin/change-password`，校验旧密码 + 复杂度（≥8 位含大小写字母数字）+ bcrypt 更新，**无 RBAC middleware**（登录即可调，改的是「自己」的密码，语义正确）。
2. **前端**：`/change-password` 页面已存在（`frontend/app/change-password/page.tsx`），可完整走通改密流程，成功后 logout 跳登录。
3. **但它是孤儿页面**：侧边栏 `navItems` 无「用户中心」入口，登录后也无跳转逻辑，只能手动输 URL 访问。
4. **`need_change_pw` 机制存在**：登录响应含 `need_change_pw`（初始密码未改标记），前端仅用于 banner 提示，未驱动入口。

**本次改动动机**：把改密能力接入「用户中心」入口，提供标准化的弹窗交互，并纳入 RBAC 权限矩阵 + seed。

## 3. 权限设计

### 3.1 新增权限

| 权限 Key | 常量名 | 名称 | 模块 | 默认角色 |
|----------|--------|------|------|:--------:|
| `sidebar:profile` | `PermSidebarProfile` | 用户中心菜单 | sidebar | user_role（普通用户） |

- 仅新增 **1 个** 权限（用户中心菜单可见性），遵循现有 `sidebar:*` 命名规范。
- **不新增**「改密操作」独立权限：改密是「修改自己密码」，人人可做（现状接口本就无 RBAC），无需角色区分。改密接口 `POST /api/v1/admin/change-password` **保持现状无 RBAC**，不额外加 `RequirePermission`（避免给存量库引入「普通用户突然无法改密」的回归风险）。
- admin/system_admin 通过 RBAC 角色层级（父角色拥有子角色权限）自动继承 user_role 的 `sidebar:profile`。

### 3.2 seed 同步（关键：增量补齐）

现有 `seedPermissions` 采用「全有或全无」逻辑——`CountDocuments > 0` 即 `return`，**导致存量库升级时新增权限永远不会补齐**。本 spec 必须升级为**逐权限增量补齐**（参考 `SeedSkills` 的 `existMap + Upsert` 模式）：

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

本 spec **不新增、不修改 API**，复用现有接口：

| Method | Path | Description | 变更 |
|--------|------|-------------|------|
| POST | `/api/v1/admin/change-password` | 修改当前登录用户密码 | 无（复用，保持无 RBAC） |
| GET | `/api/v1/rbac/me/permissions` | 返回当前用户权限列表（登录时前端已调用） | 无（自动包含新增 `sidebar:profile`） |

请求/响应（复用现有，不改变）：

```json
// 请求
{ "old_password": "OldPass1", "new_password": "NewPass1" }
// 响应（成功）
{ "message": "密码修改成功" }
// 响应（失败）
{ "error": "旧密码不正确" | "密码至少 8 位，需包含大小写字母和数字" | ... }
```

## 5. 详细设计

### 5.1 后端

**`internal/domain/model/rbac.go`**：在 Sidebar 权限常量区新增：

```go
PermSidebarProfile = "sidebar:profile"
```

**`cmd/server/migration/rbac_seed.go`**：
1. `perms` 数组新增条目：
   ```go
   {perm: RBACPerm("rbac_perm_sidebar_profile", model.PermSidebarProfile, "用户中心菜单", "sidebar"), roleIDs: []string{user}},
   ```
2. `seedPermissions` 从「全有或全无」升级为「逐权限增量补齐」（见 §3.2）。

### 5.2 前端

**`frontend/app/components/Sidebar.tsx`**：`navItems` 新增「用户中心」菜单项（置于「管理后台」之前）：

```tsx
{ perm: 'sidebar:profile', href: '/profile', label: '用户中心', icon: '👤', testid: 'nav-profile' },
```

**`frontend/lib/api.ts`**：`SIDEBAR_PERMS` 新增 `profile: 'sidebar:profile'`。

**`frontend/app/profile/page.tsx`**（新建，用户中心页面）：
- 复用 `AppLayout` 包裹，页面结构：
  - 用户信息卡片（头像 + 用户名 + 角色，`glass` 样式）
  - 「修改密码」卡片（`glass` 样式），点击打开弹窗
- 标题 `data-testid="profile-page"`。

**`frontend/app/components/ChangePasswordModal.tsx`**（新建，改密弹窗）：
- 遵循 SPEC-078 弹窗玻璃样式：遮罩 `backdrop-blur` + 变量面板 `var(--glass-bg)` / `var(--border-glass)`。
- 表单字段：旧密码 / 新密码 / 确认新密码（复用现有 `/change-password` 页面的校验逻辑：两次一致 + 复杂度 + 旧密码校验）。
- 提交复用 `apiFetch('/change-password', { method: 'POST', ... })`。
- 成功后：提示「密码修改成功，请重新登录」→ 延时 `logout()` + `router.push('/login')`（复用现有行为）。
- 颜色全部走 CSS 变量（`var(--text-primary)` 等），主按钮渐变 `linear-gradient(135deg, #5c7cfa, #7c3aed)`（SPEC-078 规范），兼容 SPEC-076 主题切换。
- `data-testid`：`pwd-modal` / `pwd-modal-old-input` / `pwd-modal-new-input` / `pwd-modal-confirm-input` / `pwd-modal-submit-btn`。

**现有 `frontend/app/change-password/page.tsx`**：**保留不动**（不改、不删，避免「修改其他功能」；它承载 `need_change_pw` 初始密码 banner 场景，且可能被书签/直链访问）。改密校验逻辑若抽取为共享函数，两处复用；否则各自保持现状。

### 5.3 数据流

```
用户点击侧边栏「用户中心」 → /profile 页面
  └─ 点击「修改密码」卡片 → 打开 ChangePasswordModal
       └─ 填旧/新/确认密码 → POST /api/v1/admin/change-password
            ├─ 后端校验旧密码 + 复杂度 + bcrypt 更新
            ├─ 成功 → 前端提示 → logout → 跳 /login
            └─ 失败 → 弹窗内展示 error
```

权限链路：登录时 `GET /api/v1/rbac/me/permissions` 已返回权限列表 → 前端 `canAccess('sidebar:profile')` 决定菜单可见性。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（复用 rbac_permissions / rbac_role_permissions） |
| 是否影响现有 API | No（复用 `POST /change-password`；`GET /rbac/me/permissions` 自动含新权限） |
| 性能影响 | 忽略（seed 启动时一次增量补齐，O(n) 遍历权限数组） |
| 是否需要新增 Skill | No |
| 是否修改其他功能 | No（现有 `/change-password` 页面保留不动；改密接口不加 middleware） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/domain/model/rbac.go` | 新增 `PermSidebarProfile` 常量 | 极小（1 行） |
| `cmd/server/migration/rbac_seed.go` | 权限矩阵加新权限 + `seedPermissions` 增量补齐 | 中 |
| `cmd/server/migration/rbac_seed_test.go` | seed 增量补齐单测（如已有则追加） | 中 |
| `frontend/app/components/Sidebar.tsx` | navItems 加「用户中心」 | 极小 |
| `frontend/lib/api.ts` | SIDEBAR_PERMS 加 `profile` | 极小 |
| `frontend/app/profile/page.tsx` | 新建用户中心页面 | New |
| `frontend/app/components/ChangePasswordModal.tsx` | 新建改密弹窗组件 | New |
| `frontend/app/change-password/page.tsx` | 保留不动（可选：抽取共享校验） | 无 |

## 8. 测试策略

1. **Unit tests**（Go）：
   - `rbac_seed.go`：新增 `seedPermissions` 增量补齐测试——① 空库全量插入（含新权限）；② 存量库（已有部分权限）仅补齐缺失权限，不重复插入；③ 补齐后 role_permissions 关联正确、permission_count 更新正确。
   - 覆盖率底线见 SPEC-045：L1 100% / L3 98%。
2. **E2E tests**（前端）：
   - `tests/ui/` 新增用例：UI-XXX「用户中心菜单可见 + 弹窗修改密码」——普通用户登录后侧边栏可见「用户中心」，进入 `/profile`，点卡片开弹窗，错误旧密码报错、正确旧密码改密成功后跳登录页。
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
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `domain/model` 常量（无逻辑） |
| L3 | 依赖 MongoDB | **98%** | `migration/rbac_seed.go`（增量补齐） |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** seed 增量补齐的 Success 测试至少验证：插入条数、缺失补齐、role_permissions 关联、幂等（二次运行零新增）
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
6. 存量库升级后，`rbac_permissions` 集合新增 `sidebar:profile`，`rbac_role_permissions` 关联到 `rbac_role_user`，普通用户权限列表包含 `sidebar:profile`（无需重建库）。
7. 全新部署（空库），seed 完整插入含 `sidebar:profile` 的全部权限。
8. CI（sonar-check + ui-tests + ut-workflow）全绿。
