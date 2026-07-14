import { test, expect } from '@playwright/test';

const uid = Date.now().toString(36);
const API_BASE = 'http://data-agent:8080/api/v1';
const USER = { username: `e2e-im-${uid}@test.local`, password: 'ImTestPass1' };

test.describe('IM — SPEC-034', () => {
  test.beforeAll(async ({ request }) => {
    await request.post(`${API_BASE}/auth/register`, { data: USER });
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: USER.username, password: USER.password } });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-im-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-161: 飞书用户绑定页 ═══
  test('[UI-161] IM — 飞书用户绑定页', async ({ page, request }) => {
    // Generate a bind token
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: USER.username, password: USER.password } });
    const authToken = (await loginRes.json()).access_token;
    const genRes = await request.post(`${API_BASE}/im/bind/generate-token`, {
      data: { feishu_user_id: 'ou_test_user_001' },
      headers: { Authorization: `Bearer ${authToken}`, 'Content-Type': 'application/json' },
    });
    expect(genRes.ok()).toBe(true);
    const { token: bindToken } = await genRes.json();

    // Visit bind page with valid token
    await page.goto(`/im/bind?token=${bindToken}`);
    await page.waitForSelector('[data-testid="im-bind-page"]', { timeout: 10000 });

    // Verify form elements
    await expect(page.locator('[data-testid="im-bind-email"]')).toBeVisible();
    await expect(page.locator('[data-testid="im-bind-password"]')).toBeVisible();
    await expect(page.locator('[data-testid="im-bind-submit"]')).toBeVisible();

    // Submit real binding via backend confirm endpoint
    await page.locator('[data-testid="im-bind-email"]').fill(USER.username);
    await page.locator('[data-testid="im-bind-password"]').fill(USER.password);
    await page.locator('[data-testid="im-bind-submit"]').click();

    // Success — the real backend validates and binds
    await expect(page.locator('text=绑定成功')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-162: 绑定 Token 过期 ═══
  test('[UI-162] IM — 绑定 Token 过期', async ({ page, request }) => {
    // Generate an already-expired token via ttl_seconds: -1
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: USER.username, password: USER.password } });
    const authToken = (await loginRes.json()).access_token;
    const genRes = await request.post(`${API_BASE}/im/bind/generate-token`, {
      data: { feishu_user_id: 'ou_test_002', ttl_seconds: -1 },
      headers: { Authorization: `Bearer ${authToken}`, 'Content-Type': 'application/json' },
    });
    expect(genRes.ok()).toBe(true);
    const { token: expiredToken } = await genRes.json();

    await page.goto(`/im/bind?token=${expiredToken}`);
    await page.waitForSelector('[data-testid="im-bind-expired"]', { timeout: 10000 });

    await expect(page.locator('[data-testid="im-bind-expired"]')).toContainText('已过期');
  });
});
