import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const U = { username: `e2e-adm-${uid}@test.local`, password: 'E2eTest123!' };

function login(page) {
  return async () => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
  };
}

test.beforeAll(async ({ request }) => {
  expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
});

test.describe('ADMIN PAGES', () => {

  const pages: any[] = [];

  for (const p of pages) {
    test(`[${p.spec}] ${p.title} — page renders`, async ({ page }) => {
      await login(page)();
      await page.goto(p.path);
      await page.waitForSelector(`[data-testid="${p.tid}-header"]`, { timeout: 5000 });
      await expect(page.locator(`[data-testid="${p.tid}-title"]`)).toHaveText(p.title);
      await expect(page.locator(`[data-testid="${p.tid}-empty"]`)).toBeVisible();
    });
  }
});
