/**
 * SPEC-072: Dashboard Real Data Integration E2E
 *
 * Verifies the dashboard shows real data (global hourly counters) instead of
 * zeros. Seeds KB docs via API, verifies KPI values and the granularity-switch
 * trend charts via the new /dashboard + /dashboard/trends API shape.
 */
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-dashi-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };

let adminToken = '';

test.describe('DASHBOARD INTEGRATION — SPEC-072', () => {
  const seededDocs: string[] = [];

  test.beforeAll(async ({ request }) => {
    expect((await request.post(`${API_BASE}/auth/register`, { data: U })).status()).toBe(201);
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: U.username, password: U.password },
    });
    adminToken = (await loginRes.json()).access_token;

    // Seed real KB docs so kb_docs is non-zero.
    console.log('[DASH-INT] Seeding dashboard data...');
    for (let i = 0; i < 3; i++) {
      const res = await request.post(`${API_BASE}/knowledge/docs`, {
        headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
        data: {
          title: `Dashboard Doc ${i + 1}`,
          file_name: `dash-doc-${i + 1}.md`,
          file_type: 'markdown',
          size_bytes: 1000,
        },
      });
      if (res.ok()) {
        const d = await res.json();
        seededDocs.push(d.id);
      }
    }
    console.log(`[DASH-INT] Docs: ${seededDocs.length}`);
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    for (const id of seededDocs) {
      await request.delete(`${API_BASE}/knowledge/docs/${id}`, { headers }).catch(() => {});
    }
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-dashi-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u: URL) => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-dashboard"]').click();
    await page.waitForTimeout(2000);
  });

  const KPI_TESTIDS = [
    'dashboard-stat-kb',
    'dashboard-stat-token',
    'dashboard-stat-llm',
    'dashboard-stat-api',
    'dashboard-stat-artifact',
    'dashboard-stat-task',
    'dashboard-stat-roi',
  ];
  const CHART_TESTIDS = ['chart-token', 'chart-llm', 'chart-api', 'chart-artifact', 'chart-task', 'chart-roi'];
  const SERIES_KEYS = ['token_tokens', 'llm_calls', 'api_calls', 'artifact_created', 'task_completed', 'roi'];

  // ═══ UI-229: 7 KPI 卡片渲染 ═══
  test('[UI-229] Dashboard — 7 个 KPI 卡片渲染', async ({ page }) => {
    for (const id of KPI_TESTIDS) {
      await expect(page.locator(`[data-testid="${id}"]`)).toBeVisible({ timeout: 5000 });
    }
  });

  // ═══ UI-230: KPI 显示真实文档数 ═══
  test('[UI-230] Dashboard — KPI 显示真实文档数', async ({ page, request }) => {
    await expect(page.locator('[data-testid="dashboard-stat-kb"]')).toBeVisible({ timeout: 5000 });

    const statsRes = await request.get(`${API_BASE}/dashboard`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(statsRes.ok()).toBeTruthy();
    const data = await statsRes.json();
    expect(data.kb_docs).toBeGreaterThanOrEqual(3);
  });

  // ═══ UI-231: summary 返回 7 指标 + ROI 派生 ═══
  test('[UI-231] Dashboard — summary 返回 7 指标 + ROI', async ({ request }) => {
    const res = await request.get(`${API_BASE}/dashboard`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(res.ok()).toBeTruthy();
    const data = await res.json();
    for (const key of ['kb_docs', 'token_tokens', 'llm_calls', 'api_calls', 'artifact_created', 'task_completed', 'roi']) {
      expect(data[key]).toBeDefined();
    }
    // ROI = (artifact + task) / token, token=0 → 0 (no NaN).
    expect(typeof data.roi).toBe('number');
    expect(Number.isNaN(data.roi)).toBeFalsy();
  });

  // ═══ UI-232: trends 返回 6 序列 + granularity ═══
  test('[UI-232] Dashboard — trends 返回 6 序列 + granularity', async ({ request }) => {
    for (const gran of ['day', 'week', 'month', 'year']) {
      const res = await request.get(`${API_BASE}/dashboard/trends?granularity=${gran}`, {
        headers: { Authorization: `Bearer ${adminToken}` },
      });
      expect(res.ok()).toBeTruthy();
      const data = await res.json();
      expect(data.granularity).toBe(gran);
      for (const key of SERIES_KEYS) {
        expect(Array.isArray(data[key])).toBeTruthy();
      }
    }
  });

  // ═══ UI-233: 6 张趋势图渲染 ═══
  test('[UI-233] Dashboard — 6 张趋势图渲染', async ({ page }) => {
    for (const id of CHART_TESTIDS) {
      await expect(page.locator(`[data-testid="${id}"]`)).toBeVisible({ timeout: 5000 });
    }
  });

  // ═══ UI-234: ROI 图表渲染 ═══
  test('[UI-234] Dashboard — ROI 图表渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="chart-roi"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="dashboard-stat-roi"]')).toBeVisible();
  });

  // ═══ UI-235: 全局统计（所有登录用户同份数据） ═══
  test('[UI-235] Dashboard — 全局统计（所有用户可见）', async ({ request }) => {
    const uidB = crypto.randomUUID().slice(0, 8);
    const userB = { username: `e2e-dashb-${uidB}@test.local`, password: 'E2eTest123!', role: 'user' };
    await request.post(`${API_BASE}/auth/register`, { data: userB }).catch(() => {});
    const loginB = await request.post(`${API_BASE}/auth/login`, {
      data: { username: userB.username, password: userB.password },
    });
    const tokenB = (await loginB.json()).access_token;

    // A non-admin user can access the global dashboard (SPEC-072: all
    // logged-in users see the same global stats).
    const statsB = await request.get(`${API_BASE}/dashboard`, {
      headers: { Authorization: `Bearer ${tokenB}` },
    });
    expect(statsB.ok()).toBeTruthy();
    const dataB = await statsB.json();
    expect(dataB.kb_docs).toBeDefined();

    // Cleanup
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers: { Authorization: `Bearer ${adminToken}` } });
    if (listRes.ok()) {
      for (const u of (await listRes.json()).users || []) {
        if (u.username?.includes(`e2e-dashb-${uidB}`)) {
          await request.delete(`${API_BASE}/users/${u.id}`, { headers: { Authorization: `Bearer ${adminToken}` } });
        }
      }
    }
  });

  // ═══ UI-236: 粒度切换器有效 ═══
  test('[UI-236] Dashboard — 粒度切换器有效', async ({ page }) => {
    await expect(page.locator('[data-testid="dashboard-time-filter"]')).toBeVisible({ timeout: 5000 });
    for (const g of ['day', 'week', 'month', 'year']) {
      await page.locator(`[data-testid="filter-${g}"]`).click();
      await page.waitForTimeout(800);
      // Charts stay rendered after switching granularity.
      await expect(page.locator('[data-testid="chart-token"]')).toBeVisible({ timeout: 5000 });
    }
  });

  // ═══ UI-237: 全 trend 真数据 + API 结构 ═══
  test('[UI-237] Dashboard — 全 trend 真数据 + API 结构', async ({ page, request }) => {
    // Trigger an enhance call to write a real token/llm counter (best-effort;
    // the chart still renders with zeroed buckets if this fails).
    try {
      await request.post(`${API_BASE}/chat/enhance`, {
        headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
        data: { prompt: '汇总销售数据' },
        timeout: 8000,
      });
    } catch (e) {
      console.log('[UI-237] enhance call failed (non-blocking):', e);
    }

    await page.goto('/');
    await page.waitForTimeout(2000);

    for (const id of CHART_TESTIDS) {
      await expect(page.locator(`[data-testid="${id}"]`)).toBeVisible({ timeout: 5000 });
    }

    // API: /dashboard returns all 7 KPI fields; kb_docs reflects the seeded docs.
    const statsRes = await request.get(`${API_BASE}/dashboard`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(statsRes.ok()).toBeTruthy();
    const stats = await statsRes.json();
    expect(stats.kb_docs).toBeGreaterThanOrEqual(3);

    // API: /dashboard/trends returns all 6 series as arrays.
    const trendsRes = await request.get(`${API_BASE}/dashboard/trends?granularity=day`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(trendsRes.ok()).toBeTruthy();
    const trends = await trendsRes.json();
    for (const key of SERIES_KEYS) {
      expect(Array.isArray(trends[key])).toBeTruthy();
    }
    expect(trends.granularity).toBe('day');
  });
});
