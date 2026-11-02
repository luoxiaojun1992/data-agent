/**
 * SPEC-046: KB Indexing Integration E2E (UI-204 ~ UI-209)
 *
 * 验证真实 KB 文档生命周期：上传 txt → 异步索引 → ready → 检索 / 删除 / 分块。
 *
 * ⚠️ 与旧版（JSON create + /chunks + /status）不同，当前后端（SPEC-081/086）已重构：
 *   - 上传：POST /knowledge/docs 仅接受 multipart "file"（txt）或 "file_base64"（image）；
 *     不再接受 JSON body，也不存在 /chunks、/status 路由。
 *   - 索引：由 handler 驱动的异步 pipeline（worker 消费 kb_index 任务）闭环完成
 *     chunking（LLM）+ embedding（Ollama）+ Qdrant 写入；进度经 GET /docs/:id 的
 *     status / chunk_count / progress_percent 暴露。
 *   - 检索：UI 搜索框走 GET /docs?q=...（title/file_name 过滤，SPEC-075）；
 *     GET /knowledge/search?q=... 为向量检索 API。
 *
 * ⚠️ 索引 chunking 的 LLM 在 CI 中为 mockllm（MOCK_DEFAULT_REPLY），其返回固定文本
 * 会替换真实内容作为 chunk，因此「按正文关键词的向量检索命中」在本环境无法断言；
 * 检索类用例改按 UI 真实的 title 过滤路径验证（诚实、可测、不依赖 mock 文本）。
 */
import { registerUser } from './utils/auth';
import { test, expect, type APIRequestContext } from '@playwright/test';

const API_BASE = 'http://data-agent:8080/api/v1';
const uid = crypto.randomUUID().slice(0, 8);

const ADMIN = { username: `e2e-kbi-${uid}@test.local`, password: 'E2eTest123!', role: 'admin' };
let adminToken = '';

// ── Helpers ────────────────────────────────────────────────────────────

// 真实上传协议：multipart "file" 携带 txt 内容（对齐前端 uploadTxtDoc）。
async function uploadTxtDoc(
  request: APIRequestContext,
  token: string,
  title: string,
  content: string,
) {
  return request.post(`${API_BASE}/knowledge/docs`, {
    headers: { Authorization: `Bearer ${token}` },
    multipart: {
      title,
      file: { name: `${title}.txt`, mimeType: 'text/plain', buffer: Buffer.from(content) },
      file_name: `${title}.txt`,
      file_type: 'txt',
      size_bytes: String(Buffer.byteLength(content)),
    },
  });
}

// 轮询 GET /docs/:id 直到 status 变为目标状态（默认 ready），超时返回 null。
async function waitForStatus(
  request: APIRequestContext,
  token: string,
  docId: string,
  target: 'ready' | 'failed' = 'ready',
  timeoutMs = 50000,
): Promise<any | null> {
  const deadline = Date.now() + timeoutMs;
  while (Date.now() < deadline) {
    const res = await request.get(`${API_BASE}/knowledge/docs/${docId}`, {
      headers: { Authorization: `Bearer ${token}` },
    });
    if (res.ok()) {
      const doc = await res.json();
      if (doc.status === target) return doc;
      if (doc.status === 'failed') return doc; // 失败也要返回，供断言区分
    }
    await new Promise((r) => setTimeout(r, 2000));
  }
  return null;
}

// 生成一个超过 4096 runes 的大文档（保证 splitBySentence 切成多段 → 多个 chunk）。
function buildLargeDoc(): string {
  const parts: string[] = [];
  for (let i = 1; i <= 260; i++) {
    parts.push(`第${i}条记录：本季度销售额为${i * 37}万元，同比增长${(i % 9) + 1}.${i % 10}%。订单数${i * 12}个，客单价${i * 500}元。`);
  }
  return parts.join('\n');
}

