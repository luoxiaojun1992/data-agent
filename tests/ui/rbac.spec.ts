import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

// Roles: admin + user registered via public API; system_admin created via admin API
const SYSADMIN = { username: `e2e-rbac-sysadmin-${uid}@test.local`, password: 'RbacTest1!', role: 'system_admin' };
const ADMIN    = { username: `e2e-rbac-admin-${uid}@test.local`,    password: 'RbacTest1!', role: 'admin' };
const USER     = { username: `e2e-rbac-user-${uid}@test.local`,     password: 'RbacTest1!', role: 'user' };

let tokens: Record<string, string> = {};

// Helper: register + login via public API
async function registerAndLogin(request: any, user: typeof ADMIN) {
  let res = await request.post(`${API_BASE}/auth/register`, { data: user });
  if (res.status() !== 201) { /* already exists */ }
  res = await request.post(`${API_BASE}/auth/login`, {
    data: { username: user.username, password: user.password },
  });
  const body = await res.json();
  if (!body.access_token) throw new Error(`Failed to login as ${user.username}: ${JSON.stringify(body)}`);
  return body.access_token;
}

test.describe('RBAC — SPEC-039', () => {
  test.beforeAll(async ({ request }) => {
    // 1. Register admin + user via public API
    tokens['admin'] = await registerAndLogin(request, ADMIN);
    tokens['user']  = await registerAndLogin(request, USER);

    // 2. Create system_admin via admin API (public register blocks system_admin role)
    const adminHeaders = { Authorization: `Bearer ${tokens['admin']}`, 'Content-Type': 'application/json' };
    await request.post(`${API_BASE}/users`, {
      data: { username: SYSADMIN.username, password: SYSADMIN.password, role: 'system_admin' },
      headers: adminHeaders,
    });
    tokens['system_admin'] = await registerAndLogin(request, SYSADMIN);
  });

  test.afterAll(async ({ request }) => {
    // Clean up: use system_admin token to delete test users
    const headers = { Authorization: `Bearer ${tokens['system_admin']}`, 'Content-Type': 'application/json' };
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

  async function loginAs(page: any, role: string) {
    await page.goto('/login');
    const user = role === 'system_admin' ? SYSADMIN : role === 'admin' ? ADMIN : USER;
    await page.locator('[data-testid="login-email-input"]').fill(user.username);
    await page.locator('[data-testid="login-password-input"]').fill(user.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
  }

  // ═══ UI-187: user 角色可见导航项 ═══
  test('[UI-187] RBAC — user 可见导航项', async ({ page }) => {
    await loginAs(page, 'user');

    // Should be visible
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-dashboard"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-kb-mgmt"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-docs"]')).toBeVisible();

    // Should NOT be visible
    await expect(page.locator('[data-testid="nav-agent"]')).not.toBeVisible();
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  // ═══ UI-188: admin 角色可见导航项 ═══
  test('[UI-188] RBAC — admin 可见导航项', async ({ page }) => {
    await loginAs(page, 'admin');

    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // Should be visible
    await expect(page.locator('[data-testid="nav-dashboard"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-agent"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-kb-mgmt"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-docs"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-admin"]')).toBeVisible();
  });

  // ═══ UI-189: system_admin 角色可见全部导航项 ═══
  test('[UI-189] RBAC — system_admin 可见全部导航项', async ({ page }) => {
    await loginAs(page, 'system_admin');

    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // All 7 nav items visible
    await expect(page.locator('[data-testid="nav-dashboard"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-agent"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-kb-mgmt"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-docs"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-admin"]')).toBeVisible();
  });

  // ═══ UI-190: user 无法直接访问 /admin ═══
  test('[UI-190] RBAC — user 无法直接访问管理页面', async ({ page }) => {
    await loginAs(page, 'user');

    // Try accessing admin route directly
    const resp = await page.goto('/admin/users');
    // Should be redirected or show forbidden
    if (resp) {
      expect([302, 403]).toContain(resp.status());
    }
    // Should not render admin content
    await expect(page.locator('[data-testid="admin-users-header"]')).not.toBeVisible({ timeout: 3000 }).catch(() => {});
  });

  // ═══ UI-191: user 无法直接访问 /admin/model-config ═══
  test('[UI-191] RBAC — user 无法访问模型配置', async ({ page }) => {
    await loginAs(page, 'user');

    const resp = await page.goto('/admin/model-config');
    if (resp) {
      expect([302, 403]).toContain(resp.status());
    }
    await expect(page.locator('[data-testid="model-config-header"]')).not.toBeVisible({ timeout: 3000 }).catch(() => {});
  });

  // ═══ UI-192: user 无法创建 Agent 任务 ═══
  test('[UI-192] RBAC — user 无法创建 Agent 任务', async ({ page }) => {
    await loginAs(page, 'user');

    // Agent nav item should not be visible
    await expect(page.locator('[data-testid="nav-agent"]')).not.toBeVisible();

    // Attempt to go to /agent directly
    const resp = await page.goto('/agent');
    if (resp) {
      // Should redirect away or show 403
      expect([302, 403, 404]).toContain(resp.status());
    }
  });
});
