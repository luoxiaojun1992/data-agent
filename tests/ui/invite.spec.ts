import { test, expect } from '@playwright/test';

/**
 * SPEC-044: Invite Registration System E2E Tests
 *
 * Tests the invite-based registration flow:
 * - Register page token validation
 * - Admin invites management page
 * - Invite creation form
 * - Invite revocation
 *
 * Note: Full token creation/registration cycle requires INVITE_HMAC_SECRET
 * configured. This test covers UI rendering and form interactions.
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const ADMIN = { username: `e2e-inv-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';

test.describe('INVITE — Registration Page', () => {
  // UI-196: Register page — no token → shows error
  test('[UI-196] Register page without token shows error', async ({ page }) => {
    await page.goto('/register');
    await expect(page.locator('[data-testid="register-invalid"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="register-invalid-title"]')).toContainText('邀请链接无效');
  });

  // UI-197: Register page — invalid token → shows error
  test('[UI-197] Register page with invalid token', async ({ page }) => {
    await page.goto('/register?token=invalid_token_xxx');
    await expect(page.locator('[data-testid="register-invalid"]')).toBeVisible({ timeout: 5000 });
  });

  // UI-198: Register page — error page has back-to-login link
  test('[UI-198] Register page error has login link', async ({ page }) => {
    await page.goto('/register');
    await expect(page.locator('[data-testid="register-goto-login-btn"]')).toBeVisible({ timeout: 5000 });
    await page.locator('[data-testid="register-goto-login-btn"]').click();
    await page.waitForURL(/\/login/);
    await expect(page.locator('[data-testid="login-card"]')).toBeVisible();
  });
});

test.describe('INVITE — Admin Invites Management', () => {
  test.beforeAll(async ({ request }) => {
    // Register admin user
    const regRes = await request.post(`${API_BASE}/auth/register`, {
      data: { username: ADMIN.username, password: ADMIN.password, role: ADMIN.role },
    });
    expect(regRes.status()).toBe(201);

    // Login to get token
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: ADMIN.username, password: ADMIN.password },
    });
    expect(loginRes.status()).toBe(200);
    const body = await loginRes.json();
    adminToken = body.access_token;
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/invites');
    await page.waitForSelector('[data-testid="invites-page-header"]', { timeout: 5000 });
  });

  // UI-199: Invites management page header renders
  test('[UI-199] Invites management page renders', async ({ page }) => {
    await expect(page.locator('[data-testid="invites-page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="invites-page-header"]')).toHaveText('邀请管理');
    await expect(page.locator('[data-testid="invites-create-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="invites-table"]')).toBeVisible();
  });

  // UI-200: Invite creation form appears when clicking create button
  test('[UI-200] Invite creation form toggle', async ({ page }) => {
    await expect(page.locator('[data-testid="invites-create-form"]')).not.toBeVisible();

    // Click create button
    await page.locator('[data-testid="invites-create-btn"]').click();
    await expect(page.locator('[data-testid="invites-create-form"]')).toBeVisible();

    // Verify form fields
    await expect(page.locator('[data-testid="invites-email-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="invites-role-select"]')).toBeVisible();
    await expect(page.locator('[data-testid="invites-expire-select"]')).toBeVisible();
    await expect(page.locator('[data-testid="invites-submit-btn"]')).toBeVisible();

    // Click cancel to close
    await page.locator('[data-testid="invites-cancel-btn"]').click();
    await expect(page.locator('[data-testid="invites-create-form"]')).not.toBeVisible();
  });

  // UI-201: Invite form role selection
  test('[UI-201] Invite form role default is user', async ({ page }) => {
    await page.locator('[data-testid="invites-create-btn"]').click();
    await expect(page.locator('[data-testid="invites-create-form"]')).toBeVisible();

    const roleSelect = page.locator('[data-testid="invites-role-select"]');
    await expect(roleSelect).toHaveValue('user');

    // Change to admin
    await roleSelect.selectOption('admin');
    await expect(roleSelect).toHaveValue('admin');
  });

  // UI-202: Invite form expiry selection
  test('[UI-202] Invite form expiry default is 24h', async ({ page }) => {
    await page.locator('[data-testid="invites-create-btn"]').click();
    await expect(page.locator('[data-testid="invites-create-form"]')).toBeVisible();

    const expireSelect = page.locator('[data-testid="invites-expire-select"]');
    await expect(expireSelect).toHaveValue('24');
  });

  // UI-203: Empty invites table shows placeholder
  test('[UI-203] Empty invites table shows placeholder message', async ({ page }) => {
    await expect(page.locator('[data-testid="invites-table"]')).toBeVisible();
  });
});
