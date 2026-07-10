import { test, expect } from '@playwright/test';

/**
 * SPEC-020: AGENT E2E Tests (UI-039 ~ UI-044 core)
 *
 * Mock agent backend API. Real API login.
 */

const log = (msg: string) => process.stderr.write(`[agent.spec] ${new Date().toISOString()} ${msg}\n`);

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const TEST_USER = {
  username: `e2e-agent-${uid}@test.local`,
  password: 'E2eTest123!',
};

test.describe('AGENT — Professional Workspace', () => {
  test.beforeAll(async ({ request }) => {
    log(`Registering: ${TEST_USER.username}`);
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

  // UI-039: Agent page header and structure
  test('[UI-039] Agent page header and empty state', async ({ page }) => {
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });

    // Page header visible
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
    await expect(page.locator('text=Agent 任务')).toBeVisible();

    // Empty state when no tasks
    await expect(page.locator('[data-testid="agent-empty"]')).toBeVisible();
    await expect(page.locator('text=暂无任务')).toBeVisible();
  });

  // UI-040/041: Create task via mock API and verify it appears
  test('[UI-040] Agent — mock task appears in list', async ({ page }) => {
    // Mock agent tasks API to return a pre-defined list
    await page.route('**/api/v1/agent/tasks', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          tasks: [
            { task_id: 'task-001-sync', title: 'Q3 销售预测分析', status: 'completed', created_at: new Date().toISOString() },
            { task_id: 'task-002-async', title: '全国客户聚类分析', status: 'running', created_at: new Date().toISOString() },
            { task_id: 'task-003-fail', title: '异常检测分析', status: 'failed', created_at: new Date().toISOString() },
          ],
        }),
      });
    });

    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });

    // Task table visible with 3 mock tasks
    const taskTable = page.locator('[data-testid="agent-task-table"]');
    await expect(taskTable).toBeVisible();

    // Verify task titles are displayed
    await expect(page.locator('text=Q3 销售预测分析')).toBeVisible();
    await expect(page.locator('text=全国客户聚类分析')).toBeVisible();
  });

  // UI-043: Filter mock — status badge rendering
  test('[UI-043] Agent — status badge rendering', async ({ page }) => {
    await page.route('**/api/v1/agent/tasks', async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          tasks: [
            { task_id: 'task-010', title: '已完成任务', status: 'completed', created_at: new Date().toISOString() },
            { task_id: 'task-011', title: '运行中任务', status: 'running', created_at: new Date().toISOString() },
            { task_id: 'task-012', title: '失败任务', status: 'failed', created_at: new Date().toISOString() },
          ],
        }),
      });
    });

    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });

    // Verify each status badge appears
    await expect(page.locator('text=已完成')).toBeVisible();
    await expect(page.locator('text=运行中')).toBeVisible();
    await expect(page.locator('text=失败')).toBeVisible();
  });

  // UI-044: Loading state
  test('[UI-044] Agent — loading state', async ({ page }) => {
    // Mock slow response (never resolves in time)
    await page.route('**/api/v1/agent/tasks', async () => {
      await new Promise(() => {}); // hang forever
    });

    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });

    // Loading state should appear
    await expect(page.locator('[data-testid="agent-loading"]')).toBeVisible({ timeout: 3000 });
  });
});
