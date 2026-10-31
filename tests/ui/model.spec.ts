import { registerUser } from './utils/auth';
import { test, expect } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-model-admin-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
const REGULAR = { username: `e2e-model-user-${uid}@test.local`, password: 'E2eTest123!' };
let adminToken = '';

/**
 * SPEC-025: MODEL CONFIG E2E（UI-093 ~ UI-103）
 * 前端模型页已重构：单一「模型配置」表单 → 「列表 + 新增/编辑 modal」。
 * 本 spec 对齐真实 testid（model-add-btn / model-edit-* / hermes-*）。
 * SPEC-084：密钥 mask 由前端视觉层做（输入框 type 切换），后端无 /vault/decrypt、
 * 无 api_key_exists，故不做 Vault 解密断言。
 */

test.describe('MODEL CONFIG — SPEC-025', () => {
  test.beforeAll(async ({ request }) => {
    await registerUser(request, ADMIN);
    const res = await request.post(`${API_BASE}/auth/login`, { data: { username: ADMIN.username, password: ADMIN.password } });
    const body = await res.json();
    adminToken = body.access_token;

    // 注册普通用户用于 UI-103
    await registerUser(request, REGULAR);
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/admin/models');
    await page.waitForSelector('[data-testid="model-add-btn"]', { timeout: 5000 });
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      const body = await listRes.json();
      for (const user of body.users || []) {
        if (user.username?.includes(`e2e-model-${uid}`) && user.role !== 'system_admin') {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  // ═══ UI-093: 模型配置页渲染 ═══
  test('[UI-093] Model — 模型配置页渲染', async ({ page }) => {
    await expect(page.locator('[data-testid="model-add-btn"]')).toBeVisible();
    await expect(page.locator('[data-testid="model-search"]')).toBeVisible();
    await expect(page.locator('[data-testid="model-list-card"]')).toBeVisible();
    await expect(page.locator('[data-testid="embedding-list-card"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-config-card"]')).toBeVisible();
  });

  // ═══ UI-094: 新增模型 Base URL 配置 ═══
  test('[UI-094] Model — 新增模型 Base URL 配置', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    await expect(page.locator('[data-testid="model-edit-modal"]')).toBeVisible();
    const input = page.locator('[data-testid="model-edit-base-url"]');
    await expect(input).toBeVisible();
    // 默认值为 OpenAI 兼容 URL
    const value = await input.inputValue();
    expect(value).toBeTruthy();
    await input.fill('https://custom-api.example.com/v1');
    await expect(input).toHaveValue('https://custom-api.example.com/v1');
  });

  // ═══ UI-095: API Key 输入与掩码（SPEC-084 前端视觉掩码） ═══
  test('[UI-095] Model — API Key 输入与掩码', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    const keyInput = page.locator('[data-testid="model-edit-api-key"]');
    await expect(keyInput).toBeVisible();
    await expect(keyInput).toHaveAttribute('type', 'password');
    // 新模型无已存 key → 眼睛切换禁用
    await expect(page.locator('[data-testid="model-edit-api-key-eye"]')).toBeDisabled();
  });

  // ═══ UI-096: 新增模型 Name 输入 ═══
  test('[UI-096] Model — 新增模型 Name 输入', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    const input = page.locator('[data-testid="model-edit-name"]');
    await expect(input).toBeVisible();
    await input.fill('custom-model-name');
    await expect(input).toHaveValue('custom-model-name');
  });

  // ═══ UI-097: 模型类型切换（LLM / Embedding） ═══
  test('[UI-097] Model — 模型类型切换', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    const llmBtn = page.locator('[data-testid="model-edit-type-llm"]');
    const embBtn = page.locator('[data-testid="model-edit-type-embedding"]');
    await expect(llmBtn).toBeVisible();
    await expect(embBtn).toBeVisible();
    // 默认 LLM：显示 instruction/context 等，无 embedding_dim
    await expect(page.locator('[data-testid="model-edit-instruction"]')).toBeVisible();
    await expect(page.locator('[data-testid="model-edit-embedding-dim"]')).toHaveCount(0);
    // 切换到 Embedding：显示 embedding_dim，隐藏 instruction
    await embBtn.click();
    await expect(page.locator('[data-testid="model-edit-embedding-dim"]')).toBeVisible();
    await expect(page.locator('[data-testid="model-edit-instruction"]')).toHaveCount(0);
  });

  // ═══ UI-098: 上下文长度配置 ═══
  test('[UI-098] Model — 上下文长度配置', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    const input = page.locator('[data-testid="model-edit-context-len"]');
    await expect(input).toBeVisible();
    const value = await input.inputValue();
    expect(value).toBeTruthy(); // 默认 128000
    await input.fill('256000');
    await expect(input).toHaveValue('256000');
  });

  // ═══ UI-099: 最大输出长度配置 ═══
  test('[UI-099] Model — 最大输出长度配置', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    const input = page.locator('[data-testid="model-edit-max-tokens"]');
    await expect(input).toBeVisible();
    const value = await input.inputValue();
    expect(value).toBeTruthy();
    await input.fill('32000');
    await expect(input).toHaveValue('32000');
  });

  // ═══ UI-100: Temperature 配置 ═══
  test('[UI-100] Model — Temperature 配置', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    const input = page.locator('[data-testid="model-edit-temperature"]');
    await expect(input).toBeVisible();
    const value = parseFloat(await input.inputValue());
    expect(value).toBeGreaterThanOrEqual(0);
    expect(value).toBeLessThanOrEqual(2);
    await input.fill('1.0');
    await expect(input).toHaveValue('1.0');
  });

  // ═══ UI-101: 系统指令配置（原 Top-P 已从前端移除） ═══
  test('[UI-101] Model — 系统指令配置', async ({ page }) => {
    await page.locator('[data-testid="model-add-btn"]').click();
    const input = page.locator('[data-testid="model-edit-instruction"]');
    await expect(input).toBeVisible();
    await input.fill('You are a helpful assistant');
    await expect(input).toHaveValue('You are a helpful assistant');
  });

  // ═══ UI-102: Hermes 配置区域 ═══
  test('[UI-102] Model — Hermes 配置区域', async ({ page }) => {
    const card = page.locator('[data-testid="hermes-config-card"]');
    await expect(card).toBeVisible();
    await expect(card).toContainText('Hermes');
    await expect(page.locator('[data-testid="hermes-url-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-api-key-input"]')).toBeVisible();
    await expect(page.locator('[data-testid="hermes-save-btn"]')).toBeVisible();
  });

  // ═══ UI-103: 仅 admin 可访问（普通用户不可见 admin 导航） ═══
  test('[UI-103] Model — 普通用户不可访问模型配置', async ({ page }) => {
    // 登出后以普通用户登录
    await page.locator('[data-testid="nav-logout-btn"]').click();
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(REGULAR.username);
    await page.locator('[data-testid="login-password-input"]').fill(REGULAR.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    // 普通用户（rbac_role_user）不可见 admin 导航（模型配置归属 admin 分组）
    await expect(page.locator('[data-testid="nav-admin"]')).not.toBeVisible();
  });
});
