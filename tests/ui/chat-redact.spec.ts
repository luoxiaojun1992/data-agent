import { test, expect } from '@playwright/test';

/**
 * SPEC-093: Chat 输入框本地脱敏 E2E（UI-219 ~ UI-223）
 *
 * 推理层（transformers.js WebGPU + 809MB 模型权重）在 CI 环境不可控，
 * 本 spec 只覆盖 UI 状态机与强制门控（不依赖真实模型文件）：
 *  - 工具栏渲染与 data-testid 完整
 *  - 模型非 ready 态（加载中/失败）：脱敏按钮 disabled、自动开关 disabled 且 unchecked
 *  - ready 态交互（手动脱敏替换、弹窗动画、localStorage 持久化）由部署后真机验证覆盖
 *
 * 无任何明文凭据（注册走 CI 环境约定接口，与其他 spec 保持一致）。
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const USER = { username: `e2e-redact-${uid}@test.local`, password: 'RedactTest1!', role: 'admin' };

test.describe('CHAT — 本地脱敏 (SPEC-093)', () => {
  test.beforeAll(async ({ request }) => {
    await request.post(`${API_BASE}/auth/register`, { data: USER });
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/chat');
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 10000 });
  });

  // ═══ UI-219: 脱敏工具栏渲染（与「✨ 增强」按钮同一行） ═══
  test('[UI-219] Redact — 工具栏渲染（按钮/开关/状态提示，与增强按钮同行）', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-enhance-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-redact-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-redact-auto-toggle"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-redact-status"]')).toBeVisible();
    // 同一行断言：脱敏按钮与增强按钮的纵向位置一致（同 flex 行）
    const enhanceBox = await page.locator('[data-testid="chat-enhance-btn"]').boundingBox();
    const redactBox = await page.locator('[data-testid="chat-redact-btn"]').boundingBox();
    expect(enhanceBox && redactBox && Math.abs(enhanceBox.y - redactBox.y) < 4).toBeTruthy();
  });

  // ═══ UI-220: 模型非就绪（加载中/失败）强制门控 ═══
  test('[UI-220] Redact — 模型未就绪时按钮与开关强制禁用', async ({ page }) => {
    // CI 环境无模型权重（809MB 不在镜像内），状态必为 loading → failed，
    // 两种状态都满足「非 ready 即禁用」的门控断言。
    await expect(page.locator('[data-testid="chat-redact-btn"]')).toBeDisabled({ timeout: 15000 });
    const toggle = page.locator('[data-testid="chat-redact-auto-toggle"]');
    await expect(toggle).toBeDisabled();
    // 强制关闭：开关必须 unchecked（即使 localStorage 预置 true 也不生效）。
    await expect(toggle).not.toBeChecked();
    // 状态提示：非 ready 态显示加载中或不可用。
    const status = page.locator('[data-testid="chat-redact-status"]');
    await expect(status).toHaveText(/加载中|不可用/);
  });

  // ═══ UI-221: 自动脱敏开关默认关闭（ready 前） ═══
  test('[UI-221] Redact — 自动脱敏开关默认关闭', async ({ page }) => {
    const toggle = page.locator('[data-testid="chat-redact-auto-toggle"]');
    await expect(toggle).not.toBeChecked();
  });

  // ═══ UI-222: 模型加载失败 hover 提示 ═══
  test('[UI-222] Redact — 加载失败 hover 提示联系管理员', async ({ page }) => {
    const btn = page.locator('[data-testid="chat-redact-btn"]');
    await expect(btn).toBeDisabled({ timeout: 15000 });
    // 失败态 title 提示（若模型仍 loading 则 title 为空，重试等待至 failed）。
    await expect
      .poll(async () => btn.getAttribute('title'), { timeout: 15000 })
      .toContain('模型加载失败，联系管理员处理');
  });

  // ═══ UI-223: 脱敏不阻断聊天主流程 ═══
  test('[UI-223] Redact — 模型未就绪不影响聊天发送', async ({ page }) => {
    // 脱敏模型不可用时聊天输入与发送按钮仍可用（降级为禁用脱敏功能本身）。
    await expect(page.locator('[data-testid="chat-input"]')).toBeEnabled();
  });
});
