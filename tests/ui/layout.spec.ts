import { test, expect } from '@playwright/test';

/**
 * SPEC-018: LAYOUT E2E Tests (UI-011 through UI-017)
 *
 * Tests sidebar structure, navigation groups, user card, page titles.
 * Real API calls for login (same approach as SPEC-017).
 */

const log = (msg: string) => process.stderr.write(`[layout.spec] ${new Date().toISOString()} ${msg}\n`);

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const TEST_USER = {
  username: `e2e-layout-${uid}@test.local`,
  password: 'E2eTest123!',
};

test.describe('LAYOUT — Sidebar & Navigation', () => {
  test.beforeAll(async ({ request }) => {
    log(`Registering test user: ${TEST_USER.username}`);
    const res = await request.post(`${API_BASE}/auth/register`, {
      data: TEST_USER,
    });
    expect(res.status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    // Login with real credentials
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(TEST_USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(TEST_USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.waitForSelector('[data-testid="sidebar"]', { state: 'visible', timeout: 10000 });
  });

  // UI-011: Main sidebar structure rendering
  test('[UI-011] Main sidebar structure rendering', async ({ page }) => {
    // Sidebar container
    const sidebar = page.locator('[data-testid="sidebar"]');
    await expect(sidebar).toBeVisible();

    // Logo elements
    await expect(page.locator('[data-testid="sidebar-logo"]')).toBeVisible();
    await expect(page.locator('[data-testid="sidebar-logo-icon"]')).toBeVisible();
    await expect(page.locator('[data-testid="sidebar-logo-text"]')).toHaveText('DataAgent');
  });

  // UI-012: Workspace navigation items
  test('[UI-012] Workspace navigation items', async ({ page }) => {
    // Chat and Agent navigation items
    const chatNav = page.locator('[data-testid="nav-chat"]');
    const agentNav = page.locator('[data-testid="nav-agent"]');

    await expect(chatNav).toBeVisible();
    await expect(chatNav).toContainText(/Chat/);
    await expect(agentNav).toBeVisible();
    await expect(agentNav).toContainText(/Agent/);
  });

  // UI-013: Monitor group — Dashboard nav item
  test('[UI-013] Dashboard navigation item', async ({ page }) => {
    const dashNav = page.locator('[data-testid="nav-dashboard"]');
    await expect(dashNav).toBeVisible();
    await expect(dashNav).toContainText('仪表盘');
  });

  // UI-014: System management navigation items
  test('[UI-014] System management navigation items', async ({ page }) => {
    const kbNav = page.locator('[data-testid="nav-kb-mgmt"]');
    const adminNav = page.locator('[data-testid="nav-admin"]');

    await expect(kbNav).toBeVisible();
    await expect(adminNav).toBeVisible();
  });

  // UI-015: Sidebar user card rendering
  test('[UI-015] Sidebar user card rendering', async ({ page }) => {
    // User card section
    const userCard = page.locator('[data-testid="nav-user-card"]');
    await expect(userCard).toBeVisible();

    // Avatar
    const avatar = page.locator('[data-testid="user-avatar"]');
    await expect(avatar).toBeVisible();
  });

  // UI-016: Navigation click switches active state
  test('[UI-016] Navigation click switches active state', async ({ page }) => {
    // Click Chat nav
    await page.locator('[data-testid="nav-chat"]').click();
    await page.waitForURL('**/chat', { timeout: 5000 });
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();

    // Click Agent nav
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });

  // UI-017: Page title rendering
  test('[UI-017] Page title rendering', async ({ page }) => {
    // Dashboard page title
    await expect(page.locator('[data-testid="page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('仪表盘');

    // Main content area
    await expect(page.locator('[data-testid="main-content"]')).toBeVisible();
  });
});
