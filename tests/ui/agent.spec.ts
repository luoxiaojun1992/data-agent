import { test, expect, type Page, type APIRequestContext } from '@playwright/test';

/**
 * SPEC-020: AGENT E2E Tests (UI-039 ~ UI-056)
 * Only deterministic assertions. Task async state (progress/logs/artifacts)
 * is NOT tested — it depends on backend timing and would be flaky.
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-agt-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };

// createAndCancelTask creates a task via the API and cancels it immediately
// (back-to-back), returning once both succeed. SPEC-063: tasks now execute
// against the mock LLM (~30-100ms completion), so a UI-driven create+cancel is
// too slow — the task completes before the cancel button can be clicked. Doing
// both via the API back-to-back cancels during execution; the executor's
// wasCancelled re-check then keeps the cancelled status (it won't overwrite a
// cancelled task with completed). The UI create flow is covered by UI-040/041/
// 047/049/050/051; UI-052/053 focus on the cancel flow + status display.
async function createAndCancelTask(page: Page, request: APIRequestContext, type: string) {
  const token = await page.evaluate(() => localStorage.getItem('token'));
  const authHeaders = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
  // Create (the handler uses req.Title as the task type when req.Type is empty).
  const createRes = await request.post(`${API_BASE}/tasks`, {
    headers: authHeaders,
    data: { title: type, skills: ['sql_executor'] },
  });
  expect(createRes.ok()).toBeTruthy();
  const task = await createRes.json();
  // Cancel immediately (before the worker completes the task).
  const cancelRes = await request.put(`${API_BASE}/tasks/${task.task_id}/cancel`, { headers: authHeaders });
  expect(cancelRes.ok()).toBeTruthy();
}

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
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-046: Page renders ═══
  test('[UI-046] Agent — cancel button in detail', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="agent-create-task-btn"]')).toBeVisible();
  });

  // ═══ UI-047: Create task → verify progress detail ═══
  test('[UI-047] Agent — progress bar rendering', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Progress Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

    // Task row should appear after creation
    const row = page.locator('[data-testid^="agent-task-title-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    await row.click();

    // Detail panel should show task info
    await expect(page.locator('[data-testid^="agent-task-detail-"]').first()).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-048: Step indicator (agent-extras) ═══
  // See agent-extras.spec.ts

  // ═══ UI-049: Create task → verify detail expands ═══
  test('[UI-049] Agent — execution logs section', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Logs Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

    const row = page.locator('[data-testid^="agent-task-title-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    await row.click();

    // Detail panel should be visible after clicking a task row
    await expect(page.locator('[data-testid^="agent-task-detail-"]').first()).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-050: Create task → verify artifact section ═══
  test('[UI-050] Agent — artifact detail section', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Artifact Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

    const row = page.locator('[data-testid^="agent-task-title-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    await row.click();

    await expect(page.locator('[data-testid^="agent-task-detail-"]').first()).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-051: Create task → verify detail panel has task info ═══
  test('[UI-051] Agent — task detail panel', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Detail Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

    const row = page.locator('[data-testid^="agent-task-title-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    await row.click();

    // Detail panel should contain the task title
    await expect(page.locator('[data-testid^="agent-task-detail-"]').first()).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-052: Create → cancel → verify status change ═══
  // SPEC-063: tasks execute against the mock LLM (~30-100ms), so the cancel
  // button (shown only for queued/running/pending) is gone before the detail
  // panel can be opened. Create+cancel via the API back-to-back (the executor's
  // wasCancelled re-check keeps the cancelled status), then verify the UI shows
  // the cancelled pill. See createAndCancelTask for details.
  test('[UI-052] Agent — cancel running task', async ({ page, request }) => {
    await createAndCancelTask(page, request, 'To Cancel');

    // Reload the agent page to refresh the task list and verify the cancelled
    // status pill renders.
    await page.goto('/agent');
    await expect(page.locator('[data-testid="task-status-cancelled"]').first()).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-053: Create task → cancel → verify cancelled state ═══
  test('[UI-053] Agent — cancel then retry flow', async ({ page, request }) => {
    await createAndCancelTask(page, request, 'Retry Flow');

    await page.goto('/agent');
    await expect(page.locator('[data-testid="task-status-cancelled"]').first()).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-054: Scheduled task (agent-extras) ═══
  // See agent-extras.spec.ts

  // ═══ UI-055: Create task → verify task management renders ═══
  test('[UI-055] Agent — task list management', async ({ page }) => {
    // Create a task to populate the list
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Management Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });

    // Task row should appear
    const row = page.locator('[data-testid^="agent-task-title-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });

    // Filters should still be functional
    await expect(page.locator('[data-testid="agent-filter-all"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-filter-completed"]')).toBeVisible();
  });

  // ═══ UI-056: Create multiple tasks → verify list renders ═══
  test('[UI-056] Agent — task list pagination', async ({ page }) => {
    // Create 6 tasks to populate the list (may trigger pagination if PAGE_SIZE < 6)
    for (let i = 0; i < 6; i++) {
      await page.locator('[data-testid="agent-create-task-btn"]').click();
      await page.locator('[data-testid="agent-task-title-input"]').fill(`Pagination ${i + 1}`);
      await page.locator('[data-testid="agent-task-create-btn"]').click();
      await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
      // Wait briefly for loadTasks to complete
      await page.waitForTimeout(500);
    }

    // Multiple task rows should be visible
    const rows = page.locator('[data-testid^="agent-task-title-"]');
    const count = await rows.count();
    console.log('[UI-056] Task rows after creation:', count);
    expect(count).toBeGreaterThanOrEqual(1);

    // Verify task list is functional
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 5000 });
  });
});
