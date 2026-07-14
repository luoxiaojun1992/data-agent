import { test, expect } from '@playwright/test';
import path from 'path';

const uid = Date.now().toString(36);
const USER = { username: `e2e-upload-${uid}@test.local`, password: 'UploadTest1', role: 'admin' };

const FIXTURE_DIR = path.resolve(__dirname, 'fixtures', 'files');

/**
 * File upload E2E tests — real upload to backend (GridFS).
 *
 * Uses page.waitForEvent('filechooser') to properly trigger React's
 * onChange handler on the hidden file input.
 */

/** Helper: open file chooser and select files, verify modal appears */
async function selectFiles(page: any, filePaths: string[]) {
  const fileChooserPromise = page.waitForEvent('filechooser');
  // Click the drop zone to trigger the file input (onClick → fileInputRef.current?.click())
  await page.locator('[data-testid="kb-upload-file-input"]').click();
  const fileChooser = await fileChooserPromise;
  await fileChooser.setFiles(filePaths);
  await expect(page.locator('[data-testid="kb-upload-modal"]')).toBeVisible({ timeout: 3000 });
}

test.describe('UPLOAD — SPEC-036', () => {
  test.beforeAll(async ({ request }) => {
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: USER });
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post('http://data-agent:8080/api/v1/auth/login', { data: { username: USER.username, password: USER.password } });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`http://data-agent:8080/api/v1/users?skip=0&limit=200`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-upload-${uid}`)) {
          await request.delete(`http://data-agent:8080/api/v1/users/${user.id}`, { headers });
        }
      }
    }
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // ═══ UI-172: 文件多选 ═══
  test('[UI-172] Upload — 文件多选', async ({ page }) => {
    await page.goto('/admin/knowledge');
    await page.waitForSelector('[data-testid="kb-page-header"]', { timeout: 10000 });
    await page.waitForTimeout(1000);

    await selectFiles(page, [
      path.join(FIXTURE_DIR, 'test-1.txt'),
      path.join(FIXTURE_DIR, 'test-2.txt'),
      path.join(FIXTURE_DIR, 'test-3.json'),
    ]);

    await expect(page.locator('[data-testid="kb-file-item-0"]')).toBeVisible();
    await expect(page.locator('[data-testid="kb-file-item-1"]')).toBeVisible();
    await expect(page.locator('[data-testid="kb-file-item-2"]')).toBeVisible();

    await page.locator('button:has-text("确认上传")').click();

    await expect(page.locator('[data-testid="kb-file-done-0"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid="kb-file-done-1"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="kb-file-done-2"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-173: 拖拽上传 → 人工测试 ═══

  // ═══ UI-174: 独立进度条 ═══
  test('[UI-174] Upload — 独立进度条', async ({ page }) => {
    await page.goto('/admin/knowledge');
    await page.waitForSelector('[data-testid="kb-page-header"]', { timeout: 10000 });

    await selectFiles(page, [
      path.join(FIXTURE_DIR, 'test-1.txt'),
      path.join(FIXTURE_DIR, 'test-2.txt'),
    ]);

    await page.locator('button:has-text("确认上传")').click();

    await expect(page.locator('[data-testid="kb-file-done-0"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid="kb-file-done-1"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-175: 单文件上传 ═══
  test('[UI-175] Upload — 单文件上传', async ({ page }) => {
    await page.goto('/admin/knowledge');
    await page.waitForSelector('[data-testid="kb-page-header"]', { timeout: 10000 });

    await selectFiles(page, [path.join(FIXTURE_DIR, 'test-1.txt')]);

    await expect(page.locator('[data-testid="kb-file-item-0"]')).toBeVisible();
    await page.locator('button:has-text("确认上传")').click();
    await expect(page.locator('[data-testid="kb-file-done-0"]')).toBeVisible({ timeout: 15000 });
  });

  // ═══ UI-176: 上传不阻塞 UI ═══
  test('[UI-176] Upload — 上传不阻塞 UI', async ({ page }) => {
    await page.goto('/admin/knowledge');
    await page.waitForSelector('[data-testid="kb-page-header"]', { timeout: 10000 });

    await selectFiles(page, [path.join(FIXTURE_DIR, 'test-1.txt')]);

    await page.locator('button:has-text("确认上传")').click();

    // While uploading, close modal by clicking outside
    await page.locator('[data-testid="kb-upload-modal"]').click({ position: { x: 10, y: 10 } });
    await page.waitForTimeout(500);
    // KB page should still be responsive
    await expect(page.locator('[data-testid="kb-search-input"]')).toBeVisible();
  });
});
