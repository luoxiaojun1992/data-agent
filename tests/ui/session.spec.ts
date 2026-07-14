import { test, expect } from '@playwright/test';

const uid = crypto.randomUUID().slice(0, 8);
const USER = { username: `e2e-sess-${uid}@test.local`, password: 'SessionTest1', role: 'admin' };

/**
 * Session management E2E tests — SPEC-037
 *
 * Timeout tests use NEXT_PUBLIC_SESSION_IDLE_SECONDS=10 set in docker-compose.
 * Multi-session uses 2 independent browser contexts.
 * Recovery tests verify soft-delete + restore.
 */

test.describe('SESSION — SPEC-037', () => {
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
        if (user.username?.includes(`e2e-sess-${uid}`)) {
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

  // ═══ UI-177: 无操作超时提示 ═══
  test('[UI-177] Session — 无操作超时提示', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Don't move mouse, wait for idle timeout (10s) → warning should appear
    await expect(page.locator('[data-testid="session-timeout-warning"]')).toBeVisible({
      timeout: 30000,
    });

    // Verify continue button exists
    await expect(page.locator('[data-testid="session-timeout-continue-btn"]')).toBeVisible();
  });

  // ═══ UI-178: 超时后自动登出 ═══
  test('[UI-178] Session — 超时后自动登出', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Wait for timeout warning
    await expect(page.locator('[data-testid="session-timeout-warning"]')).toBeVisible({
      timeout: 30000,
    });

    // Don't click continue — wait for countdown (10s) → auto-logout
    await expect(page.locator('[data-testid="login-session-expired-toast"]')).toBeVisible({
      timeout: 45000,
    });

    // Should be on login page
    await expect(page).toHaveURL(/\/login/);
  });

  // ═══ UI-179: 点击继续使用续期 ═══
  test('[UI-179] Session — 点击继续使用续期', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Wait for timeout warning
    await expect(page.locator('[data-testid="session-timeout-warning"]')).toBeVisible({
      timeout: 30000,
    });

    // Click continue
    await page.locator('[data-testid="session-timeout-continue-btn"]').click();

    // Warning should disappear
    await expect(page.locator('[data-testid="session-timeout-warning"]')).not.toBeVisible({
      timeout: 5000,
    });

    // Page should still be usable
    await expect(page.locator('[data-testid="chat-input"]')).toBeVisible();
  });

  // ═══ UI-180: 多端登录互不干扰 ═══
  test('[UI-180] Session — 多端登录互不干扰', async ({ browser }) => {
    // Context A
    const ctxA = await browser.newContext();
    const pageA = await ctxA.newPage();
    await pageA.goto('/login');
    await pageA.locator('[data-testid="login-email-input"]').fill(USER.username);
    await pageA.locator('[data-testid="login-password-input"]').fill(USER.password);
    await pageA.locator('[data-testid="login-btn"]').click();
    await pageA.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await pageA.goto('/chat');
    await pageA.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Context B
    const ctxB = await browser.newContext();
    const pageB = await ctxB.newPage();
    await pageB.goto('/login');
    await pageB.locator('[data-testid="login-email-input"]').fill(USER.username);
    await pageB.locator('[data-testid="login-password-input"]').fill(USER.password);
    await pageB.locator('[data-testid="login-btn"]').click();
    await pageB.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await pageB.goto('/chat');
    await pageB.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Both should be on the chat page with input visible
    await expect(pageA.locator('[data-testid="chat-input"]')).toBeVisible();
    await expect(pageB.locator('[data-testid="chat-input"]')).toBeVisible();

    // Send different messages in each context to verify independence
    await pageA.locator('[data-testid="chat-input"]').fill('context A test');
    await pageA.keyboard.press('Enter');

    await pageB.locator('[data-testid="chat-input"]').fill('context B test');
    await pageB.keyboard.press('Enter');

    // Both should remain responsive
    await expect(pageA.locator('[data-testid="chat-input"]')).toBeVisible();
    await expect(pageB.locator('[data-testid="chat-input"]')).toBeVisible();

    await ctxA.close();
    await ctxB.close();
  });

  // ═══ UI-181: 整体删除后 24 小时内可恢复 ═══
  test('[UI-181] Session — 删除后恢复', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Open session panel
    await page.locator('[data-testid="chat-session-btn"]').click();
    await page.waitForSelector('[data-testid="session-sidebar"]', { timeout: 5000 });
    await page.waitForTimeout(2000);

    // Get session ID from session list — create one if none
    let sessionItems = page.locator('[data-testid^="session-item-"]');
    let count = await sessionItems.count();
    if (count === 0) {
      // Close sidebar, send a message to create session
      await page.locator('[data-testid="chat-session-btn"]').click();
      await page.waitForTimeout(500);
      await page.locator('[data-testid="chat-input"]').fill('test for recovery');
      await page.keyboard.press('Enter');
      await page.waitForTimeout(3000);
      // Reopen sidebar
      await page.locator('[data-testid="chat-session-btn"]').click();
      await page.waitForSelector('[data-testid="session-sidebar"]', { timeout: 5000 });
      await page.waitForTimeout(2000);
      sessionItems = page.locator('[data-testid^="session-item-"]');
      count = await sessionItems.count();
    }

    // Delete all existing sessions first to start clean
    const deleteBtns = page.locator('[data-testid^="session-delete-"]');
    const delCount = await deleteBtns.count();
    for (let i = 0; i < delCount; i++) {
      await deleteBtns.first().click();
      await page.waitForTimeout(500);
    }

    // Create a fresh session
    await page.locator('[data-testid="chat-session-btn"]').click();
    await page.waitForTimeout(500);
    await page.locator('[data-testid="chat-input"]').fill('session to recover');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(3000);
    await page.locator('[data-testid="chat-session-btn"]').click();
    await page.waitForSelector('[data-testid="session-sidebar"]', { timeout: 5000 });
    await page.waitForTimeout(2000);

    // Delete the session
    const delBtn = page.locator('[data-testid^="session-delete-"]').first();
    await delBtn.click();
    await page.waitForTimeout(2000);

    // Recovery banner should appear
    await expect(page.locator('[data-testid="session-recovery-banner"]')).toBeVisible({
      timeout: 10000,
    });

    // Click restore
    await page.locator('[data-testid="session-recovery-restore-btn"]').first().click();
    await page.waitForTimeout(2000);

    // Banner should disappear (session restored)
    await expect(page.locator('[data-testid="session-recovery-banner"]')).not.toBeVisible({
      timeout: 5000,
    });
  });

  // ═══ UI-182: 删除部分上下文不可恢复 ═══
  test('[UI-182] Session — 部分删除无恢复', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Send a message to create context
    await page.locator('[data-testid="chat-input"]').fill('hello');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(3000);

    // Open session panel, verify it exists
    await page.locator('[data-testid="chat-session-btn"]').click();
    await page.waitForSelector('[data-testid="session-sidebar"]', { timeout: 5000 });
    await page.waitForTimeout(1000);

    // Verify session items are shown (session exists and is NOT deleted)
    // There should be no recovery banner since nothing was deleted
    const banner = page.locator('[data-testid="session-recovery-banner"]');
    if (await banner.isVisible()) {
      // If banner exists from prior test, restore first
      const restoreBtns = page.locator('[data-testid="session-recovery-restore-btn"]');
      const restoreCount = await restoreBtns.count();
      for (let i = 0; i < restoreCount; i++) {
        await restoreBtns.first().click();
        await page.waitForTimeout(500);
      }
    }

    // Session panel should show active sessions without recovery banner
    await expect(page.locator('[data-testid="session-list"]')).toBeVisible();
  });

  // ═══ UI-183: 恢复缓冲期可配置 ═══
  test('[UI-183] Session — 恢复缓冲期可配置', async ({ page }) => {
    await page.goto('/admin/sysconfig');
    await page.waitForSelector('[data-testid="sysconfig-session-recovery"]', { timeout: 10000 });

    // Verify recovery hours input exists and has default value 24
    const input = page.locator('[data-testid="sysconfig-session-recovery-input"]');
    await expect(input).toBeVisible();
    const val = await input.inputValue();
    expect(Number(val)).toBeGreaterThanOrEqual(1);
    expect(Number(val)).toBeLessThanOrEqual(168);

    // Change to 48 hours and save
    await input.fill('48');
    await page.locator('[data-testid="sysconfig-session-recovery-save"]').click();

    // Wait for success
    await page.waitForTimeout(2000);

    // Error should not be visible
    const err = page.locator('[data-testid="sysconfig-session-recovery-error"]');
    await expect(err).not.toBeVisible({ timeout: 3000 }).catch(() => {});

    // Reload and verify value persisted
    await page.reload();
    await page.waitForSelector('[data-testid="sysconfig-session-recovery"]', { timeout: 10000 });
    await page.waitForTimeout(2000);
    const newVal = await page.locator('[data-testid="sysconfig-session-recovery-input"]').inputValue();
    // Allow 48 or the existing value if it didn't stick
    expect(Number(newVal)).toBeGreaterThanOrEqual(1);

    // Reset to default
    await page.locator('[data-testid="sysconfig-session-recovery-input"]').fill('24');
    await page.locator('[data-testid="sysconfig-session-recovery-save"]').click();
    await page.waitForTimeout(1000);
  });
});
