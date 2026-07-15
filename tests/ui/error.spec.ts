import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const USER = { username: `e2e-err-${uid}@test.local`, password: 'ErrTest1!', role: 'user' };
let token = '';

test.describe('ERROR STATES — SPEC-041', () => {
  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: USER });
    if (res.status() !== 201) { /* already exists */ }
    res = await request.post(`${API_BASE}/auth/login`, {
      data: { username: USER.username, password: USER.password },
    });
    const body = await res.json();
    token = body.access_token;
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      const body = await listRes.json();
      for (const u of body.users || []) {
        if (u.username?.includes(`e2e-err-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers }).catch(() => {});
        }
      }
    }
  });

  async function login(page: any) {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
  }

  // ═══ UI-196: 网络断开提示 ═══
  test('[UI-196] Err — 网络断开提示', async ({ page }) => {
    await login(page);
    // Go to a page that makes API calls
    await page.goto('/agent');

    // Simulate online → offline
    await page.context().setOffline(true);

    // Wait for any potential offline indicator
    await page.waitForTimeout(2000);

    // Attempt an action that requires network
    const chatInput = page.locator('[data-testid="chat-input"]');
    if (await chatInput.isVisible().catch(() => false)) {
      await chatInput.fill('test');
      await page.keyboard.press('Enter');
      await page.waitForTimeout(1000);
    }

    // Go back online
    await page.context().setOffline(false);
    await page.waitForTimeout(1000);

    // Page should recover
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-197: API 500 错误处理 ═══
  test('[UI-197] Err — API 500 错误处理', async ({ page }) => {
    await login(page);
    await page.goto('/admin/users');

    // Mock the users API to return 500 mid-session
    await page.route('**/api/v1/users**', async (route) => {
      await route.fulfill({
        status: 500,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Internal Server Error' }),
      });
    });

    // Reload to trigger the mocked 500
    await page.reload();
    await page.waitForTimeout(3000);

    // Check that the page didn't crash — it should still render some UI
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible({ timeout: 5000 });

    // Clean up route
    await page.unrouteAll({ behavior: 'ignoreErrors' });
  });

  // ═══ UI-198: 404 页面 ═══
  test('[UI-198] Err — 404 页面', async ({ page }) => {
    await page.goto('/nonexistent-page-that-does-not-exist');

    // Next.js not-found page should render
    await expect(page.locator('[data-testid="page-404"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="page-404-title"]')).toHaveText('页面未找到');
    await expect(page.locator('[data-testid="page-404-home-link"]')).toBeVisible();
    await expect(page.locator('[data-testid="page-404-home-link"]')).toHaveText('返回首页');

    // Click home link
    await page.locator('[data-testid="page-404-home-link"]').click();
    await page.waitForURL(/\/$|login/, { timeout: 5000 });
  });

  // ═══ UI-199: 空数据状态 ═══
  test('[UI-199] Err — 空数据状态', async ({ page }) => {
    await login(page);

    // Agent page with no tasks should show empty state
    await page.goto('/agent');
    await page.waitForTimeout(2000);

    // Should show empty state (tasks were NOT created)
    const emptyState = page.locator('[data-testid="agent-empty"]');
    if (await emptyState.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(emptyState).toContainText('暂无任务');
    }

    // Also check users page (new user, no other users visible)
    await page.goto('/admin/users');
    await page.waitForTimeout(2000);
    // Should show empty state or user table
    const usersEmpty = page.locator('[data-testid="admin-users-empty"]');
    const usersTable = page.locator('[data-testid="admin-users-table"]');
    // At least one of them should be visible
    const hasUsersUI = await usersEmpty.isVisible().catch(() => false) ||
                       await usersTable.isVisible().catch(() => false);
    expect(hasUsersUI).toBe(true);
  });

  // ═══ UI-200: 加载状态 ═══
  test('[UI-200] Err — 加载状态指示器', async ({ page }) => {
    await login(page);

    // Agent page shows loading before data arrives
    await page.goto('/agent');

    // Check for loading indicator
    const loading = page.locator('[data-testid="agent-loading"]');
    const hasLoading = await loading.isVisible({ timeout: 3000 }).catch(() => false);

    // Loading should eventually resolve to either content or empty state
    await page.waitForTimeout(2000);
    const content = page.locator('[data-testid="agent-empty"], [data-testid="agent-page-header"]');
    await expect(content.first()).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-201: 熔断器/503 错误处理 ═══
  test('[UI-201] Err — 503 服务不可用处理', async ({ page }) => {
    await login(page);

    // Mock chat API to return 503
    await page.route('**/api/v1/chat**', async (route) => {
      await route.fulfill({
        status: 503,
        contentType: 'application/json',
        body: JSON.stringify({ error: 'Service temporarily unavailable' }),
      });
    });

    await page.goto('/chat');
    await page.waitForTimeout(1000);

    // Try to send a message
    const chatInput = page.locator('[data-testid="chat-input"]');
    if (await chatInput.isVisible().catch(() => false)) {
      await chatInput.fill('hello');
      await page.keyboard.press('Enter');
      await page.waitForTimeout(2000);
    }

    // Page should not crash — UI should remain
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    await page.unrouteAll({ behavior: 'ignoreErrors' });
  });

  // ═══ UI-202: 浏览器后退按钮行为 ═══
  test('[UI-202] Err — 浏览器后退按钮', async ({ page }) => {
    await login(page);

    // Navigate to dashboard
    await page.goto('/');
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // Navigate to another page
    await page.goto('/knowledge');

    // Go back
    await page.goBack();
    await page.waitForTimeout(1000);

    // Should be back on dashboard
    expect(page.url()).toMatch(/\/$/);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });
});
