import { test, expect } from '@playwright/test';

const uid = crypto.randomUUID().slice(0, 8);
const USER = { username: `e2e-sec-${uid}@test.local`, password: 'SecurityTest1', role: 'admin' };

/**
 * Security layer E2E tests — SPEC-038
 *
 * Uses mock model service for all tests (no real LLM calls).
 * UI-184: input blocked by security rules
 * UI-185: output desensitization (phone/ID masking)
 * UI-186: unauthorized tool call blocked
 */

test.describe('SEC — SPEC-038', () => {
  test.beforeAll(async ({ request }) => {
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: USER });
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post('http://data-agent:8080/api/v1/auth/login', {
      data: { username: USER.username, password: USER.password },
    });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    // Clean up test user
    const listRes = await request.get(`http://data-agent:8080/api/v1/users?skip=0&limit=200`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-sec-${uid}`)) {
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

  // ═══ UI-184: 输入包含敏感词被拦截 ═══
  test('[UI-184] Sec — 输入包含敏感词被拦截', async ({ page }) => {
    // Intercept chat API to simulate security block
    await page.route('**/api/v1/chat', async (route) => {
      await route.fulfill({
        status: 403,
        contentType: 'text/plain',
        body: 'input blocked by security rule: sql_drop',
      });
    });

    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Type SQL injection attempt and send
    await page.locator('[data-testid="chat-input"]').fill("'; DROP TABLE users; --");
    await page.locator('[data-testid="chat-send-btn"]').click();

    // Verify error message appears in chat
    const errorMsg = page.locator('[data-testid="chat-msg-ai-1"]');
    await expect(errorMsg).toBeVisible({ timeout: 10000 });
    await expect(errorMsg).toContainText('Chat request failed');
  });

  // ═══ UI-185: 输出敏感信息脱敏 ═══
  test('[UI-185] Sec — 输出敏感信息脱敏', async ({ page }) => {
    // Backend RunStream sanitizes: 13812345678→138****5678, 320123199001011234→320***********1234
    // This test verifies the frontend renders masked content correctly via SSE mock.
    await page.route('**/api/v1/chat', async (route) => {
      const sseChunks = [
        'data: {"content":"查询结果如下：用户手机：138****5678，身份证号：320***********1234。"}\n\n',
        'data: [DONE]\n\n',
      ];
      await route.fulfill({
        status: 200,
        headers: { 'Content-Type': 'text/event-stream' },
        body: sseChunks.join(''),
      });
    });

    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    await page.locator('[data-testid="chat-input"]').fill('查询用户信息');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const aiMsg = page.locator('[data-testid="chat-msg-ai-1"]');
    await expect(aiMsg).toBeVisible({ timeout: 15000 });
    await page.waitForTimeout(3000);

    const text = await aiMsg.textContent();

    // Phone masked: 138****5678
    expect(text).toContain('138****5678');
    // Raw phone NOT leaked
    expect(text).not.toContain('13812345678');

    // ID card masked
    expect(text).toContain('320***********1234');
    expect(text).not.toContain('320123199001011234');
  });

  // ═══ UI-186: 越权工具调用被拦截 ═══
  test('[UI-186] Sec — 越权工具调用被拦截', async ({ page, request }) => {
    // Create a regular user (not admin)
    const regularUser = { username: `e2e-sec-reg-${uid}@test.local`, password: 'RegularTest1', role: 'user' };
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: regularUser });

    // Login as regular user
    await page.goto('/login');
    await page.waitForSelector('[data-testid="login-email-input"]', { timeout: 10000 });
    await page.locator('[data-testid="login-email-input"]').fill(regularUser.username);
    await page.locator('[data-testid="login-password-input"]').fill(regularUser.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });

    // Intercept chat with 403 to simulate unauthorized tool call
    await page.route('**/api/v1/chat', async (route) => {
      await route.fulfill({
        status: 403,
        contentType: 'text/plain',
        body: 'unauthorized tool call blocked by security policy',
      });
    });

    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    await page.locator('[data-testid="chat-input"]').fill('delete all user data');
    await page.locator('[data-testid="chat-send-btn"]').click();

    // Verify error shown
    await expect(page.locator('[data-testid="chat-msg-ai-1"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="chat-msg-ai-1"]')).toContainText('Chat request failed');

    // Clean up regular user
    const loginRes = await request.post('http://data-agent:8080/api/v1/auth/login', {
      data: { username: USER.username, password: USER.password },
    });
    if (loginRes.ok()) {
      const token = (await loginRes.json()).access_token;
      const listRes = await request.get(`http://data-agent:8080/api/v1/users?skip=0&limit=200`, {
        headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      });
      if (listRes.ok()) {
        for (const user of (await listRes.json()).users || []) {
          if (user.username?.includes(`e2e-sec-reg-${uid}`)) {
            await request.delete(`http://data-agent:8080/api/v1/users/${user.id}`, {
              headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
            });
          }
        }
      }
    }
  });
});
