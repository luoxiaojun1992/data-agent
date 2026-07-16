import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-rbac-ad-${uid}@test.local`, password: 'RbacTest1!', role: 'admin' };
const USER  = { username: `e2e-rbac-us-${uid}@test.local`, password: 'RbacTest1!', role: 'user' };

let tokens: Record<string, string> = {};

async function registerAndLogin(request: any, user: typeof ADMIN) {
  let res = await request.post(`${API_BASE}/auth/register`, { data: user });
  if (res.status() !== 201) { /* ok */ }
  res = await request.post(`${API_BASE}/auth/login`, {
    data: { username: user.username, password: user.password },
  });
  const body = await res.json();
  if (!body.access_token) throw new Error(`Login failed: ${JSON.stringify(body)}`);
  return body.access_token;
}

test.describe('RBAC — SPEC-039', () => {
  test.beforeAll(async ({ request }) => {
    tokens['admin'] = await registerAndLogin(request, ADMIN);
    tokens['user']  = await registerAndLogin(request, USER);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${tokens['admin']}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      const body = await listRes.json();
      for (const u of body.users || []) {
        if (u.username?.includes(`e2e-rbac-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers }).catch(() => {});
        }
      }
    }
  });

  async function loginAs(page: any, user: typeof ADMIN) {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(user.username);
    await page.locator('[data-testid="login-password-input"]').fill(user.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
  }

  test('[UI-187] RBAC — user 可见导航项', async ({ page }) => {
    await loginAs(page, USER);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-agent"]')).not.toBeVisible();
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  test('[UI-188] RBAC — admin 可见导航项', async ({ page }) => {
    await loginAs(page, ADMIN);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-agent"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-admin"]')).toBeVisible();
  });

  test('[UI-189] RBAC — admin + system_admin 可见全部（system_admin 由系统自动创建，仅 admin 验证）', async ({ page }) => {
    // system_admin is auto-created at first boot with random password.
    // admin has same sidebar visibility as system_admin for these items.
    await loginAs(page, ADMIN);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-agent"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-admin"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible();
  });

  test('[UI-190] RBAC — user 无法直接访问管理页面', async ({ page }) => {
    await loginAs(page, USER);
    await page.goto('/admin/users');
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="admin-users-header"]')).not.toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  test('[UI-191] RBAC — user 无法访问模型配置', async ({ page }) => {
    await loginAs(page, USER);
    await page.goto('/admin/model-config');
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  test('[UI-192] RBAC — user 无法创建 Agent 任务', async ({ page }) => {
    await loginAs(page, USER);
    await expect(page.locator('[data-testid="nav-agent"]')).not.toBeVisible();
    await page.goto('/agent');
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="agent-page-header"]')).not.toBeVisible({ timeout: 5000 });
  });
});
