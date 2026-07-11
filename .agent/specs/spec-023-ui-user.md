# UI E2E 测试设计 — 用户管理 (USER)

> **SPEC-023** | Status: 已实现 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 用户管理模块的 E2E UI 测试用例规范。覆盖用户管理页渲染、用户表格列渲染、添加用户、编辑用户角色、启用/停用用户、删除用户、不可删除 system_admin、不可创建第二个 system_admin、邮箱唯一性校验和用户列表分页。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §10
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-075 ~ UI-084（共 10 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（RBAC、用户认证） |
| SPEC-013 | ✅ | 管理后台（用户管理页面定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-018 | ✅ | LAYOUT |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 用户管理页渲染 | UI-075 | P0 |
| 用户表格列渲染 | UI-076 | P0 |
| 添加用户 | UI-077 | P0 |
| 编辑用户角色 | UI-078 | P0 |
| 启用/停用用户 | UI-079 | P0 |
| 删除用户 | UI-080 | P0 |
| 不可删除 system_admin | UI-081 | P0 |
| 不可创建第二个 system_admin | UI-082 | P0 |
| 邮箱唯一性校验 | UI-083 | P1 |
| 用户列表分页 | UI-084 | P1 |

## 4. 测试用例

### UI-075: User — 用户管理页渲染

- **优先级**: P0
- **前置条件**: 以 system_admin 身份登录，点击「用户管理」
- **步骤**:
  1. 检查页面
- **预期结果**:
  - Header：「用户管理」
  - 右上角蓝紫渐变「+ 添加用户」按钮
  - 用户表格显示（5 行示例数据）
  - 表格列：姓名 / 邮箱 / 角色 / 状态 / 操作
  - `data-testid`: `user-page-header`, `user-add-btn`, `user-table`

### UI-076: User — 用户表格列渲染

- **优先级**: P0
- **前置条件**: 用户管理页已加载，有用户数据
- **步骤**:
  1. 检查表头
  2. 检查数据行
- **预期结果**:
  - 表头：11px Bold `#666` + 大写 + `rgba(255,255,255,0.03)` 背景
  - 数据行：13px `#7A7A7A` + 底部 `rgba(255,255,255,0.06)` 分割线
  - Hover 行背景 `rgba(255,255,255,0.03)`
  - 状态列：启用（绿色 Pill）/ 停用（粉色 Pill）
  - 操作列：编辑 / 启用-停用切换 / 删除 按钮
  - `data-testid`: `user-table-header-{col}`, `user-row-{userId}`, `user-status-{userId}`

### UI-077: User — 添加用户

- **优先级**: P0
- **前置条件**: 用户管理页
- **步骤**:
  1. 点击「+ 添加用户」
  2. 填写：姓名「测试用户」、邮箱「test@company.com」、角色「普通用户」
  3. 点击「确认添加」
- **预期结果**:
  - 弹出添加用户模态窗口
  - 包含字段：姓名（必填）、邮箱（必填+邮箱格式验证）、角色下拉（system_admin/admin/user）
  - 确认后用户列表新增一行
  - 显示成功 toast
  - `data-testid`: `user-add-modal`, `user-add-name`, `user-add-email`, `user-add-role`, `user-add-submit`

### UI-078: User — 编辑用户角色

- **优先级**: P0
- **前置条件**: 用户列表中有用户
- **步骤**:
  1. 点击某用户的「编辑」按钮
  2. 将角色从「普通用户」改为「分析师」
  3. 保存
- **预期结果**:
  - 弹出编辑模态窗口，预填当前信息
  - 角色下拉可修改
  - 保存后列表行刷新
  - `data-testid`: `user-edit-btn-{userId}`, `user-edit-modal`, `user-edit-role`, `user-edit-submit`

### UI-079: User — 启用/停用用户

- **优先级**: P0
- **前置条件**: 用户列表中有启用状态的用户
- **步骤**:
  1. 点击启用状态用户的「停用」按钮
  2. 确认停用
  3. 再次点击恢复启用
