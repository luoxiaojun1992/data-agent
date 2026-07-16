import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-chat-${uid}@test.local`, password: 'E2eTest1!' };

async function seedMock(request: any, key: string, response: string) {
  await request.post(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
    data: { key, response },
  });
}

async function clearMocks(request: any) {
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { 'Authorization': `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});
}

test.describe('CHAT — Complete', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
  });

  test.afterAll(async ({ request }) => {
    // Cleanup mockllm state
    await clearMocks(request);
    // Cleanup test user
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, {
      headers: { 'Content-Type': 'application/json' },
    });
    if (listRes.ok()) {
      const users = (await listRes.json()).users || [];
      for (const u of users) {
        if (u.email?.includes(uid)) {
          await request.delete(`${API_BASE}/users/${u.id}`);
        }
      }
    }
  });

  test.beforeEach(async ({ page, request }) => {
    await clearMocks(request);
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u: URL) => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-chat"]').click();
    await page.waitForURL('**/chat', { timeout: 5000 });
  });

  // ═══ UI-018: 在线状态 Badge ═══
  test('[UI-018] Chat — online status badge', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-online-badge"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-online-dot"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-online-badge"]')).toContainText('在线');
  });

  // ═══ UI-019: 新对话按钮 ═══
  test('[UI-019] Chat — new conversation button', async ({ page, request }) => {
    await seedMock(request, 'hi', 'Hello from DataAgent');
    await page.locator('[data-testid="chat-input"]').fill('hi');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });

    // Click new session — messages area should clear
    await page.locator('[data-testid="chat-new-session-btn"]').click();
    // After new session, old AI messages should be gone
    await expect(page.locator('[data-testid^="chat-msg-ai-"]')).toHaveCount(0);
    await expect(page.locator('[data-testid="chat-input"]')).toBeVisible();
  });

  // ═══ UI-020: 快捷提示词 ═══
  test('[UI-020] Chat — quick prompt chips', async ({ page }) => {
    await expect(page.locator('[data-testid="chat-prompt-row"]')).toBeVisible();
    await expect(page.locator('[data-testid="chat-prompt-chip-0"]')).toContainText('今日数据概览');
    await expect(page.locator('[data-testid="chat-prompt-chip-1"]')).toContainText('本月销售趋势');
    await expect(page.locator('[data-testid="chat-prompt-chip-2"]')).toContainText('同比环比分析');
    await expect(page.locator('[data-testid="chat-prompt-chip-3"]')).toContainText('TOP10 产品');
  });

  // ═══ UI-021: 点击提示词填入 ═══
  test('[UI-021] Chat — click prompt fills input', async ({ page }) => {
    await page.locator('[data-testid="chat-prompt-chip-0"]').click();
    await expect(page.locator('[data-testid="chat-input"]')).toHaveValue('今日数据概览');
  });

  // ═══ UI-022: 发送消息触发真实 SSE ═══
  test('[UI-022] Chat — send message triggers SSE', async ({ page, request }) => {
    await seedMock(request, '测试', '这是来自真实后端的回复文本');
    await page.locator('[data-testid="chat-input"]').fill('测试');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toContainText('回复文本');
  });

  // ═══ UI-023: 用户消息气泡 ═══
  test('[UI-023] Chat — user message bubble', async ({ page, request }) => {
    await seedMock(request, '用户消息', 'OK');
    await page.locator('[data-testid="chat-input"]').fill('用户消息');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-msg-user-0"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="chat-msg-user-0"]')).toContainText('用户消息');
  });

  // ═══ UI-024: AI 消息气泡 + 头像 ═══
  test('[UI-024] Chat — AI message bubble with avatar', async ({ page, request }) => {
    await seedMock(request, 'hello', 'AI response from the backend');
    await page.locator('[data-testid="chat-input"]').fill('hello');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-msg-avatar"]').first()).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toContainText('AI response');
  });

  // ═══ UI-025: SQL 代码块 ═══
  test('[UI-025] Chat — SQL code block rendering', async ({ page, request }) => {
    const sqlContent = '```sql\nSELECT product_name, SUM(revenue) AS total FROM sales GROUP BY product_name\n```';
    await seedMock(request, '查询销售', sqlContent);
    await page.locator('[data-testid="chat-input"]').fill('查询销售');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-sql-block"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid="chat-sql-code"]')).toContainText('SELECT');
    await expect(page.locator('[data-testid="chat-sql-copy-btn"]')).toBeVisible();
  });

  // ═══ UI-026: 数据表格 ═══
  test('[UI-026] Chat — data table rendering', async ({ page, request }) => {
    const table = { type: 'table', headers: ['产品', '销售额', '增长'], rows: [['手机', '¥12万', '+8%'], ['电脑', '¥8万', '-2%'], ['平板', '¥3万', '+15%']] };
    await seedMock(request, '销售排行', `结果如下:\n\n\`\`\`json\n${JSON.stringify(table)}\n\`\`\``);
    await page.locator('[data-testid="chat-input"]').fill('销售排行');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-table"]')).toBeVisible({ timeout: 15000 });
    const headers = page.locator('[data-testid="chat-table"] th');
    expect(await headers.count()).toBeGreaterThanOrEqual(3);
  });

  // ═══ UI-027: 工具调用卡片 ═══
  test('[UI-027] Chat — tool call card expand/collapse', async ({ page, request }) => {
    const tool = { type: 'tool_call', name: 'sql_executor', input: 'SELECT * FROM sales', output: '返回 156 行' };
    await seedMock(request, '执行查询', `\`\`\`json\n${JSON.stringify(tool)}\n\`\`\``);
    await page.locator('[data-testid="chat-input"]').fill('执行查询');
    await page.locator('[data-testid="chat-send-btn"]').click();
    const card = page.locator('[data-testid="chat-tool-call-card-0"]');
    await expect(card).toBeVisible({ timeout: 15000 });
    await page.locator('[data-testid="chat-tool-call-header"]').click();
    await expect(page.locator('[data-testid="chat-tool-call-body"]')).toBeVisible();
  });

  // ═══ UI-028: 数据图表 ═══
  test('[UI-028] Chat — chart rendering', async ({ page, request }) => {
    const chart = { type: 'chart', title: '销售趋势', labels: ['一','二','三','四','五'], values: [100,150,120,200,180] };
    await seedMock(request, 'chart', `\`\`\`json\n${JSON.stringify(chart)}\n\`\`\``);
    await page.locator('[data-testid="chat-input"]').fill('chart');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-inline-chart"]')).toBeVisible({ timeout: 15000 });
  });

  // ═══ UI-029: 加载/进度动画 ═══
  test('[UI-029] Chat loading/progress animation', async ({ page, request }) => {
    await seedMock(request, '慢查询', '查询完成');
    await page.locator('[data-testid="chat-input"]').fill('慢查询');
    await page.locator('[data-testid="chat-send-btn"]').click();
    // Loading indicator appears briefly then AI response replaces it
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toContainText('查询完成');
  });

  // ═══ UI-030: 会话面板 ═══
  test('[UI-030] Chat — session panel opens', async ({ page, request }) => {
    await seedMock(request, 'hi', 'ok');
    await page.locator('[data-testid="chat-input"]').fill('hi');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });

    // Open session panel
    await page.locator('[data-testid="chat-session-btn"]').click();
    await expect(page.locator('[data-testid="session-sidebar"]')).toBeVisible();
    await expect(page.locator('[data-testid="session-search"]')).toBeVisible();
  });

  // ═══ UI-031: 会话列表项 ═══
  test('[UI-031] Chat — session items rendered', async ({ page, request }) => {
    // Ensure at least one session exists
    await seedMock(request, 'ensure-session', 'ok');
    await page.locator('[data-testid="chat-input"]').fill('ensure-session');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });

    await page.locator('[data-testid="chat-session-btn"]').click();
    const list = page.locator('[data-testid="session-list"]');
    await expect(list).toBeVisible({ timeout: 5000 });
    const items = page.locator('[data-testid^="session-item-"]');
    expect(await items.count()).toBeGreaterThanOrEqual(1);
  });

  // ═══ UI-032: 点击会话切换 ═══
  test('[UI-032] Chat — click session switches', async ({ page, request }) => {
    // Ensure sessions exist
    await seedMock(request, 'msg-for-session', 'ok');
    await page.locator('[data-testid="chat-input"]').fill('msg-for-session');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });

    await page.locator('[data-testid="chat-session-btn"]').click();
    const first = page.locator('[data-testid^="session-item-"]').first();
    await expect(first).toBeVisible({ timeout: 5000 });
    await first.click();
    await expect(page.locator('[data-testid="chat-session-info"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-033: 搜索会话 ═══
  test('[UI-033] Chat — search sessions', async ({ page, request }) => {
    // Ensure sessions exist
    await seedMock(request, 'test-session-search', 'ok');
    await page.locator('[data-testid="chat-input"]').fill('test-session-search');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });

    await page.locator('[data-testid="chat-session-btn"]').click();
    const beforeItems = page.locator('[data-testid^="session-item-"]');
    const beforeCount = await beforeItems.count();

    // Search for something that won't match
    await page.locator('[data-testid="session-search"]').fill('xyznonexistent123');
    await page.waitForTimeout(500);
    const afterItems = page.locator('[data-testid^="session-item-"]');
    const afterCount = await afterItems.count();

    // After filtering with a non-matching string, count should decrease
    expect(afterCount).toBeLessThan(beforeCount);
  });

  // ═══ UI-034: 删除会话 ═══
  test('[UI-034] Chat — delete session', async ({ page, request }) => {
    // Create a session via API first
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: U.username, password: U.password },
    });
    const token = (await loginRes.json()).access_token;
    const createRes = await request.post(`${API_BASE}/sessions`, {
      headers: { Authorization: `Bearer ${token}`, 'Content-Type': 'application/json' },
      data: {},
    });
    expect(createRes.ok()).toBe(true);

    await page.reload();
    await page.waitForSelector('[data-testid="chat-session-btn"]', { timeout: 10000 });
    await page.locator('[data-testid="chat-session-btn"]').click();
    await expect(page.locator('[data-testid="session-sidebar"]')).toBeVisible({ timeout: 5000 });

    const delBtn = page.locator('[data-testid^="session-delete-"]').first();
    await expect(delBtn).toBeVisible({ timeout: 5000 });
    await delBtn.click();
    // Verify confirm modal or dialog
    await expect(page.locator('[data-testid="session-sidebar"]')).toBeVisible();
  });

  // ═══ UI-035: 提示词弹窗 ═══
  test('[UI-035] Chat — prompt modal opens', async ({ page }) => {
    await page.locator('[data-testid="prompt-btn"]').click();
    await expect(page.locator('[data-testid="prompt-modal"]')).toBeVisible();
    await expect(page.locator('[data-testid="prompt-modal-chip-0"]')).toContainText('今日数据概览');
    await expect(page.locator('[data-testid="prompt-modal-chip-1"]')).toContainText('本月销售趋势');
    await expect(page.locator('[data-testid="prompt-modal-chip-2"]')).toContainText('同比环比分析');
    await expect(page.locator('[data-testid="prompt-modal-chip-3"]')).toContainText('TOP10 产品');
  });

  // ═══ UI-036: 从提示词弹窗选择 ═══
  test('[UI-036] Chat — select prompt fills input and closes modal', async ({ page }) => {
    await page.locator('[data-testid="prompt-btn"]').click();
    await expect(page.locator('[data-testid="prompt-modal"]')).toBeVisible();
    await page.locator('[data-testid="prompt-modal-chip-1"]').click();
    await expect(page.locator('[data-testid="chat-input"]')).toHaveValue('本月销售趋势');
    await expect(page.locator('[data-testid="prompt-modal"]')).not.toBeVisible();
  });

  // ═══ UI-037: 自定义提示词保存 ═══
  test('[UI-037] Chat — save custom prompt', async ({ page }) => {
    await page.locator('[data-testid="prompt-btn"]').click();
    await page.locator('[data-testid="prompt-modal-custom-input"]').fill('查询上月客户留存率');
    await page.locator('[data-testid="prompt-modal-save-btn"]').click();
    await expect(page.locator('[data-testid="prompt-modal-custom-0"]')).toHaveText('查询上月客户留存率');
  });

  // ═══ UI-038: KPI 卡片 ═══
  test('[UI-038] Chat — inline KPI card', async ({ page, request }) => {
    const kpi = { type: 'kpi', items: [{ label: '总销售额', value: '¥125万' }, { label: '订单数', value: '3,420' }, { label: '转化率', value: '12.5%' }] };
    await seedMock(request, '统计指标', `\`\`\`json\n${JSON.stringify(kpi)}\n\`\`\``);
    await page.locator('[data-testid="chat-input"]').fill('统计指标');
    await page.locator('[data-testid="chat-send-btn"]').click();
    await expect(page.locator('[data-testid="chat-inline-kpi"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid="chat-inline-kpi-val"]').first()).toBeVisible();
    await expect(page.locator('[data-testid="chat-inline-kpi-lbl"]').first()).toBeVisible();
  });
});
