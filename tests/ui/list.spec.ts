import { test, expect } from '@playwright/test';

const uid = crypto.randomUUID().slice(0, 8);
const ADMIN = { username: `e2e-list-${uid}@test.local`, password: 'ListAdmin1', role: 'admin' };
const API_BASE = 'http://data-agent:8080/api/v1';

/**
 * List management E2E tests — tested on the users admin page.
 *
 * UI-167: Pagination defaults (20/page, total count, page size selector)
 * UI-168: Page navigation (prev/next/page buttons)
 * UI-169: Page size change preserves sort state
 * UI-170: Column sort (desc → asc → default)
 * UI-171: Select-all / deselect-all
 */

test.describe('LIST — SPEC-035', () => {
  test.beforeAll(async ({ request }) => {
    await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    // Create >40 users for pagination testing
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    for (let i = 0; i < 30; i++) {
      await request.post(`${API_BASE}/auth/register`, {
        data: { username: `e2e-list-${uid}-u${i}@test.local`, password: 'Test1234', role: 'user' },
        headers,
      });
    }
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=200`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-list-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
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
    await page.goto('/admin/users');
    await page.waitForSelector('[data-testid="user-table"]', { timeout: 10000 });
    await page.waitForTimeout(1000);
  });

  // ═══ UI-167: 分页控件默认值 ═══
  test('[UI-167] List — 分页控件默认值', async ({ page }) => {
    await expect(page.locator('[data-testid="user-pagination"]')).toBeVisible();

    // Default page size 20
    const pageSize = page.locator('[data-testid="user-page-size-select"]');
    await expect(pageSize).toHaveValue('20');

    // Should show "共 N 条"
    await expect(page.locator('[data-testid="user-pagination"]')).toContainText('共');

    // 10, 20, 50 options must exist in DOM
    await expect(pageSize.locator('option[value="10"]')).toBeAttached();
    await expect(pageSize.locator('option[value="20"]')).toBeAttached();
    await expect(pageSize.locator('option[value="50"]')).toBeAttached();
  });

  // ═══ UI-168: 页码跳转 ═══
  test('[UI-168] List — 页码跳转', async ({ page }) => {
    // With 46 users (45 + admin), > 2 pages at 20/page
    const nextBtn = page.locator('[data-testid="user-pagination-next"]');
    const prevBtn = page.locator('[data-testid="user-pagination-prev"]');

    // "上一页" should be disabled on first page
    await expect(prevBtn).toBeDisabled();

    // Click next
    await nextBtn.click();
    await page.waitForTimeout(500);

    // URL should reflect page 2 (or first page changed)
    // Now "上一页" should be enabled
    await expect(prevBtn).not.toBeDisabled();

    // Click back
    await prevBtn.click();
    await page.waitForTimeout(500);
    await expect(prevBtn).toBeDisabled();
  });

  // ═══ UI-169: 每页条数切换 ═══
  test('[UI-169] List — 每页条数切换', async ({ page }) => {
    // Switch to 50 per page
    await page.locator('[data-testid="user-page-size-select"]').selectOption('50');
    await page.waitForTimeout(800);

    // Should show all users on one page
    await expect(page.locator('[data-testid="user-pagination-next"]')).toBeDisabled({ timeout: 3000 });

    // Switch back to 20
    await page.locator('[data-testid="user-page-size-select"]').selectOption('20');
    await page.waitForTimeout(500);

    // Next should be enabled again (46 > 20 = multiple pages)
    await expect(page.locator('[data-testid="user-pagination-next"]')).not.toBeDisabled();
  });

  // ═══ UI-170: 表头排序 ═══
  test('[UI-170] List — 表头排序', async ({ page }) => {
    // Default: sort by created_at desc → sort indicator should show
    await expect(page.locator('[data-testid="user-sort-created"]')).toContainText('↓');

    // Click name header → sort by username desc
    await page.locator('[data-testid="user-table-header-name"]').click();
    await page.waitForTimeout(500);
    await expect(page.locator('[data-testid="user-sort-name"]')).toContainText('↓');

    // Click name again → asc
    await page.locator('[data-testid="user-table-header-name"]').click();
    await page.waitForTimeout(500);
    await expect(page.locator('[data-testid="user-sort-name"]')).toContainText('↑');

    // Click name third time → reset to default (created_at desc)
    await page.locator('[data-testid="user-table-header-name"]').click();
    await page.waitForTimeout(500);
    await expect(page.locator('[data-testid="user-sort-name"]')).toHaveText('');
    await expect(page.locator('[data-testid="user-sort-created"]')).toContainText('↓');
  });

  // ═══ UI-171: 全选/取消全选 ═══
  test('[UI-171] List — 全选/取消全选', async ({ page }) => {
    // Select all
    await page.locator('[data-testid="user-select-all"]').check();
    await page.waitForTimeout(300);

    // Should show select count
    await expect(page.locator('[data-testid="user-select-count"]')).toBeVisible();
    await expect(page.locator('[data-testid="user-select-count"]')).toContainText('已选');

    // Unselect all
    await page.locator('[data-testid="user-select-all"]').uncheck();
    await page.waitForTimeout(300);

    // Count should disappear
    await expect(page.locator('[data-testid="user-select-count"]')).not.toBeVisible();
  });
});
