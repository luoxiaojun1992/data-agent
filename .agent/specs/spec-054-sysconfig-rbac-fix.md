# Sysconfig 页面权限不足修复

> **SPEC-054** | Status: 设计中 | Date: 2026-07-20 | Phase: P13

## 1. 目标

修复 admin 用户访问 `/sysconfig` 页面时显示 `insufficient permissions` 红色错误条幅的问题。根本原因是后端 RBAC 中间件或 API 权限配置错误，导致即使是拥有 `system_admin` 角色的用户也无法正常读取/修改系统配置。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-039 (RBAC) | ✅ | UI-187~192 RBAC 测试已实现 |
| SPEC-025 (UI-模型配置) | ✅ | 模型配置页渲染正常 |
| SPEC-026 (UI-系统配置) | ✅ | 系统配置页渲染 — 但有权限 bug |
| — | — | 无阻塞依赖 |

## 2. 背景

2026-07-20 截图审查发现（SPEC-047 关联）：

- `sysconfig-SYSCONFIG-...修改-Session-缓冲期.png` 截图显示页面顶部出现 **红色 "insufficient permissions"** 条幅
- 测试使用 `system_admin` 角色用户
- 同时期其他 admin 管理页面（审计、用户管理、API 审核）正常渲染，说明 RBAC 基础框架工作正常，但 sysconfig 端点的权限检查有特殊问题

## 3. 架构概述

### 3.1 问题定位

```
GET/PUT /api/v1/sysconfig
  → JWT 鉴权（通过）✅
  → RBAC 中间件检查 ❌ ← permission 配置错误
    → "insufficient permissions" 红色条幅
  → 页面部分渲染（仍在加载其他数据）
```

### 3.2 排查方向

1. `cmd/server/main.go` 中 sysconfig 路由的 `RequirePermission` 参数
2. `model.Perm*` 枚举定义中 sysconfig 对应的权限
3. `internal/middleware/rbac.go` 中 admin 角色的权限列表是否包含 sysconfig
4. 前端 `sysconfig/page.tsx` 的 `GET` 请求 URL 是否正确

## 5. 详细设计

### 5.1 后端权限修复

`cmd/server/main.go` 中 sysconfig 路由定义：

```go
// 当前（推测错误）
sysconfigRoutes.GET("/", middleware.RequirePermission(model.PermSysManage), ...)

// 可能需要的修复
sysconfigRoutes.GET("/", middleware.RequirePermission(model.PermSysConfig), ...)
```

或者 admin 角色缺少 `perm_sys_config` 权限。

### 5.2 修复步骤

1. 查后端日志确认具体 403 端点
2. 打印当前请求的 `user_id` + `role` + 权限集合
3. 对比 admin 角色的权限列表与 sysconfig 端点要求的权限
4. 修复权限不匹配

### 5.3 测试策略

```typescript
// tests/ui/sysconfig.spec.ts — 新增
test('[UI-XXX] Sysconfig — admin 无权限错误条幅', async ({ page }) => {
  await loginAsAdmin(page);
  await page.goto('/admin/sysconfig');
  // 不应显示 "insufficient permissions"
  await expect(page.locator('text=insufficient permissions')).not.toBeVisible({ timeout: 5000 });
  // Session 缓冲期配置应正常渲染
  await expect(page.locator('[data-testid="sysconfig-session-buffer"]')).toBeVisible();
});
```

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No |
| 是否影响现有 API | 修复 sysconfig 权限映射 |
| 性能影响 | 无 |
| 是否需要新增 Skill | No |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `cmd/server/main.go` | sysconfig 路由权限参数 | Edit (~2 行) |
| `internal/model/permission.go` | 权限枚举（如需新增） | Edit (~1 行) |
| `internal/middleware/rbac.go` | admin 权限列表（如需补充） | Edit (~3 行) |

## 9. UI Test / E2E 验收规则

- 新增 UI 用例：admin 访问 sysconfig 不显示权限错误
- 现有 sysconfig 用例继续通过
- 合并前 `ui-tests = success`
