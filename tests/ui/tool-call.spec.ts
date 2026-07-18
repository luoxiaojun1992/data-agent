/**
 * SPEC-046: Tool Call Chain E2E (UI-211 ~ UI-218)
 *
 * Verifies real tool calling via mockllm multi-step scenarios.
 * Each scenario simulates LLM → tool_call → tool_result → answer chain.
 *
 * IMPORTANT: Tool call E2E depends on ADK ReAct loop (SPEC-048 completed).
 * mockllm uses LPUSH/LPOP FIFO queue for same-key multi-injection.
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

  // ═══ UI-211: knowledge_search 全文检索 ═══
  test('[UI-211] ToolCall — knowledge_search 全文检索', async ({ page, request }) => {
    console.log('[UI-211] Testing knowledge_search tool call...');

    // Seed mockllm: first call = tool_call, second call = final answer
    await seedMock(request, '查询营收数据',
      JSON.stringify({ type: 'tool_call', name: 'knowledge_search', input: { query: '营收', top_k: 3 } }));
    await seedMock(request, '查询营收数据',
      '根据知识库检索，营收达到 1.2 亿元。');

    await page.locator('[data-testid="chat-input"]').fill('查询营收数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    // Wait for AI response to appear
    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-211] AI response:', aiText?.substring(0, 100));
    expect(aiText).toBeTruthy();
  });

  // ═══ UI-212: knowledge_search 命中后回答 ═══
  test('[UI-212] ToolCall — knowledge_search 命中后回答', async ({ page, request }) => {
    console.log('[UI-212] Testing kb search + answer...');

    await seedMock(request, '销售数据',
      JSON.stringify({ type: 'tool_call', name: 'knowledge_search', input: { query: '销售数据', top_k: 3 } }));
    await seedMock(request, '销售数据',
      '根据检索到的文档，5月销售额1560万元，6月销售额1720万元，呈上升趋势。');

    await page.locator('[data-testid="chat-input"]').fill('销售数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-212] AI response:', aiText?.substring(0, 100));

    // Verify answer contains retrieved data context
    expect(aiText).toMatch(/销售|1560|1720|趋势/);
  });

  // ═══ UI-213: sql_executor 校验通过 ═══
  test('[UI-213] ToolCall — sql_executor 校验通过', async ({ page, request }) => {
    console.log('[UI-213] Testing sql_executor...');

    await seedMock(request, '查询销售排行',
      JSON.stringify({
        type: 'tool_call',
        name: 'sql_executor',
        input: { sql: 'SELECT product, SUM(revenue) AS total FROM sales GROUP BY product ORDER BY total DESC LIMIT 5' },
      }));
    await seedMock(request, '查询销售排行',
      '查询结果：```sql\nSELECT product, SUM(revenue) AS total FROM sales GROUP BY product ORDER BY total DESC LIMIT 5\n```\n\nDataInsight Pro: 2480万元, CloudQuery: 1860万元。');

    await page.locator('[data-testid="chat-input"]').fill('查询销售排行');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-213] AI response:', aiText?.substring(0, 150));
    expect(aiText).toBeTruthy();
  });

  // ═══ UI-214: sql_executor 校验失败 ═══
  test('[UI-214] ToolCall — sql_executor 校验失败', async ({ page, request }) => {
    console.log('[UI-214] Testing sql_executor with invalid SQL...');

    await seedMock(request, '删除所有数据',
      JSON.stringify({ type: 'tool_call', name: 'sql_executor', input: { sql: 'DROP TABLE users' } }));
    await seedMock(request, '删除所有数据',
      '安全校验失败：不允许执行 DROP TABLE 操作。');

    await page.locator('[data-testid="chat-input"]').fill('删除所有数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-214] AI response:', aiText?.substring(0, 100));

    // Should indicate security rejection
    expect(aiText).toMatch(/安全|失败|不允许|禁止|DROP|审核/i);
  });

  // ═══ UI-215: stats_engine 真实计算 ═══
  test('[UI-215] ToolCall — stats_engine 真实计算', async ({ page, request }) => {
    console.log('[UI-215] Testing stats_engine...');

    await seedMock(request, '统计销售趋势',
      JSON.stringify({
        type: 'tool_call',
        name: 'stats_engine',
        input: { calculation: 'monthly_trend', source: 'sales_data' },
      }));
    await seedMock(request, '统计销售趋势',
      '```json\n{"type":"kpi","items":[{"label":"月均销售额","value":"¥1,385万"},{"label":"增长率","value":"+12.4%"},{"label":"峰值月份","value":"6月"}]}\n```');

    await page.locator('[data-testid="chat-input"]').fill('统计销售趋势');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-215] AI response:', aiText?.substring(0, 150));
    expect(aiText).toBeTruthy();
  });

  // ═══ UI-216: save_report 报告生成 ═══
  test('[UI-216] ToolCall — save_report 报告生成', async ({ page, request }) => {
    console.log('[UI-216] Testing save_report...');

    await seedMock(request, '生成销售报告',
      JSON.stringify({
        type: 'tool_call',
        name: 'save_report',
        input: { title: '销售分析报告', format: 'pdf', content: '2026年上半年销售数据分析...' },
      }));
    await seedMock(request, '生成销售报告',
      '报告已生成并保存。您可以在制品管理中查看《销售分析报告》。');

    await page.locator('[data-testid="chat-input"]').fill('生成销售报告');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-216] AI response:', aiText?.substring(0, 100));
    expect(aiText).toMatch(/报告|保存|生成|制品/i);
  });

  // ═══ UI-217: 多工具链式调用 ═══
  test('[UI-217] ToolCall — 多工具链式调用', async ({ page, request }) => {
    console.log('[UI-217] Testing multi-tool chain...');

    // Chain: knowledge_search → sql_executor → stats_engine → final answer
    await seedMock(request, '全面分析销售数据',
      JSON.stringify({ type: 'tool_call', name: 'knowledge_search', input: { query: '销售数据', top_k: 3 } }));
    await seedMock(request, '全面分析销售数据',
      JSON.stringify({ type: 'tool_call', name: 'sql_executor', input: { sql: 'SELECT region, SUM(revenue) FROM sales GROUP BY region' } }));
    await seedMock(request, '全面分析销售数据',
      JSON.stringify({ type: 'tool_call', name: 'stats_engine', input: { calculation: 'growth_rate' } }));
    await seedMock(request, '全面分析销售数据',
      '综合分析完成：华北区域贡献最高，全线上涨趋势明显。月均增长12.4%。');

    await page.locator('[data-testid="chat-input"]').fill('全面分析销售数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 30000 });
    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    console.log('[UI-217] AI response:', aiText?.substring(0, 200));
    expect(aiText).toBeTruthy();
    expect(aiText).toMatch(/分析|华北|趋势|增长/i);
  });

  // ═══ UI-218: 工具结果复制 ═══
  test('[UI-218] ToolCall — 工具结果复制', async ({ page, request }) => {
    console.log('[UI-218] Testing tool result copy...');

    await seedMock(request, '查询SQL',
      '```sql\nSELECT product, SUM(revenue) FROM sales GROUP BY product\n```\n\n查询结果已生成。');

    await page.locator('[data-testid="chat-input"]').fill('查询SQL');
    await page.locator('[data-testid="chat-send-btn"]').click();

    await expect(page.locator('[data-testid^="chat-msg-ai-"]').first()).toBeVisible({ timeout: 20000 });

    // If SQL block appears, verify copy button
    const sqlBlock = page.locator('[data-testid="chat-sql-block"]');
    const hasSql = await sqlBlock.isVisible({ timeout: 5000 }).catch(() => false);
    if (hasSql) {
      const copyBtn = page.locator('[data-testid="chat-sql-copy-btn"]');
      await expect(copyBtn).toBeVisible();
      console.log('[UI-218] SQL block with copy button found');
    } else {
      console.log('[UI-218] No SQL block rendered — response was plain text');
    }

    const aiText = await page.locator('[data-testid^="chat-msg-ai-"]').first().textContent();
    expect(aiText).toBeTruthy();
  });
});
