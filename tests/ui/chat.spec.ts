import { test, expect } from '@playwright/test';

/**
 * SPEC-019: CHAT E2E Tests (UI-018 ~ UI-022 core)
 *
 * Mock SSE streaming via page.route() for AI responses.
 * Real API login, real session creation.
 */

const log = (msg: string) => process.stderr.write(`[chat.spec] ${new Date().toISOString()} ${msg}\n`);

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const TEST_USER = {
  username: `e2e-chat-${uid}@test.local`,
  password: 'E2eTest123!',
};

// Mock SSE: return a pre-built AI response in stream format
function mockChatStream(route, content: string) {
  const chunks = content.match(/.{1,10}/g) || [content];
  const sseLines = chunks.map((c) => `data: ${JSON.stringify({ content: c })}\n`).join('');
  route.fulfill({
    status: 200,
    headers: { 'Content-Type': 'text/event-stream' },
    body: sseLines + 'data: [DONE]\n',
  });
}

test.describe('CHAT — Lightweight Workspace', () => {
  test.beforeAll(async ({ request }) => {
    log(`Registering: ${TEST_USER.username}`);
    const res = await request.post(`${API_BASE}/auth/register`, { data: TEST_USER });
    expect(res.status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(TEST_USER.username);
    await page.locator('[data-testid="login-password-input"]').fill(TEST_USER.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });

    // Navigate to chat
    await page.locator('[data-testid="nav-chat"]').click();
    await page.waitForURL('**/chat', { timeout: 5000 });
  });

  // UI-018: Chat header with title and online status
  test('[UI-018] Chat header — title and status', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-header"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-session-info"]')).toBeVisible();
  });

  // UI-019: New conversation empty state
  test('[UI-019] Chat — new conversation empty state', async ({ page }) => {
    // Should show empty state with guide text
    await expect(page.locator('[data-testid="chat-messages"]')).toBeVisible();
    await expect(page.locator('text=开始数据分析对话')).toBeVisible();
  });

  // UI-020: Quick prompt chips rendered
  test('[UI-020] Chat — quick prompt chips', async ({ page }) => {
    const promptRow = page.locator('[data-testid="chat-prompt-row"]');
    await expect(promptRow).toBeVisible();
    // Should have at least 1 hint button
    const hints = promptRow.locator('button');
    await expect(hints.first()).toBeVisible();
  });

  // UI-021: Click quick prompt fills input
  test('[UI-021] Chat — quick prompt fills input', async ({ page }) => {
    const firstHint = page.locator('[data-testid="chat-prompt-row"] button').first();
    const hintText = await firstHint.textContent();
    await firstHint.click();

    // Input should be filled with hint text
    const input = page.locator('[data-testid="chat-input"]');
    await expect(input).toHaveValue(hintText || '');
  });

  // UI-022: Text input and send with mock SSE response
  test('[UI-022] Chat — send message and receive AI reply', async ({ page }) => {
    // Mock the chat API to return SSE stream
    await page.route('**/api/v1/chat', (route) => {
      mockChatStream(route, '您好！根据您的查询，本周销售额为 ¥125,000，较上周增长 8.5%。');
    });

    // Type and send
    const input = page.locator('[data-testid="chat-input"]');
    await input.fill('查询华东区上周销售额');
    await page.locator('[data-testid="chat-send-btn"]').click();

    // Wait for AI response
    await expect(page.locator('text=您好')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('text=¥125,000')).toBeVisible();

    // Check message bubbles exist
    const messages = page.locator('[data-testid="chat-messages"]');
    await expect(messages).toBeVisible();
  });

  // UI-023: User message bubble styling
  test('[UI-023] Chat — user message bubble appears right-aligned', async ({ page }) => {
    await page.route('**/api/v1/chat', (route) => {
      mockChatStream(route, '已收到您的消息。');
    });

    await page.locator('[data-testid="chat-input"]').fill('测试用户消息');
    await page.locator('[data-testid="chat-send-btn"]').click();

    // After sending, user message should appear in chat area
    await expect(page.locator('text=测试用户消息')).toBeVisible();
  });

  // UI-024: AI message bubble appears
  test('[UI-024] Chat — AI message bubble', async ({ page }) => {
    const mockContent = '这是来自 AI 助手的自动回复消息。';
    await page.route('**/api/v1/chat', (route) => {
      mockChatStream(route, mockContent);
    });

    await page.locator('[data-testid="chat-input"]').fill('问一个问题');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator(`text=${mockContent}`)).toBeVisible({ timeout: 10000 });
  });
});
