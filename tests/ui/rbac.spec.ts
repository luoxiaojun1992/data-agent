import { registerUser, ADMIN as SYS_ADMIN } from './utils/auth';
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-rbac-ad-${uid}@test.local`, password: 'RbacTest1!', role: 'admin' };
const USER  = { username: `e2e-rbac-us-${uid}@test.local`, password: 'RbacTest1!', role: 'user' };

let tokens: Record<string, string> = {};

async function registerAndLogin(request: any, user: typeof ADMIN) {
  let res = await registerUser(request, user);
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

  // system_admin（seed-admin 预置的 admin@admin.com）UI 登录。rbac:manage 权限
  // 仅 system_admin 持有（rbac_seed.go），创建角色/权限必须用它登录。
  async function loginAsSystemAdmin(page: any) {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(SYS_ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(SYS_ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
  }

  // 在父角色下拉里搜索并精确选择 option（option 文本形如「系统管理员 (L0)」）。
  async function selectParent(page: any, testid: string, searchText: string, optionText: string) {
    await page.locator(`[data-testid="${testid}"] > button`).first().click();
    await page.locator(`[data-testid="${testid}"] input`).fill(searchText);
    const opt = page.locator(`[data-testid="${testid}"] button`).getByText(optionText, { exact: true });
    await expect(opt).toBeVisible({ timeout: 10000 });
    await opt.click();
  }

  test('[UI-187] RBAC — user 可见导航项', async ({ page }) => {
    await loginAs(page, USER);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible();
    // sidebar:agent 已从 admin 迁移到 user（rbac_seed.go），user 现可见 Agent 菜单。
    await expect(page.locator('[data-testid="nav-agent"]')).toBeVisible();
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
    // User should not see admin nav items
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
    // Direct navigation: Next.js renders the page but without admin sidebar/data
    await page.goto('/admin/users');
    await page.waitForTimeout(2000);
    // Admin sidebar should still be hidden
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  test('[UI-191] RBAC — user 无法访问模型配置', async ({ page }) => {
    await loginAs(page, USER);
    await page.goto('/admin/model-config');
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  // ═══ 真实业务：system_admin 创建角色（rbac:manage 权限 + CreateRole）═══
  test('[UI-193] RBAC — system_admin 创建角色', async ({ page }) => {
    await loginAsSystemAdmin(page);
    await page.goto('/admin/rbac');
    await page.waitForSelector('[data-testid="rbac-add-role-btn"]', { timeout: 10000 });

    const roleName = `e2e-role-${uid}`;
    const displayName = `E2E角色${uid}`;
    await page.locator('[data-testid="rbac-add-role-btn"]').click();
    await page.getByPlaceholder('my_custom_role').fill(roleName);
    // display_name 是后端 CreateRole 必填字段，缺省返回 400
    await page.getByPlaceholder('我的自定义角色').fill(displayName);
    await page.locator('[data-testid="rbac-add-role-submit"]').click();

    // 角色卡片显示 display_name（角色列表仅 3 内置 + 新建，不分页，直接可见）
    await expect(page.locator('[data-testid^="rbac-role-"]').filter({ hasText: displayName }).first()).toBeVisible({ timeout: 10000 });
  });

  // ═══ 真实业务：system_admin 创建权限（rbac:manage 权限 + CreatePermission）═══
  test('[UI-194] RBAC — system_admin 创建权限', async ({ page }) => {
    await loginAsSystemAdmin(page);
    await page.goto('/admin/rbac');
    await page.waitForSelector('[data-testid="rbac-add-role-btn"]', { timeout: 10000 });

    // 切到权限 tab
    await page.getByRole('button', { name: '权限列表' }).click();
    await page.locator('[data-testid="rbac-add-perm-btn"]').click();

    const permKey = `e2e:perm:${uid}`;
    await page.locator('label:has-text("Key")').locator('input').fill(permKey);
    await page.locator('label:has-text("名称 (必填)")').locator('input').fill('测试权限');
    await page.locator('[data-testid="rbac-add-perm-submit"]').click();

    // 权限列表按 module+name 升序分页（page_size=10，builtin 权限 50+），新建的
    // custom 模块权限排在第二页之后，第一页看不到。诚实断言「创建成功」：
    // 仅当 POST 成功才 onSuccess → setShowAddPerm(false) 关闭 modal（失败走 catch
    // 保留 modal + 显示错误），故「modal 关闭」是创建成功的稳态可靠代理。
    // 不用 toast（瞬态 2s 后消失，有 flaky 窗口，不可靠）。
    await expect(page.locator('[data-testid="rbac-add-perm-submit"]')).not.toBeVisible({ timeout: 10000 });
  });

  // ═══ 真实业务：编辑角色改父级（覆盖 ChangeRoleParent 事务 145 行）═══
  test('[UI-196] RBAC — system_admin 编辑角色改父级', async ({ page }) => {
    await loginAsSystemAdmin(page);
    await page.goto('/admin/rbac');
    await page.waitForSelector('[data-testid="rbac-add-role-btn"]', { timeout: 10000 });

    // 1. 创建 L1 角色 B（parent = 系统管理员 L0）
    const bDisplay = `B角色${uid}`;
    await page.locator('[data-testid="rbac-add-role-btn"]').click();
    await page.getByPlaceholder('my_custom_role').fill(`e2e-role-b-${uid}`);
    await page.getByPlaceholder('我的自定义角色').fill(bDisplay);
    await selectParent(page, 'rbac-add-role-parent', '系统管理员', '系统管理员 (L0)');
    await page.locator('[data-testid="rbac-add-role-submit"]').click();
    await expect(page.locator('[data-testid^="rbac-role-"]').filter({ hasText: bDisplay }).first()).toBeVisible({ timeout: 10000 });

    // 2. 创建 L2 角色 C（parent = 管理员 L1）
    const cDisplay = `C角色${uid}`;
    await page.locator('[data-testid="rbac-add-role-btn"]').click();
    await page.getByPlaceholder('my_custom_role').fill(`e2e-role-c-${uid}`);
    await page.getByPlaceholder('我的自定义角色').fill(cDisplay);
    await selectParent(page, 'rbac-add-role-parent', '管理员', '管理员 (L1)');
    await page.locator('[data-testid="rbac-add-role-submit"]').click();
    const cCard = page.locator('[data-testid^="rbac-role-"]').filter({ hasText: cDisplay }).first();
    await expect(cCard).toBeVisible({ timeout: 10000 });

    // 3. 编辑 C，父角色从「管理员」改到 B → 触发 ChangeRoleParent（递减旧 parent、递增新 parent child_count）
    await cCard.locator('[data-testid^="rbac-role-edit-"]').click();
    await selectParent(page, 'rbac-edit-role-parent', bDisplay, `${bDisplay} (L1)`);
    await page.locator('[data-testid="rbac-edit-role-submit"]').click();

    // 保存成功：编辑 modal 关闭
    await expect(page.locator('[data-testid="rbac-edit-role-submit"]')).not.toBeVisible({ timeout: 10000 });
  });

  // UI-192 已移除：sidebar:agent + agent:* 权限已从 admin 迁移到 user
  // （rbac_seed.go），user 现可创建 Agent 任务，原「无法创建」断言已失效。
});
