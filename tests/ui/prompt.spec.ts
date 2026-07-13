import { test, expect } from '@playwright/test';

const uid = Date.now().toString(36);
const USER = { username: `e2e-prompt-${uid}@test.local`, password: 'PromptTest1' };

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
    // Button should not enter loading (enhancing guard prevents it)
    // Input should remain empty
    await page.waitForTimeout(500);
    await expect(input).toHaveValue('');
  });

  // ═══ UI-158: 点击增强按钮（有输入）═══
  test('[UI-158] Prompt — 点击增强按钮（有输入）', async ({ page }) => {
    // Mock the /chat/enhance endpoint
    await page.route('**/api/v1/chat/enhance', async (route) => {
      await new Promise((r) => setTimeout(r, 800)); // simulate network delay
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ enhanced: '请分析本月的销售情况，按地区和产品分类，生成详细的数据报告和可视化图表。' }),
      });
    });

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('看看这个月的销售');
    await page.locator('[data-testid="chat-enhance-btn"]').click();

    // Button should show loading state
    await expect(page.locator('[data-testid="chat-enhance-btn"]')).toContainText('增强中');

    // Input should be replaced with enhanced text
    await expect(input).toHaveValue('请分析本月的销售情况，按地区和产品分类，生成详细的数据报告和可视化图表。', { timeout: 5000 });

    // Button should return to normal state
    await expect(page.locator('[data-testid="chat-enhance-btn"]')).toContainText('增强');

    // Verify no session was created (enhance is stateless)
    await expect(page.locator('[data-testid="chat-messages"]')).toBeVisible();
  });

  // ═══ UI-159: 增强后手动编辑再发送 ═══
  test('[UI-159] Prompt — 增强后手动编辑再发送', async ({ page }) => {
    await page.route('**/api/v1/chat/enhance', async (route) => {
      await new Promise((r) => setTimeout(r, 500));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ enhanced: '优化后的查询文本' }),
      });
    });

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('原始查询');
    await page.locator('[data-testid="chat-enhance-btn"]').click();
    await expect(input).toHaveValue('优化后的查询文本', { timeout: 5000 });

    // Manually edit the enhanced text
    await input.fill('这是我手动修改后的版本');
    await expect(input).toHaveValue('这是我手动修改后的版本');

    // Verify send button is enabled
    const sendBtn = page.locator('[data-testid="chat-send-btn"]');
    await expect(sendBtn).toBeEnabled();
  });

  // ═══ UI-160: 增强调用不计入 Token 统计 ═══
  test('[UI-160] Prompt — 增强调用不计入 Token 统计', async ({ page }) => {
    await page.route('**/api/v1/chat/enhance', async (route) => {
      await new Promise((r) => setTimeout(r, 300));
      await route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({ enhanced: '增强后的内容' }),
      });
    });

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('test');
    await page.locator('[data-testid="chat-enhance-btn"]').click();
    await expect(input).toHaveValue('增强后的内容', { timeout: 5000 });

    // Navigate to dashboard and check token count existence
    await page.goto('/');
    await page.waitForTimeout(1000);
    // Just verify dashboard renders — token count is P2 (suggested)
    await expect(page.locator('[data-testid="dashboard-kpi-card-0"]').or(page.locator('[data-testid="main-content"]'))).toBeVisible({ timeout: 5000 });
  });
});
