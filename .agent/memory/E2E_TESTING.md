# DataAgent — E2E 测试

> E2E 框架已就绪，占位用例保证 CI Pipeline 不报错。
> **前端功能开发完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。**
> CI 配置与 game-dev-studio 一致：sonar-check → ui-tests，两者均通过才算完成。

## 测试框架

- **工具**: Playwright (TypeScript)
- **配置**: `tests/playwright.config.ts`
- **目录**: `tests/ui/`

## 用例编号规则

`UI-XXX`，三位数字递增。

## 测试矩阵总览

| 用例编号 | 用例名称 | 状态 |
|---------|---------|:---:|
| UI-075 | User — 用户管理页渲染 | ✅ 已实现 |
| UI-076 | User — 用户表格列渲染 | ✅ 已实现 |
| UI-077 | User — 添加用户 | ✅ 已实现 |
| UI-078 | User — 编辑用户角色 | ✅ 已实现 |
| UI-079 | User — 启用/停用用户 | ✅ 已实现 |
| UI-080 | User — 删除用户 | ✅ 已实现 |
| UI-081 | User — 不可删除 system_admin | ✅ 已实现 |
| UI-082 | User — 不可创建第二个 system_admin | ✅ 已实现 |
| UI-083 | User — 邮箱唯一性校验 | ✅ 已实现 |
| UI-084 | User — 用户列表分页 | ✅ 已实现 |
| UI-085 | Role — 权限管理页渲染 | ✅ 已实现 |
| UI-086 | Role — 固定角色卡片 | ✅ 已实现 |
| UI-087 | Role — 自定义角色表格 | ✅ 已实现 |
| UI-088 | Role — 新建自定义角色 | ✅ 已实现 |
| UI-089 | Role — 权限 Tab 渲染 | ✅ 已实现 |
| UI-090 | Role — 编辑角色权限 | ✅ 已实现 |
| UI-091 | Role — 删除自定义角色 | ✅ 已实现 |
| UI-092 | Role — 不可删除固定角色 | ✅ 已实现 |

**合计**: 18 个真实用例 + 1 个占位用例（SPEC-024 阶段）

## data-testid 命名规范

```
{component}-{element}
```

示例: `nav-login-btn`, `chart-revenue`, `input-query`

## 运行 E2E

```bash
cd tests && npx playwright test
```

## 占位用例

```typescript
// tests/ui/placeholder.spec.ts
import { test, expect } from '@playwright/test';

test('UI-000: Placeholder — Always Pass', async ({ page }) => {
  expect(true).toBe(true);
});
```
