import { test, expect } from '@playwright/test';

const uid = crypto.randomUUID().slice(0, 8);
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const API_BASE = 'http://data-agent:8080/api/v1';
const USER = { username: `e2e-prompt-${uid}@test.local`, password: 'PromptTest1' };
const ADMIN = { username: `e2e-prompt-${uid}-admin@test.local`, password: 'PromptAdmin1!', role: 'admin' };
let adminToken = '';

async function seedMock(request: any, key: string, response: string) {
  await request.post(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
    data: { key, response },
  });
}

async function clearMocks(request: any) {
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});
}

// getLLMStats calls the real /api/v1/stats/llm endpoint (admin-only) to verify
// that enhance calls are recorded into llm_usage. SPEC-059 UI-160.
async function getLLMStats(request: any, token: string, callPoint: string) {
  const res = await request.get(`${API_BASE}/stats/llm`, {
    params: callPoint ? { call_point: callPoint } : {},
    headers: { Authorization: `Bearer ${token}` },
  });
  if (!res.ok()) return { stats: [] };
  return res.json();
}

test.describe.serial('PROMPT — SPEC-033', () => {
  test.beforeAll(async ({ request }) => {
    // Clear any leftover mock responses from previous runs
    await clearMocks(request);
    await request.post('http://data-agent:8080/api/v1/auth/register', { data: USER });

    // Register an admin so UI-160 can query /stats/llm (admin-only).
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    adminToken = (await res.json()).access_token;
  });

  test.afterAll(async ({ request }) => {
    await clearMocks(request);
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

  // ═══ UI-158: 点击增强按钮（有输入 → mockllm exact match）═══
  test('[UI-158] Prompt — 点击增强按钮（有输入）', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, '看看这个月的销售',
      '请分析本月销售数据：按地区、产品类别、月度对比维度，生成趋势图和数据汇总表。');

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('看看这个月的销售');
    await page.locator('[data-testid="chat-enhance-btn"]').click();

    // Wait for enhanced text to replace input (mockllm response)
    await expect(input).toHaveValue('请分析本月销售数据：按地区、产品类别、月度对比维度，生成趋势图和数据汇总表。', { timeout: 10000 });

    // Button back to normal (skip transient "增强中" — too fast to catch reliably)
    await expect(page.locator('[data-testid="chat-enhance-btn"]')).toContainText('增强');
  });

  // ═══ UI-159: 增强后手动编辑再发送 ═══
  test('[UI-159] Prompt — 增强后手动编辑再发送', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, '原始查询', '优化后的查询文本');

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('原始查询');
    await page.locator('[data-testid="chat-enhance-btn"]').click();
    await expect(input).toHaveValue('优化后的查询文本', { timeout: 5000 });

    await input.fill('这是我手动修改后的版本');
    await expect(input).toHaveValue('这是我手动修改后的版本');
    await expect(page.locator('[data-testid="chat-send-btn"]')).toBeEnabled();
  });

  // ═══ UI-160: 增强调用计入 Token 统计 ═══
  test('[UI-160] Prompt — 增强调用计入 Token 统计', async ({ page, request }) => {
    await clearMocks(request);
    // Unique key per run guarantees a cache miss so the LLM is actually invoked
    // and recordEnhanceTokens writes a new llm_usage row.
    await seedMock(request, 'test token ' + uid, '增强后的测试内容');

    const before = await getLLMStats(request, adminToken, 'enhance');
    const beforeCount = before.stats?.[0]?.count ?? 0;

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('test token ' + uid);
    await page.locator('[data-testid="chat-enhance-btn"]').click();
    await expect(input).toHaveValue('增强后的测试内容', { timeout: 5000 });

    // recordEnhanceTokens writes llm_usage asynchronously; poll /stats/llm
    // until the enhance call_point count increments.
    await expect.poll(async () => {
      const after = await getLLMStats(request, adminToken, 'enhance');
      return after.stats?.[0]?.count ?? 0;
    }, { timeout: 5000 }).toBe(beforeCount + 1);
  });
});
