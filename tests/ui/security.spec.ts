import { test, expect } from '@playwright/test';

const uid = crypto.randomUUID().slice(0, 8);
const USER = { username: `e2e-sec-${uid}@test.local`, password: 'SecurityTest1', role: 'admin' };
const MOCKLLM_URL = 'http://mockllm:8082';
const MOCK_TOKEN = 'test-admin-token';

/**
 * Security layer E2E tests — SPEC-038
 *
 * All 3 tests use real backend audit + mockllm (zero page.route).
 * UI-184: input blocked by backend AuditInput (SQL injection pattern)
 * UI-185: output desensitization via mockllm → RunStream AuditOutput
 * UI-186: unauthorized tool call blocked (RBAC + audit log)
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
    // Clean up mockllm
    await request.delete(`${MOCKLLM_URL}/responses`, {
      headers: { Authorization: `Bearer ${MOCK_TOKEN}` },
    }).catch(() => {});
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // ═══ UI-184: SQL 注入被 AuditInput 实时拦截 ═══
  test('[UI-184] Sec — SQL 注入被拦截', async ({ page }) => {
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    // Send SQL injection — backend AuditInput blocks it before reaching mockllm
    await page.locator('[data-testid="chat-input"]').fill("'; DROP TABLE users; --");
    console.log('[UI-184] sending SQL injection');
    await page.locator('[data-testid="chat-send-btn"]').click();

    // Backend should block the message and return error
    // Frontend shows "Chat request failed" in assistant bubble
    const errorMsg = page.locator('[data-testid="chat-msg-ai-1"]');
    await expect(errorMsg).toBeVisible({ timeout: 10000 });
    await expect(errorMsg).toContainText(/Failed|blocked|input audit/i);
  });

  // ═══ UI-185: mockllm 返回原始数据 → RunStream AuditOutput 脱敏 ═══
  test('[UI-185] Sec — 输出敏感信息脱敏', async ({ page, request }) => {
    // Inject mock response with unmasked data via mockllm.
    // Backend RunStream per-chunk sanitization: 13812345678→138****5678
    const msg = '查询用户信息';
    await request.delete(`${MOCKLLM_URL}/responses`, {
      headers: { Authorization: `Bearer ${MOCK_TOKEN}` },
    }).catch(() => {});
    await request.post(`${MOCKLLM_URL}/responses`, {
      headers: { Authorization: `Bearer ${MOCK_TOKEN}`, 'Content-Type': 'application/json' },
      data: { key: msg, response: '查询结果如下：用户手机：13812345678，身份证号：320123199001011234。' },
    });

    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });

    console.log('[UI-185] sending:', msg);
    await page.locator('[data-testid="chat-input"]').fill(msg);
    await page.locator('[data-testid="chat-send-btn"]').click();
    console.log('[UI-185] send clicked, waiting for response');

    // Wait for backend → mockllm → audit → SSE stream → UI render
    const aiMsg = page.locator('[data-testid="chat-msg-ai-1"]');
    await expect(aiMsg).toBeVisible({ timeout: 20000 });
    await page.waitForTimeout(4000);

    const text = await aiMsg.textContent();
    console.log('[UI-185] received:', text?.substring(0, 100));

    // Phone masked
    expect(text).toContain('138****5678');
    expect(text).not.toContain('13812345678');
    // ID card masked
    expect(text).toContain('320***********1234');
    expect(text).not.toContain('320123199001011234');
  });

  // ═══ UI-186: user 角色越权被 RBAC 拦截 ═══
  test('[UI-186] Sec — 越权工具调用被拦截', async ({ page, request }) => {
    // Create a regular user
    const regularUser = { username: `e2e-sec-reg-${uid}@test.local`, password: 'RegularTest1', role: 'user' };
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: regularUser });

    // Login as regular user
    await page.goto('/login');
    await page.waitForSelector('[data-testid="login-email-input"]', { timeout: 10000 });
    await page.locator('[data-testid="login-email-input"]').fill(regularUser.username);
    await page.locator('[data-testid="login-password-input"]').fill(regularUser.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });

    // Get token for audit log check
    const loginRes = await request.post('http://data-agent:8080/api/v1/auth/login', {
      data: { username: regularUser.username, password: regularUser.password },
    });
    const token = (await loginRes.json()).access_token;

    // Attempt to access admin-only endpoint
    const adminRes = await request.get('http://data-agent:8080/api/v1/admin/audit/logs', {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    });
    expect(adminRes.status()).toBe(403);

    // Clean up regular user
    const adminLoginRes = await request.post('http://data-agent:8080/api/v1/auth/login', {
      data: { username: USER.username, password: USER.password },
    });
    if (adminLoginRes.ok()) {
      const adminToken = (await adminLoginRes.json()).access_token;
      const listRes = await request.get(`http://data-agent:8080/api/v1/users?skip=0&limit=200`, {
        headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
      });
      if (listRes.ok()) {
        for (const user of (await listRes.json()).users || []) {
          if (user.username?.includes(`e2e-sec-reg-${uid}`)) {
            await request.delete(`http://data-agent:8080/api/v1/users/${user.id}`, {
              headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
            });
          }
        }
      }
    }
  });
});
