import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const USER = { username: `e2e-resp-${uid}@test.local`, password: 'RespTest1', role: 'user' };
let token = '';

test.describe('RESPONSIVE — SPEC-040', () => {
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
    // Cleanup
    const adminHeaders = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers: adminHeaders });
    if (listRes.ok()) {
      const body = await listRes.json();
      for (const u of body.users || []) {
        if (u.username?.includes(`e2e-resp-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers: adminHeaders }).catch(() => {});
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

  // ═══ UI-193: 移动端布局适配 ═══
  test('[UI-193] Resp — 移动端布局适配 (375px)', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 }); // iPhone X
    await loginAndGoHome(page);

    // Sidebar should be hidden by default on mobile
    await expect(page.locator('[data-testid="sidebar"]')).not.toBeVisible();

    // Hamburger button should be visible
    await expect(page.locator('[data-testid="sidebar-hamburger"]')).toBeVisible();

    // Main content should fill full width (no ml-60)
    const mainContent = page.locator('[data-testid="main-content"]');
    await expect(mainContent).toBeVisible();
    const box = await mainContent.boundingBox();
    expect(box!.x).toBeLessThan(10); // No left margin on mobile

    // Open sidebar via hamburger
    await page.locator('[data-testid="sidebar-hamburger"]').click();
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();

    // Overlay should appear
    await expect(page.locator('[data-testid="sidebar-overlay"]')).toBeVisible();

    // Close sidebar via overlay
    await page.locator('[data-testid="sidebar-overlay"]').click();
    await expect(page.locator('[data-testid="sidebar"]')).not.toBeVisible();

    // Open and close via close button
    await page.locator('[data-testid="sidebar-hamburger"]').click();
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
    await page.locator('[data-testid="sidebar-close"]').click();
    await expect(page.locator('[data-testid="sidebar"]')).not.toBeVisible();
  });

  // ═══ UI-193b: 移动端登录页适配 ═══
  test('[UI-193b] Resp — 移动端登录卡片适配', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await page.goto('/login');

    // Login card should fit within viewport
    const loginCard = page.locator('[data-testid="login-card"]');
    await expect(loginCard).toBeVisible();
    const cardBox = await loginCard.boundingBox();
    expect(cardBox!.width).toBeLessThanOrEqual(375);
  });

  // ═══ UI-194: 平板布局适配 ═══
  test('[UI-194] Resp — 平板布局适配 (768px)', async ({ page }) => {
    await page.setViewportSize({ width: 768, height: 1024 }); // iPad Mini
    await loginAndGoHome(page);

    // At 768px (md breakpoint, below lg), sidebar should still be hidden by default
    await expect(page.locator('[data-testid="sidebar"]')).not.toBeVisible();
    await expect(page.locator('[data-testid="sidebar-hamburger"]')).toBeVisible();

    // Open and verify sidebar works on tablet too
    await page.locator('[data-testid="sidebar-hamburger"]').click();
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
    await page.locator('[data-testid="sidebar-close"]').click();
  });

  // ═══ UI-194b: 桌面端布局 ═══
  test('[UI-194b] Resp — 桌面端布局 (1024px+)', async ({ page }) => {
    await page.setViewportSize({ width: 1440, height: 900 });
    await loginAndGoHome(page);

    // At lg breakpoint (1024px+), sidebar should be always visible
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();

    // Hamburger should be hidden on desktop
    await expect(page.locator('[data-testid="sidebar-hamburger"]')).not.toBeVisible();

    // Overlay should not exist
    await expect(page.locator('[data-testid="sidebar-overlay"]')).not.toBeVisible();

    // Main content has left margin for sidebar
    const mainContent = page.locator('[data-testid="main-content"]');
    const box = await mainContent.boundingBox();
    expect(box!.x).toBeGreaterThan(100); // ml-60 = 240px
  });

  // ═══ UI-195: 触摸友好交互 ═══
  test('[UI-195] Resp — 触摸友好交互 (tap targets)', async ({ page }) => {
    await page.setViewportSize({ width: 375, height: 812 });
    await loginAndGoHome(page);

    // Hamburger button should be ≥ 44px (Apple HIG minimum)
    const hamburger = page.locator('[data-testid="sidebar-hamburger"]');
    const hbBox = await hamburger.boundingBox();
    expect(hbBox!.width).toBeGreaterThanOrEqual(44);
    expect(hbBox!.height).toBeGreaterThanOrEqual(44);

    // Sidebar close button should be ≥ 44px
    await hamburger.click();
    const closeBtn = page.locator('[data-testid="sidebar-close"]');
    const cbBox = await closeBtn.boundingBox();
    expect(cbBox!.width).toBeGreaterThanOrEqual(44);
    expect(cbBox!.height).toBeGreaterThanOrEqual(44);

    // Navigation items should have adequate height for touch
    const navItem = page.locator('[data-testid="nav-dashboard"]');
    const niBox = await navItem.boundingBox();
    expect(niBox!.height).toBeGreaterThanOrEqual(36);
  });
});
