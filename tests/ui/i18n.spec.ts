import { test, expect } from '@playwright/test';

/**
 * SPEC-103: 纯前端国际化（i18n）E2E 用例。
 *
 * 覆盖：默认中文、语言切换即时生效、cookie 持久化（刷新后保持）。
 * 仅断言前端静态 UI 文案（导航/页面标题/切换按钮），不涉及后端返回内容。
 */

const log = (msg: string) => process.stderr.write(`[i18n.spec] ${new Date().toISOString()} ${msg}\n`);

// CI 在 docker 网络内运行（data-agent 为 compose 服务名）；本地可通过 API_BASE 覆盖走 ssh 隧道。
const API_BASE = process.env.API_BASE || 'http://data-agent:8080/api/v1';
// SPEC-084 起自注册已禁用（改为邀请制），测试用户需预置：admin 邀请 或 /admin/users API。
// 默认使用一次性测试账号，可通过 E2E_USERNAME / E2E_PASSWORD 覆盖。
const TEST_USER = {
  username: process.env.E2E_USERNAME || 'e2e-i18n@test.local',
  password: process.env.E2E_PASSWORD || 'E2eTest123!',
};

test.describe('I18N — 语言切换与持久化', () => {
  test.beforeAll(async ({ request }) => {
    // 校验预置用户可登录（SPEC-084 后自注册已禁用，测试依赖预置账号）。
    const res = await request.post(`${API_BASE}/auth/login`, {
      data: { username: TEST_USER.username, password: TEST_USER.password },
    });
    expect(res.status(), `登录失败，请预置测试用户 ${TEST_USER.username}`).toBe(200);
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
  // 锚点说明：nav-* 侧边栏项受 RBAC 权限过滤（sidebar:dashboard 等），
  // 预置测试用户无 RBAC 角色时不渲染；page-title / KPI 卡片不受权限影响。
  test('[UI-i18n-1] 默认语言为中文', async ({ page }) => {
    // 语言切换按钮默认显示「目标语言」EN（表示当前为中文）
    await expect(page.locator('[data-testid="language-toggle"]')).toHaveText('EN');
    // 页面标题与 KPI 卡片为中文
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('仪表盘');
    await expect(page.locator('[data-testid="dashboard-stat-kb"]')).toContainText('知识库文档');
  });

  // UI-i18n-2: 切换到英文后全站静态文案即时切换
  test('[UI-i18n-2] 切换到英文后文案即时切换', async ({ page }) => {
    await page.locator('[data-testid="language-toggle"]').click();
    // router.refresh() 后服务端按 en 重渲染，等待标题文案变化
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('Dashboard', { timeout: 10000 });
    await expect(page.locator('[data-testid="dashboard-stat-kb"]')).toContainText('KB Documents', { timeout: 10000 });
    // 切换按钮变为「中文」（目标语言），表示当前为英文
    await expect(page.locator('[data-testid="language-toggle"]')).toHaveText('中文');

    // 再切回中文
    await page.locator('[data-testid="language-toggle"]').click();
    await expect(page.locator('[data-testid="page-title"]')).toHaveText('仪表盘', { timeout: 10000 });
    await expect(page.locator('[data-testid="dashboard-stat-kb"]')).toContainText('知识库文档', { timeout: 10000 });
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
