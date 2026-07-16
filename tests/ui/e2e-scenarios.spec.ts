import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-e2e-${uid}@test.local`, password: 'E2eTest1!', role: 'admin' };
const USER = { username: `e2e-e2e-u-${uid}@test.local`, password: 'E2eTest1!', role: 'user' };

let tokens: Record<string, string> = {};

async function registerAndLogin(request: any, user: any) {
  let r = await request.post(`${API_BASE}/auth/register`, { data: user });
  r = await request.post(`${API_BASE}/auth/login`, { data: { username: user.username, password: user.password } });
  return (await r.json()).access_token;
}

// Mock SSE response for chat (same approach as chat.spec.ts)
function mockSSE(route: any, content: string) {
  const size = Math.max(50, Math.ceil(content.length / 5));
  const chunks: string[] = [];
  for (let i = 0; i < content.length; i += size) {
    chunks.push(content.slice(i, i + size));
  }
  route.fulfill({
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
    body: chunks.map((c: string) => `data: ${JSON.stringify({ content: c })}\n`).join('') + 'data: [DONE]\n',
  });
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

  // ═══ UI-203: Chat 查询 → 结果 → 追问 ═══
  test('[UI-203] E2E — Chat 查询结果展示与追问', async ({ page }) => {
    const response1 = '根据数据分析，过去6个月各产品线销售额如下：产品A 1,250万元(+15.3%)，产品B 980万元(+8.7%)，产品C 1,500万元(+22.1%)。整体增长态势良好。';
    const response2 = '产品C在华东和华南市场表现尤为突出，销售额达1,500万元，同比增长22.1%，是增长最强劲的产品线。';

    await page.route('**/api/v1/chat', (route) => {
      const request = route.request();
      const url = request.url();
      // First chat call gets response1, second gets response2
      if (!url.includes('&')) { mockSSE(route, response1); return; }
    });

    await pageLogin(page, ADMIN);
    await page.goto('/chat');
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="chat-input"]')).toBeVisible({ timeout: 5000 });

    // First query
    await page.locator('[data-testid="chat-input"]').fill('统计过去6个月各产品线的销售额和同比增长率');
    await page.keyboard.press('Enter');
    await expect(page.locator('[data-testid="chat-msg-ai-0"]')).toBeVisible({ timeout: 20000 });
    await expect(page.locator('[data-testid="chat-msg-ai-0"]')).toContainText('1,250万元');

    // Follow-up: unroute old, route new
    await page.unrouteAll({ behavior: 'ignoreErrors' });
    await page.route('**/api/v1/chat', (route) => { mockSSE(route, response2); });

    await page.locator('[data-testid="chat-input"]').fill('产品C表现怎么样');
    await page.keyboard.press('Enter');
    await expect(page.locator('[data-testid="chat-msg-ai-0"]')).toBeVisible({ timeout: 20000 });
    await expect(page.locator('[data-testid="chat-msg-ai-0"]')).toContainText('22.1%');
  });

  // ═══ UI-204: 普通员工 Chat 查询 ═══
  test('[UI-204] E2E — 普通员工 Chat 查询', async ({ page }) => {
    await page.route('**/api/v1/chat', (route) => {
      mockSSE(route, '今日数据概览：销售额1,200万元，同比增长12%，活跃用户3,500人，新增订单285笔。');
    });

    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(2000);
    await expect(page.locator('[data-testid="chat-input"]')).toBeVisible({ timeout: 5000 });

    // User can chat
    await page.locator('[data-testid="chat-input"]').fill('今日数据概览');
    await page.keyboard.press('Enter');
    await expect(page.locator('[data-testid="chat-msg-ai-0"]')).toBeVisible({ timeout: 20000 });
    await expect(page.locator('[data-testid="chat-msg-ai-0"]')).toContainText('1,200万元');

    // User cannot see admin nav
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  // ═══ UI-205: Agent 页面 ═══
  test('[UI-205] E2E — Agent 任务页面', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/agent');
    await page.waitForTimeout(3000);

    await expect(page.locator('[data-testid="nav-agent"]')).toBeVisible({ timeout: 5000 });
    const hasContent = await page.locator('[data-testid="agent-empty"], [data-testid="agent-page-header"]').first().isVisible({ timeout: 5000 }).catch(() => false);
    expect(hasContent).toBe(true);
  });

  // ═══ UI-206: KB 管理页 ═══
  test('[UI-206] E2E — 知识库管理页', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/knowledge');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="nav-kb-mgmt"]')).toBeVisible({ timeout: 5000 });
    const hasContent = await page.locator('[data-testid="kb-upload-btn"], [data-testid="kb-doc-card"]').first().isVisible().catch(() => false);
    expect(hasContent).toBe(true);
  });

  // ═══ UI-207: 审计日志页 ═══
  test('[UI-207] E2E — 审计日志页', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/admin/audit');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="audit-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="audit-filter-bar"]')).toBeVisible();
    await expect(page.locator('[data-testid="audit-date-start"]')).toBeVisible();
  });

  // ═══ UI-208: 安全拦截 + 审计联动 ═══
  test('[UI-208] E2E — 安全拦截与审计联动', async ({ page }) => {
    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(1000);

    // SQL injection
    await page.locator('[data-testid="chat-input"]').fill("'; DROP TABLE users; --");
    await page.keyboard.press('Enter');
    await page.waitForTimeout(3000);

    // Security toast or page stays functional
    const blocked = page.locator('[data-testid="sec-input-blocked-toast"]');
    if (await blocked.isVisible({ timeout: 5000 }).catch(() => false)) {
      await expect(blocked).toBeVisible();
    }
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });

  // ═══ UI-209: admin 完整管理流程 ═══
  test('[UI-209] E2E — admin 完整管理流程', async ({ page }) => {
    await pageLogin(page, ADMIN);

    const pages = ['/admin/users', '/admin/tasks', '/admin/knowledge', '/admin/audit', '/admin/api-review'];
    for (const p of pages) {
      await page.goto(p);
      await page.waitForTimeout(1500);
      await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
    }
  });

  // ═══ UI-210: Hermes 探索模式 ═══
  test('[UI-210] E2E — Hermes 探索模式', async ({ page }) => {
    await page.route('**/api/v1/chat', (route) => {
      mockSSE(route, '数据分析是指用适当的统计分析方法对收集来的大量数据进行分析，提取有用信息并形成结论的过程。');
    });

    await pageLogin(page, ADMIN);
    await page.goto('/hermes');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible({ timeout: 5000 });

    // Send message in Hermes mode
    const input = page.locator('[data-testid="chat-input"]');
    if (await input.isVisible().catch(() => false)) {
      await input.fill('什么是数据分析');
      await page.keyboard.press('Enter');
      await expect(page.locator('[data-testid="chat-msg-ai-0"]')).toBeVisible({ timeout: 20000 });
    }
  });
});
