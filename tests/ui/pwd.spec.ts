import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();

const ADMIN = { username: `e2e-pwd-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
const USER = { username: `e2e-pwd-user-${uid}@test.local`, password: 'UserTest1' };
let adminToken = '';

test.describe('PASSWORD — SPEC-032', () => {
  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    adminToken = (await res.json()).access_token;
    await request.post(`${API_BASE}/auth/register`, { data: USER });

    // Ensure admin has password_changed=true by changing password first
    await request.post(`${API_BASE}/change-password`, {
      data: { old_password: ADMIN.password, new_password: 'NewPass1' },
      headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
    });
    // Login again with new password
    const newLogin = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: 'NewPass1' } });
    adminToken = (await newLogin.json()).access_token;
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill('NewPass1');
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/change-password');
    await page.waitForSelector('[data-testid="pwd-page"]', { timeout: 10000 });
    await page.waitForTimeout(1000);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-pwd-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-150: 修改密码页 ═══
  test('[UI-150] Pwd — 修改密码页', async ({ page }) => {
    await expect(page.locator('[data-testid="pwd-old-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-new-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-confirm-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-change-btn"]')).toBeVisible();

    // All inputs should be password type
    await expect(page.locator('[data-testid="pwd-old-input"]')).toHaveAttribute('type', 'password');
    await expect(page.locator('[data-testid="pwd-new-input"]')).toHaveAttribute('type', 'password');
    await expect(page.locator('[data-testid="pwd-confirm-input"]')).toHaveAttribute('type', 'password');
  });

  // ═══ UI-151: 成功修改密码 ═══
  test('[UI-151] Pwd — 成功修改密码', async ({ page }) => {
    await page.locator('[data-testid="pwd-old-input"]').fill('NewPass1');
    await page.locator('[data-testid="pwd-new-input"]').fill('ComplexPass1');
    await page.locator('[data-testid="pwd-confirm-input"]').fill('ComplexPass1');
    await page.locator('[data-testid="pwd-change-btn"]').click();

    // Should show success toast
    await expect(page.locator('[data-testid="pwd-change-success-toast"]')).toBeVisible({ timeout: 5000 });
    // Should redirect to login after 2s
    await page.waitForURL(/login/, { timeout: 5000 });
  });

  // ═══ UI-152: 旧密码错误 ═══
  test('[UI-152] Pwd — 旧密码错误', async ({ page }) => {
    await page.locator('[data-testid="pwd-old-input"]').fill('WrongPassword');
    await page.locator('[data-testid="pwd-new-input"]').fill('NewPass1');
    await page.locator('[data-testid="pwd-confirm-input"]').fill('NewPass1');
    await page.locator('[data-testid="pwd-change-btn"]').click();

    await expect(page.locator('[data-testid="pwd-old-error"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="pwd-old-error"]')).toContainText('旧密码');
  });

  // ═══ UI-153: 新密码不一致 ═══
  test('[UI-153] Pwd — 新密码不一致', async ({ page }) => {
    await page.locator('[data-testid="pwd-old-input"]').fill('ComplexPass1');
    await page.locator('[data-testid="pwd-new-input"]').fill('NewPass1');
    await page.locator('[data-testid="pwd-confirm-input"]').fill('NewPass2');
    await page.locator('[data-testid="pwd-change-btn"]').click();

    await expect(page.locator('[data-testid="pwd-confirm-error"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-confirm-error"]')).toContainText('不一致');
  });

  // ═══ UI-154: 新密码强度校验 ═══
  test('[UI-154] Pwd — 新密码强度校验', async ({ page }) => {
    await page.locator('[data-testid="pwd-old-input"]').fill('ComplexPass1');
    await page.locator('[data-testid="pwd-new-input"]').fill('123456');
    await page.locator('[data-testid="pwd-confirm-input"]').fill('123456');
    await page.locator('[data-testid="pwd-change-btn"]').click();

    await expect(page.locator('[data-testid="pwd-old-error"]')).toBeVisible({ timeout: 3000 });
  });

  // ═══ UI-155: 所有角色均可修改密码 ═══
  test('[UI-155] Pwd — user 角色也可修改密码', async ({ page }) => {
    // Logout and login as regular user
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/change-password');
    await page.waitForSelector('[data-testid="pwd-page"]', { timeout: 10000 });
    await expect(page.locator('[data-testid="pwd-change-btn"]')).toBeVisible();
  });
});
