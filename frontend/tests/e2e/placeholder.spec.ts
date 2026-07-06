import { test, expect } from '@playwright/test';

test('UI-000: Placeholder — Always Pass', async () => {
  // MVP 占位用例：保证 E2E CI Pipeline 不报错
  // 前端功能开发时逐步替换为真实用例
  expect(true).toBe(true);
});
