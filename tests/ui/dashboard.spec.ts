import { registerUser } from './utils/auth';
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-dash-${uid}@test.local`, password: 'E2eTest123!' };

test.describe('DASHBOARD — System Overview', () => {
  test.beforeAll(async ({ request }) => {
    expect((await registerUser(request, U)).status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL(u => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-dashboard"]').click();
    await page.waitForURL('**/', { timeout: 5000 });
  });

  test('[UI-062] Dashboard — greeting and date', async ({ page }) => {
    await expect(page.locator('[data-testid="dashboard-greeting"]')).toBeVisible();
    await expect(page.locator('[data-testid="dashboard-date"]')).toBeVisible();
  });

  test('[UI-074] Dashboard — global online indicator', async ({ page }) => {
    await expect(page.locator('[data-testid="global-online-indicator"]')).toBeVisible();
    await expect(page.locator('[data-testid="global-online-dot"]')).toBeVisible();
  });
});
