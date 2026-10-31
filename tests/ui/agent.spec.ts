import { registerUser } from './utils/auth';
import { test, expect } from '@playwright/test';

/**
 * SPEC-020: AGENT E2E Tests (UI-039 ~ UI-056)
 * Only deterministic assertions.
 *
 * 前端已重构（SPEC-082/086）：
 *  - 任务列表不再有状态筛选（筛选移到 /agent/tasks/:id 运行列表页 runs-status-filter）
 *  - 任务详情从「行内展开」改为独立路由 /agent/tasks/:id（运行列表）与 /agent/runs/:id（运行详情）
 *  - 「异步任务」改为「定时任务」（cron 开关 agent-task-cron-toggle）
 *  - 「取消任务」改为「删除任务」；「取消运行」位于运行列表页 run-cancel-*
 * 本 spec 只覆盖可确定性断言的 UI 状态与导航，不依赖后端异步 run 时序。
 */

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);
const U = { username: `e2e-agt-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };

test.describe('AGENT — Professional Workspace', () => {
  test.beforeAll(async ({ request }) => {
    expect((await registerUser(request, U)).status()).toBe(201);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(U.username);
    await page.locator('[data-testid="login-password-input"]').fill(U.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((u: URL) => !u.pathname.includes('/login'), { timeout: 10000 });
    await page.locator('[data-testid="nav-agent"]').click();
    await page.waitForURL('**/agent', { timeout: 5000 });
  });

  // ═══ UI-039: Page header + empty state ═══
  test('[UI-039] Agent page header and empty state', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="agent-page-header"] h2')).toHaveText('Agent 任务');
    await expect(page.locator('[data-testid="agent-empty"]')).toBeVisible({ timeout: 5000 });
  });

  // ═══ UI-040: Create task modal opens ═══
  test('[UI-040] Agent — create task modal', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await expect(page.locator('[data-testid="agent-task-modal"]')).toBeVisible({ timeout: 5000 });
    await expect(page.locator('[data-testid="agent-task-title-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-task-create-btn"]')).toBeDisabled();
  });

  // ═══ UI-041: Create task — modal closes ═══
  test('[UI-041] Agent — create task', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('E2E 同步分析');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible();
  });

  // ═══ UI-042: 定时任务开关交互（异步 → 定时 cron） ═══
  test('[UI-042] Agent — 定时任务开关交互', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('E2E 定时分析');
    // 默认未开启定时 → 无调度配置区
    await expect(page.locator('[data-testid="agent-task-cron-config"]')).toHaveCount(0);
    // 开启定时开关 → 调度配置区出现
    await page.locator('[data-testid="agent-task-cron-toggle"]').check();
    await expect(page.locator('[data-testid="agent-task-cron-config"]')).toBeVisible();
    // 关闭开关 → 配置区消失
    await page.locator('[data-testid="agent-task-cron-toggle"]').uncheck();
    await expect(page.locator('[data-testid="agent-task-cron-config"]')).toHaveCount(0);
  });

  // ═══ UI-043: 常用模版菜单（SPEC-086，替代已移除的列表筛选） ═══
  test('[UI-043] Agent — 常用模版菜单', async ({ page }) => {
    await page.locator('[data-testid="agent-template-btn"]').click();
    await expect(page.locator('[data-testid="agent-template-menu"]')).toBeVisible();
    await page.locator('[data-testid="agent-template-daily-summary"]').click();
    await expect(page.locator('[data-testid="agent-template-confirm"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-template-daily-summary-title"]')).toBeVisible();
  });

  // ═══ UI-044: 任务状态 pill 渲染 ═══
  test('[UI-044] Agent — 任务状态 pill 渲染', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Status Pill Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    const row = page.locator('[data-testid^="agent-task-row-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    // 新任务渲染状态 pill（task-status-<status>）
    await expect(row.locator('[data-testid^="task-status-"]')).toBeVisible();
  });

  // ═══ UI-045: 点击任务 → 导航到运行列表 ═══
  test('[UI-045] Agent — 点击任务进入运行列表', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Detail Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    const row = page.locator('[data-testid^="agent-task-title-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    await row.click();
    await page.waitForURL(/\/agent\/tasks\//, { timeout: 10000 });
    await expect(page.locator('[data-testid="runs-status-filter"]')).toBeVisible();
  });

  // ═══ UI-046: 页面操作按钮渲染 ═══
  test('[UI-046] Agent — 页面操作按钮渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 10000 });
    await expect(page.locator('[data-testid="agent-create-task-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="agent-template-btn"]')).toBeVisible();
  });

  // ═══ UI-047: 运行列表页渲染（触发运行 + 状态筛选） ═══
  test('[UI-047] Agent — 运行列表页渲染', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Runs Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    await page.locator('[data-testid^="agent-task-title-"]').first().click();
    await page.waitForURL(/\/agent\/tasks\//, { timeout: 10000 });
    // 返回 + 触发运行 + 状态筛选
    await expect(page.locator('[data-testid="runs-back-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="runs-trigger-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="runs-status-filter"]')).toBeVisible();
    await expect(page.locator('[data-testid="runs-filter-all"]')).toBeVisible();
    await expect(page.locator('[data-testid="runs-filter-running"]')).toBeVisible();
    // 新任务无 run → 空态
    await expect(page.locator('[data-testid="runs-empty"]')).toBeVisible();
  });

  // ═══ UI-052: 删除任务流程（原「取消任务」已改为「删除任务」） ═══
  test('[UI-052] Agent — 删除任务', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('To Delete');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    const row = page.locator('[data-testid^="agent-task-row-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    await expect(page.getByText('To Delete', { exact: true })).toBeVisible();
    // 点击删除并接受 confirm 对话框
    page.once('dialog', (d) => d.accept());
    await row.locator('[data-testid^="agent-task-delete-"]').click();
    // 删除后该任务标题消失
    await expect(page.getByText('To Delete', { exact: true })).toHaveCount(0, { timeout: 10000 });
  });

  // ═══ UI-053: 触发运行 → run 记录出现（原「取消运行」改为触发运行，取消依赖时序不测） ═══
  test('[UI-053] Agent — 触发运行', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Run Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    await page.locator('[data-testid^="agent-task-title-"]').first().click();
    await page.waitForURL(/\/agent\/tasks\//, { timeout: 10000 });
    // 触发运行（mock-llm 默认响应）
    await page.locator('[data-testid="runs-trigger-btn"]').click();
    // run 记录出现（pending/running/completed/failed 任一状态）
    await expect(page.locator('[data-testid^="runs-row-"]').first()).toBeVisible({ timeout: 20000 });
  });

  // ═══ UI-055: 任务行元信息渲染（替代已移除的列表筛选断言） ═══
  test('[UI-055] Agent — 任务行元信息', async ({ page }) => {
    await page.locator('[data-testid="agent-create-task-btn"]').click();
    await page.locator('[data-testid="agent-task-title-input"]').fill('Management Test');
    await page.locator('[data-testid="agent-task-create-btn"]').click();
    await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    const row = page.locator('[data-testid^="agent-task-row-"]').first();
    await expect(row).toBeVisible({ timeout: 10000 });
    // 运行次数元信息
    await expect(row.locator('[data-testid^="agent-task-run-count-"]')).toBeVisible();
    // 删除按钮
    await expect(row.locator('[data-testid^="agent-task-delete-"]')).toBeVisible();
  });

  // ═══ UI-056: 批量创建任务 → 列表渲染 ═══
  test('[UI-056] Agent — 批量创建任务列表渲染', async ({ page }) => {
    for (let i = 0; i < 6; i++) {
      await page.locator('[data-testid="agent-create-task-btn"]').click();
      await page.locator('[data-testid="agent-task-title-input"]').fill(`Batch ${i + 1}`);
      await page.locator('[data-testid="agent-task-create-btn"]').click();
      await page.locator('[data-testid="agent-task-modal"]').waitFor({ state: 'hidden', timeout: 10000 });
    }
    const rows = page.locator('[data-testid^="agent-task-title-"]');
    const count = await rows.count();
    expect(count).toBeGreaterThanOrEqual(1);
    await expect(page.locator('[data-testid="agent-page-header"]')).toBeVisible({ timeout: 5000 });
  });
});
