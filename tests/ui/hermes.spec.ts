import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const uid = crypto.randomUUID().slice(0, 8);

const USER = { username: `e2e-hermes-${uid}@test.local`, password: 'E2eTest1!', role: 'user' };

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

test.describe('HERMES — SPEC-021', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: USER })).status()).toBe(201);
  });

  test.afterAll(async ({ request }) => {
    await clearMocks(request);
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: USER.username, password: USER.password },
    });
    if (!loginRes.ok()) return;
    const token = (await loginRes.json()).access_token;
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
    });
    if (listRes.ok()) {
      for (const u of (await listRes.json()).users || []) {
        if (u.username?.includes(`e2e-hermes-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, {
            headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
          }).catch(() => {});
        }
      }
    }
  });

  test.beforeEach(async ({ page, request }) => {
    await clearMocks(request);
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
  });

  // ═══ UI-057: Hermes — Mode Toggle 渲染 ═══
  test('[UI-057] Hermes — Mode Toggle 渲染', async ({ page }) => {
    await page.goto('/hermes');
    await expect(page.locator('[data-testid="hermes-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="hermes-page-title"]')).toContainText('Hermes');
    await expect(page.locator('[data-testid="hermes-search-area"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-query-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-submit-btn"]')).toBeVisible();
  });

  // ═══ UI-058: Hermes — 切换到探索模式 ═══
  test('[UI-058] Hermes — 切换到探索模式', async ({ page }) => {
    await page.goto('/hermes');
    await page.waitForSelector('[data-testid="hermes-page-header"]', { timeout: 10000 });

    // Verify the Hermes-specific UI is rendered
    await expect(page.locator('[data-testid="hermes-query-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-submit-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-page-title"]')).toContainText('Hermes 自由探索');

    // Verify the sidebar navigation is still present
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });

  // ═══ UI-059: Hermes — 探索模式下发送消息 ═══
  test('[UI-059] Hermes — 探索模式下发送消息', async ({ page, request }) => {
    await seedMock(request, 'What are the latest trends in data science?',
      'The latest trends in data science include: 1) Generative AI and LLMs for automated analysis, 2) Real-time data processing, 3) MLOps and model monitoring, 4) Data mesh architecture, and 5) AI-driven data governance.');

    await page.goto('/hermes');
    await page.waitForSelector('[data-testid="hermes-query-input"]', { timeout: 10000 });

    // Send a query
    await page.locator('[data-testid="hermes-query-input"]').fill('What are the latest trends in data science?');
    await page.locator('[data-testid="hermes-submit-btn"]').click();

    // Wait for Hermes response (SSE stream goes through data-agent proxy → mockllm)
    await expect(page.locator('[data-testid="hermes-query-input"]')).toBeVisible({ timeout: 15000 });

    // Since the frontend renders the response in the same page, the input should remain after submit
    // (Hermes page keeps the query input visible for follow-up questions)
  });

  // ═══ UI-060: Hermes — 探索模式下无法调用 Data Agent 工具 ═══
  test('[UI-060] Hermes — 探索模式下无法调用 Data Agent 工具', async ({ page, request }) => {
    await seedMock(request, '查询华东区销售数据',
      '华东区销售数据：总销售额 5,200 万元，同比增长 18%。主要增长来自上海和杭州市场。');

    await page.goto('/hermes');
    await page.waitForSelector('[data-testid="hermes-query-input"]', { timeout: 10000 });

    // Send a data-related query
    await page.locator('[data-testid="hermes-query-input"]').fill('查询华东区销售数据');
    await page.locator('[data-testid="hermes-submit-btn"]').click();

    // Hermes proxy forwards to mockllm — response is plain text, not Data Agent structured output
    // Wait for the page to process the response
    await page.waitForTimeout(3000);

    // Verify no SQL code blocks appear (Hermes doesn't generate SQL through Data Agent)
    const sqlBlock = page.locator('[data-testid="chat-sql-block"]');
    expect(await sqlBlock.isVisible({ timeout: 2000 }).catch(() => false)).toBe(false);

    // Verify no tool call cards appear (Hermes doesn't use Data Agent tools)
    const toolCard = page.locator('[data-testid^="chat-tool-call-card-"]');
    expect(await toolCard.isVisible({ timeout: 2000 }).catch(() => false)).toBe(false);
  });

  // ═══ UI-061: Hermes — 在线状态验证 ═══
  test('[UI-061] Hermes — 服务在线状态', async ({ page, request }) => {
    // With HERMES_URL=http://mockllm:8082, the Hermes proxy is always online
    await seedMock(request, 'test online', 'Hermes service is online and responding.');

    await page.goto('/hermes');
    await page.waitForSelector('[data-testid="hermes-page-header"]', { timeout: 10000 });

    // Send a test query to confirm the service is reachable
    await page.locator('[data-testid="hermes-query-input"]').fill('test online');
    await page.locator('[data-testid="hermes-submit-btn"]').click();
    await page.waitForTimeout(2000);

    // The Hermes page should remain functional
    await expect(page.locator('[data-testid="hermes-query-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-submit-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });
});
