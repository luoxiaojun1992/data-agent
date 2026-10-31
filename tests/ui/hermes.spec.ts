import { registerUser } from './utils/auth';
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const uid = crypto.randomUUID().slice(0, 8);

const USER = { username: `e2e-hermes-${uid}@test.local`, password: 'E2eTest1!', role: 'user' };

async function seedMock(request: any, key: string, response: string) {
  await request.post(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
    data: { key, response },
  });
}

async function clearMocks(request: any) {
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});
}

test.describe('HERMES — SPEC-021', () => {
  test.beforeAll(async ({ request }) => {
    expect((await registerUser(request, USER)).status()).toBe(201);
  });

  test.afterAll(async ({ request }) => {
    await clearMocks(request);
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: USER.username, password: USER.password },
    });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    });
    if (listRes.ok()) {
      for (const u of (await listRes.json()).users || []) {
        if (u.username?.includes(`e2e-hermes-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, {
            headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
          }).catch(() => {});
        }
      }
    }
  });

  test.beforeEach(async ({ page, request }) => {
    await clearMocks(request);
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // Hermes 探索模式尚未实现（页面为 hermes-under-construction 建设中状态），
  // UI-057~061 原断言 hermes-search-area/query-input/submit-btn 等未实现的
  // 交互元素，现收敛为「建设中」断言，待探索模式落地后再恢复交互测试。
  test('[UI-057] Hermes — 页面渲染（建设中）', async ({ page }) => {
    await page.goto('/hermes');
    await expect(page.locator('[data-testid="hermes-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="hermes-page-title"]')).toContainText('Hermes');
    await expect(page.locator('[data-testid="hermes-under-construction"]')).toBeVisible();
  });
});
