import { test, expect } from '@playwright/test';

/**
 * SPEC-022: DASHBOARD E2E Tests (UI-062, UI-064)
 *
 * Real API calls only. Tests verify dashboard page structure.
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const TEST_USER = {
  username: `e2e-dash-${uid}@test.local`,
  password: 'E2eTest123!',
};

test.describe('DASH — Dashboard', () => {
  test.beforeAll(async ({ request }) => {
    const res = await request.post(`${API_BASE}/auth/register`, { data: TEST_USER });
    expect(res.status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(TEST_USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(TEST_USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.waitForSelector('[data-testid="main-content"]', { state: 'visible', timeout: 10000 });
  });

  // UI-062: Dashboard page title and header
  test('[UI-062] Dashboard — greeting and page structure', async ({ page }) => {
    await expect(page.locator('[data-testid="page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('仪表盘');
    await expect(page.locator('[data-testid="main-content"]')).toBeVisible();
  });

  // UI-064: Dashboard — stats cards visible
  test('[UI-064] Dashboard — stats cards', async ({ page }) => {
    const main = page.locator('[data-testid="main-content"]');
    // Dashboard has 4 stat cards with labels, scoped to main content
    await expect(main.locator('text=活跃 Chat 会话')).toBeVisible();
    await expect(main.locator('text=知识库文档')).toBeVisible();
    await expect(main.locator('text=系统可用率')).toBeVisible();
  });
});
