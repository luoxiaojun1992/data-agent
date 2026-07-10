import { test, expect } from '@playwright/test';

/**
 * SPEC-017: AUTH E2E Tests
 * Test cases UI-001 through UI-010
 * Login mocked to get known token (password is random-generated at startup).
 * All other API calls use real backend endpoints.
 */

const log = (msg: string) => process.stderr.write(`[auth.spec] ${new Date().toISOString()} ${msg}\n`);

test.describe('AUTH - Login Page', () => {
  test.beforeEach(async ({ page, baseURL }, testInfo) => {
    log(`${testInfo.title}: navigating to /login (baseURL=${baseURL})`);
    // Clear localStorage to ensure logged-out state
    await page.goto('/login');
    await page.evaluate(() => {
      localStorage.clear();
    });
  });

  // UI-001: Brand elements rendering
  test('[UI-001] Login page brand elements rendering', async ({ page }) => {
    await expect(page.locator('[data-testid="login-card"]')).toBeVisible();

    // Black background
    const bgColor = await page.evaluate(() => {
      const el = document.querySelector('[data-testid="login-card"]') as HTMLElement;
      return getComputedStyle(el).backgroundColor;
    });
    expect(bgColor).toBe('rgb(0, 0, 0)');

    // DA icon
    const logoIcon = page.locator('[data-testid="login-logo-icon"]');
    await expect(logoIcon).toBeVisible();
    const iconBox = await logoIcon.boundingBox();
    expect(iconBox?.width).toBeGreaterThanOrEqual(30);
    expect(iconBox?.height).toBeGreaterThanOrEqual(30);

    // Brand name
    await expect(page.locator('[data-testid="login-logo-name"]')).toHaveText('DataAgent');

    // Title
    await expect(page.locator('[data-testid="login-title"]')).toHaveText('登录企业数据分析平台');
  });

  // UI-002: Email input interaction and validation
  test('[UI-002] Email input interaction and validation', async ({ page }) => {
    // Label
    await expect(page.locator('[data-testid="login-email-label"]')).toHaveText('邮箱地址');

    // Placeholder
    const emailInput = page.locator('[data-testid="login-email-input"]');
    await expect(emailInput).toHaveAttribute('placeholder', 'name@company.com');

    // Focus border color
    await emailInput.focus();
    const focusBorder = await emailInput.evaluate((el) => {
      return getComputedStyle(el).borderColor;
    });
    // Should have a border color set
    expect(focusBorder).toBeTruthy();

    // Invalid email format
    await emailInput.fill('notanemail');
    await emailInput.blur();
    await expect(page.locator('[data-testid="login-email-error"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-email-error"]')).toHaveText('请输入有效的邮箱地址');

    // Valid email
    await emailInput.fill('test@company.com');
    await emailInput.blur();
    await expect(page.locator('[data-testid="login-email-error"]')).not.toBeVisible();
  });

  // UI-003: Password input interaction
  test('[UI-003] Password input interaction', async ({ page }) => {
    // Label
    await expect(page.locator('[data-testid="login-password-label"]')).toHaveText('密码');

    // Input type is password
    const passwordInput = page.locator('[data-testid="login-password-input"]');
    await expect(passwordInput).toHaveAttribute('type', 'password');

    // Can type
    await passwordInput.fill('testpass123');
    await expect(passwordInput).toHaveValue('testpass123');
  });

  // UI-004: Login button interaction and state
  test('[UI-004] Login button interaction and state', async ({ page }) => {
    // Mock successful login
    await page.route((url) => url.toString().includes('/api/v1/auth/login'), async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          access_token: 'mock-jwt-token',
          user_id: 'user-001',
          username: 'admin@company.com',
          role: 'admin',
        }),
      });
    });

    // Button visible with correct text
    const loginBtn = page.locator('[data-testid="login-btn"]');
    await expect(loginBtn).toBeVisible();
    await expect(loginBtn).toHaveText('登录');

    // Fill valid credentials
    await page.locator('[data-testid="login-email-input"]').fill('admin@company.com');
    await page.locator('[data-testid="login-password-input"]').fill('password123');

    // Click login
    await loginBtn.click();

    // Should redirect after login (navigate away from /login)
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // UI-005: SSO button
  test('[UI-005] SSO button presence and interaction', async ({ page }) => {
    // Divider visible
    await expect(page.locator('[data-testid="login-divider"]')).toBeVisible();

    // SSO button visible
    const ssoBtn = page.locator('[data-testid="login-sso-btn"]');
    await expect(ssoBtn).toBeVisible();
    await expect(ssoBtn).toHaveText('企业 SSO 单点登录');

    // Click SSO button — should at minimum not crash
    await ssoBtn.click();
    // Note: SSO redirect is external, we just verify no error thrown
  });

  // UI-006: Email format validation
  test('[UI-006] Email format validation on submit', async ({ page }) => {
    await page.locator('[data-testid="login-email-input"]').fill('notanemail');
    await page.locator('[data-testid="login-password-input"]').fill('password123');
    await page.locator('[data-testid="login-btn"]').click();

    await expect(page.locator('[data-testid="login-email-error"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-email-error"]')).toHaveText('请输入有效的邮箱地址');

    // URL should not change (no navigation happened)
    expect(page.url()).toContain('/login');
  });

  // UI-007: Empty field validation
  test('[UI-007] Empty field validation', async ({ page }) => {
    // Click login with empty fields
    await page.locator('[data-testid="login-btn"]').click();

    // Both error messages should appear
    await expect(page.locator('[data-testid="login-email-error"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-password-error"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-email-error"]')).toHaveText('请输入邮箱地址');
    await expect(page.locator('[data-testid="login-password-error"]')).toHaveText('请输入密码');
  });

  // UI-008: Wrong credentials handling
  test('[UI-008] Wrong credentials shows error toast', async ({ page }) => {
    // Mock failed login
    await page.route((url) => url.toString().includes('/api/v1/auth/login'), async (route) => {
      await route.fulfill({
        status: 401,
        contentType: 'application/json',
        body: JSON.stringify({ error: '邮箱或密码错误' }),
      });
    });

    await page.locator('[data-testid="login-email-input"]').fill('wrong@company.com');
    await page.locator('[data-testid="login-password-input"]').fill('wrongpass');
    await page.locator('[data-testid="login-btn"]').click();

    // Error toast should appear
    await expect(page.locator('[data-testid="login-error-toast"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-error-toast"]')).toHaveText('邮箱或密码错误');

    // Button should be re-enabled
    await expect(page.locator('[data-testid="login-btn"]')).toBeEnabled();

    // Password field should be cleared
    await expect(page.locator('[data-testid="login-password-input"]')).toHaveValue('');
  });

  // UI-009: JWT Token expired redirect
  test('[UI-009] JWT Token expired shows session-expired toast', async ({ page }) => {
    // Navigate to login with expired=true param
    await page.goto('/login?expired=true');

    // Session expired toast should be visible
    await expect(page.locator('[data-testid="login-session-expired-toast"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-session-expired-toast"]')).toHaveText('登录已过期，请重新登录');
  });

  // UI-010: Logout flow
  test('[UI-010] Logout clears session and redirects to login', async ({ page }) => {
    // Mock successful login
    await page.route((url) => url.toString().includes('/api/v1/auth/login'), async (route) => {
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          access_token: 'mock-jwt-token',
          user_id: 'user-001',
          username: 'admin@company.com',
          role: 'admin',
        }),
      });
    });

    // Login (page already at /login from beforeEach)
    await page.locator('[data-testid="login-email-input"]').fill('admin@company.com');
    await page.locator('[data-testid="login-password-input"]').fill('password123');
    await page.locator('[data-testid="login-btn"]').click();

    // Wait for redirect after login
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.waitForLoadState('networkidle');

    // Verify token is in localStorage (from mock login)
    const token = await page.evaluate(() => localStorage.getItem('token'));
    expect(token).toBeTruthy();

    // User card and logout button should be visible
    await expect(page.locator('[data-testid="nav-user-card"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-logout-btn"]')).toBeVisible({ timeout: 5000 });

    // Click logout
    await page.locator('[data-testid="nav-logout-btn"]').click();

    // Should redirect to login page
    await page.waitForURL((url) => url.pathname.includes('/login'), { timeout: 5000 });

    // Token should be cleared
    const tokenAfter = await page.evaluate(() => localStorage.getItem('token'));
    expect(tokenAfter).toBeNull();

    // Session expired toast should be visible
    await expect(page.locator('[data-testid="login-session-expired-toast"]')).toBeVisible();
  });
});
