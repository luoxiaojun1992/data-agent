/**
 * SPEC-080: 时间 Skill + 规划 Skill + Plan 意图隐藏引导 E2E
 *
 * 覆盖 spec §8 验收规则中可在 E2E 环境稳定验证的部分：
 * - UI-250: get_current_time / get_plan_method 两个 skill 已通过三步注册
 *           （tools.go + predefinedSkills + seed），admin skill 列表可见且 enabled
 * - UI-251: [plan_hint] 等 hidden 内部提示事件不出现在前端聊天记录
 *           （后端透传 hidden 字段，前端渲染时过滤）
 *
 * 说明：真实 ADK 工具调用（get_current_time 被 LLM 触发）依赖意图识别走
 * 真实模型；E2E 环境意图分类走 mockllm 会降级为 chat，故不在此断言工具
 * 的真实执行链路（该链路由 Go 单测 TestCurrentTime/TestGetPlanMethod 覆盖）。
 */
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const ADMIN = { username: `e2e-tps-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';
let sessionId = '';

test.describe('TIME/PLAN SKILLS — SPEC-080', () => {
  test.beforeAll(async ({ request }) => {
    let res = await request.post(`${API_BASE}/auth/register`, { data: ADMIN });
    if (res.status() !== 201) {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    } else {
      res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    }
    adminToken = (await res.json()).access_token;

    // 创建一个 chat session，供 UI-251 hidden 过滤测试加载历史用。
    const sRes = await request.post(`${API_BASE}/sessions?type=chat`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    if (sRes.ok()) sessionId = (await sRes.json()).session_id;
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const u of (await listRes.json()).users || []) {
        if (u.username?.includes(`e2e-tps-${uid}`) && u.role !== 'system_admin') {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-250: 工具三步注册验证 ═══
  test('[UI-250] TimePlan — get_current_time/get_plan_method 已注册', async ({ request }) => {
    const res = await request.get(`${API_BASE}/admin/skills`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(res.status()).toBe(200);
    const body = await res.json();
    const byName: Record<string, { name: string; display_name: string; enabled: boolean }> = {};
    for (const s of (body.skills || [])) byName[s.name] = s;

    expect(byName['get_current_time'], 'get_current_time should be registered').toBeTruthy();
    expect(byName['get_current_time'].enabled).toBe(true);
    expect(byName['get_current_time'].display_name).toContain('当前时间');

    expect(byName['get_plan_method'], 'get_plan_method should be registered').toBeTruthy();
    expect(byName['get_plan_method'].enabled).toBe(true);
    expect(byName['get_plan_method'].display_name).toContain('规划');
  });

  // ═══ UI-251: hidden 消息不渲染 ═══
  test('[UI-251] TimePlan — [plan_hint] 隐藏消息不出现在聊天记录', async ({ page }) => {
    expect(sessionId, 'session should be created in beforeAll').toBeTruthy();

    // mock 历史 messages API：一条用户消息 + 一条 hidden 内部提示。
    await page.route('**/sessions/*/messages', (route) => {
      return route.fulfill({
        status: 200,
        contentType: 'application/json',
        body: JSON.stringify({
          messages: [
            { event_id: 'e1', role: 'user', type: 'text', content: '帮我制定学习计划', timestamp: new Date().toISOString() },
            { event_id: 'e2', role: 'system', type: 'text', content: '[plan_hint] 检测到本次任务需要制定执行计划。', timestamp: new Date().toISOString(), hidden: true },
          ],
        }),
      });
    });

    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u) => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-chat"]').click();
    await page.waitForURL('**/chat', { timeout: 5000 });

    // 打开会话列表，点击 session 触发历史加载（命中 mock）。
    await page.locator('[data-testid="chat-session-btn"]').click();
    await page.locator(`[data-testid="session-item-${sessionId}"]`).click();

    // 用户消息正常渲染。
    await expect(page.locator('[data-testid^="chat-msg-user-"]').first()).toContainText('帮我制定学习计划');

    // hidden 消息不渲染：无 [plan_hint] 文本、无 system 胶囊。
    await expect(page.locator('body')).not.toContainText('[plan_hint]');
    await expect(page.locator('[data-testid^="chat-msg-system-"]')).toHaveCount(0);
  });
});
