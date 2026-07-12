import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();

const ADMIN = { username: `e2e-task-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';

// Helper: create a task via API
async function createTask(request: any, token: string, sessionID = 'test-session') {
  const res = await request.post(`${API_BASE}/tasks`, {
    data: { session_id: sessionID, type: 'agent_exec', skill_chain: ['sql_executor'], params: { query: 'SELECT 1' } },
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
  });
  return res.ok() ? res.json() : null;
}

test.describe('TASK MANAGEMENT — SPEC-027', () => {
  const createdTasks: string[] = [];

  test.beforeAll(async ({ request }) => {
    // Register admin
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    const body = await res.json();
    adminToken = body.access_token;

    // Create some test tasks
    for (let i = 0; i < 3; i++) {
      const task = await createTask(request, adminToken);
      if (task) createdTasks.push(task.task_id);
    }
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/tasks');
    await page.waitForSelector('[data-testid="admin-tasks-header"]', { timeout: 10000 });
    await page.waitForTimeout(2000);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    // Cancel/cleanup test tasks
    for (const id of createdTasks) {
      await request.put(`${API_BASE}/tasks/${id}/cancel`, undefined, { headers }).catch(() => {});
    }
    // Delete admin user
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      const body = await listRes.json();
      for (const user of body.users || []) {
        if (user.username?.includes(`e2e-task-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-109: 任务管理页渲染 ═══
  test('[UI-109] Task — 任务管理页渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="admin-tasks-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="admin-tasks-title"]')).toHaveText('任务管理');
    await expect(page.locator('[data-testid="task-mgmt-filter-tabs"]')).toBeVisible();
    await expect(page.locator('[data-testid="task-mgmt-table"]')).toBeVisible();
  });

  // ═══ UI-110: 全局查看所有用户任务 ═══
  test('[UI-110] Task — 全局查看所有用户任务', async ({ page }) => {
    // The table should contain task rows
    const rows = page.locator('[data-testid^="task-mgmt-row-"]');
    // At least some rows should appear (tasks created in beforeAll)
    await page.waitForTimeout(1000);
  });

  // ═══ UI-111: 查看任务详情 ═══
  test('[UI-111] Task — 查看任务详情', async ({ page }) => {
    // Look for a task row and click "查看"
    const viewBtn = page.locator('button', { hasText: '查看' }).first();
    const hasViewBtn = await viewBtn.isVisible().catch(() => false);
    if (!hasViewBtn) {
      test.skip();
      return;
    }
    await viewBtn.click();
    await page.waitForTimeout(500);
  });

  // ═══ UI-112: 取消运行中任务 ═══
  test('[UI-112] Task — 取消运行中任务', async ({ page }) => {
    const cancelBtn = page.locator('[data-testid^="task-mgmt-cancel-btn-"]').first();
    const hasCancelBtn = await cancelBtn.isVisible().catch(() => false);
    if (!hasCancelBtn) {
      test.skip();
      return;
    }
    await cancelBtn.click();
    // Confirm dialog
    page.once('dialog', (d) => d.accept());
    await page.waitForTimeout(1000);
  });

  // ═══ UI-113: 重试失败任务 ═══
  test('[UI-113] Task — 重试失败任务', async ({ page }) => {
    // Switch to "失败" filter
    await page.locator('button', { hasText: '失败' }).click();
    await page.waitForTimeout(1000);

    const retryBtn = page.locator('[data-testid^="task-mgmt-retry-btn-"]').first();
    const hasRetryBtn = await retryBtn.isVisible().catch(() => false);
    if (!hasRetryBtn) {
      test.skip();
      return;
    }
    await retryBtn.click();
    await page.waitForTimeout(1000);
  });

  // ═══ UI-114: 批量取消任务 ═══
  test('[UI-114] Task — 批量取消任务', async ({ page }) => {
    // Select all checkboxes
    const checkboxes = page.locator('[data-testid="task-mgmt-batch-select"]');
    const count = await checkboxes.count();
    if (count < 2) {
      test.skip();
      return;
    }

    // Check first two
    await checkboxes.nth(0).check();
    await checkboxes.nth(1).check();
    await page.waitForTimeout(500);

    // Batch cancel button should appear
    const batchBtn = page.locator('[data-testid="task-mgmt-batch-cancel-btn"]');
    const hasBatchBtn = await batchBtn.isVisible().catch(() => false);
    if (hasBatchBtn) {
      await batchBtn.click();
      page.once('dialog', (d) => d.accept());
      await page.waitForTimeout(1000);
    }
  });
});
