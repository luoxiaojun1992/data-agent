import { registerUser } from './utils/auth';
import { test, expect } from '@playwright/test';
import path from 'path';

const uid = crypto.randomUUID().slice(0, 8);
const ADMIN = { username: `e2e-apicol-${uid}@test.local`, password: 'ApiCol1!', role: 'admin' };
const FIXTURE = path.resolve(__dirname, 'fixtures', 'files', 'openapi-spec.json');
const API_BASE = 'http://data-agent:8080/api/v1';

test.describe('API COLLECTION — SPEC-065', () => {
  test.beforeAll(async ({ request }) => {
    await registerUser(request, ADMIN);
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=200`, { headers });
    if (listRes.ok()) {
      for (const u of (await listRes.json()).users || []) {
        if (u.username?.includes(`e2e-apicol-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers }).catch(() => {});
        }
      }
    }
  });

  // 真实业务：admin 通过 UI 上传 OpenAPI 集合（覆盖 api_collection.go 的
  // Create handler + f.Read(rawSpec)，走完整前端链路而非 UT 打桩）。
  test('[UI-195] API Collection — admin 上传 OpenAPI 集合', async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 10000 });

    await page.goto('/admin/api-collections');
    await page.getByRole('button', { name: '+ 上传 OpenAPI 文件' }).click();

    const collName = `e2e-api-col-${uid}`;
    await page.getByPlaceholder('名称').fill(collName);
    await page.getByPlaceholder('描述').fill('E2E 测试集合');
    await page.locator('input[type="file"]').setInputFiles(FIXTURE);
    await page.getByRole('button', { name: '确认上传' }).click();

    // 上传成功后 modal 关闭、集合名出现在列表
    await expect(page.getByText(collName).first()).toBeVisible({ timeout: 10000 });
  });
});