- **预期结果**:
  - 弹出确认弹窗
  - 确认后状态 Pill 变为🔴「停用」(粉色)
  - 停用用户无法登录
  - 恢复启用后 Pill 变🟢「启用」(绿色)
  - `data-testid`: `user-toggle-btn-{userId}`, `user-toggle-confirm-modal`

### UI-080: User — 删除用户

- **优先级**: P0
- **前置条件**: 用户列表中有非 system_admin 用户
- **步骤**:
  1. 点击某用户的「删除」按钮
  2. 在确认弹窗中点击「取消」
  3. 再次点击删除 → 点击「确认删除」
- **预期结果**:
  - 弹出确认弹窗：「确定要删除用户 XXX 吗？此操作不可撤销。」（SPEC-013 §5）
  - 默认焦点在「取消」按钮
  - 「确认删除」为红色警告按钮
  - 取消后用户仍在列表中
  - 确认后用户从列表中移除
  - `data-testid`: `user-delete-btn-{userId}`, `user-delete-confirm-modal`, `user-delete-confirm-btn`
- **相关 Spec**: SPEC-013 §5

### UI-081: User — 不可删除 system_admin

- **优先级**: P0
- **前置条件**: 用户列表中有 system_admin
- **步骤**:
  1. 尝试找到 system_admin 的删除按钮
- **预期结果**:
  - system_admin 行不显示「删除」按钮
  - 或「删除」按钮为禁用状态 + tooltip 提示「不可删除系统管理员」
  - `data-testid`: `user-row-{systemAdminId}`

### UI-082: User — 不可创建第二个 system_admin

- **优先级**: P0
- **前置条件**: 以 system_admin 身份登录，用户列表中已有 system_admin
- **步骤**:
  1. 点击「+ 添加用户」
  2. 填写用户信息，角色选择「system_admin」
  3. 点击确认
- **预期结果**:
  - 角色下拉中「system_admin」不可选或带 tooltip「系统管理员唯一，不可创建第二个」
  - 或创建时后端返回错误并显示提示：「系统管理员已存在，无法创建」
  - 已有 system_admin 不可被降级为其他角色
  - `data-testid`: `user-add-role`, `user-add-role-system-admin-disabled`
- **相关 PRD**: PRD §F-11, SPEC-003 §6.3

### UI-083: User — 邮箱唯一性校验

- **优先级**: P1
- **前置条件**: 已有用户 test@company.com
- **步骤**:
  1. 尝试添加同名邮箱的新用户
  2. 点击确认
- **预期结果**:
  - 显示错误提示：「该邮箱已被注册」
  - 用户未被创建
  - `data-testid`: `user-add-email-error`

### UI-084: User — 用户列表分页

- **优先级**: P1
- **前置条件**: 用户总数 > 20
- **步骤**:
  1. 检查分页控件
- **预期结果**:
  - 底部显示分页信息（如「共52条」）
  - 页码按钮（1/2/3）
  - 每页条数切换（10/20/50）
  - `data-testid`: `user-pagination`, `user-page-size-select`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 用户 CRUD API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 多个角色的用户 mock 数据、system_admin 测试账号 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 标准 CRUD 操作 mock，无特殊复杂性 |
| 是否有无法实现的用例 | **待确认** — `UI-079`「停用用户无法登录」需要跨 test 验证（需要独立测试用例验证登录被拒）；`UI-082` system_admin 唯一性校验的前后端协调验证 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/user.spec.ts` | USER E2E 测试实现 | New |
| `tests/ui/fixtures/user.fixture.ts` | User mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-075 ~ UI-082）必须通过
- [ ] 所有 P1 用例（UI-083, UI-084）必须通过
- [ ] 用户管理 UI 100% 符合 SPEC-013 §5 规范
- [ ] system_admin 保护规则生效

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端 User API（`page.route()` 拦截 `/api/v1/users/**`）
- [ ] **必须** 验证模态窗口的字段完整性
- [ ] **必须** 测试删除/停用的确认弹窗取消和确认两种路径
- [ ] **必须** 验证 system_admin 不可删除/不可创建第二个的保护逻辑
- [ ] **严禁** 使用真实数据库操作用户

参考: `.agent/memory/E2E_TESTING.md`
