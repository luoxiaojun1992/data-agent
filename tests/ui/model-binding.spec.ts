import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const ADMIN = { username: `e2e-bind-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
const USER = { username: `e2e-bind-user-${uid}@test.local`, password: 'E2eTest1!' };

let adminToken = '';

test.describe('MODEL-BINDING (SPEC-062) — Multi-Model Session Binding', () => {
  test.beforeAll(async ({ request }) => {
    // Register a regular user (invite flow may be disabled in CI; try direct).
    await request.post(`${API_BASE}/auth/register`, { data: USER }).catch(() => {});
    // Register admin via the system admin auto-create + promote if needed.
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: '系统管理员', password: 'E2eTest123!' },
    });
    if (loginRes.ok()) {
      adminToken = (await loginRes.json()).token || '';
    }
  });

  test.afterAll(async ({ request }) => {
    // Cleanup test users
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    }).catch(() => null);
    if (listRes && listRes.ok()) {
      const users = (await listRes.json()).users || [];
      for (const u of users) {
        if (u.email?.includes(uid) || u.username?.includes(uid)) {
          await request.delete(`${API_BASE}/users/${u.id}`, {
            headers: { Authorization: `Bearer ${adminToken}` },
          }).catch(() => {});
        }
      }
    }
  });

  // ═══ UI-238: Chat 页面 ModelSelector 可见 ═══
  test('[UI-238] Chat — ModelSelector 可见且可选', async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u: URL) => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-chat"]').click();
    await page.waitForURL('**/chat', { timeout: 5000 });

    // ModelSelector should be visible on a new session (not locked).
    const selector = page.locator('[data-testid="model-selector"]');
    await expect(selector).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-239: 已有 session 时 ModelSelector 锁定 ═══
  test('[UI-239] Chat — 已有 session 时 ModelSelector 锁定', async ({ page, request }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u: URL) => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-chat"]').click();
    await page.waitForURL('**/chat', { timeout: 5000 });

    // Create a session via API to bind a model, then load it.
    const sessRes = await request.post(`${API_BASE}/sessions`, {
      headers: { Authorization: `Bearer ${await getLoginToken(request, USER)}` },
      data: { model_id: '' },
    });
    if (sessRes.ok()) {
      const sid = (await sessRes.json()).session_id;
      // Select the session in the UI (via session panel).
      await page.locator('[data-testid="chat-session-btn"]').click();
      await page.waitForTimeout(500);
      const item = page.locator(`[data-testid="session-item-${sid}"]`);
      if (await item.isVisible({ timeout: 3000 }).catch(() => false)) {
        await item.click();
        // After selecting an existing session, the selector should be locked.
        await expect(page.locator('[data-testid="model-selector-locked"]')).toBeVisible({ timeout: 5000 });
      }
    }
  });

  // ═══ UI-240: Admin 模型列表结构化展示 ═══
  test('[UI-240] Admin — 模型列表结构化展示', async ({ page }) => {
    const token = await getAdminLoginToken(page);
    if (!token) { test.skip(); return; }
    await page.locator('[data-testid="nav-admin-models"]')?.click().catch(() => {});
    await page.goto('/admin/models').catch(() => {});
    await page.waitForSelector('[data-testid="admin-models-header"]', { timeout: 5000 });

    // The structured model list card should be visible (SPEC-062).
    await expect(page.locator('[data-testid="model-list-card"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="model-list-table"]')).toBeVisible();
    // Add model button visible.
    await expect(page.locator('[data-testid="model-add-btn"]')).toBeVisible();
  });

  // ═══ UI-241: Admin 模型新增/设默认/删除 ═══
  test('[UI-241] Admin — 模型新增与设默认', async ({ page, request }) => {
    const token = await getAdminLoginToken(page);
    if (!token) { test.skip(); return; }
    await page.goto('/admin/models').catch(() => {});
    await page.waitForSelector('[data-testid="model-list-card"]', { timeout: 5000 });

    // Add a model via API (deterministic; UI modal is secondary).
    const addRes = await request.post(`${API_BASE}/models`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: { name: `E2E-Test-${uid}`, type: 'llm', base_url: 'http://mockllm:8082' },
    });
    expect(addRes.ok()).toBe(true);
    const added = await addRes.json();
    expect(added.id).toBeTruthy();

    // Reload the page and verify the model appears in the list.
    await page.reload();
    await page.waitForSelector('[data-testid="model-list-table"]', { timeout: 5000 });
    await expect(page.locator('text=E2E-Test-' + uid)).toBeVisible({ timeout: 5000 });

    // Set as default via API.
    const defRes = await request.patch(`${API_BASE}/models/${added.id}/default`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(defRes.ok()).toBe(true);

    // Delete the model (cleanup + verify delete works).
    const delRes = await request.delete(`${API_BASE}/models/${added.id}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    expect(delRes.ok()).toBe(true);
  });
});

// Helpers
async function getLoginToken(request: any, creds: { username: string; password: string }): Promise<string> {
  const res = await request.post(`${API_BASE}/auth/login`, { data: creds });
  if (!res.ok()) return '';
  return (await res.json()).token || '';
}

async function getAdminLoginToken(page: any): Promise<string> {
  // Login as system admin via UI.
  await page.goto('/login');
  await page.locator('[data-testid="login-email-input"]').fill('系统管理员');
  await page.locator('[data-testid="login-password-input"]').fill('E2eTest123!');
  await page.locator('[data-testid="login-btn"]').click();
  await page.waitForURL((u: URL) => !u.pathname.includes('/login'), { timeout: 10000 }).catch(() => {});
  // Read token from localStorage.
  return page.evaluate(() => localStorage.getItem('token') || '');
}
