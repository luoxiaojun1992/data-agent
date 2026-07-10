import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const TEST_USER = { username: `e2e-hms-${uid}@test.local`, password: 'E2eTest123!' };

test.describe('HERMES — Free Exploration', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: TEST_USER })).status()).toBe(201);
  });
  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(TEST_USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(TEST_USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-hermes"]').click();
    await page.waitForURL('**/hermes', { timeout: 5000 });
  });

  test('[UI-057] Hermes — page header and search area', async ({ page }) => {
    await expect(page.locator('[data-testid="hermes-page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-page-title"]')).toHaveText('Hermes 自由探索');
    await expect(page.locator('[data-testid="hermes-search-area"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-query-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-submit-btn"]')).toHaveText('执行查询');
  });
});
