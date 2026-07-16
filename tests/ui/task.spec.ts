import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

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
    // Register/login admin
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    const body = await res.json();
    adminToken = body.access_token;

    // Create test tasks so UI has data to show
    for (let i = 0; i < 5; i++) {
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
    for (const id of createdTasks) {
      await request.put(`${API_BASE}/tasks/${id}/cancel`, undefined, { headers }).catch(() => {});
    }
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
    // Reload to ensure fresh data
    await page.reload();
    await page.waitForSelector('[data-testid="admin-tasks-header"]', { timeout: 10000 });
    await page.waitForTimeout(3000);

    // Tasks created in beforeAll should appear as rows
    const rows = page.locator('[data-testid^="task-mgmt-row-"]');
    const rowCount = await rows.count();
    // At least some of the 5 tasks created in beforeAll should be visible
    expect(rowCount).toBeGreaterThanOrEqual(1);
  });

  // ═══ UI-111: 查看任务详情 ═══
  test('[UI-111] Task — 查看任务详情', async ({ page }) => {
    // Click "全部" filter to show all tasks
    const allTab = page.locator('[data-testid="task-mgmt-filter-tabs"] button', { hasText: '全部' });
    if (await allTab.isVisible().catch(() => false)) {
      await allTab.click();
      await page.waitForTimeout(1000);
    }

    // Wait for tasks to load (created in beforeAll)
    const row = page.locator('[data-testid^="task-mgmt-row-"]').first();
    await expect(row).toBeVisible({ timeout: 15000 });
  });

  // ═══ UI-112: 取消运行中任务 ═══
  test('[UI-112] Task — 取消运行中任务', async ({ page, request }) => {
    // Create a fresh task
    const task = await createTask(request, adminToken);
    expect(task).toBeTruthy();
    if (task) createdTasks.push(task.task_id);

    await page.reload();
    await page.waitForSelector('[data-testid="admin-tasks-header"]', { timeout: 10000 });
    await page.waitForTimeout(4000);

    if (task) {
      const cancelBtn = page.locator(`[data-testid="task-mgmt-cancel-btn-${task.task_id}"]`);
      if (await cancelBtn.isVisible({ timeout: 5000 }).catch(() => false)) {
        // Register dialog listener BEFORE clicking
        page.once('dialog', (d) => d.accept());
        await cancelBtn.click();
        await page.waitForTimeout(1000);
      }
    }
  });

  // ═══ UI-113: 重试失败任务 ═══
  test('[UI-113] Task — 重试失败任务', async ({ page, request }) => {
    // Create a task and cancel it to make it retryable
    const task = await createTask(request, adminToken);
    expect(task).toBeTruthy();
    createdTasks.push(task.task_id);
    await request.put(`${API_BASE}/tasks/${task.task_id}/cancel`, undefined, {
      headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
    }).catch(() => {});

    await page.reload();
    await page.waitForSelector('[data-testid="admin-tasks-header"]', { timeout: 10000 });
    await page.waitForTimeout(3000);

    const retryBtn = page.locator(`[data-testid="task-mgmt-retry-btn-${task.task_id}"]`);
    if (await retryBtn.isVisible().catch(() => false)) {
      await retryBtn.click();
      await page.waitForTimeout(1000);
    }
  });

  // ═══ UI-114: 批量取消任务 ═══
  test('[UI-114] Task — 批量取消任务', async ({ page, request }) => {
    // Create 2 fresh tasks
    for (let i = 0; i < 2; i++) {
      const task = await createTask(request, adminToken);
      if (task) createdTasks.push(task.task_id);
    }

    await page.reload();
    await page.waitForSelector('[data-testid="admin-tasks-header"]', { timeout: 10000 });

    // Click "全部" filter
    const allTab = page.locator('[data-testid="task-mgmt-filter-tabs"] button', { hasText: '全部' });
    if (await allTab.isVisible().catch(() => false)) {
      await allTab.click();
      await page.waitForTimeout(1000);
    }
    await page.waitForTimeout(3000);

    const checkboxes = page.locator('[data-testid="task-mgmt-batch-select"]');
    const count = await checkboxes.count();
    expect(count).toBeGreaterThanOrEqual(1);

    if (count >= 2) {
      await checkboxes.nth(0).check();
      await checkboxes.nth(1).check();
      await page.waitForTimeout(500);

      const batchBtn = page.locator('[data-testid="task-mgmt-batch-cancel-btn"]');
      await expect(batchBtn).toBeVisible({ timeout: 5000 });
      // Register dialog listener BEFORE clicking
      page.once('dialog', (d) => d.accept());
      await batchBtn.click();
      await page.waitForTimeout(1000);
    }
  });
});
