import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-dash-${uid}@test.local`, password: 'E2eTest123!' };

test.describe('DASHBOARD — System Overview', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
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

  test('[UI-063] Dashboard — time filter', async ({ page }) => {
    await expect(page.locator('[data-testid="dashboard-time-filter"]')).toBeVisible();
    await expect(page.locator('[data-testid="filter-today"]')).toBeVisible();
    await page.locator('[data-testid="filter-week"]').click();
    await expect(page.locator('[data-testid="filter-week"]')).toHaveClass(/accent/);
  });

  test('[UI-064] Dashboard — stats cards', async ({ page }) => {
    await expect(page.locator('[data-testid="dashboard-stat-0"]')).toBeVisible();
    await expect(page.locator('[data-testid="dashboard-stat-1"]')).toBeVisible();
    await expect(page.locator('[data-testid="dashboard-stat-2"]')).toBeVisible();
    await expect(page.locator('[data-testid="dashboard-stat-3"]')).toBeVisible();
  });

  test('[UI-065] Dashboard — call trend chart', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-call-trend"]')).toBeVisible();
  });

  test('[UI-066] Dashboard — task status distribution', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-status-pie"]')).toBeVisible();
  });

  test('[UI-067] Dashboard — task duration histogram', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-duration-dist"]')).toBeVisible();
  });

  test('[UI-068] Dashboard — 24h request distribution', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-req-dist"]')).toBeVisible();
  });

  test('[UI-069] Dashboard — success rate trend', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-success-trend"]')).toBeVisible();
  });

  test('[UI-070] Dashboard — Token/ROI KPIs', async ({ page }) => {
    await expect(page.locator('[data-testid="dashboard-token-kpi-0"]')).toContainText('Token');
    await expect(page.locator('[data-testid="dashboard-token-kpi-1"]')).toContainText('AI 产出');
    await expect(page.locator('[data-testid="dashboard-token-kpi-2"]')).toContainText('ROI');
  });

  test('[UI-071] Dashboard — Token consumption chart', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-token-trend"]')).toBeVisible();
  });

  test('[UI-072] Dashboard — output stats chart', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-output-stats"]')).toBeVisible();
  });

  test('[UI-073] Dashboard — ROI dual-axis chart', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-roi-dual"]')).toBeVisible();
  });

  test('[UI-074] Dashboard — real-time badge', async ({ page }) => {
    await expect(page.locator('[data-testid="dashboard-realtime-badge"]')).toBeVisible();
    await expect(page.locator('[data-testid="dashboard-realtime-badge"]')).toContainText('实时');
  });
});
