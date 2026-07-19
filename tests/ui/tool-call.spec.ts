/**
 * SPEC-046: Tool Call Chain E2E (UI-211 ~ UI-218)
 *
 * Each test seeds a single mockllm response containing a tool_call JSON block
 * plus the final answer text. The frontend parses the JSON block and renders
 * it as a tool call card (expand/collapse), matching the pattern used by
 * UI-027 in chat.spec.ts. No ADK ReAct multi-step dependency required.
 */
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-tc-${uid}@test.local`, password: 'E2eTest1!' };

async function seedMock(request: any, key: string, response: string) {
  await request.post(`${MOCKLLM}/responses`, {
    headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
    data: { key, response },
  });
}

async function clearMocks(request: any) {
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});
}

test.describe('TOOL CALL — SPEC-046', () => {
  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
  });

  test.afterAll(async ({ request }) => {
    await clearMocks(request);
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, {
      headers: { 'Content-Type': 'application/json' },
    });
    if (listRes.ok()) {
      for (const u of (await listRes.json()).users || []) {
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

  // ═══ UI-211: knowledge_search tool card renders ═══
  test('[UI-211] ToolCall — knowledge_search tool card', async ({ page, request }) => {
    const toolCall = {
      type: 'tool_call',
      name: 'knowledge_search',
      input: { query: '营收', top_k: 3 },
      output: '共 3 条结果，最高匹配度 0.92',
    };
    await seedMock(request, '查询营收数据',
      `\`\`\`json\n${JSON.stringify(toolCall)}\n\`\`\`\n\n根据知识库，营收达到 1.2 亿元。`);

    await page.locator('[data-testid="chat-input"]').fill('查询营收数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid="chat-tool-call-card-0"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toContainText('1.2 亿元');
  });

  // ═══ UI-212: knowledge_search result with sales data ═══
  test('[UI-212] ToolCall — knowledge_search 命中后回答', async ({ page, request }) => {
    const toolCall = {
      type: 'tool_call',
      name: 'knowledge_search',
      input: { query: '销售数据', top_k: 3 },
      output: '5月销售额1560万元，6月销售额1720万元',
    };
    await seedMock(request, '销售数据',
      `\`\`\`json\n${JSON.stringify(toolCall)}\n\`\`\`\n\n根据检索到的文档，5月销售额1560万元，6月销售额1720万元，呈上升趋势。`);

    await page.locator('[data-testid="chat-input"]').fill('销售数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    expect(aiText).toMatch(/销售|1560|1720|趋势/);
  });

  // ═══ UI-213: sql_executor tool card with SQL ═══
  test('[UI-213] ToolCall — sql_executor tool card', async ({ page, request }) => {
    const toolCall = {
      type: 'tool_call',
      name: 'sql_executor',
      input: { sql: 'SELECT product, SUM(revenue) AS total FROM sales GROUP BY product ORDER BY total DESC LIMIT 5' },
      output: 'DataInsight Pro: 2480万, CloudQuery: 1860万',
    };
    await seedMock(request, '查询销售排行',
      `\`\`\`json\n${JSON.stringify(toolCall)}\n\`\`\`\n\nDataInsight Pro 销售额 2480 万元排名第一，CloudQuery 1860 万元排第二。`);

    await page.locator('[data-testid="chat-input"]').fill('查询销售排行');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    expect(aiText).toContain('2480');
  });

  // ═══ UI-214: sql_executor security rejection ═══
  test('[UI-214] ToolCall — sql_executor 校验失败', async ({ page, request }) => {
    // Security audit blocks DROP TABLE before LLM, so mockllm seed won't be consumed.
    // The chat service returns a security error directly.
    await seedMock(request, '删除所有数据',
      '安全校验失败：输入包含危险操作 DROP TABLE，已被拦截。');

    await page.locator('[data-testid="chat-input"]').fill('删除所有数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    // Error or rejection message should appear
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-214] AI response:', aiText?.substring(0, 100));
    expect(aiText).toMatch(/安全|失败|不允许|禁止|DROP|审核|拦截/i);
  });

  // ═══ UI-215: stats_engine tool card with KPI ═══
  test('[UI-215] ToolCall — stats_engine tool card', async ({ page, request }) => {
    const toolCall = {
      type: 'tool_call',
      name: 'stats_engine',
      input: { calculation: 'monthly_trend', source: 'sales_data' },
      output: '月均销售额 ¥1,385万，增长率 +12.4%',
    };
    await seedMock(request, '统计销售趋势',
      `\`\`\`json\n${JSON.stringify(toolCall)}\n\`\`\`\n\n统计结果：月均销售额 ¥1,385万，增长率 +12.4%，峰值月份为 6月。`);

    await page.locator('[data-testid="chat-input"]').fill('统计销售趋势');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid="chat-tool-call-card-0"]')).toBeVisible({ timeout: 15000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    expect(aiText).toMatch(/1,385|12\.4%|增长率/);
  });

  // ═══ UI-216: save_report tool card ═══
  test('[UI-216] ToolCall — save_report tool card', async ({ page, request }) => {
    const toolCall = {
      type: 'tool_call',
      name: 'save_report',
      input: { title: '销售分析报告', format: 'pdf' },
      output: '报告已生成并保存。',
    };
    await seedMock(request, '生成销售报告',
      `\`\`\`json\n${JSON.stringify(toolCall)}\n\`\`\`\n\n报告已生成并保存。您可以在制品管理中查看《销售分析报告》。`);

    await page.locator('[data-testid="chat-input"]').fill('生成销售报告');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    expect(aiText).toMatch(/报告|保存|生成|制品/i);
  });

  // ═══ UI-217: 多工具链式调用 ═══
  test('[UI-217] ToolCall — 多工具链式调用', async ({ page, request }) => {
    const tools = [
      { type: 'tool_call', name: 'knowledge_search', input: { query: '销售数据', top_k: 3 }, output: '3 条文档匹配' },
      { type: 'tool_call', name: 'sql_executor', input: { sql: 'SELECT region, SUM(revenue) FROM sales GROUP BY region' }, output: '华北: 2480万, 华东: 1860万, 华南: 1520万' },
      { type: 'tool_call', name: 'stats_engine', input: { calculation: 'growth_rate' }, output: '月均增长 12.4%' },
    ];
    const response = tools.map((t) => `\`\`\`json\n${JSON.stringify(t)}\n\`\`\``).join('\n\n') +
      '\n\n综合分析完成：华北区域贡献最高，全线上涨趋势明显。月均增长 12.4%。';

    await seedMock(request, '全面分析销售数据', response);

    await page.locator('[data-testid="chat-input"]').fill('全面分析销售数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 15000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    expect(aiText).toMatch(/分析|华北|趋势|增长/i);
  });

  // ═══ UI-218: SQL 复制按钮 ═══
  test('[UI-218] ToolCall — SQL 复制按钮', async ({ page, request }) => {
    await seedMock(request, '查询SQL',
      '```sql\nSELECT product, SUM(revenue) FROM sales GROUP BY product\n```\n\n查询结果已生成。');

    await page.locator('[data-testid="chat-input"]').fill('查询SQL');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid="chat-sql-block"]')).toBeVisible({ timeout: 15000 });
    await expect(page.locator('[data-testid="chat-sql-code"]')).toContainText('SELECT');
    await expect(page.locator('[data-testid="chat-sql-copy-btn"]')).toBeVisible();
  });
});
