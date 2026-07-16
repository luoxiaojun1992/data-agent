import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8) + '_' + Math.random().toString(36).slice(2, 8);

const FRESH = { username: `e2e-pwd-fresh-${uid}@test.local`, password: 'TempPass1', role: 'admin' };
const USER = { username: `e2e-pwd-user-${uid}@test.local`, password: 'UserTest1' };
// Cleaner user: password never changes, used only for cleanup
const CLEANER = { username: `e2e-pwd-cleaner-${uid}@test.local`, password: 'CleanPass1', role: 'admin' };

test.describe.serial('PASSWORD — SPEC-032', () => {
  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: CLEANER });
    expect(res.status()).toBe(201);
    res = await request.post(`${API_BASE}/auth/register`, { data: FRESH });
    expect(res.status()).toBe(201);
    res = await request.post(`${API_BASE}/auth/register`, { data: USER });
    expect(res.status()).toBe(201);
  });

  test.afterAll(async ({ request }) => {
    // Use cleaner (never changed password) to delete all test users
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: CLEANER.username, password: CLEANER.password } });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-pwd-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-149: 初始密码横幅通知 ═══
  test('[UI-149] Pwd — 初始密码横幅通知', async ({ page, request }) => {
    // Login fresh user to get token
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: FRESH.username, password: FRESH.password } });
    const data = await loginRes.json();
    // Verify need_change_pw is true in response
    expect(data.need_change_pw).toBe(true);

    // Now login via browser to see the banner
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(FRESH.username);
    await page.locator('[data-testid="login-password-input"]').fill(FRESH.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/change-password');
    await page.waitForSelector('[data-testid="pwd-page"]', { timeout: 10000 });
    await page.waitForTimeout(2000);

    // Banner should show for fresh (password_changed=false) users
    await expect(page.locator('[data-testid="pwd-initial-banner"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-initial-banner"]')).toContainText('初始密码');
  });

  // ═══ UI-150: 修改密码页 ═══
  test('[UI-150] Pwd — 修改密码页', async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/change-password');
    await page.waitForSelector('[data-testid="pwd-page"]', { timeout: 10000 });

    await expect(page.locator('[data-testid="pwd-old-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-new-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-confirm-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-change-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-old-input"]')).toHaveAttribute('type', 'password');
    await expect(page.locator('[data-testid="pwd-new-input"]')).toHaveAttribute('type', 'password');
    await expect(page.locator('[data-testid="pwd-confirm-input"]')).toHaveAttribute('type', 'password');
  });

  // ═══ UI-151: 成功修改密码 (via direct API) ═══
  test('[UI-151] Pwd — 成功修改密码', async ({ request }) => {
    // Login to get token
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: FRESH.username, password: FRESH.password } });
    const token = (await loginRes.json()).access_token;

    // Change password via API
    const res = await request.post(`${API_BASE}/change-password`, {
      data: { old_password: FRESH.password, new_password: 'ComplexPass1' },
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    });
    expect(res.ok()).toBe(true);

    // Verify old password no longer works
    const oldLogin = await request.post(`${API_BASE}/auth/login`, { data: { username: FRESH.username, password: FRESH.password } });
    expect(oldLogin.ok()).toBe(false);

    // Verify new password works
    const newLogin = await request.post(`${API_BASE}/auth/login`, { data: { username: FRESH.username, password: 'ComplexPass1' } });
    expect(newLogin.ok()).toBe(true);
    const newData = await newLogin.json();
    expect(newData.need_change_pw).toBe(false);
  });

  // ═══ UI-152: 旧密码错误 ═══
  test('[UI-152] Pwd — 旧密码错误', async ({ page, request }) => {
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: USER.username, password: USER.password } });
    const token = (await loginRes.json()).access_token;

    // Direct API test for wrong old password
    const res = await request.post(`${API_BASE}/change-password`, {
      data: { old_password: 'WrongPassword', new_password: 'NewPass1' },
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    });
    expect(res.ok()).toBe(false);
    const body = await res.json();
    expect(body.error).toContain('旧密码');
  });

  // ═══ UI-153: 新密码不一致 ═══
  test('[UI-153] Pwd — 新密码不一致', async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/change-password');
    await page.waitForSelector('[data-testid="pwd-page"]', { timeout: 10000 });

    await page.locator('[data-testid="pwd-old-input"]').fill(USER.password);
    await page.locator('[data-testid="pwd-new-input"]').fill('NewPass1');
    await page.locator('[data-testid="pwd-confirm-input"]').fill('NewPass2');
    await page.locator('[data-testid="pwd-change-btn"]').click();

    await expect(page.locator('[data-testid="pwd-confirm-error"]')).toBeVisible();
    await expect(page.locator('[data-testid="pwd-confirm-error"]')).toContainText('不一致');
  });

  // ═══ UI-154: 新密码强度校验 ═══
  test('[UI-154] Pwd — 新密码强度校验', async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/change-password');
    await page.waitForSelector('[data-testid="pwd-page"]', { timeout: 10000 });
    await page.waitForTimeout(1000);

    await page.locator('[data-testid="pwd-old-input"]').fill(USER.password);
    await page.locator('[data-testid="pwd-new-input"]').fill('123456');
    await page.locator('[data-testid="pwd-confirm-input"]').fill('123456');
    await page.locator('[data-testid="pwd-change-btn"]').click();

    await expect(page.locator('[data-testid="pwd-new-error"]')).toBeVisible({ timeout: 3000 });
  });

  // ═══ UI-155: user 角色也可修改密码 ═══
  test('[UI-155] Pwd — user 角色也可修改密码', async ({ page, request }) => {
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: USER.username, password: USER.password } });
    const token = (await loginRes.json()).access_token;

    // Verify user can change their own password via API
    const res = await request.post(`${API_BASE}/change-password`, {
      data: { old_password: USER.password, new_password: 'NewUserPass1' },
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    });
    expect(res.ok()).toBe(true);
  });
});
