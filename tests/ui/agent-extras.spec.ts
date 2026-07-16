import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8); // unique user
const U = { username: `e2e-agt2-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };

test.describe('AGENT — Steps & Cron', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL(u => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });
    await page.waitForSelector('[data-testid="agent-page-header"]', { timeout: 10000 });
  });

  test('[UI-048] Agent — step indicator', async ({ page }) => {
    // Create a task first so there's a row to expand
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Step Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    // Check steps in task detail
    const row = page.locator('[data-testid="agent-task-title-0"]');
    await expect(row).toBeVisible({ timeout: 10000 });
    await row.click();
    await expect(page.locator('[data-testid="agent-step-0"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="agent-step-3"]')).toBeVisible();
  });

  test('[UI-054] Agent — scheduled task creation', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.waitForSelector('[data-testid="agent-task-title-input"]', { timeout: 5000 });
    await page.locator('[data-testid="agent-task-title-input"]').fill('E2E 定时任务');
    await page.locator('[data-testid="agent-task-cron-toggle"]').check();
    await expect(page.locator('[data-testid="agent-task-cron-config"]')).toBeVisible();
    await page.locator('[data-testid="agent-task-cron-select"]').selectOption('0 8 * * *');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
  });
});
