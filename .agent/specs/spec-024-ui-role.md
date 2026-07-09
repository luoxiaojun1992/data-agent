# UI E2E 测试设计 — 权限管理 (ROLE)

> **SPEC-024** | Status: 设计中 | Date: 2026-07-09 | Phase: P8

## 1. 目标

定义 DataAgent 权限管理模块的 E2E UI 测试用例规范。覆盖权限管理页渲染、固定角色卡片、自定义角色表格、新建/编辑/删除自定义角色、权限 Tab 渲染和固定角色不可删除规则。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §11
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-085 ~ UI-092（共 8 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（RBAC、权限定义） |
| SPEC-013 | ✅ | 管理后台（权限管理页面定义） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH |
| SPEC-018 | ✅ | LAYOUT |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| 权限管理页渲染 | UI-085 | P0 |
| 固定角色卡片 | UI-086 | P0 |
| 自定义角色表格 | UI-087 | P0 |
| 新建自定义角色 | UI-088 | P0 |
| 权限 Tab 渲染 | UI-089 | P0 |
| 编辑角色权限 | UI-090 | P0 |
| 删除自定义角色 | UI-091 | P1 |
| 不可删除固定角色 | UI-092 | P1 |

## 4. 测试用例

### UI-085: Role — 权限管理页渲染

- **优先级**: P0
- **前置条件**: 以 system_admin 身份登录，点击「权限管理」
- **步骤**:
  1. 检查页面
- **预期结果**:
  - Header：「权限管理」+ 「新建角色」按钮
  - 2 个 Tab：角色 / 权限
  - 默认显示「角色」Tab
  - `data-testid`: `role-page-header`, `role-create-btn`, `role-tabs`
- **相关设计**: UI原型设计文档 §3.7

### UI-086: Role — 固定角色卡片

- **优先级**: P0
- **前置条件**: 权限管理页，「角色」Tab 激活
- **步骤**:
  1. 检查固定角色区域
- **预期结果**:
  - 显示 4 个固定角色卡片：
    - 系统管理员：全部系统权限 / 「固定角色」Badge
    - 数据分析师：发起批量分析、创建定时任务、使用全部 MCP 工具 / 「固定角色」Badge
    - 知识管理员：管理知识库文档、审核 API→MCP 转换 / 「固定角色」Badge
    - 审计员：只读查看所有审计日志 / 「固定角色」Badge
  - 每个卡片有角色名称 (18px Bold) + 权限描述 (13px #7A7A7A)
  - 「固定角色」Badge 为灰色 Pill
  - `data-testid`: `role-fixed-cards`, `role-fixed-card-{0-3}`, `role-fixed-badge`

### UI-087: Role — 自定义角色表格

- **优先级**: P0
- **前置条件**: 权限管理页，「角色」Tab 激活
- **步骤**:
  1. 检查自定义角色区域
- **预期结果**:
  - 表格列出所有自定义角色
  - 列：角色名称 / 权限数量 / 创建时间 / 操作
  - 操作列：编辑 / 删除
  - `data-testid`: `role-custom-table`

### UI-088: Role — 新建自定义角色

- **优先级**: P0
- **前置条件**: 权限管理页
- **步骤**:
  1. 点击「新建角色」
  2. 输入角色名「销售经理」
  3. 勾选权限：`knowledge.search`, `task.create`
  4. 点击确认
- **预期结果**:
  - 弹出新建角色模态窗口
  - 包含角色名称输入框 + 权限多选列表
  - 确认后新增角色出现在自定义角色表格中
  - `data-testid`: `role-create-modal`, `role-create-name`, `role-create-permissions`, `role-create-submit`

### UI-089: Role — 权限 Tab 渲染

- **优先级**: P0
- **前置条件**: 权限管理页
- **步骤**:
  1. 点击「权限」Tab
- **预期结果**:
  - 显示权限列表表格（9 行示例数据）
  - 列：权限标识 / 权限名称 / 描述 / 类型（固定/自定义）
  - 6 个固定权限 + 3 个自定义权限
  - 权限标识如 `user.manage`, `model.config`, `task.manage`
  - `data-testid`: `role-permissions-tab`, `role-permission-table`

### UI-090: Role — 编辑角色权限

- **优先级**: P0
- **前置条件**: 自定义角色表格中有角色
- **步骤**:
  1. 点击某自定义角色的「编辑」按钮
  2. 增删权限勾选
  3. 保存
- **预期结果**:
  - 角色权限更新
  - 拥有该角色的用户权限同步更新
  - `data-testid`: `role-edit-btn-{roleId}`, `role-edit-modal`

### UI-091: Role — 删除自定义角色

- **优先级**: P1
- **前置条件**: 有自定义角色，且无用户使用
- **步骤**:
  1. 点击自定义角色的「删除」按钮
  2. 确认
- **预期结果**:
  - 弹出确认弹窗
  - 确认后角色从列表中移除
  - `data-testid`: `role-delete-btn-{roleId}`, `role-delete-confirm-modal`

### UI-092: Role — 不可删除固定角色

- **优先级**: P1
- **前置条件**: 权限管理页
- **步骤**:
  1. 检查固定角色的操作按钮
- **预期结果**:
  - 固定角色不显示「删除」按钮
  - 固定角色不显示「编辑」按钮（或编辑按钮禁用）
  - `data-testid`: `role-fixed-card-{0-3}`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 角色 CRUD API、权限列表 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 固定角色 + 自定义角色 mock 数据 |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 标准 CRUD + Tab 切换验证 |
| 是否有无法实现的用例 | 无 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/role.spec.ts` | ROLE E2E 测试实现 | New |
| `tests/ui/fixtures/role.fixture.ts` | Role mock 数据 fixture | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-085 ~ UI-090）必须通过
- [ ] 所有 P1 用例（UI-091, UI-092）必须通过
- [ ] 权限管理 UI 100% 符合 UI 原型设计文档 §3.7 和 SPEC-013 规范
- [ ] 固定角色保护规则生效

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端 Role/Permission API
- [ ] **必须** 验证角色卡片和表格的完整渲染
- [ ] **必须** 验证 Tab 切换后权限表格数据正确
- [ ] **必须** 测试固定角色不可删除/不可编辑的保护逻辑
- [ ] **严禁** 使用真实数据库操作角色权限

参考: `.agent/memory/E2E_TESTING.md`
