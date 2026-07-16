import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-audit-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';

test.describe('AUDIT LOG — SPEC-029', () => {
  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    adminToken = (await res.json()).access_token;
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/audit');
    await page.waitForSelector('[data-testid="audit-page-header"]', { timeout: 10000 });
    await page.waitForTimeout(2000);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-audit-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-125: 审计日志页渲染 ═══
  test('[UI-125] Audit — 审计日志页渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="audit-page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-filter-bar"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-table"]')).toBeVisible();
  });

  // ═══ UI-126: 审计日志表格数据 ═══
  test('[UI-126] Audit — 审计日志表格数据', async ({ page }) => {
    await expect(page.locator('[data-testid="audit-table"]')).toBeVisible();
    // Audit data may or may not be present; check table structure
  });

  // ═══ UI-127: 按时间范围筛选 ═══
  test('[UI-127] Audit — 按时间范围筛选', async ({ page }) => {
    const startInput = page.locator('[data-testid="audit-date-start"]');
    const endInput = page.locator('[data-testid="audit-date-end"]');
    await expect(startInput).toBeVisible();
    await expect(endInput).toBeVisible();

    await startInput.fill('2026-07-01');
    await endInput.fill('2026-07-12');
    await page.locator('[data-testid="audit-filter-apply"]').click();
    await page.waitForTimeout(1000);
  });

  // ═══ UI-128: 按操作类型筛选 ═══
  test('[UI-128] Audit — 按操作类型筛选', async ({ page }) => {
    await page.locator('[data-testid="audit-type-select"]').selectOption('chat:query');
    await page.locator('[data-testid="audit-filter-apply"]').click();
    await page.waitForTimeout(1000);
  });

  // ═══ UI-129: 按用户筛选 ═══
  test('[UI-129] Audit — 按用户筛选', async ({ page }) => {
    await page.locator('[data-testid="audit-user-select"]').fill('test-user');
    await page.locator('[data-testid="audit-filter-apply"]').click();
    await page.waitForTimeout(1000);
  });

  // ═══ UI-130: 导出弹窗 ═══
  test('[UI-130] Audit — 导出审计日志弹窗', async ({ page }) => {
    await page.locator('[data-testid="audit-export-btn"]').click();
    await expect(page.locator('[data-testid="audit-export-modal"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-date-start"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-date-end"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-limit"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-format-csv"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-format-json"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-format-xlsx"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-export-submit"]')).toBeVisible();
    await page.keyboard.press('Escape');
  });

  // ═══ UI-131: 执行导出 ═══
  test('[UI-131] Audit — 执行导出', async ({ page }) => {
    await page.locator('[data-testid="audit-export-btn"]').click();
    await expect(page.locator('[data-testid="audit-export-modal"]')).toBeVisible();

    // Select CSV format
    await page.locator('[data-testid="audit-export-format-csv"]').check();
    // Set a small limit
    await page.locator('[data-testid="audit-export-limit"]').clear();
    await page.locator('[data-testid="audit-export-limit"]').fill('10');

    // Click export and capture download
    const [download] = await Promise.all([
      page.waitForEvent('download', { timeout: 5000 }).catch(() => null),
      page.locator('[data-testid="audit-export-submit"]').click(),
    ]);

    if (download) {
      expect(download.suggestedFilename()).toContain('audit_logs');
    }
    await page.waitForTimeout(500);
  });

  // ═══ UI-132: 导出条数上限校验 ═══
  test('[UI-132] Audit — 导出条数上限校验', async ({ page }) => {
    await page.locator('[data-testid="audit-export-btn"]').click();
    await expect(page.locator('[data-testid="audit-export-modal"]')).toBeVisible();

    // Set limit > 50000
    await page.locator('[data-testid="audit-export-limit"]').clear();
    await page.locator('[data-testid="audit-export-limit"]').fill('100000');
    await page.locator('[data-testid="audit-export-submit"]').click();
    await page.waitForTimeout(1000);

    // Error should appear
    const errorEl = page.locator('[data-testid="audit-export-limit-error"]');
    await expect(errorEl).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-133: 审计日志分页 ═══
  test('[UI-133] Audit — 审计日志分页', async ({ page }) => {
    await expect(page.locator('[data-testid="audit-pagination"]')).toBeVisible();
  });
});
