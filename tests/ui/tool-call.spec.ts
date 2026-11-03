/**
 * SPEC-046: Tool Call Chain E2E (UI-211 ~ UI-218)
 *
 * 真实工具调用测试（区别于旧的「文本解析渲染」）：
 * - 旧实现注入 ```json 代码块包裹的伪 tool_call，mockllm 走「普通文本」分支，
 *   前端 parseBlocks 解析成 chat-tool-call-card-*（纯前端展示，后端工具从未执行）。
 * - 本实现注入「纯 JSON tool_call」（{"type":"tool_call","name":"x","input":{...}}），
 *   mockllm tryAsFunctionCalls 识别后转成 OpenAI function_call（finish_reason=tool_calls），
 *   ADK ReAct 真正执行工具 → 后端下发 tool_call/tool_result 事件 → 前端渲染
 *   chat-live-tool-call-* / chat-live-tool-result-*。
 *
 * 注入机制（mockllm key=sha256(last_msg)，LPush/LPop 即 LIFO）：
 * - guard.CheckIntent 先于正式 run、last_msg 相同 → 会 LPop 抢占注入的响应。
 *   seedMock 注入两份：先 tool_call JSON（给正式 run），后 intent JSON
 *   （{"is_task":false,"is_plan":false}，先被 intent 检测 LPop 弹出）。
 * - ReAct 第二轮（last_msg 变为 tool_result）无精确 key → 走 mockllm default reply，
 *   不影响 tool_call 卡片断言。
 *
 * 工具选型：dir_create/file_delete/dir_delete（文件操作，覆盖 golangci-lint 的
 * gosimple S1016 三处）、stats_compute/get_current_time/memory_list（无外部依赖）。
 * sql_executor 需 MySQL、save_report 有副作用，CI 无此依赖，不在此真实调用。
 */
import { registerUser } from './utils/auth';
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-tc-${uid}@test.local`, password: 'E2eTest1!' };

async function seedMock(request: any, key: string, response: string) {
  // intent 检测（guard.CheckIntent，SPEC-080）先于正式 run，用相同 last_msg 调 LLM，
  // 会 LPop 抢占注入的响应。按 LIFO 顺序注入两份：先注入正式响应（给正式 run），
  // 再注入 intent JSON（给 intent 检测，先被 LPop 弹出）。
  const headers = { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` };
  await request.post(`${MOCKLLM}/responses`, { headers, data: { key, response } });
  await request.post(`${MOCKLLM}/responses`, { headers, data: { key, response: '{"is_task":false,"is_plan":false}' } });
}

// seedToolCall 注入「纯 JSON tool_call」（不含 ```json 代码块），mockllm 的
// tryAsFunctionCalls 会识别为 OpenAI function_call（finish_reason=tool_calls），
// 从而触发 ADK ReAct 真正执行工具（而非前端 parseBlocks 的文本解析渲染）。
async function seedToolCall(request: any, key: string, name: string, input: any) {
  await seedMock(request, key, JSON.stringify({ type: 'tool_call', name, input }));
}

async function clearMocks(request: any) {
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});
}

test.describe('TOOL CALL — SPEC-046', () => {
  test.beforeAll(async ({ request }) => {
    expect((await registerUser(request, U)).status()).toBe(201);
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

  // ═══ UI-211: dir_create 真实调用（覆盖 gosimple S1016 #1）═══
  test('[UI-211] ToolCall — dir_create 真实调用', async ({ page, request }) => {
    await seedToolCall(request, '创建一个测试目录', 'dir_create', { path: 'e2e-dir' });
    await page.locator('[data-testid="chat-input"]').fill('创建一个测试目录');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const card = page.locator('[data-testid^="chat-live-tool-call-"]').first();
    await expect(card).toBeVisible({ timeout: 20000 });
    await expect(card).toContainText('dir_create');
  });

  // ═══ UI-212: file_delete 真实调用（覆盖 gosimple S1016 #2）═══
  test('[UI-212] ToolCall — file_delete 真实调用', async ({ page, request }) => {
    await seedToolCall(request, '删除一个测试文件', 'file_delete', { path: 'nonexistent.txt' });
    await page.locator('[data-testid="chat-input"]').fill('删除一个测试文件');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const card = page.locator('[data-testid^="chat-live-tool-call-"]').first();
    await expect(card).toBeVisible({ timeout: 20000 });
    await expect(card).toContainText('file_delete');
  });

  // ═══ UI-213: dir_delete 真实调用（覆盖 gosimple S1016 #3）═══
  test('[UI-213] ToolCall — dir_delete 真实调用', async ({ page, request }) => {
    await seedToolCall(request, '删除一个测试目录', 'dir_delete', { path: 'nonexistent-dir' });
    await page.locator('[data-testid="chat-input"]').fill('删除一个测试目录');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const card = page.locator('[data-testid^="chat-live-tool-call-"]').first();
    await expect(card).toBeVisible({ timeout: 20000 });
    await expect(card).toContainText('dir_delete');
  });

  // ═══ UI-214: stats_compute 真实调用（修旧 stats_engine 工具名错误）═══
  test('[UI-214] ToolCall — stats_compute 真实调用', async ({ page, request }) => {
    await seedToolCall(request, '统计一组数据', 'stats_compute', { method: 'descriptive', values: [1, 2, 3, 4, 5] });
    await page.locator('[data-testid="chat-input"]').fill('统计一组数据');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const card = page.locator('[data-testid^="chat-live-tool-call-"]').first();
    await expect(card).toBeVisible({ timeout: 20000 });
    await expect(card).toContainText('stats_compute');
  });

  // ═══ UI-215: get_current_time 真实调用（无参数工具）═══
  test('[UI-215] ToolCall — get_current_time 真实调用', async ({ page, request }) => {
    await seedToolCall(request, '现在几点了', 'get_current_time', {});
    await page.locator('[data-testid="chat-input"]').fill('现在几点了');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const card = page.locator('[data-testid^="chat-live-tool-call-"]').first();
    await expect(card).toBeVisible({ timeout: 20000 });
    await expect(card).toContainText('get_current_time');
  });

  // ═══ UI-216: memory_list 真实调用（无外部依赖）═══
  test('[UI-216] ToolCall — memory_list 真实调用', async ({ page, request }) => {
    await seedToolCall(request, '列出我的记忆', 'memory_list', { limit: 5 });
    await page.locator('[data-testid="chat-input"]').fill('列出我的记忆');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const card = page.locator('[data-testid^="chat-live-tool-call-"]').first();
    await expect(card).toBeVisible({ timeout: 20000 });
    await expect(card).toContainText('memory_list');
  });

  // ═══ UI-217: 多工具链式真实调用（multi tool_calls 格式）═══
  test('[UI-217] ToolCall — 多工具链式真实调用', async ({ page, request }) => {
    // mockllm 多调用格式：{"type":"tool_calls","calls":[...]}
    const multi = JSON.stringify({
      type: 'tool_calls',
      calls: [
        { name: 'dir_create', input: { path: 'e2e-multi' } },
        { name: 'stats_compute', input: { method: 'descriptive', values: [10, 20, 30] } },
      ],
    });
    await seedMock(request, '批量操作', multi);
    await page.locator('[data-testid="chat-input"]').fill('批量操作');
    await page.locator('[data-testid="chat-send-btn"]').click();

    const calls = page.locator('[data-testid^="chat-live-tool-call-"]');
    await expect(calls.first()).toBeVisible({ timeout: 20000 });
    // 两个工具调用均被真实执行并渲染（第二个工具也出现）
    await expect(calls.nth(1)).toBeVisible({ timeout: 20000 });
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
