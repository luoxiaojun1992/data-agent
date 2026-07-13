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
| UI-093 | Model — 模型配置页渲染 | ✅ 已实现 |
| UI-094 | Model — OpenAI 兼容 API URL 配置 | ✅ 已实现 |
| UI-095 | Model — API Key 输入与 Vault 加密 | ✅ 已实现 |
| UI-096 | Model — 眼睛按钮切换 API Key | ✅ 已实现 |
| UI-097 | Model — Model Name 下拉选择 | ✅ 已实现 |
| UI-098 | Model — 上下文长度配置 (Stepper) | ✅ 已实现 |
| UI-099 | Model — 最大输出长度配置 | ✅ 已实现 |
| UI-100 | Model — Temperature 配置 | ✅ 已实现 |
| UI-101 | Model — Top-P 配置 | ✅ 已实现 |
| UI-102 | Model — Hermes 配置区域 | ✅ 已实现 |
| UI-103 | Model — 仅 admin 可访问 | ✅ 已实现 |
| UI-104 | SysConfig — 系统配置页渲染 | ✅ 已实现 |
| UI-105 | SysConfig — 修改保存全局参数 | ✅ 已实现 |
| UI-106 | SysConfig — 仅 system_admin 可访问 | ✅ 已实现 |
| UI-107 | SysConfig — 缓冲期上限校验 | ✅ 已实现 |
| UI-108 | SysConfig — 配置优先级验证 | ✅ 已实现 |
| UI-109 | Task — 任务管理页渲染 | ✅ 已实现 |
| UI-110 | Task — 全局查看所有用户任务 | ✅ 已实现 |
| UI-111 | Task — 查看任务详情 | ✅ 已实现 |
| UI-112 | Task — 取消运行中任务 | ✅ 已实现 |
| UI-113 | Task — 重试失败任务 | ✅ 已实现 |
| UI-114 | Task — 批量取消任务 | ✅ 已实现 |
| UI-115 | KB — 知识库管理页渲染 | ✅ 已实现 |
| UI-116 | KB — 文档卡片渲染 | ✅ 已实现 |
| UI-117 | KB — 上传单个文档 | ✅ 已实现 |
| UI-118 | KB — 批量上传文档 | ✅ 已实现 |
| UI-120 | KB — 索引状态实时更新 | ✅ 已实现 |
| UI-121 | KB — 搜索知识库文档 | ✅ 已实现 |
| UI-123 | KB — 删除知识库文档 | ✅ 已实现 |
| UI-124 | KB — 文档分页 | ✅ 已实现 |
| UI-125 | Audit — 审计日志页渲染 | ✅ 已实现 |
| UI-126 | Audit — 审计日志表格数据 | ✅ 已实现 |
| UI-127 | Audit — 按时间范围筛选 | ✅ 已实现 |
| UI-128 | Audit — 按操作类型筛选 | ✅ 已实现 |
| UI-129 | Audit — 按用户筛选 | ✅ 已实现 |
| UI-130 | Audit — 导出弹窗 | ✅ 已实现 |
| UI-131 | Audit — 执行导出 | ✅ 已实现 |
| UI-132 | Audit — 导出条数上限校验 | ✅ 已实现 |
| UI-133 | Audit — 审计日志分页 | ✅ 已实现 |
| UI-134 | API — API 转换审核页渲染 | ✅ 已实现 |
| UI-135 | API — API 卡片渲染 | ✅ 已实现 |
| UI-136 | API — 上传 OpenAPI 文件 | ✅ 已实现 |
| UI-137 | API — 批准 API 转换 | ✅ 已实现 |
| UI-138 | API — 驳回 API 转换 | ✅ 已实现 |
| UI-139 | API — 双重审核校验 | ✅ 已实现 |
| UI-140 | API — 批量上传 | ✅ 已实现 |
| UI-141 | Notif — 铃铛图标与未读数 | ✅ 已实现 |
| UI-142 | Notif — 展开通知列表 | ✅ 已实现 |
| UI-143 | Notif — 标记已读 | ✅ 已实现 |
| UI-144 | Notif — 一键全部已读 | ✅ 已实现 |
| UI-145 | Notif — 发送站内信 | ✅ 已实现 |
| UI-150 | Pwd — 修改密码页 | ✅ 已实现 |
| UI-151 | Pwd — 成功修改密码 | ✅ 已实现 |
| UI-152 | Pwd — 旧密码错误 | ✅ 已实现 |
| UI-153 | Pwd — 新密码不一致 | ✅ 已实现 |
| UI-154 | Pwd — 新密码强度校验 | ✅ 已实现 |
| UI-155 | Pwd — 所有角色可修改密码 | ✅ 已实现 |

**合计**: 75 个真实用例 + 2 个手动测试用例（SPEC-032 阶段）

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
