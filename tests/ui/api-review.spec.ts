import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-api-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
const ADMIN2 = { username: `e2e-api-adm2-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';
let admin2Token = '';

// Helper: create an API review via API
async function createAPIReview(request: any, token: string, name = 'test-api') {
  const res = await request.post(`${API_BASE}/admin/api-reviews`, {
    data: { name, file_name: `${name}.yaml`, domain: 'api.example.com', version: '3.0', endpoints: 3, rate_limit: 100 },
    headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
  });
  return res.ok() ? res.json() : null;
}

test.describe('API REVIEW — SPEC-030', () => {
  const createdIDs: string[] = [];

  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    adminToken = (await res.json()).access_token;

    // Register second admin for dual-review testing
    res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN2 });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN2.username, password: ADMIN2.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN2.username, password: ADMIN2.password } });
    }
    admin2Token = (await res.json()).access_token;

    // Create a test review (from ADMIN, so ADMIN2 can approve)
    const r = await createAPIReview(request, adminToken, 'CRM 客户查询 API');
    if (r) createdIDs.push(r.id);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/api-review');
    await page.waitForSelector('[data-testid="api-page-header"]', { timeout: 10000 });
    await page.waitForTimeout(1500);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-api-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-134: API 审核页渲染 ═══
  test('[UI-134] API — API 转换审核页渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="api-page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="api-upload-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="api-batch-upload-btn"]')).toBeVisible();
  });

  // ═══ UI-135: API 卡片渲染 ═══
  test('[UI-135] API — API 卡片渲染', async ({ page }) => {
    const cards = page.locator('[data-testid^="api-card-"]');
    const count = await cards.count();
    expect(count).toBeGreaterThanOrEqual(1);
    const firstCard = cards.first();
    await expect(firstCard.locator('[data-testid="api-card-name"]')).toBeVisible();
    await expect(firstCard.locator('[data-testid="api-card-desc"]')).toBeVisible();
    await expect(firstCard.locator('[data-testid="api-card-meta"]')).toBeVisible();
    await expect(firstCard.locator('[data-testid="api-card-status"]')).toBeVisible();
  });

  // ═══ UI-136: 上传 OpenAPI 文件 ═══
  test('[UI-136] API — 上传 OpenAPI 文件', async ({ page }) => {
    await page.locator('[data-testid="api-upload-btn"]').click();
    await expect(page.locator('[data-testid="api-upload-modal"]')).toBeVisible();
    await expect(page.locator('[data-testid="api-upload-file"]')).toBeVisible();
    await expect(page.locator('[data-testid="api-upload-rate-limit"]')).toBeVisible();

    // Fill form
    await page.locator('[data-testid="api-upload-file"]').fill('sales-api.yaml');
    await page.keyboard.press('Escape');
  });

  // ═══ UI-137: 批准 API 转换 ═══
  test('[UI-137] API — 批准 API 转换', async ({ page }) => {
    // Login as ADMIN2 (different from the creator ADMIN) to see approve buttons
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN2.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN2.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/api-review');
    await page.waitForSelector('[data-testid="api-page-header"]', { timeout: 10000 });
    await page.waitForTimeout(1500);

    const approveBtn = page.locator('[data-testid^="api-approve-btn-"]').first();
    await expect(approveBtn).toBeVisible({ timeout: 5000 });
    await approveBtn.click();
    await page.waitForTimeout(500);
  });

  // ═══ UI-138: 驳回 API 转换 ═══
  test('[UI-138] API — 驳回 API 转换', async ({ page }) => {
    // Login as ADMIN2
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN2.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN2.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/api-review');
    await page.waitForSelector('[data-testid="api-page-header"]', { timeout: 10000 });
    await page.waitForTimeout(1500);

    const rejectBtn = page.locator('[data-testid^="api-reject-btn-"]').first();
    await expect(rejectBtn).toBeVisible({ timeout: 5000 });
    await rejectBtn.click();
    await expect(page.locator('[data-testid="api-reject-reason"]')).toBeVisible();
    await page.locator('[data-testid="api-reject-reason"]').fill('域名不在白名单中');
    await page.keyboard.press('Escape');
  });

  // ═══ UI-139: 双重审核校验 ═══
  test('[UI-139] API — 双重审核校验', async ({ page }) => {
    // Login as ADMIN (the creator) — should see own submission without approve/reject
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/api-review');
    await page.waitForSelector('[data-testid="api-page-header"]', { timeout: 10000 });
    await page.waitForTimeout(1500);

    const cards = page.locator('[data-testid^="api-card-"]');
    const count = await cards.count();
    expect(count).toBeGreaterThanOrEqual(1);

    // Own submission should show status text, NOT approve/reject buttons
    const firstCard = cards.first();
    const cardText = (await firstCard.textContent()) || '';
    // Should not contain action buttons for own submission
    const approveBtns = firstCard.locator('[data-testid^="api-approve-btn-"]');
    const rejectBtns = firstCard.locator('[data-testid^="api-reject-btn-"]');
    expect(await approveBtns.count()).toBe(0);
    expect(await rejectBtns.count()).toBe(0);
  });

  // ═══ UI-140: 批量上传 ═══
  test('[UI-140] API — 批量上传 OpenAPI 文件', async ({ page }) => {
    await page.locator('[data-testid="api-batch-upload-btn"]').click();
    await expect(page.locator('[data-testid="api-upload-modal"]')).toBeVisible();
    await page.keyboard.press('Escape');
  });
});
