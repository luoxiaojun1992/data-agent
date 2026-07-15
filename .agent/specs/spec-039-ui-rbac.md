# UI E2E 测试设计 — 角色权限访问控制 (RBAC)

> **SPEC-039** | Status: 已实现 | Date: 2026-07-15 | Phase: P8

## 1. 目标

定义 DataAgent 角色权限访问控制（RBAC）的 E2E UI 测试用例规范。覆盖 Viewer/Analyst/Admin/System_admin 四种角色的导航项可见性、Viewer 无法直接访问管理页面 URL 和 Viewer 无法创建 Agent 任务。

> **设计依据**: `docs/DataAgent-UI测试用例文档.md` §26
>
> **测试框架**: Playwright (TypeScript)
>
> **用例范围**: UI-187 ~ UI-192（共 6 个用例）

## 2. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-003 | ✅ | 基础设施（RBAC 权限定义 §6） |
| SPEC-014 | ✅ | 测试体系（Playwright 框架） |
| SPEC-017 | ✅ | AUTH（多角色登录） |
| SPEC-018 | ✅ | LAYOUT（导航项可见性） |
| SPEC-023 | ✅ | USER |
| SPEC-024 | ✅ | ROLE |

## 3. 测试范围

| 子功能 | 用例 | 优先级 |
|--------|------|:------:|
| Viewer 可见导航项 | UI-187 | P0 |
| Analyst 可见导航项 | UI-188 | P0 |
| Admin 可见导航项 | UI-189 | P0 |
| System_admin 可见全部导航项 | UI-190 | P0 |
| Viewer 无法直接访问管理页面 URL | UI-191 | P0 |
| Viewer 无法创建 Agent 任务 | UI-192 | P0 |

## 4. 测试用例

### UI-187: RBAC — Viewer 可见导航项

- **优先级**: P0
- **前置条件**: 以 Viewer (普通用户) 身份登录
- **步骤**:
  1. 检查侧边栏导航项
- **预期结果**:
  - 可见：轻量工作区
  - 不可见：专业工作区、数据看板
  - 不可见：系统管理下所有导航项（用户管理/权限管理/模型配置/任务管理/知识库管理/审计日志/API审核）
  - `data-testid`: `sidebar`

### UI-188: RBAC — Analyst 可见导航项

- **优先级**: P0
- **前置条件**: 以 Analyst (分析师) 身份登录
- **步骤**:
  1. 检查侧边栏导航项
- **预期结果**:
  - 可见：轻量工作区、专业工作区
  - 不可见：数据看板
  - 不可见：系统管理下所有导航项
  - `data-testid`: `sidebar`

### UI-189: RBAC — Admin 可见导航项

- **优先级**: P0
- **前置条件**: 以 Admin (普通管理员) 身份登录
- **步骤**:
  1. 检查侧边栏导航项
- **预期结果**:
  - 可见：轻量工作区、专业工作区、数据看板
  - 可见：用户管理、权限管理（仅可管理普通用户）
  - 不可见：模型配置
  - 可见：任务管理、知识库管理、审计日志、API 转换审核
  - `data-testid`: `sidebar`

### UI-190: RBAC — System_admin 可见全部导航项

- **优先级**: P0
- **前置条件**: 以 system_admin 身份登录
- **步骤**:
  1. 检查侧边栏
- **预期结果**:
  - 所有 12 个导航项全部可见
  - `data-testid`: `sidebar`

### UI-191: RBAC — Viewer 无法直接访问管理页面 URL

- **优先级**: P0
- **前置条件**: 以 Viewer 身份登录
- **步骤**:
  1. 直接在浏览器中输入 `/admin/users` URL
  2. 直接在浏览器中输入 `/admin/model-config` URL
- **预期结果**:
  - 返回 403 Forbidden 或重定向到首页
  - 显示无权限提示
  - `data-testid`: N/A

### UI-192: RBAC — Viewer 无法创建 Agent 任务

- **优先级**: P0
- **前置条件**: 以 Viewer 身份登录
- **步骤**:
  1. 如果专业工作区页面可访问
  2. 检查「新建分析任务」按钮
- **预期结果**:
  - 按钮不存在或禁用 + tooltip「无权限」
  - `data-testid`: `agent-create-btn`

## 5. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要模拟后端 API | Yes — 登录 API（返回不同角色）、页面权限检查 API |
| 是否依赖第三方服务 | No |
| 是否需要特殊测试数据 | Yes — 4 个角色的测试账号 mock |
| 是否需要新增 DB 集合 | No |
| 是否影响现有 API | No |
| 性能影响 | 无 |
| 实现复杂度 | Medium — 需要多角色登录的 fixture 管理 |
| 是否有无法实现的用例 | **待确认** — `UI-191` URL 直接访问需要 verify 403/redirect 响应；`UI-192` 需要确保 Viewer 角色即使访问到了专业工作区页面也无法创建任务 |

## 6. 相关文件

| File | Role | Change Magnitude |
|------|------|:---:|
| `tests/ui/rbac.spec.ts` | RBAC E2E 测试实现 | New |
| `tests/ui/fixtures/rbac.fixture.ts` | RBAC mock 数据（多角色） | New |

## 7. 验证标准

- [ ] 所有 P0 用例（UI-187 ~ UI-192）必须通过
- [ ] RBAC 权限控制 100% 符合 SPEC-003 §6
- [ ] 每个角色的导航项可见性完全正确

## 8. UI Test / E2E 验收规则

- [ ] **必须** mock 后端认证 API（返回不同角色的 JWT）
- [ ] **必须** 为每个角色创建独立的 test fixture
- [ ] **必须** 验证导航项的出现/缺失（使用 data-testid 选择器）
- [ ] **必须** 验证直接访问保护路由的 403/redirect 行为
- [ ] **严禁** 修改后端 RBAC 逻辑来适应测试

参考: `.agent/memory/E2E_TESTING.md`
