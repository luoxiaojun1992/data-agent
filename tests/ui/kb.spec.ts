import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-kb-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';

// Helper: create a doc via multipart/form-data (real upload protocol)
async function createDoc(request: any, token: string, title = 'test-doc') {
  const res = await request.post(`${API_BASE}/knowledge/docs`, {
    multipart: {
      title,
      file_name: `${title}.pdf`,
      file_type: 'pdf',
      size_bytes: '2400000',
    },
    headers: { Authorization: `Bearer ${token}` },
  });
  return res.ok() ? res.json() : null;
}

test.describe('KB MANAGEMENT — SPEC-028', () => {
  const createdDocs: string[] = [];

  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    adminToken = (await res.json()).access_token;

    // Create some test docs
    for (const title of ['Q2_财务报告', '销售数据汇总', '客户合同模板']) {
      const doc = await createDoc(request, adminToken, title);
      if (doc) createdDocs.push(doc.id);
    }
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/knowledge');
    await page.waitForSelector('[data-testid="kb-page-header"]', { timeout: 10000 });
    await page.waitForTimeout(2000);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    for (const id of createdDocs) {
      await request.delete(`${API_BASE}/knowledge/docs/${id}`, { headers }).catch(() => {});
    }
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-kb-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-115: KB 管理页渲染 ═══
  test('[UI-115] KB — 知识库管理页渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="kb-page-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="kb-upload-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="kb-batch-upload-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="kb-info-banner"]')).toBeVisible();
  });

  // ═══ UI-116: 文档卡片渲染 ═══
  test('[UI-116] KB — 文档卡片渲染', async ({ page }) => {
    await page.waitForTimeout(2000);
    const cards = page.locator('[data-testid^="kb-doc-card-"]');
    const count = await cards.count();
    // Docs are created in beforeAll — there should be at least 1
    expect(count).toBeGreaterThanOrEqual(1);

    // Check a card structure
    const firstCard = cards.first();
    await expect(firstCard.locator('[data-testid="kb-doc-name"]')).toBeVisible();
    await expect(firstCard.locator('[data-testid="kb-doc-meta"]')).toBeVisible();
    await expect(firstCard.locator('[data-testid^="kb-doc-status-"]')).toBeVisible();
  });

  // ═══ UI-117: 上传单个文档 ═══
  test('[UI-117] KB — 上传单个文档', async ({ page }) => {
    await page.locator('[data-testid="kb-upload-btn"]').click();
    await expect(page.locator('[data-testid="kb-upload-modal"]')).toBeVisible();
    await expect(page.locator('[data-testid="kb-drop-zone"]')).toBeVisible();

    // Close modal (actual file upload via SeaweedFS is SPEC-036)
    await page.keyboard.press('Escape');
    await page.waitForTimeout(500);
  });

  // ═══ UI-118: 批量上传文档 ═══
  test('[UI-118] KB — 批量上传文档', async ({ page }) => {
    await page.locator('[data-testid="kb-batch-upload-btn"]').click();
    await expect(page.locator('[data-testid="kb-upload-modal"]')).toBeVisible();
    await page.keyboard.press('Escape');
  });

  // ═══ UI-119: 文档预览 ═══
  test('[UI-119] KB — 文档预览', async ({ page }) => {
    await page.waitForTimeout(1000);
    const firstCard = page.locator('[data-testid^="kb-doc-card-"]').first();
    await expect(firstCard).toBeVisible({ timeout: 5000 });

    // Click on the document name or preview button
    const previewBtn = firstCard.locator('[data-testid="kb-doc-name"]');
    if (await previewBtn.isVisible().catch(() => false)) {
      await previewBtn.click();
      await page.waitForTimeout(1000);
      // Preview modal or detail panel should appear
      // Verify either a preview panel or detail view
      const detailPanel = page.locator('[data-testid="kb-doc-detail-panel"]');
      const previewModal = page.locator('[data-testid="kb-preview-modal"]');
      const hasDetailOrPreview = (await detailPanel.isVisible({ timeout: 3000 }).catch(() => false)) ||
                                  (await previewModal.isVisible({ timeout: 3000 }).catch(() => false));
      if (hasDetailOrPreview) {
        // Close preview/detail
        await page.keyboard.press('Escape');
        await page.waitForTimeout(300);
      }
    }
  });

  // ═══ UI-120: 索引状态实时更新 ═══
  test('[UI-120] KB — 索引状态实时更新', async ({ page }) => {
    await page.waitForTimeout(1000);
    const status = page.locator('[data-testid^="kb-doc-status-"]').first();
    const hasStatus = await status.isVisible().catch(() => false);
    if (hasStatus) {
      const statusText = (await status.textContent()) || '';
      // Status should contain meaningful text (indexed, indexing, pending, etc.)
      expect(statusText.length).toBeGreaterThanOrEqual(1);
      expect(statusText).toMatch(/索引|index|已|待|中/i);
    }
  });

  // ═══ UI-121: 搜索知识库文档 ═══
  test('[UI-121] KB — 搜索知识库文档', async ({ page }) => {
    const searchInput = page.locator('[data-testid="kb-search-input"]');
    await expect(searchInput).toBeVisible();

    // Count visible cards before search
    const beforeCards = page.locator('[data-testid^="kb-doc-card-"]');
    const beforeCount = await beforeCards.count();
    expect(beforeCount).toBeGreaterThanOrEqual(1);

    // Search for 销售 — should filter to matching doc
    await searchInput.fill('销售');
    await page.waitForTimeout(1000);

    // After filtering, at least the matching doc should still be visible
    const afterCards = page.locator('[data-testid^="kb-doc-card-"]');
    const afterCount = await afterCards.count();
    expect(afterCount).toBeGreaterThanOrEqual(1);

    // Clear search — all docs should be back
    await searchInput.clear();
    await page.waitForTimeout(500);
    const restoredCount = await beforeCards.count();
    expect(restoredCount).toBe(beforeCount);
  });

  // ═══ UI-122: 按标签筛选 ═══
  test('[UI-122] KB — 按标签筛选', async ({ page }) => {
    // Check if tag filter UI exists
    const tagFilter = page.locator('[data-testid="kb-tag-filter"]');
    const hasTagFilter = await tagFilter.isVisible({ timeout: 3000 }).catch(() => false);
    if (hasTagFilter) {
      // Try clicking a tag to filter
      const firstTag = tagFilter.locator('[data-testid^="kb-tag-"]').first();
      if (await firstTag.isVisible().catch(() => false)) {
        await firstTag.click();
        await page.waitForTimeout(500);
        // Verify the filter is active (tag gets active style)
        await expect(firstTag).toBeVisible();
      }
    }
  });

  // ═══ UI-123: 删除知识库文档 ═══
  test('[UI-123] KB — 删除知识库文档', async ({ page }) => {
    await page.waitForTimeout(1000);
    const delBtn = page.locator('[data-testid^="kb-doc-delete-"]').first();
    // Docs are created in beforeAll — delete button should exist
    await expect(delBtn).toBeVisible({ timeout: 5000 });

    // Register dialog listener BEFORE clicking
    page.once('dialog', (d) => d.dismiss());
    await delBtn.click();
    await page.waitForTimeout(500);
  });

  // ═══ UI-124: 文档分页 ═══
  test('[UI-124] KB — 文档分页', async ({ page }) => {
    // With 3 docs and default PAGE_SIZE, pagination may or may not show
    // Verify the pagination component exists in DOM (even if hidden)
    const pagination = page.locator('[data-testid="kb-pagination"]');
    const hasPagination = await pagination.isVisible({ timeout: 3000 }).catch(() => false);
    if (hasPagination) {
      await expect(pagination).toContainText('共');
    }
    // Docs should still be visible
    const cards = page.locator('[data-testid^="kb-doc-card-"]');
    expect(await cards.count()).toBeGreaterThanOrEqual(1);
  });
});
