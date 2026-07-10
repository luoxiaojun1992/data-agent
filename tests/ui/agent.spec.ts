import { test, expect } from '@playwright/test';

/**
 * SPEC-020: AGENT E2E Tests (UI-039 ~ UI-040)
 *
 * Real API calls only. No page.route() mocks.
 * Only AI model calls use mockllm (SPEC-043).
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const TEST_USER = {
  username: `e2e-agent-${uid}@test.local`,
  password: 'E2eTest123!',
};

test.describe('AGENT — Professional Workspace', () => {
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
  });

  // UI-039: Agent page header and empty state
  test('[UI-039] Agent page header and empty state', async ({ page }) => {
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });

    // Page header visible
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
    await expect(page.locator('text=Agent 任务')).toBeVisible();

    // Empty state — real API returns no tasks
    await expect(page.locator('[data-testid="agent-empty"]')).toBeVisible();
    await expect(page.locator('text=暂无任务')).toBeVisible();
  });

  // UI-040: Available skills section
  test('[UI-040] Agent — skills section', async ({ page }) => {
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });

    // Skills section with all 4 skills
    await expect(page.locator('text=可用技能')).toBeVisible();
    await expect(page.locator('text=sql_executor')).toBeVisible();
    await expect(page.locator('text=stats_engine')).toBeVisible();
    await expect(page.locator('text=knowledge_search')).toBeVisible();
    await expect(page.locator('text=save_report')).toBeVisible();
  });
});
