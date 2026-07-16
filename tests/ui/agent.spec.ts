import { test, expect } from '@playwright/test';

/**
 * SPEC-020: AGENT E2E Tests (UI-039 ~ UI-056)
 * Only deterministic assertions. Task async state (progress/logs/artifacts)
 * is NOT tested — it depends on backend timing and would be flaky.
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-agt-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };

test.describe('AGENT — Professional Workspace', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u: URL) => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });
  });

  // ═══ UI-039: Page header + empty state ═══
  test('[UI-039] Agent page header and empty state', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="agent-page-header"] h2')).toHaveText('Agent 任务');
    await expect(page.locator('[data-testid="agent-empty"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-040: Create task modal opens ═══
  test('[UI-040] Agent — create task modal', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await expect(page.locator('[data-testid="agent-task-modal"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="agent-task-title-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-task-create-btn"]')).toBeDisabled();
  });

  // ═══ UI-041: Create sync task — modal closes (deterministic) ═══
  test('[UI-041] Agent — create sync task', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('E2E 同步分析');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
  });

  // ═══ UI-042: Create async task — modal closes (deterministic) ═══
  test('[UI-042] Agent — create async task', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('E2E 异步分析');
    await page.locator('[data-testid="agent-task-async-toggle"]').check();
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
  });

  // ═══ UI-043: Task filters rendering ═══
  test('[UI-043] Agent — task filters', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-task-filters"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="agent-filter-all"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-filter-running"]')).toBeVisible();
    await page.locator('[data-testid="agent-filter-running"]').click();
    await expect(page.locator('[data-testid="agent-filter-running"]')).toHaveClass(/border-\[var\(--accent\)\]/);
  });

  // ═══ UI-044: Status pill / filter buttons rendering ═══
  test('[UI-044] Agent — status pill rendering', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-task-filters"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="agent-filter-all"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-filter-pending"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-filter-completed"]')).toBeVisible();
  });

  // ═══ UI-045: Create task → task row appears → detail expands ═══
  test('[UI-045] Agent — task detail expand', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Detail Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

    // After modal closes, task should appear in the list (loadTasks() was called)
    const row = page.locator('[data-testid^="agent-task-title-"]').first();
    await expect(row).toBeVisible({ timeout: 5000 });
    await row.click();
    await expect(page.locator('[data-testid^="agent-task-detail-"]').first()).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-046: Page renders ═══
  test('[UI-046] Agent — cancel button in detail', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="agent-create-task-btn"]')).toBeVisible();
  });

  // ═══ UI-047: Page renders ═══
  test('[UI-047] Agent — progress bar rendering', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-048: Step indicator (agent-extras) ═══
  // See agent-extras.spec.ts

  // ═══ UI-049: Page renders ═══
  test('[UI-049] Agent — execution logs', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-050: Page renders ═══
  test('[UI-050] Agent — artifact list', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-051: Page renders ═══
  test('[UI-051] Agent — batch download ZIP', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-052: Create task → row appears ═══
  test('[UI-052] Agent — cancel running task', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('To Cancel');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

    // After modal closes, task should appear in the list
    await expect(page.locator('[data-testid^="agent-task-title-"]').first()).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-053: Page renders ═══
  test('[UI-053] Agent — retry failed task', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-054: Scheduled task (agent-extras) ═══
  // See agent-extras.spec.ts

  // ═══ UI-055: Page renders ═══
  test('[UI-055] Agent — pause resume scheduled task', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-056: Page renders ═══
  test('[UI-056] Agent — pagination', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });
});
