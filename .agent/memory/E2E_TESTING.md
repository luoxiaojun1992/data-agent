# DataAgent — E2E 测试

> MVP 阶段：框架已就绪，占位用例保证 CI Pipeline 不报错。
> 前端功能开发时逐步添加真实用例。

## 测试框架

- **工具**: Playwright (TypeScript)
- **配置**: `frontend/playwright.config.ts`
- **目录**: `frontend/tests/e2e/`

## 用例编号规则

`UI-XXX`，三位数字递增。

## 测试矩阵总览

| 用例编号 | 用例名称 | 状态 |
|---------|---------|:---:|
| UI-000 | Placeholder — Always Pass | ✅ 永远通过 |

**合计**: 0 个真实用例 + 1 个占位用例（MVP 阶段）

## data-testid 命名规范

```
{component}-{element}
```

示例: `nav-login-btn`, `chart-revenue`, `input-query`

## 运行 E2E

```bash
cd frontend && npx playwright test
```

## 占位用例

```typescript
// frontend/tests/e2e/placeholder.spec.ts
import { test, expect } from '@playwright/test';

test('UI-000: Placeholder — Always Pass', async ({ page }) => {
  expect(true).toBe(true);
});
```
