import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const U = { username: `e2e-hms-${uid}@test.local`, password: 'E2eTest123!' };

test.describe('HERMES — Mode Toggle & Explore', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL(u => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-chat"]').click();
    await page.waitForURL('**/chat', { timeout: 5000 });
  });

  // UI-057: Mode toggle renders both tabs
  test('[UI-057] Hermes — mode toggle renders', async ({ page }) => {
    await expect(page.locator('[data-testid="mode-toggle"]')).toBeVisible();
    await expect(page.locator('[data-testid="mode-toggle-analysis"]')).toContainText('分析模式');
    await expect(page.locator('[data-testid="mode-toggle-hermes"]')).toContainText('探索模式');
    // Default: analysis mode selected
    await expect(page.locator('[data-testid="mode-toggle-analysis"]')).toHaveClass(/bg-\[var\(--accent\)\]/);
  });

  // UI-058: Switch to Hermes mode
  test('[UI-058] Hermes — switch to explore mode', async ({ page }) => {
    await page.locator('[data-testid="mode-toggle-hermes"]').click();
    // Header should change
    await expect(page.locator('[data-testid="chat-header"] h2')).toContainText('Hermes');
    // Should show offline status (Hermes service not running)
    await expect(page.locator('[data-testid="chat-session-info"]')).toContainText('Offline');
  });

  // UI-061: Offline status indicator
  test('[UI-061] Hermes — offline status', async ({ page }) => {
    await page.locator('[data-testid="mode-toggle-hermes"]').click();
    await expect(page.locator('[data-testid="chat-session-info"]')).toContainText('Offline');
    // Switch back to analysis — should show online
    await page.locator('[data-testid="mode-toggle-analysis"]').click();
    await expect(page.locator('[data-testid="chat-online-badge"]')).toBeVisible();
  });

  // UI-059: Send message in Hermes mode (mock SSE)
  test('[UI-059] Hermes — send message in explore mode', async ({ page }) => {
    // Mock Hermes endpoint
    await page.route('**/api/v1/hermes/chat', route => {
      const mockResponse = 'Hermes response: trends in data science include LLMs, MLOps, and real-time analytics.';
      const chunks = [mockResponse.slice(0, 50), mockResponse.slice(50)];
      route.fulfill({ status: 200, headers: { 'Content-Type': 'text/event-stream' },
        body: chunks.map(c => `data: ${JSON.stringify({ content: c })}\n`).join('') + 'data: [DONE]\n' });
    });
    await page.locator('[data-testid="mode-toggle-hermes"]').click();
    await page.waitForSelector('[data-testid="chat-input"]', { timeout: 5000 });
    await page.locator('[data-testid="chat-input"]').fill('trends in data science');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('text=Hermes response')).toBeVisible({ timeout: 15000 });
  });
});
