/**
 * SPEC-079: 全局在线指示灯 + 后端健康检查 API E2E
 *
 * 覆盖 spec §9 验收规则：
 * - 三态 mock（ok/degraded/后端不可达）断言指示灯颜色/文案切换
 * - hover 指示灯弹出 tooltip，断言后端 API 延时 + 逐依赖在线/离线/未启用
 *   （up 显示 latency，down/skipped 不显示）
 * - 红灯（后端不可达）tooltip 显示「后端服务不可达」而非空列表
 * - 登录页 session-expired + error 两个 toast 纵向堆叠、互不遮挡、不与指示灯重叠
 * - 覆盖登录页 / Dashboard / Chat / admin 页指示灯存在
 *
 * 仅 mock 健康检查 API（`**/api/v1/health`）；其余请求走真实后端。
 */
import { test, expect, type Page } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-ind-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };

// ── 健康检查 mock 载荷 ──────────────────────────────────────────────
const OK_HEALTH = {
  status: 'ok',
  time: '2026-09-02T12:00:00Z',
  version: 'v1.5.0',
  uptime_seconds: 86400,
  latency_ms: 24,
  dependencies: {
    mongodb: { status: 'up', latency_ms: 3 },
    redis: { status: 'up', latency_ms: 2 },
    qdrant: { status: 'up', latency_ms: 8 },
    vault: { status: 'up', latency_ms: 12 },
    seaweedfs: { status: 'up', latency_ms: 6 },
    mysql: { status: 'up', latency_ms: 5 },
    arcadedb: { status: 'skipped' },
    presidio: { status: 'skipped' },
  },
};

const DEGRADED_HEALTH = {
  status: 'degraded',
  time: '2026-09-02T12:00:00Z',
  version: 'v1.5.0',
  uptime_seconds: 86400,
  latency_ms: 30,
  dependencies: {
    mongodb: { status: 'up', latency_ms: 3 },
    redis: { status: 'up', latency_ms: 2 },
    qdrant: { status: 'up', latency_ms: 8 },
    vault: { status: 'up', latency_ms: 12 },
    seaweedfs: { status: 'down', latency_ms: 0, error: 'connection refused' },
    mysql: { status: 'up', latency_ms: 5 },
    arcadedb: { status: 'skipped' },
    presidio: { status: 'skipped' },
  },
};

// mock 健康检查 API：body 传 null 表示后端不可达（abort → fetch reject → 红灯）
function mockHealth(page: Page, body: object | null) {
  return page.route('**/api/v1/health', (route) => {
    if (body === null) return route.abort();
    return route.fulfill({
      status: 200,
      contentType: 'application/json',
      body: JSON.stringify(body),
    });
  });
}

const indicator = (page: Page) => page.locator('[data-testid="global-online-indicator"]');
const dot = (page: Page) => page.locator('[data-testid="global-online-dot"]');

// ══════════════════════════════════════════════════════════════════
// 三态 + hover tooltip（登录页，无需认证；指示灯挂 RootLayout，全页面可见）
// ══════════════════════════════════════════════════════════════════
test.describe('ONLINE INDICATOR — SPEC-079 (三态 & tooltip)', () => {
  test('[UI-242] Indicator — ok 绿点「在线」', async ({ page }) => {
    await mockHealth(page, OK_HEALTH);
    await page.goto('/login');

    await expect(indicator(page)).toBeVisible();
    await expect(dot(page)).toHaveClass(/bg-emerald-400/, { timeout: 5000 });
    await expect(indicator(page)).toContainText('在线');
  });

  test('[UI-243] Indicator — degraded 黄点「服务降级」', async ({ page }) => {
    await mockHealth(page, DEGRADED_HEALTH);
    await page.goto('/login');

    await expect(indicator(page)).toBeVisible();
    await expect(dot(page)).toHaveClass(/bg-amber-400/, { timeout: 5000 });
    await expect(indicator(page)).toContainText('服务降级');
  });

  test('[UI-244] Indicator — down 红点「服务离线」(后端不可达)', async ({ page }) => {
    await mockHealth(page, null);
    const reqSeen = page.waitForRequest('**/api/v1/health');
    await page.goto('/login');
    await reqSeen; // 确认健康检查请求确实发起（被 abort）

    await expect(indicator(page)).toBeVisible();
    await expect(dot(page)).toHaveClass(/bg-red-400/, { timeout: 5000 });
    await expect(indicator(page)).toContainText('服务离线');
  });

  test('[UI-245] Indicator — hover tooltip 显示 API 延时 + 逐依赖探活', async ({ page }) => {
    await mockHealth(page, DEGRADED_HEALTH);
    await page.goto('/login');
    await expect(dot(page)).toHaveClass(/bg-amber-400/, { timeout: 5000 });

    await indicator(page).hover();
    const tooltip = page.locator('[data-testid="global-online-tooltip"]');
    await expect(tooltip).toBeVisible();

    // 顶部：后端 API 延时（degraded mock 为 30ms）
    const apiLatency = page.locator('[data-testid="tooltip-api-latency"]');
    await expect(apiLatency).toBeVisible();
    await expect(apiLatency).toContainText('后端 API');
    await expect(apiLatency).toContainText('30ms');

    // up 依赖：显示「在线」+ 延时
    const mongo = page.locator('[data-testid="tooltip-dep-mongodb"]');
    await expect(mongo).toBeVisible();
    await expect(mongo).toContainText('在线');
    await expect(mongo).toContainText('3ms');

    // down 依赖：显示「离线」、不显示延时
    const seaweed = page.locator('[data-testid="tooltip-dep-seaweedfs"]');
    await expect(seaweed).toBeVisible();
    await expect(seaweed).toContainText('离线');
    await expect(seaweed).not.toContainText(/\d+ms/);

    // skipped 依赖：显示「未启用」、不显示延时
    const arcade = page.locator('[data-testid="tooltip-dep-arcadedb"]');
    await expect(arcade).toBeVisible();
    await expect(arcade).toContainText('未启用');
    await expect(arcade).not.toContainText(/\d+ms/);
  });

  test('[UI-246] Indicator — 红灯 tooltip 显示「后端服务不可达」', async ({ page }) => {
    await mockHealth(page, null);
    await page.goto('/login');
    await expect(dot(page)).toHaveClass(/bg-red-400/, { timeout: 5000 });

    await indicator(page).hover();
    const tooltip = page.locator('[data-testid="global-online-tooltip"]');
    await expect(tooltip).toBeVisible();
    await expect(tooltip).toContainText('后端服务不可达');

    // 红灯时不渲染 API 延时行与依赖列表
    await expect(page.locator('[data-testid="tooltip-api-latency"]')).toHaveCount(0);
    await expect(page.locator('[data-testid^="tooltip-dep-"]')).toHaveCount(0);
  });
});

// ══════════════════════════════════════════════════════════════════
// 登录页 toast 治理（无认证）
// ══════════════════════════════════════════════════════════════════
test.describe('ONLINE INDICATOR — SPEC-079 (登录页 toast 治理)', () => {
  test('[UI-247] Login — 两个 toast 纵向堆叠且不与指示灯重叠', async ({ page }) => {
    await mockHealth(page, OK_HEALTH);
    // session-expired 场景：?expired=true 触发第一个 toast
    await page.goto('/login?expired=true');
    await expect(page.locator('[data-testid="login-session-expired-toast"]')).toBeVisible();

    // 输错密码触发第二个 toast（真实后端 401 → generalError）
    await page.locator('[data-testid="login-email-input"]').fill('wrong@user.com');
    await page.locator('[data-testid="login-password-input"]').fill('WrongPass1');
    await page.locator('[data-testid="login-btn"]').click();
    await expect(page.locator('[data-testid="login-error-toast"]')).toBeVisible({ timeout: 10000 });

    // 两个 toast 都在同一个堆叠容器内
    const stack = page.locator('[data-testid="login-toast-stack"]');
    await expect(stack).toBeVisible();
    await expect(stack.locator('[data-testid="login-session-expired-toast"]')).toBeVisible();
    await expect(stack.locator('[data-testid="login-error-toast"]')).toBeVisible();

    // 两个 toast 纵向堆叠、互不遮挡
    const toast1 = await page.locator('[data-testid="login-session-expired-toast"]').boundingBox();
    const toast2 = await page.locator('[data-testid="login-error-toast"]').boundingBox();
    expect(toast1).not.toBeNull();
    expect(toast2).not.toBeNull();
    expect(toast2!.y).toBeGreaterThanOrEqual(toast1!.y + toast1!.height - 1);

    // 堆叠容器不与右上角指示灯重叠（top-14 ≈ 56px 在 top-4 指示灯下方）
    const indBox = await indicator(page).boundingBox();
    const stackBox = await stack.boundingBox();
    expect(indBox).not.toBeNull();
    expect(stackBox).not.toBeNull();
    expect(stackBox!.y).toBeGreaterThanOrEqual(indBox!.y + indBox!.height - 1);
  });
});

// ══════════════════════════════════════════════════════════════════
// 多页面指示灯存在（需认证）
// ══════════════════════════════════════════════════════════════════
test.describe('ONLINE INDICATOR — SPEC-079 (多页面存在)', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await mockHealth(page, OK_HEALTH);
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 10000 });
  });

  test('[UI-248] Indicator — Dashboard / Chat / admin 页均存在', async ({ page }) => {
    // Dashboard
    await page.goto('/');
    await expect(indicator(page)).toBeVisible({ timeout: 5000 });
    await expect(dot(page)).toBeVisible();

    // Chat
    await page.goto('/chat');
    await expect(indicator(page)).toBeVisible({ timeout: 5000 });
    await expect(dot(page)).toBeVisible();

    // admin 页
    await page.goto('/admin');
    await expect(indicator(page)).toBeVisible({ timeout: 5000 });
    await expect(dot(page)).toBeVisible();
  });

  // 回归：SPEC-079 上线后曾把 AppLayout header 的站内信铃铛整体盖住
  // （fixed right-4 指示灯与铃铛位置重叠且 z-60 更高），header 已预留 96px 让位。
  test('[UI-249] Indicator — 站内信铃铛与指示灯不重叠、可点击', async ({ page }) => {
    await page.goto('/');
    await expect(indicator(page)).toBeVisible({ timeout: 5000 });

    const bell = page.locator('[data-testid="notif-bell-icon"]');
    await expect(bell).toBeVisible();

    const indBox = await indicator(page).boundingBox();
    const bellBox = await bell.boundingBox();
    expect(indBox).not.toBeNull();
    expect(bellBox).not.toBeNull();
    const overlaps = !(
      indBox!.x + indBox!.width <= bellBox!.x ||
      bellBox!.x + bellBox!.width <= indBox!.x ||
      indBox!.y + indBox!.height <= bellBox!.y ||
      bellBox!.y + bellBox!.height <= indBox!.y
    );
    expect(overlaps, `indicator=${JSON.stringify(indBox)} bell=${JSON.stringify(bellBox)}`).toBe(false);

    // 铃铛未被遮挡：可点击并展开通知下拉
    await bell.click();
    await expect(page.locator('[data-testid="notif-dropdown"]')).toBeVisible();
  });
});
