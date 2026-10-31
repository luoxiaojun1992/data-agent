import { registerUser } from './utils/auth';
import { test, expect } from '@playwright/test';

/**
 * SPEC-093: Chat 输入框脱敏 E2E（UI-219 ~ UI-223）
 *
 * 脱敏能力由后端 Presidio 服务提供（POST /api/v1/chat/redact），前端只负责
 * 调用与开关持久化（lib/redact.ts），无本地模型加载/「模型就绪」门控。
 * 本 spec 只覆盖可确定性断言的 UI 状态：
 *  - 工具栏渲染（脱敏按钮 + 自动开关，与增强按钮同行）
 *  - 空输入门控（脱敏按钮禁用，输入后启用）
 *  - 自动脱敏开关交互与默认态
 * 手动脱敏的真实后端替换由部署后真机验证（依赖 Presidio 服务）。
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const USER = { username: `e2e-redact-${uid}@test.local`, password: 'RedactTest1!', role: 'admin' };

test.describe('CHAT — 脱敏 (SPEC-093)', () => {
  test.beforeAll(async ({ request }) => {
    await registerUser(request, USER);
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

  // ═══ UI-219: 脱敏工具栏渲染（与增强按钮同行） ═══
  test('[UI-219] Redact — 工具栏渲染（与增强按钮同行）', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-enhance-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-redact-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-redact-auto-toggle"]')).toBeVisible();
    // 同一行断言：脱敏按钮与增强按钮的纵向位置一致（同 flex 行）
    const enhanceBox = await page.locator('[data-testid="chat-enhance-btn"]').boundingBox();
    const redactBox = await page.locator('[data-testid="chat-redact-btn"]').boundingBox();
    expect(enhanceBox && redactBox && Math.abs(enhanceBox.y - redactBox.y) < 4).toBeTruthy();
  });

  // ═══ UI-220: 空输入门控 ═══
  test('[UI-220] Redact — 空输入时脱敏按钮禁用', async ({ page }) => {
    // 初始输入为空 → 手动脱敏按钮禁用（disabled = redacting || !input.trim()）
    await expect(page.locator('[data-testid="chat-redact-btn"]')).toBeDisabled();
    // 输入文本后 → 启用
    await page.locator('[data-testid="chat-input"]').fill('测试脱敏文本');
    await expect(page.locator('[data-testid="chat-redact-btn"]')).toBeEnabled();
  });

  // ═══ UI-221: 自动脱敏开关默认关闭 ═══
  test('[UI-221] Redact — 自动脱敏开关默认关闭', async ({ page }) => {
    const toggle = page.locator('[data-testid="chat-redact-auto-toggle"]');
    await expect(toggle).not.toBeChecked();
  });

  // ═══ UI-222: 自动脱敏开关交互 ═══
  test('[UI-222] Redact — 自动脱敏开关交互', async ({ page }) => {
    const toggle = page.locator('[data-testid="chat-redact-auto-toggle"]');
    await expect(toggle).not.toBeChecked();
    await toggle.check();
    await expect(toggle).toBeChecked();
    await toggle.uncheck();
    await expect(toggle).not.toBeChecked();
  });

  // ═══ UI-223: 脱敏不阻断聊天主流程 ═══
  test('[UI-223] Redact — 脱敏功能不阻断聊天发送', async ({ page }) => {
    // 聊天输入始终可用（脱敏为可选增强，不阻断主流程）
    await expect(page.locator('[data-testid="chat-input"]')).toBeEnabled();
  });
});