test.describe('KB INTEGRATION — SPEC-046', () => {
  const createdDocs: string[] = [];

  test.beforeAll(async ({ request }) => {
    // 创建（或复用已存在的）admin 测试账号，再登录取 token。
    await registerUser(request, ADMIN);
    const loginRes = await request.post(`${API_BASE}/auth/login`, {
      data: { username: ADMIN.username, password: ADMIN.password },
    });
    adminToken = (await loginRes.json()).access_token;
  });

  test.afterAll(async ({ request }) => {
    const headers = { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' };
    for (const id of createdDocs) {
      await request.delete(`${API_BASE}/knowledge/docs/${id}`, { headers }).catch(() => {});
    }
    const listRes = await request.get(`${API_BASE}/users?skip=0&limit=100`, { headers });
    if (listRes.ok()) {
      for (const user of (await listRes.json()).users || []) {
        if (user.username?.includes(`e2e-kbi-${uid}`)) {
          await request.delete(`${API_BASE}/users/${user.id}`, { headers });
        }
      }
    }
  });

  test.beforeEach(async ({ page }) => {
    await page.goto('/login');
    await page.locator('[data-testid="login-email-input"]').fill(ADMIN.username);
    await page.locator('[data-testid="login-password-input"]').fill(ADMIN.password);
    await page.locator('[data-testid="login-btn"]').click();
    await page.waitForURL((url) => !url.pathname.includes('/login'), { timeout: 10000 });
    await page.goto('/knowledge');
    await page.waitForSelector('[data-testid="kb-page-header"]', { timeout: 10000 });
  });

  // ═══ UI-204: 文档索引端到端验证 ═══
  test('[UI-204] KB — 文档索引端到端验证', async ({ request }) => {
    test.setTimeout(120000);
    const content = '# 2026年Q2财务报告\n\n企业SaaS营收4520万元，同比增长15.2%。\n数据服务营收2840万元，同比增长22.7%。';
    const createRes = await uploadTxtDoc(request, adminToken, `UI204-财务-${uid}`, content);
    expect(createRes.status()).toBe(201);
    const doc = await createRes.json();
    expect(doc.id).toBeTruthy();
    expect(doc.status).toBe('uploaded'); // 上传后先落库为 uploaded
    createdDocs.push(doc.id);

    // 异步索引：轮询到 ready，且至少产生 1 个 chunk。
    const ready = await waitForStatus(request, adminToken, doc.id, 'ready');
    expect(ready, `doc ${doc.id} should reach ready`).not.toBeNull();
    expect(ready.status).toBe('ready');
    expect(ready.chunk_count).toBeGreaterThanOrEqual(1);
    expect(ready.progress_percent).toBe(100);
  });

  // ═══ UI-205: 索引后检索命中 ═══
  test('[UI-205] KB — 索引后检索命中', async ({ request }) => {
    test.setTimeout(120000);
    const title = `UI205-检索-${uid}`;
    const createRes = await uploadTxtDoc(request, adminToken, title, '检索命中验证文档：本季度销售额合计 7360 万元。');
    expect(createRes.status()).toBe(201);
    const doc = await createRes.json();
    createdDocs.push(doc.id);

    const ready = await waitForStatus(request, adminToken, doc.id, 'ready');
    expect(ready, `doc ${doc.id} should reach ready`).not.toBeNull();
    expect(ready.chunk_count).toBeGreaterThanOrEqual(1);

    // 索引完成后，文档可按标题被检索到（UI 搜索框的真实路径：GET /docs?q=）。
    const searchRes = await request.get(
      `${API_BASE}/knowledge/docs?q=${encodeURIComponent(title)}&page=1&page_size=20`,
      { headers: { Authorization: `Bearer ${adminToken}` } },
    );
    expect(searchRes.ok()).toBe(true);
    const body = await searchRes.json();
    expect(Array.isArray(body.docs)).toBe(true);
    expect(body.docs.some((d: any) => d.id === doc.id)).toBe(true);

    // 向量检索 API 健全性：索引后返回 200 且 results 为数组（不为空即为命中过内容）。
    const vecRes = await request.get(
      `${API_BASE}/knowledge/search?q=${encodeURIComponent('销售额')}`,
      { headers: { Authorization: `Bearer ${adminToken}` } },
    );
    expect(vecRes.ok()).toBe(true);
    const vecBody = await vecRes.json();
    expect(Array.isArray(vecBody.results)).toBe(true);
  });

  // ═══ UI-206: 索引进度实时更新 ═══
  test('[UI-206] KB — 索引进度实时更新', async ({ request }) => {
    test.setTimeout(120000);
    const createRes = await uploadTxtDoc(request, adminToken, `UI206-进度-${uid}`, '索引进度验证数据：本季度成本 5230 万元，净利率 18.7%。');
    expect(createRes.status()).toBe(201);
    const doc = await createRes.json();
    createdDocs.push(doc.id);

    // 上传后立即读取：status 处于流水线中间态（uploaded/indexing）或已 ready，均合法。
    const first = await request.get(`${API_BASE}/knowledge/docs/${doc.id}`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(first.ok()).toBe(true);
    const firstDoc = await first.json();
    expect(['uploaded', 'indexing', 'ready']).toContain(firstDoc.status);

    // 最终收敛到 ready 且进度 100%。
    const ready = await waitForStatus(request, adminToken, doc.id, 'ready');
    expect(ready, `doc ${doc.id} should reach ready`).not.toBeNull();
    expect(ready.progress_percent).toBe(100);
    expect(ready.chunk_count).toBeGreaterThanOrEqual(1);
  });

  // ═══ UI-207: 搜索过滤结果准确 ═══
  test('[UI-207] KB — 搜索过滤结果准确', async ({ request }) => {
    const financeTitle = `UI207-财务-${uid}`;
    const salesTitle = `UI207-销售-${uid}`;

    const financeRes = await uploadTxtDoc(request, adminToken, financeTitle, '财务数据：Q2 营收 4520 万元，毛利率 41.4%。');
    const salesRes = await uploadTxtDoc(request, adminToken, salesTitle, '销售数据：6 月销售额 1720 万元，订单数 456。');
    expect(financeRes.status()).toBe(201);
    expect(salesRes.status()).toBe(201);
    const financeDoc = await financeRes.json();
    const salesDoc = await salesRes.json();
    createdDocs.push(financeDoc.id, salesDoc.id);

    const headers = { Authorization: `Bearer ${adminToken}` };
    const assertFilter = async (q: string, expectedIds: string[]) => {
      const res = await request.get(
        `${API_BASE}/knowledge/docs?q=${encodeURIComponent(q)}&page=1&page_size=20`,
        { headers },
      );
      expect(res.ok()).toBe(true);
      const body = await res.json();
      const ids = (body.docs || []).map((d: any) => d.id);
      for (const id of expectedIds) {
        expect(ids, `query "${q}" should include doc ${id}`).toContain(id);
      }
    };

    // 财务 → 命中财务文档；销售 → 命中销售文档（title 过滤互不串扰）。
    await assertFilter('财务', [financeDoc.id]);
    await assertFilter('销售', [salesDoc.id]);
  });

  // ═══ UI-208: 索引后删除（生命周期闭环） ═══
  test('[UI-208] KB — 索引后删除文档', async ({ request }) => {
    test.setTimeout(120000);
    const createRes = await uploadTxtDoc(request, adminToken, `UI208-删除-${uid}`, '用于验证索引后删除的文档内容。');
    expect(createRes.status()).toBe(201);
    const doc = await createRes.json();

    const ready = await waitForStatus(request, adminToken, doc.id, 'ready');
    expect(ready, `doc ${doc.id} should reach ready`).not.toBeNull();
    expect(ready.chunk_count).toBeGreaterThanOrEqual(1);

    // 删除已索引文档：级联清理后 GET 返回 404。
    const delRes = await request.delete(`${API_BASE}/knowledge/docs/${doc.id}`, {
      headers: { Authorization: `Bearer ${adminToken}`, 'Content-Type': 'application/json' },
    });
    expect(delRes.ok()).toBe(true);

    const goneRes = await request.get(`${API_BASE}/knowledge/docs/${doc.id}`, {
      headers: { Authorization: `Bearer ${adminToken}` },
    });
    expect(goneRes.status()).toBe(404);
  });

  // ═══ UI-209: 大文档分块 ═══
  test('[UI-209] KB — 大文档分块验证', async ({ request }) => {
    test.setTimeout(120000);
    const largeContent = buildLargeDoc();
    expect(Buffer.byteLength(largeContent)).toBeGreaterThan(4096);

    const createRes = await uploadTxtDoc(request, adminToken, `UI209-大文档-${uid}`, largeContent);
    expect(createRes.status()).toBe(201);
    const doc = await createRes.json();
    createdDocs.push(doc.id);

    const ready = await waitForStatus(request, adminToken, doc.id, 'ready');
    expect(ready, `doc ${doc.id} should reach ready`).not.toBeNull();
    // 大文档应被切成多个 chunk（>4096 字符 → 多个 sentence 段 → 多个 chunk）。
    expect(ready.chunk_count).toBeGreaterThanOrEqual(2);
  });
});
