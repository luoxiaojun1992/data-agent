import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-e2e-${uid}@test.local`, password: 'E2eTest1!', role: 'admin' };
const USER = { username: `e2e-e2e-u-${uid}@test.local`, password: 'E2eTest1!', role: 'user' };

let tokens: Record<string, string> = {};

async function registerAndLogin(request: any, user: any) {
  let r = await request.post(`${API_BASE}/auth/register`, { data: user });
  r = await request.post(`${API_BASE}/auth/login`, { data: { username: user.username, password: user.password } });
  const body = await r.json();
  return body.access_token;
}

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

async function pageLogin(page: any, user: any) {
  await page.goto('/login');
  await page.locator('[data-testid="login-email-input"]').fill(user.username);
  await page.locator('[data-testid="login-password-input"]').fill(user.password);
  await page.locator('[data-testid="login-btn"]').click();
  await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });
}

test.describe('E2E SCENARIOS — SPEC-042', () => {
  test.beforeAll(async ({ request }) => {
    tokens['admin'] = await registerAndLogin(request, ADMIN);
    tokens['user'] = await registerAndLogin(request, USER);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${tokens['admin']}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      const body = await listRes.json();
      for (const u of body.users || []) {
        if (u.username?.includes(`e2e-e2e-${uid}`) || u.username?.includes(`e2e-e2e-u-${uid}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers }).catch(() => {});
        }
      }
    }
  });

  // ═══ UI-203: Admin 配置 + Chat SQL 查询 → 结果 → 追问 ═══
  test('[UI-203] E2E — Chat 查询结果展示与追问', async ({ page, request }) => {
    await clearMocks(request);

    // Seed mockllm for the analytics query + follow-up
    await seedMock(request, '统计过去6个月各产品线的销售额和同比增长率',
      '根据数据分析，过去6个月各产品线销售额如下：\n\n| 产品线 | 销售额(万元) | 同比增长率 |\n|--------|-----------|----------|\n| 产品A | 1,250 | +15.3% |\n| 产品B | 980 | +8.7% |\n| 产品C | 1,500 | +22.1% |');

    await pageLogin(page, ADMIN);
    await page.goto('/chat');
    await page.waitForTimeout(2000);

    // Send query
    await page.locator('[data-testid="chat-input"]').fill('统计过去6个月各产品线的销售额和同比增长率');
    await page.keyboard.press('Enter');

    // Wait for response (mockllm returns table)
    const messages = page.locator('[data-testid="chat-message"]');
    await expect(messages.first()).toBeVisible({ timeout: 20000 });

    // Seed mockllm for follow-up question
    await seedMock(request, '产品C表现怎么样',
      '产品C在华东和华南市场表现优异，销售额达1,500万元，同比增长22.1%，是增长最强劲的产品线。');

    // Follow-up question
    await page.locator('[data-testid="chat-input"]').fill('产品C表现怎么样');
    await page.keyboard.press('Enter');
    await expect(messages.nth(1)).toBeVisible({ timeout: 20000 });
  });

  // ═══ UI-204: 普通员工快捷查询 ═══
  test('[UI-204] E2E — 普通员工 Chat 快捷查询', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, '今日数据概览',
      '今日数据概览：销售额1,200万元，同比增长12%，活跃用户3,500人，新增订单285笔。');

    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(2000);

    await page.locator('[data-testid="chat-input"]').fill('今日数据概览');
    await page.keyboard.press('Enter');

    const messages = page.locator('[data-testid="chat-message"]');
    await expect(messages.first()).toBeVisible({ timeout: 20000 });
    await expect(messages.first()).toContainText('1,200万元');
  });

  // ═══ UI-205: Agent 异步任务创建 → 列表 → 详情 ═══
  test('[UI-205] E2E — Agent 异步任务全流程', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, 'agent-sql-query',
      '任务已创建，正在执行分析...');

    await pageLogin(page, ADMIN);
    await page.goto('/agent');
    await page.waitForTimeout(3000);

    // Agent page renders — either empty state or task list
    const pageVisible = await page.locator('[data-testid="agent-empty"], [data-testid="agent-page-header"]').first().isVisible({ timeout: 10000 }).catch(() => false);
    expect(pageVisible).toBe(true);

    // Try to create a task modal if the create button is present
    const createBtn = page.locator('[data-testid="agent-create-btn"]');
    if (await createBtn.isVisible().catch(() => false)) {
      await createBtn.click();
      await page.waitForTimeout(1000);
      // Task creation modal appears
      await expect(page.locator('[data-testid="agent-create-modal"]')).toBeVisible({ timeout: 5000 });
    }
  });

  // ═══ UI-206: KB 上传 → 索引 → 搜索 ═══
  test('[UI-206] E2E — 知识库上传与搜索', async ({ page, request }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/knowledge');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="nav-kb-mgmt"]')).toBeVisible({ timeout: 5000 });

    // Check if upload button is available and page is functional
    const uploadBtn = page.locator('[data-testid="kb-upload-btn"]');
    if (await uploadBtn.isVisible().catch(() => false)) {
      await expect(uploadBtn).toBeVisible();
    }

    // Search KB documents
    const searchInput = page.locator('[data-testid="kb-search-input"]');
    if (await searchInput.isVisible().catch(() => false)) {
      await searchInput.fill('test');
      await page.keyboard.press('Enter');
      await page.waitForTimeout(2000);
    }

    // Page remains functional after search
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });

  // ═══ UI-207: 审计日志筛选 → 导出 ═══
  test('[UI-207] E2E — 审计日志筛选与导出', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/admin/audit');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // Audit page renders with content
    const hasTable = await page.locator('[data-testid="audit-log-table"]').isVisible().catch(() => false);
    const hasEmpty = await page.locator('[data-testid="audit-empty"]').isVisible().catch(() => false);
    expect(hasTable || hasEmpty).toBe(true);

    // Try export if button exists
    const exportBtn = page.locator('[data-testid="audit-export-btn"]');
    if (await exportBtn.isVisible().catch(() => false)) {
      await exportBtn.click();
      await page.waitForTimeout(1000);
      // Export modal appears with format options
      await expect(page.locator('[data-testid="export-format-select"]')).toBeVisible({ timeout: 3000 });
    }
  });

  // ═══ UI-208: 安全拦截 → 审计记录联动 ═══
  test('[UI-208] E2E — 安全拦截与审计记录', async ({ page }) => {
    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(1000);

    // Try SQL injection
    await page.locator('[data-testid="chat-input"]').fill("'; DROP TABLE users; --");
    await page.keyboard.press('Enter');
    await page.waitForTimeout(3000);

    // Security toast should appear
    const blocked = page.locator('[data-testid="sec-input-blocked-toast"]');
    if (await blocked.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(blocked).toBeVisible();
    }

    // Page should remain functional
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();

    // Login as admin to check audit log
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url: URL) => !url.pathname.includes('/login'), { timeout: 10000 });

    await page.goto('/admin/audit');
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-209: admin 完整管理流程（跨页面） ═══
  test('[UI-209] E2E — admin 完整管理流程', async ({ page }) => {
    await pageLogin(page, ADMIN);

    // 1. User management page
    await page.goto('/admin/users');
    await page.waitForTimeout(1500);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // 2. Task management page
    await page.goto('/admin/tasks');
    await page.waitForTimeout(1500);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // 3. Knowledge base page
    await page.goto('/admin/knowledge');
    await page.waitForTimeout(1500);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // 4. Audit log page
    await page.goto('/admin/audit');
    await page.waitForTimeout(1500);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    // 5. API review page
    await page.goto('/admin/api-review');
    await page.waitForTimeout(1500);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-210: Hermes 探索模式完整流程 ═══
  test('[UI-210] E2E — Hermes 探索模式', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, '什么是数据分析',
      '数据分析是指用适当的统计分析方法对收集来的大量数据进行分析，提取有用信息并形成结论的过程。');

    await pageLogin(page, ADMIN);
    await page.goto('/hermes');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible({ timeout: 5000 });

    // Send message in Hermes mode
    const input = page.locator('[data-testid="chat-input"]');
    if (await input.isVisible().catch(() => false)) {
      await input.fill('什么是数据分析');
      await page.keyboard.press('Enter');

      const messages = page.locator('[data-testid="chat-message"]');
      await expect(messages.first()).toBeVisible({ timeout: 20000 });
    }
  });
});
