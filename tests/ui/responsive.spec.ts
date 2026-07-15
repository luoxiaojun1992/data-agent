import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const USER = { username: `e2e-resp-${uid}@test.local`, password: 'RespTest1!', role: 'admin' };
let token = '';

test.describe('RESPONSIVE — SPEC-040', () => {
  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: USER });
    if (res.status() !== 201) { /* ok */ }
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
        if (u.username?.includes(`e2e-resp-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers }).catch(() => {});
        }
      }
    }
  });

  async function loginAndGoHome(page: any) {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
  }

  // ═══ UI-193: 移动端 ═══
  test('[UI-193] Resp — 移动端布局适配 (375px)', async ({ page }) => {
    // Login on wider viewport first (avoids mobile form clipping)
    await page.setViewportSize({ width: 1024, height: 768 });
    await loginAndGoHome(page);
    // Then resize to mobile
    await page.setViewportSize({ width: 375, height: 812 });
    await page.waitForTimeout(500);

    // On mobile, sidebar should be off-screen (translated left)
    const sidebar = page.locator('[data-testid="sidebar"]');
    const sidebarBox = await sidebar.boundingBox();
    // Sidebar should be positioned off-screen (x < 0) when closed on mobile
    expect(sidebarBox!.x).toBeLessThan(0);

    // Hamburger should be visible
    await expect(page.locator('[data-testid="sidebar-hamburger"]')).toBeVisible();

    // Open sidebar
    await page.locator('[data-testid="sidebar-hamburger"]').click();
    await page.waitForTimeout(300);

    // Sidebar should now be on-screen
    const openBox = await sidebar.boundingBox();
    expect(openBox!.x).toBeGreaterThanOrEqual(0);

    // Overlay should be visible
    await expect(page.locator('[data-testid="sidebar-overlay"]')).toBeVisible();

    // Close via overlay
    await page.locator('[data-testid="sidebar-overlay"]').click();
    await page.waitForTimeout(300);
    const closedBox = await sidebar.boundingBox();
    expect(closedBox!.x).toBeLessThan(0);
  });

  // ═══ UI-193b: 登录页移动适配 ═══
  test('[UI-193b] Resp — 移动端登录卡片适配', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/login');
    const loginCard = page.locator('[data-testid="login-card"]');
    await expect(loginCard).toBeVisible();
    const box = await loginCard.boundingBox();
    expect(box!.width).toBeLessThanOrEqual(375);
  });

  // ═══ UI-194: 平板 ═══
  test('[UI-194] Resp — 平板布局适配 (768px)', async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 768 });
    await loginAndGoHome(page);
    await page.setViewportSize({ width: 768, height: 1024 });
    await page.waitForTimeout(500);

    // At 768px (below lg=1024), sidebar should be off-screen
    const sidebar = page.locator('[data-testid="sidebar"]');
    const box = await sidebar.boundingBox();
    expect(box!.x).toBeLessThan(0);

    // Hamburger visible
    await expect(page.locator('[data-testid="sidebar-hamburger"]')).toBeVisible();
  });

  // ═══ UI-194b: 桌面 ═══
  test('[UI-194b] Resp — 桌面端布局 (1440px)', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await loginAndGoHome(page);

    // On desktop, sidebar always visible on-screen
    const sidebar = page.locator('[data-testid="sidebar"]');
    const box = await sidebar.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);

    // Hamburger hidden
    await expect(page.locator('[data-testid="sidebar-hamburger"]')).not.toBeVisible();
  });

  // ═══ UI-195: 触摸友好 ═══
  test('[UI-195] Resp — 触摸友好交互 (tap targets)', async ({ page }) => {
    await page.setViewportSize({ width: 1024, height: 768 });
    await loginAndGoHome(page);
    await page.setViewportSize({ width: 375, height: 812 });
    await page.waitForTimeout(500);

    const hamburger = page.locator('[data-testid="sidebar-hamburger"]');
    const hbBox = await hamburger.boundingBox();
    expect(hbBox!.width).toBeGreaterThanOrEqual(32);
    expect(hbBox!.height).toBeGreaterThanOrEqual(32);
  });
});
