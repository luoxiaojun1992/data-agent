import { test, expect } from '@playwright/test';

/**
 * SPEC-103: 纯前端国际化（i18n）E2E 用例。
 *
 * 覆盖：默认中文、语言切换即时生效、cookie 持久化（刷新后保持）。
 * 仅断言前端静态 UI 文案（导航/页面标题/切换按钮），不涉及后端返回内容。
 */

const log = (msg: string) => process.stderr.write(`[i18n.spec] ${new Date().toISOString()} ${msg}\n`);

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const TEST_USER = {
  username: `e2e-i18n-${uid}@test.local`,
  password: 'E2eTest123!', role: 'admin',
};

test.describe('I18N — 语言切换与持久化', () => {
  test.beforeAll(async ({ request }) => {
    log(`Registering test user: ${TEST_USER.username}`);
    const res = await request.post(`${API_BASE}/auth/register`, {
      data: TEST_USER,
    });
    expect(res.status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(TEST_USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(TEST_USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.waitForSelector('[data-testid="sidebar"]', { state: 'visible', timeout: 10000 });
  });

  // UI-i18n-1: 默认语言为中文
  test('[UI-i18n-1] 默认语言为中文', async ({ page }) => {
    // 语言切换按钮默认显示「目标语言」EN（表示当前为中文）
    await expect(page.locator('[data-testid="language-toggle"]')).toHaveText('EN');
    // 侧边栏导航与页面标题为中文
    await expect(page.locator('[data-testid="nav-dashboard"]')).toContainText('仪表盘');
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('仪表盘');
  });

  // UI-i18n-2: 切换到英文后全站静态文案即时切换
  test('[UI-i18n-2] 切换到英文后文案即时切换', async ({ page }) => {
    await page.locator('[data-testid="language-toggle"]').click();
    // router.refresh() 后服务端按 en 重渲染，等待导航文案变化
    await expect(page.locator('[data-testid="nav-dashboard"]')).toContainText('Dashboard', { timeout: 10000 });
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('Dashboard', { timeout: 10000 });
    // 切换按钮变为「中文」（目标语言），表示当前为英文
    await expect(page.locator('[data-testid="language-toggle"]')).toHaveText('中文');

    // 再切回中文
    await page.locator('[data-testid="language-toggle"]').click();
    await expect(page.locator('[data-testid="nav-dashboard"]')).toContainText('仪表盘', { timeout: 10000 });
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('仪表盘', { timeout: 10000 });
  });

  // UI-i18n-3: 语言选择经 cookie 持久化，刷新后保持
  test('[UI-i18n-3] 语言选择刷新后保持', async ({ page }) => {
    await page.locator('[data-testid="language-toggle"]').click();
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('Dashboard', { timeout: 10000 });

    // 刷新页面，cookie（NEXT_LOCALE=en）应让 SSR 仍按英文渲染首屏
    await page.reload();
    await page.waitForSelector('[data-testid="sidebar"]', { state: 'visible', timeout: 10000 });
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('Dashboard', { timeout: 10000 });
    await expect(page.locator('[data-testid="language-toggle"]')).toHaveText('中文');
  });
});
