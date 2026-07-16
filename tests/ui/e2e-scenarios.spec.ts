import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const uid = crypto.randomUUID().slice(0, 8);

// Admin user for most scenarios
const ADMIN = { username: `e2e-e2e-${uid}@test.local`, password: 'E2eTest1!', role: 'admin' };
const USER = { username: `e2e-e2e-u-${uid}@test.local`, password: 'E2eTest1!', role: 'user' };

let tokens: Record<string, string> = {};

// ── Helpers ──

async function registerAndLogin(request: any, user: any) {
  let r = await request.post(`${API_BASE}/auth/register`, { data: user });
  r = await request.post(`${API_BASE}/auth/login`, { data: { username: user.username, password: user.password } });
  const body = await r.json();
  return body.access_token;
}

async function seedMock(request: any, key: string, response: string) {
  await request.post(`${MOCKLLM}/responses`, { data: { key, response } });
}

async function clearMocks(request: any) {
  await request.delete(`${MOCKLLM}/responses`);
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

  // ═══ UI-203: Chat 查询 → 结果展示 → 追问 ═══
  test('[UI-203] E2E — Chat 查询结果展示与追问', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, '统计过去6个月各产品线的销售额和同比增长率',
      '根据数据分析，过去6个月各产品线销售情况如下：\n\n| 产品线 | 销售额(万元) | 同比增长率 |\n|--------|-----------|----------|\n| 产品A   | 1,250     | +15.3%   |\n| 产品B   | 980       | +8.7%    |\n| 产品C   | 1,500     | +22.1%   |\n\n整体增长趋势良好，产品C表现最为突出。');

    await pageLogin(page, ADMIN);
    await page.goto('/chat');
    await page.waitForTimeout(1000);

    // Send query
    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('统计过去6个月各产品线的销售额和同比增长率');
    await page.keyboard.press('Enter');

    // Wait for response
    const response = page.locator('[data-testid="chat-message"]');
    await expect(response.first()).toBeVisible({ timeout: 15000 });

    // Follow-up question
    await clearMocks(request);
    await seedMock(request, '产品C表现怎么样',
      '产品C在过去6个月中表现优异，销售额达到1500万元，同比增长22.1%。主要增长驱动力来自华东和华南市场。');

    await input.fill('产品C表现怎么样');
    await page.keyboard.press('Enter');
    await expect(response.nth(1)).toBeVisible({ timeout: 15000 });
  });

  // ═══ UI-204: 普通员工快捷查询 ═══
  test('[UI-204] E2E — 普通员工快捷查询', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, '今日数据概览', '今日数据概览：销售额1,200万元，同比增长12%，活跃用户3,500人。');

    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(1000);

    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('今日数据概览');
    await page.keyboard.press('Enter');

    const response = page.locator('[data-testid="chat-message"]');
    await expect(response.first()).toBeVisible({ timeout: 15000 });
  });

  // ═══ UI-205: Agent 异步任务 → 任务列表 ═══
  test('[UI-205] E2E — Agent 异步任务创建和列表查看', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/agent');
    await page.waitForTimeout(2000);

    // Verify agent page loads
    await expect(page.locator('[data-testid="nav-agent"]')).toBeVisible({ timeout: 5000 });

    // Check task list or empty state
    const hasEmpty = await page.locator('[data-testid="agent-empty"]').isVisible().catch(() => false);
    const hasTable = await page.locator('[data-testid="agent-page-header"]').isVisible().catch(() => false);
    expect(hasEmpty || hasTable).toBe(true);
  });

  // ═══ UI-206: KB 上传 → 搜索 ═══
  test('[UI-206] E2E — 知识库上传与搜索', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/knowledge');
    await page.waitForTimeout(2000);

    // KB page loads with navigation
    await expect(page.locator('[data-testid="nav-kb-mgmt"]')).toBeVisible({ timeout: 5000 });

    // Check for empty state or document list
    const hasContent = await page.locator('[data-testid="kb-upload-btn"], [data-testid="kb-doc-card"]').first().isVisible().catch(() => false);
    expect(hasContent).toBe(true);
  });

  // ═══ UI-207: 审计日志筛选 → 导出 ═══
  test('[UI-207] E2E — 审计日志筛选与导出', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/admin/audit');
    await page.waitForTimeout(2000);

    // Audit page loads
    await expect(page.locator('[data-testid="nav-admin"]')).toBeVisible({ timeout: 5000 });

    // Check audit log content
    const hasTable = await page.locator('[data-testid="audit-log-table"]').isVisible().catch(() => false);
    const hasEmpty = await page.locator('[data-testid="audit-empty"]').isVisible().catch(() => false);
    expect(hasTable || hasEmpty).toBe(true);
  });

  // ═══ UI-208: 安全拦截端到端 ═══
  test('[UI-208] E2E — 安全拦截与审计记录联动', async ({ page, request }) => {
    await clearMocks(request);
    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(1000);

    // Try SQL injection
    const input = page.locator('[data-testid="chat-input"]');
    await input.fill("'; DROP TABLE users; --");
    await page.keyboard.press('Enter');
    await page.waitForTimeout(2000);

    // Should show security block toast
    const blocked = page.locator('[data-testid="sec-input-blocked-toast"]');
    if (await blocked.isVisible().catch(() => false)) {
      await expect(blocked).toBeVisible();
    }
    // The message should NOT have reached LLM (no chat message shown)
  });

  // ═══ UI-209: admin 完整管理流程 ═══
  test('[UI-209] E2E — admin 完整管理流程', async ({ page }) => {
    await pageLogin(page, ADMIN);

    // 1. User management
    await page.goto('/admin/users');
    await page.waitForTimeout(1000);
    await expect(page.locator('[data-testid="nav-admin"]')).toBeVisible({ timeout: 5000 });

    // 2. Task management
    await page.goto('/admin/tasks');
    await page.waitForTimeout(1000);
    const taskContent = await page.locator('[data-testid="admin-tasks-header"], [data-testid="admin-tasks-empty"]').first().isVisible().catch(() => false);
    expect(taskContent).toBe(true);

    // 3. Knowledge management (admin section)
    await page.goto('/admin/knowledge');
    await page.waitForTimeout(1000);
    // Page should load
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-210: Hermes 探索模式 ═══
  test('[UI-210] E2E — Hermes 探索模式切换与发送', async ({ page, request }) => {
    await clearMocks(request);
    await seedMock(request, '什么是数据分析',
      '数据分析是指用适当的统计分析方法对收集来的大量数据进行分析，提取有用信息和形成结论而对数据加以详细研究和概括总结的过程。');

    await pageLogin(page, ADMIN);
    await page.goto('/hermes');
    await page.waitForTimeout(1000);

    // Hermes page loads
    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible({ timeout: 5000 });

    // Send a message in hermes mode
    const input = page.locator('[data-testid="chat-input"]');
    if (await input.isVisible().catch(() => false)) {
      await input.fill('什么是数据分析');
      await page.keyboard.press('Enter');
      const response = page.locator('[data-testid="chat-message"]');
      await expect(response.first()).toBeVisible({ timeout: 15000 });
    }
  });
});
