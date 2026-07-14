import { test, expect } from '@playwright/test';

const uid = Date.now().toString(36);
const ADMIN = { username: `e2e-im-${uid}@test.local`, password: 'ImAdminPass1', role: 'admin' };

/**
 * IM (Feishu) E2E tests.
 *
 * UI-161: Feishu config page — admin enters App ID / App Secret
 * UI-162: Feishu config saved to backend
 * UI-163~166: Feishu client-side features → manual testing only
 */

test.describe('IM — SPEC-034', () => {
  test.beforeAll(async ({ request }) => {
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: ADMIN });
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post('http://data-agent:8080/api/v1/auth/login', { data: { username: ADMIN.username, password: ADMIN.password } });
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
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // ═══ UI-161: 飞书 App ID / Secret 配置页 ═══
  test('[UI-161] IM — 飞书配置页', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.waitForSelector('[data-testid="sysconfig-im-feishu"]', { timeout: 10000 });

    // Verify Feishu config inputs exist
    const appIdInput = page.locator('[data-testid="im-feishu-app-id"]');
    const appSecretInput = page.locator('[data-testid="im-feishu-app-secret"]');
    const saveBtn = page.locator('[data-testid="im-feishu-save"]');

    await expect(appIdInput).toBeVisible();
    await expect(appSecretInput).toBeVisible();
    await expect(saveBtn).toBeVisible();
    // App Secret should be password type
    await expect(appSecretInput).toHaveAttribute('type', 'password');
  });

  // ═══ UI-162: 保存飞书配置 ═══
  test('[UI-162] IM — 保存飞书配置', async ({ page }) => {
    await page.goto('/admin/settings');
    await page.waitForSelector('[data-testid="sysconfig-im-feishu"]', { timeout: 10000 });

    await page.locator('[data-testid="im-feishu-app-id"]').fill('cli_test_app_id');
    await page.locator('[data-testid="im-feishu-app-secret"]').fill('test_secret_key');
    await page.locator('[data-testid="im-feishu-save"]').click();

    // Toast confirms save
    await expect(page.locator('[data-testid="sysconfig-save-success-toast"]')).toBeVisible({ timeout: 3000 });
    await expect(page.locator('[data-testid="sysconfig-save-success-toast"]')).toContainText('保存成功');
  });

  // ═══ UI-163~166: 飞书客户端功能 → 人工测试 ═══
});
