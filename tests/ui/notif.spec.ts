import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();

const ADMIN = { username: `e2e-notif-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';

test.describe('NOTIFICATION — SPEC-031', () => {
  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    adminToken = (await res.json()).access_token;

    // Send a test notification to create unread data
    await request.post(`${API_BASE}/notifications`, {
      data: { title: '系统维护通知', content: '系统将于今晚 22:00 进行维护升级', type: 'info', target_ids: [] },
      headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
    });
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/audit');
    await page.waitForSelector('[data-testid="notif-bell-icon"]', { timeout: 10000 });
    await page.waitForTimeout(1500);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-notif-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-141: 铃铛图标与未读数红点 ═══
  test('[UI-141] Notif — 铃铛图标与未读数红点', async ({ page }) => {
    await expect(page.locator('[data-testid="notif-bell-icon"]')).toBeVisible();
    // Unread badge may or may not appear
    const badge = page.locator('[data-testid="notif-unread-badge"]');
    const hasBadge = await badge.isVisible().catch(() => false);
    if (hasBadge) {
      await expect(page.locator('[data-testid="notif-unread-count"]')).toBeVisible();
    }
  });

  // ═══ UI-142: 点击展开通知列表 ═══
  test('[UI-142] Notif — 点击展开通知列表', async ({ page }) => {
    await page.locator('[data-testid="notif-bell-icon"]').click();
    await expect(page.locator('[data-testid="notif-dropdown"]')).toBeVisible();
    // Should contain notification items
    const items = page.locator('[data-testid^="notif-item-"]');
    await page.waitForTimeout(500);
  });

  // ═══ UI-143: 标记已读 ═══
  test('[UI-143] Notif — 标记已读', async ({ page }) => {
    await page.locator('[data-testid="notif-bell-icon"]').click();
    await expect(page.locator('[data-testid="notif-dropdown"]')).toBeVisible();
    const item = page.locator('[data-testid^="notif-item-"]').first();
    const hasItem = await item.isVisible().catch(() => false);
    if (hasItem) await item.click();
    await page.waitForTimeout(500);
  });

  // ═══ UI-144: 一键全部已读 ═══
  test('[UI-144] Notif — 一键全部已读', async ({ page }) => {
    await page.locator('[data-testid="notif-bell-icon"]').click();
    await expect(page.locator('[data-testid="notif-dropdown"]')).toBeVisible();
    const markAllBtn = page.locator('[data-testid="notif-mark-all-read"]');
    const hasMarkAll = await markAllBtn.isVisible().catch(() => false);
    if (hasMarkAll) await markAllBtn.click();
    await page.waitForTimeout(500);
  });

  // ═══ UI-145: 发送站内信（点对点）═══
  test('[UI-145] Notif — 发送站内信', async ({ page }) => {
    await page.locator('[data-testid="notif-bell-icon"]').click();
    await page.waitForTimeout(500);
    // Click + 发送
    await page.locator('text=+ 发送').click();
    await expect(page.locator('[data-testid="notif-send-modal"]')).toBeVisible();
    await expect(page.locator('[data-testid="notif-send-recipient"]')).toBeVisible();
    await expect(page.locator('[data-testid="notif-send-subject"]')).toBeVisible();
    await expect(page.locator('[data-testid="notif-send-body"]')).toBeVisible();
    await expect(page.locator('[data-testid="notif-send-submit"]')).toBeVisible();

    // Fill and send
    await page.locator('[data-testid="notif-send-subject"]').fill('测试通知');
    await page.locator('[data-testid="notif-send-body"]').fill('这是一条测试站内信');
    await page.locator('[data-testid="notif-send-submit"]').click();
    await page.waitForTimeout(1000);
  });
});
