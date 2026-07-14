import { test, expect } from '@playwright/test';

const uid = crypto.randomUUID().slice(0, 8);
const USER = { username: `e2e-im-${uid}@test.local`, password: 'ImTestPass1' };

/**
 * IM (Feishu) E2E tests.
 *
 * Each user binds their own Feishu bot's App ID + App Secret.
 * When the Feishu webhook fires with that App ID, the backend
 * maps it to the bound user and their session context.
 *
 * UI-161: Bind page renders App ID / App Secret inputs
 * UI-162: Save bind config + verify persistence
 * UI-163~166: Feishu client-side features → manual testing only
 */

test.describe('IM — SPEC-034', () => {
  test.beforeAll(async ({ request }) => {
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: USER });
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post('http://data-agent:8080/api/v1/auth/login', { data: { username: USER.username, password: USER.password } });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`http://data-agent:8080/api/v1/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-im-${uid}`)) {
          await request.delete(`http://data-agent:8080/api/v1/users/${user.id}`, { headers });
        }
      }
    }
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // ═══ UI-161: 飞书绑定页 ═══
  test('[UI-161] IM — 飞书绑定页', async ({ page }) => {
    await page.goto('/im/bind');
    await page.waitForSelector('[data-testid="im-bind-page"]', { timeout: 10000 });

    await expect(page.locator('[data-testid="im-bind-app-id"]')).toBeVisible();
    await expect(page.locator('[data-testid="im-bind-app-secret"]')).toBeVisible();
    await expect(page.locator('[data-testid="im-bind-submit"]')).toBeVisible();
    await expect(page.locator('[data-testid="im-bind-app-secret"]')).toHaveAttribute('type', 'password');
  });

  // ═══ UI-162: 保存飞书绑定配置 ═══
  test('[UI-162] IM — 保存飞书配置', async ({ page }) => {
    await page.goto('/im/bind');
    await page.waitForSelector('[data-testid="im-bind-page"]', { timeout: 10000 });

    // Enter Feishu credentials
    await page.locator('[data-testid="im-bind-app-id"]').fill('cli_test_app_123');
    await page.locator('[data-testid="im-bind-app-secret"]').fill('secret_key_abc');
    await page.locator('[data-testid="im-bind-submit"]').click();

    // Verify success
    await expect(page.locator('[data-testid="im-bind-success"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="im-bind-success"]')).toContainText('绑定成功');
  });

  // ═══ UI-163~166: 飞书客户端功能 → 人工测试 ═══
});
