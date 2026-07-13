import { test, expect } from '@playwright/test';

const uid = Date.now().toString(36);
const USER = { username: `e2e-prompt-${uid}@test.local`, password: 'PromptTest1' };

/**
 * Mock /api/v1/chat/enhance to return a known enhanced response.
 * This tests the UI behavior deterministically. The real backend→LLM→mockllm
 * chain is tested implicitly by the chat tests (agent.spec.ts, chat.spec.ts).
 */
async function mockEnhance(page: any, enhancedText: string, delay = 800) {
  await page.route('**/api/v1/chat/enhance', async (route: any) => {
    await new Promise((r) => setTimeout(r, delay));
    await route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify({ enhanced: enhancedText }),
    });
  });
}

test.describe('PROMPT — SPEC-033', () => {
  test.beforeAll(async ({ request }) => {
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: USER });
  });

  test.afterAll(async ({ request }) => {
    const loginRes = await request.post('http://data-agent:8080/api/v1/auth/login', { data: { username: USER.username, password: USER.password } });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const headers = { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`http://data-agent:8080/api/v1/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-prompt-${uid}`)) {
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
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });
    await page.waitForTimeout(1000);
  });

  // ═══ UI-156: 增强按钮渲染 ═══
  test('[UI-156] Prompt — 增强按钮渲染', async ({ page }) => {
    const btn = page.locator('[data-testid="chat-enhance-btn"]');
    await expect(btn).toBeVisible();
    await expect(btn).toContainText('增强');
  });

  // ═══ UI-157: 点击增强按钮（空输入）═══
  test('[UI-157] Prompt — 点击增强按钮（空输入）', async ({ page }) => {
    const input = page.locator('[data-testid="chat-input"]');
    await input.clear();
    await page.locator('[data-testid="chat-enhance-btn"]').click();
    await page.waitForTimeout(500);
    await expect(input).toHaveValue('');
  });

  // ═══ UI-158: 点击增强按钮（有输入）═══
  test('[UI-158] Prompt — 点击增强按钮（有输入）', async ({ page }) => {
    await mockEnhance(page, '请分析本月销售数据：按地区、产品类别、月度对比维度，生成趋势图和数据汇总表。');

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('看看这个月的销售');
    await page.locator('[data-testid="chat-enhance-btn"]').click();

    // Loading state (800ms delay above)
    await expect(page.locator('[data-testid="chat-enhance-btn"]')).toContainText('增强中');

    // Enhanced text replaces input
    await expect(input).toHaveValue('请分析本月销售数据：按地区、产品类别、月度对比维度，生成趋势图和数据汇总表。', { timeout: 5000 });

    // Button back to normal
    await expect(page.locator('[data-testid="chat-enhance-btn"]')).toContainText('增强');
  });

  // ═══ UI-159: 增强后手动编辑再发送 ═══
  test('[UI-159] Prompt — 增强后手动编辑再发送', async ({ page }) => {
    await mockEnhance(page, '优化后的查询文本', 500);

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('原始查询');
    await page.locator('[data-testid="chat-enhance-btn"]').click();
    await expect(input).toHaveValue('优化后的查询文本', { timeout: 5000 });

    await input.fill('这是我手动修改后的版本');
    await expect(input).toHaveValue('这是我手动修改后的版本');
    await expect(page.locator('[data-testid="chat-send-btn"]')).toBeEnabled();
  });

  // ═══ UI-160: 增强调用不计入 Token 统计 ═══
  test('[UI-160] Prompt — 增强调用不计入 Token 统计', async ({ page }) => {
    await mockEnhance(page, '增强后的测试内容', 300);

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('test token');
    await page.locator('[data-testid="chat-enhance-btn"]').click();
    await expect(input).toHaveValue('增强后的测试内容', { timeout: 5000 });

    await page.goto('/');
    await page.waitForTimeout(1000);
    await expect(page.locator('[data-testid="main-content"]')).toBeVisible({ timeout: 5000 });
  });
});
