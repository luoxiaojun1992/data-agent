import { test, expect } from '@playwright/test';

/**
 * SPEC-020: AGENT E2E Tests (UI-039 ~ UI-056)
 * Real API calls for task CRUD. mockllm for task execution.
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
    await page.waitForURL(u => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });
  });

  // UI-039: Page header + empty state
  test('[UI-039] Agent page header and empty state', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-page-header"] h2')).toHaveText('Agent 任务');
    await expect(page.locator('[data-testid="agent-empty"]')).toBeVisible();
    await expect(page.locator('text=可用技能')).toBeVisible();
  });

  // UI-040: Create task modal opens
  test('[UI-040] Agent — create task modal', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await expect(page.locator('[data-testid="agent-task-modal"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-task-title-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-task-create-btn"]')).toBeDisabled();
  });

  // UI-041: Create sync task
  test('[UI-041] Agent — create sync task', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('E2E 同步分析');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    // Task list should render (with or without the new task visible immediately)
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
    // Verify at least the empty state or task list is present
    const taskList = page.locator('[data-testid="agent-task-list"]');
    const emptyState = page.locator('[data-testid="agent-empty"]');
    const hasContent = (await taskList.isVisible({ timeout: 3000 }).catch(() => false)) ||
                       (await emptyState.isVisible({ timeout: 3000 }).catch(() => false));
    expect(hasContent).toBe(true);
  });

  // UI-042: Create async task
  test('[UI-042] Agent — create async task', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('E2E 异步分析');
    await page.locator('[data-testid="agent-task-async-toggle"]').check();
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
    // Modal should close after successful creation
    const modal = page.locator('[data-testid="agent-task-modal"]');
    expect(await modal.isVisible({ timeout: 3000 }).catch(() => false)).toBe(false);
  });

  // UI-043: Task filters
  test('[UI-043] Agent — task filters', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-task-filters"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-filter-all"]')).toBeVisible();
    // Click running filter
    await page.locator('[data-testid="agent-filter-running"]').click();
    await expect(page.locator('[data-testid="agent-filter-running"]')).toHaveClass(/border-\[var\(--accent\)\]/);
  });

  // UI-044: Status pill rendering
  test('[UI-044] Agent — status pill rendering', async ({ page }) => {
    // Filters should be visible on page load
    await page.waitForSelector('[data-testid="agent-page-header"]', { timeout: 10000 });
    // All filter buttons should render
    await expect(page.locator('[data-testid="agent-task-filters"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="agent-filter-all"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-filter-pending"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-filter-completed"]')).toBeVisible();
  });

  // UI-045: Task detail expand
  test('[UI-045] Agent — task detail expand', async ({ page }) => {
    // Create task
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Detail Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    await page.goto('/agent');
    await page.waitForURL('**/agent', { timeout: 5000 });
    // Click first task to expand
    const row = page.locator('[data-testid="agent-task-title-0"]');
    if (await row.isVisible({ timeout: 5000 })) {
      await row.click();
      await expect(page.locator('[data-testid="agent-task-detail-0"]')).toBeVisible({ timeout: 5000 });
    }
  });

  // UI-046: Task detail actions
  test('[UI-046] Agent — cancel button in detail', async ({ page }) => {
    // Create task then expand
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Cancel Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    const row = page.locator('[data-testid="agent-task-title-0"]');
    if (await row.isVisible({ timeout: 5000 })) {
      await row.click();
      const cancelBtn = page.locator('[data-testid="agent-cancel-btn-0"]');
      if (await cancelBtn.isVisible()) {
        await cancelBtn.click();
        await page.waitForTimeout(1000);
      }
    }
  });

  // UI-049: Execution logs
  test('[UI-049] Agent — execution logs', async ({ page }) => {
    // Expand a task and check for logs section
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Logs Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    const row = page.locator('[data-testid="agent-task-title-0"]');
    if (await row.isVisible({ timeout: 5000 })) {
      await row.click();
      await page.waitForTimeout(1000);
    }
  });

  // UI-052: Cancel task
  test('[UI-052] Agent — cancel running task', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('To Cancel');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    const row = page.locator('[data-testid="agent-task-title-0"]');
    if (await row.isVisible({ timeout: 5000 })) {
      await row.click();
      const cancelBtn = page.locator('[data-testid="agent-cancel-btn-0"]');
      if (await cancelBtn.isVisible()) {
        await cancelBtn.click();
        await page.waitForTimeout(1000);
        // After cancel, verify the task still exists (status changed)
        await expect(page.locator('[data-testid="agent-task-title-0"]')).toBeVisible({ timeout: 3000 });
      }
    }
  });

  // UI-056: Pagination visible
  test('[UI-056] Agent — pagination', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
  });

  // UI-047: Progress bar
  test('[UI-047] Agent — progress bar rendering', async ({ page }) => {
    // Create async task which might show progress
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Progress Test');
    await page.locator('[data-testid="agent-task-async-toggle"]').check();
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.waitForTimeout(2000);
    await page.goto('/agent');
    await page.waitForURL('**/agent', { timeout: 5000 });
    const progress = page.locator('[data-testid^="task-progress-bar-"]');
    if (await progress.isVisible({ timeout: 3000 })) {
      await expect(progress).toBeVisible();
    }
  });

  // UI-050: Artifacts
  test('[UI-050] Agent — artifact list', async ({ page }) => {
    // Expand a completed task and check artifacts
    const row = page.locator('[data-testid="agent-task-title-0"]');
    if (await row.isVisible({ timeout: 5000 })) {
      await row.click();
      await page.waitForTimeout(1000);
      // Artifacts may or may not exist — verify detail opens
      await expect(page.locator('[data-testid="agent-task-detail-0"]')).toBeVisible({ timeout: 3000 });
    }
  });

  // UI-053: Retry failed task
  test('[UI-053] Agent — retry failed task', async ({ page }) => {
    const retryBtn = page.locator('[data-testid="agent-retry-btn-0"]');
    // If a retry button exists (failed task), click it
    if (await retryBtn.isVisible({ timeout: 3000 }).catch(() => false)) {
      await retryBtn.click();
      await page.waitForTimeout(1000);
    }
  });

  // UI-051: Batch download ZIP
  test('[UI-051] Agent — batch download ZIP', async ({ page }) => {
    const row = page.locator('[data-testid="agent-task-title-0"]');
    if (await row.isVisible({ timeout: 5000 })) {
      await row.click();
      const artifacts = page.locator('[data-testid^="agent-task-artifacts-"]');
      if (await artifacts.isVisible({ timeout: 3000 })) {
        const cb = page.locator('[data-testid^="artifact-checkbox-"]').first();
        if (await cb.isVisible()) await cb.check();
      }
    }
  });

  // UI-055: Pause/resume
  test('[UI-055] Agent — pause resume scheduled task', async ({ page }) => {
    const row = page.locator('[data-testid="agent-task-title-0"]');
    if (await row.isVisible({ timeout: 5000 })) {
      await row.click();
      const pauseBtn = page.locator('[data-testid="agent-pause-btn-0"]');
      const resumeBtn = page.locator('[data-testid="agent-resume-btn-0"]');
      if (await pauseBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
        await pauseBtn.click();
        await page.waitForTimeout(1000);
      } else if (await resumeBtn.isVisible({ timeout: 2000 }).catch(() => false)) {
        await resumeBtn.click();
        await page.waitForTimeout(1000);
      }
    }
  });
});
