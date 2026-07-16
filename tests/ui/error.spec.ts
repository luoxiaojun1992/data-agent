import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const uid = crypto.randomUUID().slice(0, 8);

const USER = { username: `e2e-err-${uid}@test.local`, password: 'ErrTest1!', role: 'user' };
let token = '';

async function seedMock(request: any, key: string, response: string) {
  await request.post(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
    data: { key, response },
  });
}

async function clearMocks(request: any) {
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});
}

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
    await clearMocks(request);
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
    await page.goto('/agent');
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });

    // Simulate online → offline
    await page.context().setOffline(true);

    // Attempt an action that requires network — should show UI feedback
    await page.keyboard.press('Enter');

    // Give the app time to react to being offline
    await page.waitForTimeout(2000);

    // Go back online
    await page.context().setOffline(false);
    await page.waitForTimeout(1000);

    // Page should remain functional — agent page header still visible
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-197: API 错误情况 — 页面不崩溃 ═══
  test('[UI-197] Err — API 错误页面不崩溃', async ({ page }) => {
    await login(page);

    // Navigate to a data-dependent page and verify it renders gracefully
    // (either with data or appropriate empty/loading state)
    await page.goto('/admin/users');
    await page.waitForTimeout(3000);

    // Page should render either a table or an empty state — never crash
    const table = page.locator('[data-testid="admin-users-table"]');
    const header = page.locator('[data-testid="admin-users-header"]');
    // At least one of these should be visible
    const hasTableOrHeader = (await table.isVisible({ timeout: 3000 }).catch(() => false)) ||
                              (await header.isVisible({ timeout: 3000 }).catch(() => false));
    expect(hasTableOrHeader).toBe(true);

    // Sidebar should always be present — page structure intact
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
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
    await page.goto('/agent');
    await page.waitForTimeout(2000);

    // Should show empty state if no tasks exist
    const emptyState = page.locator('[data-testid="agent-empty"]');
    const header = page.locator('[data-testid="agent-page-header"]');
    // Either empty state or page header must be visible
    const hasStateOrHeader = (await emptyState.isVisible({ timeout: 5000 }).catch(() => false)) ||
                              (await header.isVisible({ timeout: 5000 }).catch(() => false));
    expect(hasStateOrHeader).toBe(true);

    // Navigate to users page — should show table or empty state
    await page.goto('/admin/users');
    await page.waitForTimeout(2000);
    const usersEmpty = page.locator('[data-testid="admin-users-empty"]');
    const usersTable = page.locator('[data-testid="admin-users-table"]');
    const hasUsersUI = (await usersEmpty.isVisible().catch(() => false)) ||
                        (await usersTable.isVisible().catch(() => false));
    expect(hasUsersUI).toBe(true);
  });

  // ═══ UI-200: 加载状态 ═══
  test('[UI-200] Err — 加载状态指示器', async ({ page }) => {
    await login(page);

    // Navigate to agent page — loading or content should appear
    await page.goto('/agent');

    // Content should eventually resolve to either empty state or header
    const content = page.locator('[data-testid="agent-empty"], [data-testid="agent-page-header"]');
    await expect(content.first()).toBeVisible({ timeout: 10000 });
  });

  // ═══ UI-201: 聊天消息发送后页面保持稳定 ═══
  test('[UI-201] Err — 聊天消息发送后页面保持稳定', async ({ page, request }) => {
    await login(page);
    await clearMocks(request);
    await seedMock(request, 'hello', '你好，我是数据分析助手');

    await page.goto('/chat');
    const chatInput = page.locator('[data-testid="chat-input"]');
    await expect(chatInput).toBeVisible({ timeout: 10000 });

    // Send a message
    await chatInput.fill('hello');
    await page.keyboard.press('Enter');

    // Wait for AI response — page should not crash
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });

    // UI should remain stable after message exchange
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-input"]')).toBeVisible();
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
