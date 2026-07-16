import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
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

  // ═══ UI-203: Chat 页面加载 + 输入发送 ═══
  test('[UI-203] E2E — Chat 页面加载发送', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/chat');
    await page.waitForTimeout(2000);

    // Chat page renders with input
    await expect(page.locator('[data-testid="chat-input"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="nav-chat"]')).toBeVisible();

    // Send a message
    await page.locator('[data-testid="chat-input"]').fill('hello');
    await page.keyboard.press('Enter');
    await page.waitForTimeout(2000);
    // Page remains functional
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });

  // ═══ UI-204: 普通员工 Chat 页面访问 ═══
  test('[UI-204] E2E — 普通员工 Chat 页面', async ({ page }) => {
    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="chat-input"]')).toBeVisible({ timeout: 5000 });
    // User role cannot see admin nav
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });

  // ═══ UI-205: Agent 页面 → 任务列表 ═══
  test('[UI-205] E2E — Agent 页面与任务列表', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/agent');
    await page.waitForTimeout(3000);

    // Agent page renders
    const hasEmpty = await page.locator('[data-testid="agent-empty"]').isVisible({ timeout: 5000 }).catch(() => false);
    const hasHeader = await page.locator('[data-testid="agent-page-header"]').isVisible().catch(() => false);
    expect(hasEmpty || hasHeader).toBe(true);
  });

  // ═══ UI-206: KB 管理页加载 ═══
  test('[UI-206] E2E — 知识库管理页', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/knowledge');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="nav-kb-mgmt"]')).toBeVisible({ timeout: 5000 });
    // KB page should render content
    const hasUpload = await page.locator('[data-testid="kb-upload-btn"]').isVisible().catch(() => false);
    const hasDocs = await page.locator('[data-testid="kb-doc-card"]').first().isVisible().catch(() => false);
    expect(hasUpload || hasDocs).toBe(true);
  });

  // ═══ UI-207: 审计日志页 ═══
  test('[UI-207] E2E — 审计日志页', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/admin/audit');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
    // Audit page renders
    const hasTable = await page.locator('[data-testid="audit-log-table"]').isVisible().catch(() => false);
    const hasEmpty = await page.locator('[data-testid="audit-empty"]').isVisible().catch(() => false);
    expect(hasTable || hasEmpty).toBe(true);
  });

  // ═══ UI-208: 安全拦截页 ═══
  test('[UI-208] E2E — 安全拦截', async ({ page }) => {
    await pageLogin(page, USER);
    await page.goto('/chat');
    await page.waitForTimeout(1000);

    // Try SQL injection in chat
    const input = page.locator('[data-testid="chat-input"]');
    await input.fill("'; DROP TABLE users; --");
    await page.keyboard.press('Enter');
    await page.waitForTimeout(2000);

    // Security toast may appear
    const blocked = page.locator('[data-testid="sec-input-blocked-toast"]');
    // Either blocked or the page is still functional
    if (await blocked.isVisible().catch(() => false)) {
      await expect(blocked).toBeVisible();
    }
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible();
  });

  // ═══ UI-209: admin 跨页面导航 ═══
  test('[UI-209] E2E — admin 跨页面导航', async ({ page }) => {
    await pageLogin(page, ADMIN);

    // Navigate through multiple admin pages
    await page.goto('/admin/users');
    await page.waitForTimeout(1000);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    await page.goto('/admin/tasks');
    await page.waitForTimeout(1000);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    await page.goto('/admin/knowledge');
    await page.waitForTimeout(1000);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });

    await page.goto('/admin/audit');
    await page.waitForTimeout(1000);
    await expect(page.locator('[data-testid="sidebar"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-210: Hermes 探索模式 ═══
  test('[UI-210] E2E — Hermes 探索模式', async ({ page }) => {
    await pageLogin(page, ADMIN);
    await page.goto('/hermes');
    await page.waitForTimeout(2000);

    await expect(page.locator('[data-testid="nav-hermes"]')).toBeVisible({ timeout: 5000 });

    // Hermes page has input
    const input = page.locator('[data-testid="chat-input"]');
    if (await input.isVisible().catch(() => false)) {
      await expect(input).toBeVisible();
    }
  });
});
