import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();

const ADMIN = { username: `e2e-role-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';

test.describe('ROLE MANAGEMENT — SPEC-024', () => {
  test.beforeAll(async ({ request }) => {
    const regRes = await request.post(`${API_BASE}/auth/register`, {
      data: { username: ADMIN.username, password: ADMIN.password, role: ADMIN.role },
    });
    if (regRes.status() !== 201) {
      // Might already exist from previous run
      const loginRes = await request.post(`${API_BASE}/auth/login`, {
        data: { username: ADMIN.username, password: ADMIN.password },
      });
      const body = await loginRes.json();
      adminToken = body.access_token;
      return;
    }
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: ADMIN.username, password: ADMIN.password },
    });
    const body = await loginRes.json();
    adminToken = body.access_token;
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/roles');
    await page.waitForSelector('[data-testid="admin-roles-header"]', { timeout: 5000 });
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    // Delete custom roles created during tests
    const listRes = await request.get(`${API_BASE}/roles`, { headers });
    if (listRes.ok()) {
      const body = await listRes.json();
      for (const role of body.roles || []) {
        if (role.type === 'custom') {
          await request.delete(`${API_BASE}/roles/${role.id}`, { headers });
        }
      }
    }
    // Delete admin user
    const userListRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (userListRes.ok()) {
      const body = await userListRes.json();
      for (const user of body.users || []) {
        if (user.username === ADMIN.username) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-085: 权限管理页渲染 ═══
  test('[UI-085] Role — 权限管理页渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="admin-roles-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="admin-roles-title"]')).toHaveText('权限管理');
    await expect(page.locator('[data-testid="role-create-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="role-tabs"]')).toBeVisible();
    // Default tab should be "roles" (角色)
    await expect(page.locator('[data-testid="role-tab-roles"]')).toBeVisible();
  });

  // ═══ UI-086: 固定角色卡片 ═══
  test('[UI-086] Role — 固定角色卡片', async ({ page }) => {
    await page.waitForTimeout(2000); // wait for API response

    const cards = page.locator('[data-testid="role-fixed-cards"]');
    await expect(cards).toBeVisible({ timeout: 5000 });

    // Should have 4 fixed role cards
    const cardEls = page.locator('[data-testid^="role-fixed-card-"]');
    const count = await cardEls.count();
    expect(count).toBe(4);

    // Check first card content
    const card0 = page.locator('[data-testid="role-fixed-card-0"]');
    await expect(card0).toContainText('系统管理员');
    await expect(card0.locator('[data-testid="role-fixed-badge"]')).toContainText('固定角色');

    const card1 = page.locator('[data-testid="role-fixed-card-1"]');
    await expect(card1).toContainText('数据分析师');

    const card2 = page.locator('[data-testid="role-fixed-card-2"]');
    await expect(card2).toContainText('知识管理员');

    const card3 = page.locator('[data-testid="role-fixed-card-3"]');
    await expect(card3).toContainText('审计员');
  });

  // ═══ UI-087: 自定义角色表格 ═══
  test('[UI-087] Role — 自定义角色表格', async ({ page, request }) => {
    // Create a custom role first
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const createRes = await request.post(`${API_BASE}/roles`, {
      data: { name: 'test_role_087', display_name: '表格测试角色', permissions: ['password:change'] },
      headers,
    });
    expect(createRes.ok()).toBe(true);
    const newRole = await createRes.json();

    // Reload page
    await page.reload();
    await page.waitForSelector('[data-testid="admin-roles-header"]', { timeout: 5000 });
    await page.waitForTimeout(2000);

    // Custom role table should be visible
    const table = page.locator('[data-testid="role-custom-table"]');
    await expect(table).toBeVisible({ timeout: 5000 });
    await expect(table).toContainText('表格测试角色');

    // Cleanup
    await request.delete(`${API_BASE}/roles/${newRole.id}`, { headers });
  });

  // ═══ UI-088: 新建自定义角色 ═══
  test('[UI-088] Role — 新建自定义角色', async ({ page, request }) => {
    await page.locator('[data-testid="role-create-btn"]').click();
    await expect(page.locator('[data-testid="role-create-modal"]')).toBeVisible();

    // Fill form
    await page.locator('[data-testid="role-create-name"]').fill('销售经理');
    await page.waitForTimeout(300);

    // Check some permissions
    const permCheckboxes = page.locator('[data-testid="role-create-permissions"] input[type="checkbox"]');
    const count = await permCheckboxes.count();
    if (count >= 2) {
      await permCheckboxes.nth(0).check();
      await permCheckboxes.nth(1).check();
    }

    // Submit
    await page.locator('[data-testid="role-create-submit"]').click();

    // Modal should close
    await expect(page.locator('[data-testid="role-create-modal"]')).not.toBeVisible({ timeout: 5000 });
    await page.waitForTimeout(1000);

    // Verify via API that role was created
    const headers = { Authorization: `Bearer ${adminToken}` };
    const listRes = await request.get(`${API_BASE}/roles`, { headers });
    const body = await listRes.json();
    const found = body.roles.some((r: any) => r.display_name === '销售经理' && r.type === 'custom');
    expect(found).toBe(true);
  });

  // ═══ UI-089: 权限 Tab 渲染 ═══
  test('[UI-089] Role — 权限 Tab 渲染', async ({ page }) => {
    // Click permissions tab
    await page.locator('[data-testid="role-tab-permissions"]').click();
    await page.waitForTimeout(500);

    // Check permission tab content
    await expect(page.locator('[data-testid="role-permissions-tab"]')).toBeVisible();
    const permTable = page.locator('[data-testid="role-permission-table"]');
    await expect(permTable).toBeVisible({ timeout: 5000 });

    // Should have multiple permission rows
    const rows = permTable.locator('tbody tr');
    const count = await rows.count();
    expect(count).toBeGreaterThan(0);

    // Should include known permissions
    await expect(permTable).toContainText('user:manage');
  });

  // ═══ UI-090: 编辑角色权限 ═══
  test('[UI-090] Role — 编辑角色权限', async ({ page, request }) => {
    // Create a custom role
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const createRes = await request.post(`${API_BASE}/roles`, {
      data: { name: 'test_edit_role', display_name: '编辑测试角色', permissions: ['password:change'] },
      headers,
    });
    expect(createRes.ok()).toBe(true);
    const newRole = await createRes.json();

    // Reload
    await page.reload();
    await page.waitForSelector('[data-testid="admin-roles-header"]', { timeout: 5000 });
    await page.waitForTimeout(2000);

    // Click edit
    const editBtn = page.locator(`[data-testid="role-edit-btn-${newRole.id}"]`);
    await expect(editBtn).toBeVisible({ timeout: 5000 });
    await editBtn.click();
    await expect(page.locator('[data-testid="role-edit-modal"]')).toBeVisible();

    // Toggle one permission
    const checkboxes = page.locator('[data-testid="role-edit-modal"] input[type="checkbox"]');
    const cbCount = await checkboxes.count();
    if (cbCount > 1) {
      await checkboxes.nth(1).check();
    }

    // Save
    await page.locator('[data-testid="role-edit-submit"]').click();
    await expect(page.locator('[data-testid="role-edit-modal"]')).not.toBeVisible({ timeout: 5000 });

    // Verify via API
    const verifyRes = await request.get(`${API_BASE}/roles`, { headers });
    const body = await verifyRes.json();
    const updated = body.roles.find((r: any) => r.id === newRole.id);
    expect(updated).toBeDefined();
    expect(updated.permissions.length).toBeGreaterThanOrEqual(1);

    // Cleanup
    await request.delete(`${API_BASE}/roles/${newRole.id}`, { headers });
  });

  // ═══ UI-091: 删除自定义角色 ═══
  test('[UI-091] Role — 删除自定义角色', async ({ page, request }) => {
    // Create a disposable role
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const createRes = await request.post(`${API_BASE}/roles`, {
      data: { name: 'to_delete', display_name: '待删除角色', permissions: [] },
      headers,
    });
    expect(createRes.ok()).toBe(true);
    const newRole = await createRes.json();

    // Reload
    await page.reload();
    await page.waitForSelector('[data-testid="admin-roles-header"]', { timeout: 5000 });
    await page.waitForTimeout(2000);

    // Click delete
    const deleteBtn = page.locator(`[data-testid="role-delete-btn-${newRole.id}"]`);
    await expect(deleteBtn).toBeVisible({ timeout: 5000 });
    await deleteBtn.click();

    // Confirmation modal
    await expect(page.locator('[data-testid="role-delete-confirm-modal"]')).toBeVisible();

    // Cancel first
    await page.locator('[data-testid="role-delete-confirm-modal"]').getByText('取消').click();
    await expect(page.locator('[data-testid="role-delete-confirm-modal"]')).not.toBeVisible();

    // Confirm delete
    await page.locator(`[data-testid="role-delete-btn-${newRole.id}"]`).click();
    await expect(page.locator('[data-testid="role-delete-confirm-modal"]')).toBeVisible();
    await page.locator('[data-testid="role-delete-confirm-btn"]').click();
    await expect(page.locator('[data-testid="role-delete-confirm-modal"]')).not.toBeVisible({ timeout: 3000 });

    // Verify via API that role is gone
    const verifyRes = await request.get(`${API_BASE}/roles/${newRole.id}`, { headers });
    expect(verifyRes.status()).toBe(404);
  });

  // ═══ UI-092: 不可删除固定角色 ═══
  test('[UI-092] Role — 不可删除固定角色', async ({ page }) => {
    await page.waitForTimeout(2000);

    // Fixed roles cards should NOT have delete buttons
    const fixedCards = page.locator('[data-testid="role-fixed-cards"]');
    await expect(fixedCards).toBeVisible();

    // Fixed cards should not contain delete buttons
    const deleteBtns = page.locator('[data-testid="role-fixed-cards"] [data-testid^="role-delete-btn-"]');
    await expect(deleteBtns).toHaveCount(0);

    // Fixed cards should not contain edit buttons either
    const editBtns = page.locator('[data-testid="role-fixed-cards"] [data-testid^="role-edit-btn-"]');
    await expect(editBtns).toHaveCount(0);
  });
});
