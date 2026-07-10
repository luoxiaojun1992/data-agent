import { test, expect } from '@playwright/test';

/**
 * SPEC-017: AUTH E2E Tests (UI-001 through UI-010)
 *
 * Real API calls only — no page.route() mocks.
 * A test user is registered via the backend API before all tests.
 * Only permitted mock: AI model calls via mockllm service (SPEC-043).
 */

const log = (msg: string) => process.stderr.write(`[auth.spec] ${new Date().toISOString()} ${msg}\n`);

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const TEST_USER = {
  username: `e2e-auth-${uid}@test.local`,
  password: 'E2eTest123!',
};

test.describe('AUTH - Login Page', () => {
  test.beforeAll(async ({ request }) => {
    log(`Registering test user: ${TEST_USER.username}`);
    const res = await request.post(`${API_BASE}/auth/register`, {
      data: TEST_USER,
    });
    const body = await res.json();
    log(`Register response: ${res.status()} ${JSON.stringify(body)}`);
    expect(res.status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.evaluate(() => localStorage.clear());
  });

  // UI-001: Login page brand elements rendering
  test('[UI-001] Login page brand elements rendering', async ({ page }) => {
    // Logo + brand
    await expect(page.locator('[data-testid="login-logo"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-logo-icon"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-logo-name"]')).toHaveText('DataAgent');

    // Title
    await expect(page.locator('[data-testid="login-title"]')).toBeVisible();

    // Form elements
    await expect(page.locator('[data-testid="login-email-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-password-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-btn"]')).toBeEnabled();

    // SSO divider + button
    await expect(page.locator('[data-testid="login-divider"]')).toBeVisible();
    await expect(page.locator('[data-testid="login-sso-btn"]')).toBeVisible();
  });

  // UI-002: Email input interaction and validation
  test('[UI-002] Email input interaction and validation', async ({ page }) => {
    const emailInput = page.locator('[data-testid="login-email-input"]');

    // Input accepts text
    await emailInput.fill('test@example.com');
    await expect(emailInput).toHaveValue('test@example.com');

    // Focus styling (border-color set on focus)
    await emailInput.focus();
    const focusBorder = await emailInput.evaluate((el) => {
      return getComputedStyle(el).borderColor;
    });
    expect(focusBorder).toBeTruthy();
  });

  // UI-003: Password input interaction
  test('[UI-003] Password input interaction', async ({ page }) => {
    const passwordInput = page.locator('[data-testid="login-password-input"]');

    // Should be type password
    await expect(passwordInput).toHaveAttribute('type', 'password');

    // Should accept input
    await passwordInput.fill('mypassword');
    await expect(passwordInput).toHaveValue('mypassword');
  });

  // UI-004: Login button redirects after successful login
  test('[UI-004] Login button interaction and state', async ({ page }) => {
    const loginBtn = page.locator('[data-testid="login-btn"]');
    await expect(loginBtn).toBeVisible();
    await expect(loginBtn).toHaveText('登录');

    // Fill real test credentials
    await page.locator('[data-testid="login-email-input"]').fill(TEST_USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(TEST_USER.password);

    await loginBtn.click();

    // Should redirect away from /login after successful login
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // UI-005: SSO button presence and interaction
  test('[UI-005] SSO button presence and interaction', async ({ page }) => {
    // SSO divider visible
    await expect(page.locator('[data-testid="login-divider"]')).toBeVisible();

    // SSO button visible with correct text
    const ssoBtn = page.locator('[data-testid="login-sso-btn"]');
    await expect(ssoBtn).toBeVisible();
    await expect(ssoBtn).toHaveText(/企业 SSO/);
  });

  // UI-006: Email format validation on submit
  test('[UI-006] Email format validation on submit', async ({ page }) => {
    // Fill invalid email, valid password, click submit
    await page.locator('[data-testid="login-email-input"]').fill('invalid-email');
    await page.locator('[data-testid="login-password-input"]').fill('password123');
    await page.locator('[data-testid="login-btn"]').click();

    // Client-side validation error should appear
    await expect(page.locator('[data-testid="login-email-error"]')).toBeVisible();
  });

  // UI-007: Empty field validation
  test('[UI-007] Empty field validation', async ({ page }) => {
    // Click login with empty fields
    await page.locator('[data-testid="login-btn"]').click();

    // Should show email error (empty + invalid)
    await expect(page.locator('[data-testid="login-email-error"]')).toBeVisible();
  });

  // UI-008: Wrong credentials shows error toast
  test('[UI-008] Wrong credentials shows error toast', async ({ page }) => {
    // Fill wrong credentials
    await page.locator('[data-testid="login-email-input"]').fill('wrong@user.com');
    await page.locator('[data-testid="login-password-input"]').fill('WrongPass1');
    await page.locator('[data-testid="login-btn"]').click();

    // Real API returns 401 → generalError → toast appears
    await expect(page.locator('[data-testid="login-error-toast"]')).toBeVisible({ timeout: 10000 });
  });

  // UI-009: JWT Token expired shows session-expired toast
  test('[UI-009] JWT Token expired shows session-expired toast', async ({ page }) => {
    // Navigate with expired=true query param
    await page.goto('/login?expired=true');

    // Session expired toast should appear
    await expect(page.locator('[data-testid="login-session-expired-toast"]')).toBeVisible();
  });

  // UI-010: Logout clears session and redirects to login
  test('[UI-010] Logout clears session and redirects to login', async ({ page }) => {
    // Login with real credentials
    await page.locator('[data-testid="login-email-input"]').fill(TEST_USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(TEST_USER.password);
    await page.locator('[data-testid="login-btn"]').click();

    // Wait for redirect to dashboard
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.waitForLoadState('networkidle');

    // Token should be in localStorage
    const token = await page.evaluate(() => localStorage.getItem('token'));
    expect(token).toBeTruthy();

    // Sidebar should be visible with user card and logout button
    await expect(page.locator('[data-testid="nav-user-card"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="nav-logout-btn"]')).toBeVisible({ timeout: 10000 });

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
