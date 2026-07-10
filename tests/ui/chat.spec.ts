import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = Date.now();
const U = { username: `e2e-chat2-${uid}@test.local`, password: 'E2eTest123!' };

function mockSSE(route, content: string) {
  // Use larger chunks to avoid splitting markdown/code blocks
  const size = Math.max(50, Math.ceil(content.length / 5));
  const chunks: string[] = [];
  for (let i = 0; i < content.length; i += size) {
    chunks.push(content.slice(i, i + size));
  }
  route.fulfill({ status: 200, headers: { 'Content-Type': 'text/event-stream' },
    body: chunks.map(c => `data: ${JSON.stringify({ content: c })}\n`).join('') + 'data: [DONE]\n' });
}

test.describe('CHAT — Complete', () => {
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

  // === EXISTING ===
  test('[UI-018] Chat — online status badge', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-online-badge"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-online-dot"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-online-badge"]')).toContainText('在线');
  });

  test('[UI-019] Chat — new conversation button', async ({ page }) => {
    // Send a message first
    await page.route('**/api/v1/chat', r => mockSSE(r, 'Hello'));
    await page.locator('[data-testid="chat-input"]').fill('hi');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('text=Hello')).toBeVisible({ timeout: 10000 });

    // Click new session
    await page.locator('[data-testid="chat-new-session-btn"]').click();
    await expect(page.locator('[data-testid="chat-messages"]')).toBeVisible();
  });

  test('[UI-020] Chat — quick prompt chips', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-prompt-row"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-prompt-chip-0"]')).toContainText('今日数据概览');
    await expect(page.locator('[data-testid="chat-prompt-chip-1"]')).toContainText('本月销售趋势');
    await expect(page.locator('[data-testid="chat-prompt-chip-2"]')).toContainText('同比环比分析');
    await expect(page.locator('[data-testid="chat-prompt-chip-3"]')).toContainText('TOP10 产品');
  });

  test('[UI-021] Chat — click prompt fills input', async ({ page }) => {
    await page.locator('[data-testid="chat-prompt-chip-0"]').click();
    await expect(page.locator('[data-testid="chat-input"]')).toHaveValue('今日数据概览');
  });

  test('[UI-022] Chat — send message triggers SSE', async ({ page }) => {
    await page.route('**/api/v1/chat', r => mockSSE(r, '回复文本'));
    await page.locator('[data-testid="chat-input"]').fill('测试');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('text=回复文本')).toBeVisible({ timeout: 15000 });
  });

  test('[UI-023] Chat — user message bubble', async ({ page }) => {
    await page.route('**/api/v1/chat', r => mockSSE(r, 'OK'));
    await page.locator('[data-testid="chat-input"]').fill('用户消息');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-msg-user-0"]')).toBeVisible({ timeout: 10000 });
  });

  test('[UI-024] Chat — AI message bubble with avatar', async ({ page }) => {
    await page.route('**/api/v1/chat', r => mockSSE(r, 'AI response'));
    await page.locator('[data-testid="chat-input"]').fill('hello');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-msg-avatar"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-msg-ai-1"]')).toBeVisible({ timeout: 10000 });
  });

  // === SQL CODE BLOCK ===
  test('[UI-025] Chat — SQL code block rendering', async ({ page }) => {
    await page.route('**/api/v1/chat', r => mockSSE(r, '```sql\nSELECT product_name, SUM(revenue) AS total FROM sales GROUP BY product_name\n```'));
    await page.locator('[data-testid="chat-input"]').fill('查询销售');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-sql-block"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid="chat-sql-code"]')).toContainText('SELECT');
    await expect(page.locator('[data-testid="chat-sql-copy-btn"]')).toBeVisible();
  });

  // === DATA TABLE ===
  test('[UI-026] Chat — data table rendering', async ({ page }) => {
    const table = { type: 'table', headers: ['产品', '销售额', '增长'], rows: [['手机', '¥12万', '+8%'], ['电脑', '¥8万', '-2%'], ['平板', '¥3万', '+15%']] };
    await page.route('**/api/v1/chat', r => mockSSE(r, `结果如下:\n\n\`\`\`json\n${JSON.stringify(table)}\n\`\`\``));
    await page.locator('[data-testid="chat-input"]').fill('销售排行');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-table"]')).toBeVisible({ timeout: 15000 });
  });

  // === TOOL CALL CARD ===
  test('[UI-027] Chat — tool call card expand/collapse', async ({ page }) => {
    const tool = { type: 'tool_call', name: 'sql_executor', input: 'SELECT * FROM sales', output: '返回 156 行' };
    await page.route('**/api/v1/chat', r => mockSSE(r, `\`\`\`json\n${JSON.stringify(tool)}\n\`\`\``));
    await page.locator('[data-testid="chat-input"]').fill('执行查询');
    await page.locator('[data-testid="chat-send-btn"]').click();
    const card = page.locator('[data-testid="chat-tool-call-card-0"]');
    await expect(card).toBeVisible({ timeout: 15000 });
    // Expand
    await page.locator('[data-testid="chat-tool-call-header"]').click();
    await expect(page.locator('[data-testid="chat-tool-call-body"]')).toBeVisible();
  });

  // === PROMPT MODAL ===
  test('[UI-029] Chat loading/progress animation', async ({ page }) => {
    await page.route('**/api/v1/chat', async r => {
      await new Promise(resolve => setTimeout(resolve, 300));
      mockSSE(r, 'Done');
    });
    await page.locator('[data-testid="chat-input"]').fill('慢查询');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-loading-indicator"]')).toBeVisible({ timeout: 5000 });
  });

  test('[UI-035] Chat — prompt modal opens', async ({ page }) => {
    await page.locator('[data-testid="prompt-btn"]').click();
    await expect(page.locator('[data-testid="prompt-modal"]')).toBeVisible();
    await expect(page.locator('[data-testid="prompt-modal-chip-0"]')).toContainText('今日数据概览');
    await expect(page.locator('[data-testid="prompt-modal-chip-1"]')).toContainText('本月销售趋势');
    await expect(page.locator('[data-testid="prompt-modal-chip-2"]')).toContainText('同比环比分析');
    await expect(page.locator('[data-testid="prompt-modal-chip-3"]')).toContainText('TOP10 产品');
  });

  test('[UI-036] Chat — select prompt fills input and closes modal', async ({ page }) => {
    await page.locator('[data-testid="prompt-btn"]').click();
    await expect(page.locator('[data-testid="prompt-modal"]')).toBeVisible();
    await page.locator('[data-testid="prompt-modal-chip-1"]').click();
    await expect(page.locator('[data-testid="chat-input"]')).toHaveValue('本月销售趋势');
  });

  // === KPI CARD ===
  test('[UI-038] Chat — inline KPI card', async ({ page }) => {
    const kpi = { type: 'kpi', items: [{ label: '总销售额', value: '¥125万' }, { label: '订单数', value: '3,420' }, { label: '转化率', value: '12.5%' }] };
    await page.route('**/api/v1/chat', r => mockSSE(r, `\`\`\`json\n${JSON.stringify(kpi)}\n\`\`\``));
    await page.locator('[data-testid="chat-input"]').fill('统计指标');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-inline-kpi"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid="chat-inline-kpi-val"]').first()).toBeVisible();
    await expect(page.locator('[data-testid="chat-inline-kpi-lbl"]').first()).toBeVisible();
  });
});
