# UI E2E 测试增强与真实集成验证

> **SPEC-046** | Status: 设计中 | Date: 2026-07-17 | Phase: P11

## 1. 目标

解决当前 UI E2E 测试中存在的「假性成功」问题 — 大量用例仅校验 DOM 可见性，**没有真正验证系统可用性**。具体改进：

1. **KB 索引任务执行**：上传文档后必须等待 LLM 索引流程完成（chunk 状态从 `pending` → `indexed`），并通过 Qdrant/MongoDB 验证数据真正落库
2. **工具真正调用**：覆盖 `knowledge_search`（向量/全文检索）、`sql_executor`（SQL 校验→真实查询）、`stats_engine`（真实统计计算）三类 Skill 的端到端调用链
3. **Dashboard 真实数据**：测试启动时通过 fixture API 写入任务/会话/文档等真实数据，断言 KPI 数值和图表非空、与写入数据一致

> **核心原则**: **E2E 测试必须验证"行为结果"，禁止只验证"控件存在"**。一个按钮可见不代表它工作。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-006 | ✅ | 知识库系统（已实现） |
| SPEC-008 | ✅ | Skill 实现层（已实现） |
| SPEC-010 | ✅ | 系统统计监控（已实现） |
| SPEC-022 | ✅ | Dashboard UI E2E（已实现但浅） |
| SPEC-028 | ✅ | KB UI E2E（已实现但浅） |
| SPEC-043 | ✅ | Mock Model Service（已实现） |
| SPEC-048 | ⚠️ **阻塞依赖** | **引擎层 ADK 迁移** — Tool call E2E 用例依赖 ADK ReAct loop，KB 索引 E2E 依赖 `tool.Context.State()` 注入 `kb_id`。SPEC-048 未完成前工具调用链测试无法执行 |
| SPEC-045 | ⚠️ | Go Service UT 全覆盖（设计中）— 非阻塞，但本 spec 涉及的部分服务 UT 未完成时 E2E 需更彻底 |

### 1.7 工具调用测试替代方案（SPEC-048 完成前）

在 ADK 迁移完成前，工具调用 E2E 测试可降级为**直接 HTTP 调用验证**：
- `POST /api/v1/knowledge/search` 验证全文检索
- `POST /api/v1/agent/tasks` + 特定 skill 验证各 skill 执行力
- 不依赖 chat ReAct loop，改为直接 API 调用

## 2. 背景

### 2.1 现状问题（基于 `docs/manual-screenshots/` 主分支截图分析）

#### 2.1.1 Dashboard 假数据（05/22/23-dashboard.png）
- 4 个 KPI 卡片全部显示 `0`（活跃 Chat 会话、Agent 任务、知识库文档、系统可用率 99.9%）
- 6 个图表均为空数据：
  - **Agent 调用量趋势**：仅 6 个扁平线段（无高度）
  - **任务状态分布**：4 个空柱
  - **任务耗时分布**：5 个空柱
  - **24h 请求量分布**：6 个空柱
  - **成功率趋势**：Mon-Sun 7 个等高满绿块（应为变化高度）
  - **Token 消耗/产出/ROI 趋势**：3 组空柱
- 根本原因：`tests/ui/dashboard.spec.ts` 中只断言 `chart-*` 元素 `.toBeVisible()`，没有触发任何数据写入

#### 2.1.2 KB 文档永远停在"已上传"（10-knowledge-base.png, 21-kb-search.png）
- 3 个测试文档（Q2 财务报告 / 销售数据汇总 / 客户合同模板）状态全部为 `已上传 · 0 分片`
- `6a59f224` 哈希指纹表明是同一占位符
- `kb-search.png` 与 `kb-list.png` 二进制 MD5 完全一致（`4675328c2f901dad1e0bd38f8ebe9256`）— 搜索功能未真正执行
- 根本原因：`tests/ui/kb.spec.ts` 中 `createDoc` 只调用 `POST /knowledge/docs` 写入元数据，**没有调用 `POST /knowledge/docs/:id/chunks` 触发 LLM 索引**；`UI-121` 搜索测试仅检查卡片数 ≥ 1（不验证过滤结果）

#### 2.1.3 Skill 调用链路未验证（chat.spec.ts）
- Chat 测试只验证 AI 消息出现（mockllm 返回固定文本）
- 没有任何用例验证：用户消息 → LLM 推理 → 工具调用 → 工具结果回传 → 最终回答
- 4 个 Skill（`sql_executor` / `stats_engine` / `knowledge_search` / `save_report`）没有任何 E2E 验证

### 2.2 设计动机

当前测试套件覆盖率（cases.json）显示 81 个用例 ID 全部声明 ✅，但「覆盖率 100%」≠「系统可用」。`tests/ui/agent-extras.spec.ts` 中多个用例（UI-047~056）只检查 `agent-page-header` 可见性，等同占位符。

**真实集成测试的目标**：

| 维度 | 当前 | 目标 |
|------|------|------|
| KB 索引 | 仅上传元数据 | 上传 → 分块 → 索引 → 检索命中 |
| 工具调用 | Mockllm 固定返回 | 用户输入 → LLM 推理 → 工具调用 → 结果回传 |
| Dashboard 数据 | 全 0 | 写入 fixture → 真实 KPI 非 0 → 图表显示数据点 |
| 失败检测 | 仅 HTTP 5xx | 业务结果错误（数据不一致）也能发现 |

## 3. 架构概述

### 3.1 测试数据流（增强后）

```
┌──────────────────────────────────────────────────────────────┐
│                     Playwright E2E                            │
│                                                                │
│  ┌─────────────────┐    ┌──────────────────┐   ┌──────────┐  │
│  │  Test Fixture    │    │  API 直接写入     │   │ mockllm  │  │
│  │  (tests/ui/      │───▶│  - 创建 Session  │   │ 注入     │  │
│  │   fixtures/)     │    │  - 创建 Task     │   │ 工具调用 │  │
│  │  - test-1.txt    │    │  - 创建 KB Doc   │   │ 响应链   │  │
│  │  - sales.csv     │    │  - Add Chunks    │   └────┬─────┘  │
│  │  - profile.json  │    │  - List Tasks    │        │         │
│  └─────────────────┘    └────────┬─────────┘        │         │
│                                  │                  │         │
│                                  ▼                  ▼         │
│                          ┌──────────────────────────────┐    │
│                          │      DataAgent Backend        │    │
│                          │  - MongoDB (真实数据)         │    │
│                          │  - Qdrant (向量索引)          │    │
│                          │  - Redis (任务队列)           │    │
│                          │  - SeaweedFS (文件存储)        │    │
│                          └──────────────┬───────────────┘    │
│                                         │                    │
│                                         ▼                    │
│                              ┌──────────────────────┐        │
│                              │  Frontend (Next.js)  │        │
│                              │  - 真实渲染 KPI/图表 │        │
│                              │  - 真实展示工具结果   │        │
│                              └──────────────────────┘        │
└──────────────────────────────────────────────────────────────┘
```

### 3.2 新增测试文件组织

```
tests/ui/
├── fixtures/
│   ├── files/
│   │   ├── test-1.txt           # 已有：基础 KB 文档
│   │   ├── test-2.txt           # 已有
│   │   ├── test-3.json          # 已有
│   │   ├── test-large.bin       # 已有
│   │   ├── kb-finance.md        # 新增：财务报告样例（≥1KB，含表格）
│   │   ├── kb-sales.md          # 新增：销售数据样例（≥1KB，含数字）
│   │   └── sales-data.csv       # 新增：CSV 表格数据
│   └── data/                    # 新增目录
│       ├── dashboard-fixture.ts # Dashboard 测试数据生成器
│       ├── kb-index-fixture.ts  # KB 索引端到端 fixture
│       └── tool-call-fixture.ts # 工具调用链 fixture
├── utils/
│   └── mongo-query.ts           # 新增：测试辅助，直接查 MongoDB 验证
├── kb-integration.spec.ts       # 新增：KB 索引全链路
├── tool-call.spec.ts            # 新增：工具调用链端到端
├── dashboard-integration.spec.ts # 新增：Dashboard 真实数据
└── (existing) ...               # 保持现有文件
```

## 4. API/接口设计

### 4.1 新增工具函数（`tests/ui/utils/mongo-query.ts`）

| 函数 | 用途 | 输入 | 输出 |
|------|------|------|------|
| `getKBDocStatus(token, docId)` | 查询文档索引状态 | token, docId | `{ status, chunk_count, indexed_at }` |
| `waitForKBIndex(token, docId, timeoutMs)` | 轮询等待索引完成 | token, docId, 30000 | `boolean` |
| `getKBSearchHits(token, query, topK)` | 真实调用 KB 检索 | token, query, 5 | `SearchResult[]` |
| `getDashboardKPIs(token)` | 获取 Dashboard 真实 KPI | token | `{kpis, task_stats}` |
| `getTaskById(token, taskId)` | 查任务详情 | token, taskId | `Task` |

### 4.2 新增 fixture 模块（`tests/ui/fixtures/data/`）

| 模块 | 导出 | 说明 |
|------|------|------|
| `dashboard-fixture.ts` | `seedDashboardData(token, opts)` | 创建 N 个 Session + M 个 Task + K 个 KB Doc，写入真实 MongoDB，返回预期 KPI 数值 |
| `kb-index-fixture.ts` | `createAndIndexDoc(token, content)` | 创建文档 → AddChunks → 等待索引完成 |
| `tool-call-fixture.ts` | `seedToolCallScenario(scenario)` | 预设 mockllm 响应（模拟工具调用链） |

## 5. 详细设计

### 5.1 KB 索引全链路（`tests/ui/kb-integration.spec.ts`）

#### 5.1.1 用例设计

| 用例 ID | 标题 | 优先级 | 关键断言 |
|---------|------|:------:|----------|
| UI-204 | KB — 文档索引端到端验证 | P0 | 上传→AddChunks→等待 indexed→MongoDB 验证 chunk_count > 0 |
| UI-205 | KB — 索引后检索命中 | P0 | 索引完成后用关键词检索，结果包含该文档 chunk |
| UI-206 | KB — 索引进度实时更新 | P1 | UI 状态从 `已上传` → `索引中` → `已索引` 流转 |
| UI-207 | KB — 搜索过滤结果准确 | P0 | 输入"销售"只返回销售文档，输入"财务"只返回财务文档 |
| UI-208 | KB — 索引失败重试 | P1 | 故意写入无效 chunk，验证状态变为 `failed` 并可重试 |
| UI-209 | KB — 大文档分块（>10KB） | P2 | 大文档分块后 chunk_count ≥ 3 |

#### 5.1.2 核心测试代码模式

```typescript
// 伪代码示例
test('[UI-204] KB — 文档索引端到端验证', async ({ page, request }) => {
  const token = await loginAsAdmin(request);
  
  // 1. 上传文档元数据
  const doc = await createDoc(request, token, 'finance-report');
  expect(doc.id).toBeTruthy();
  
  // 2. 写入 chunk（模拟 LLM 分块结果）
  const chunks = [
    '本季度营收达到 1.2 亿元，同比增长 15%。',
    '主要增长来自华北区域，占总营收 40%。',
    '下季度预测：营收目标 1.35 亿元。',
  ];
  const res = await request.post(`${API_BASE}/knowledge/docs/${doc.id}/chunks`, {
    headers: { Authorization: `Bearer ${token}` },
    data: { chunks },
  });
  expect(res.status()).toBe(200);
  
  // 3. 等待索引完成（最长 30s）
  const indexed = await waitForKBIndex(token, doc.id, 30000);
  expect(indexed).toBe(true);
  
  // 4. 直接查 MongoDB 验证
  const status = await getKBDocStatus(token, doc.id);
  expect(status.status).toBe('ready');
  expect(status.chunk_count).toBe(3);
  
  // 5. 真实检索验证
  const hits = await getKBSearchHits(token, '营收', 5);
  expect(hits.length).toBeGreaterThan(0);
  expect(hits.some(h => h.doc_id === doc.id)).toBe(true);
  
  // 6. UI 验证：刷新页面，文档状态为"已索引"
  await page.goto('/admin/knowledge');
  await expect(page.locator(`[data-testid="kb-doc-${doc.id}"]`)).toContainText(/已索引|indexed/i);
});
```

#### 5.1.3 MongoDB 验证实现

通过 data-agent 后端的 `/api/v1/admin/knowledge/docs/:id/debug` 端点（或新增内部端点 `/api/v1/knowledge/docs/:id/status`）读取状态。如果后端无对应接口，则通过 `mongo` 容器 shell 直连查询：

```bash
docker exec data-agent-mongodb-1 mongosh dataagent --eval \
  "db.kb_chunks.find({doc_id: 'xxx'}, {chunk_text: 1, status: 1})"
```

### 5.2 工具真正调用（`tests/ui/tool-call.spec.ts`）

#### 5.2.1 用例设计

| 用例 ID | 标题 | 优先级 | 关键断言 |
|---------|------|:------:|----------|
| UI-211 | ToolCall — knowledge_search 全文检索 | P0 | 用户问"营收" → mockllm 模拟调用 knowledge_search → UI 显示检索结果 |
| UI-212 | ToolCall — knowledge_search 命中后回答 | P0 | mockllm 收到工具结果后生成包含 chunk 内容的回答 |
| UI-213 | ToolCall — sql_executor 校验通过 | P0 | 用户问"查询销售总额" → mockllm 调用 sql_executor → UI 显示生成的 SQL |
| UI-214 | ToolCall — sql_executor 校验失败 | P1 | mockllm 返回 DROP TABLE → UI 显示安全错误 |
| UI-215 | ToolCall — stats_engine 真实计算 | P0 | 注入 mockllm 返回 stats_engine 结果 → UI 显示统计卡片 |
| UI-216 | ToolCall — save_report 报告生成 | P1 | mockllm 调用 save_report → 报告卡片出现在消息流 |
| UI-217 | ToolCall — 多工具链式调用 | P0 | 用户问复杂问题 → mockllm 链式调用 3 个工具 → 全部结果展示 |
| UI-218 | ToolCall — 工具结果复制 | P2 | 点击 SQL 块"复制"按钮，剪贴板内容正确 |

#### 5.2.2 mockllm 工具调用链响应模板

```typescript
// fixture: seedToolCallScenario('kb-search-chain')
const responses = [
  // 第一轮：LLM 决定调用 knowledge_search
  { key: '查询营收', response: JSON.stringify({
    type: 'tool_call',
    name: 'knowledge_search',
    input: { query: '营收', top_k: 3 },
  })},
  // 第二轮：工具结果返回后，LLM 生成最终回答
  { key: 'TOOL_RESULT:knowledge_search', response: '根据文档，营收达到 1.2 亿元...' },
];
```

**说明**: mockllm 已验证支持 LPUSH/LPOP 队列 — 同 key 多次注入即为 FIFO 队列。但工具调用链 E2E 的完整验证需依赖 **SPEC-048（ADK ReAct loop 迁移）**。迁移完成前，工具调用测试降级为 API 直调（见 §1.7）。

### 5.3 Mem0 长期记忆（`tests/ui/mem0.spec.ts`）

#### 5.3.1 用例设计

| 用例 ID | 标题 | 优先级 | 关键断言 |
|---------|------|:------:|----------|
| UI-219 | Mem0 — 会话自动写入记忆 | P0 | Chat 后搜索该会话关键词 → MemoryService 命中 |
| UI-220 | Mem0 — memory_search 工具调用 | P0 | LLM 主动调 memory_search → 回答引用记忆内容 |
| UI-221 | Mem0 — 多用户隔离 | P0 | 用户 B 搜不到用户 A 的记忆 |
| UI-222 | Mem0 — 长对话压缩后记忆保留 | P1 | 50 轮后 token 不爆炸，关键信息仍可检索 |

#### 5.3.2 核心测试代码模式

```typescript
test('[UI-219] Mem0 — 会话自动写入记忆', async ({ page, request }) => {
  const token = await loginAsUser(request);
  
  // 1. 第一轮对话：用户说关键信息
  await seedMock(request, '我叫张三，最喜欢的项目是 Alpha', '好的，记住了');
  await page.locator('[data-testid="chat-input"]').fill('我叫张三，最喜欢的项目是 Alpha');
  await page.locator('[data-testid="chat-send-btn"]').click();
  await expect(page.locator('[data-testid^="chat-msg-ai-"]').last()).toBeVisible();
  
  // 2. 等待 ADK Runner 自动写入 Mem0（write 是副作用）
  await page.waitForTimeout(3000);
  
  // 3. 直接搜 MemoryService 验证
  const memRes = await request.get(`${API_BASE}/admin/memory/search`, {
    headers: { Authorization: `Bearer ${token}` },
    params: { query: 'Alpha 项目', user_id: USER_ID },
  });
  const memories = await memRes.json();
  expect(memories.results.length).toBeGreaterThanOrEqual(1);
  expect(memories.results.some(m => m.memory.includes('Alpha'))).toBe(true);
});

test('[UI-220] Mem0 — memory_search 工具调用', async ({ page, request }) => {
  // seed 第一轮
  await seedMock(request, '我叫张三，最喜欢的项目是 Alpha', '好的');
  // 第二轮：LLM 应调 memory_search 查记忆
  await seedMock(request, '我叫什么名字', '根据记忆，你叫张三');  // ADK ReAct loop 自动处理
  await page.locator('[data-testid="chat-input"]').fill('我叫什么名字');
  await page.locator('[data-testid="chat-send-btn"]').click();
  await expect(page.locator('[data-testid^="chat-msg-ai-"]').last()).toContainText('张三');
});

test('[UI-221] Mem0 — 多用户隔离', async ({ page, request }) => {
  // 用户 A 写
  await loginAndChat(request, USER_A, '我是用户A', '好的');
  // 等写入
  await page.waitForTimeout(3000);
  
  // 用户 B 搜
  const tokenB = await loginAsUserB(request);
  const memRes = await request.get(`${API_BASE}/admin/memory/search`, {
    headers: { Authorization: `Bearer ${tokenB}` },
    params: { query: '用户A', user_id: USER_B_ID },
  });
  const memories = await memRes.json();
  expect(memories.results.length).toBe(0);
});

test('[UI-222] Mem0 — 长对话压缩后记忆保留', async ({ page, request }) => {
  // 发送 50 轮对话触发 CompactionConfig (maxEvents=50)
  for (let i = 0; i < 50; i++) {
    await seedMock(request, `这是第${i}轮消息`, `回复${i}`);
    await page.locator('[data-testid="chat-input"]').fill(`这是第${i}轮消息`);
    await page.locator('[data-testid="chat-send-btn"]').click();
    await page.waitForTimeout(100);
  }
  
  // 关键信息：第 1 轮说的
  const memRes = await request.get(`${API_BASE}/admin/memory/search`, {
    headers: { Authorization: `Bearer ${token}` },
    params: { query: '第0轮消息', user_id: USER_ID },
  });
  // 压缩后摘要应保留关键信息
  expect(memRes.ok()).toBe(true);
});
```

### 5.4 Dashboard 真实数据（`tests/ui/dashboard-integration.spec.ts`）

#### 5.4.1 用例设计

| 用例 ID | 标题 | 优先级 | 关键断言 |
|---------|------|:------:|----------|
| UI-229 | Dashboard — KPI 显示真实任务数 | P0 | 创建 5 个 Task → KPI 数值 = 5 |
| UI-230 | Dashboard — KPI 显示真实文档数 | P0 | 索引 3 个 KB Doc → 知识库文档 KPI = 3 |
| UI-231 | Dashboard — 任务状态分布准确 | P0 | 创建 2 completed + 1 failed + 1 running → 饼图数据正确 |
| UI-232 | Dashboard — 24h 趋势有时间戳分布 | P0 | 创建 6 小时内任务 → 24h 柱状图非空且分布合理 |
| UI-233 | Dashboard — Token KPI 非 0 | P1 | 创建 ≥1 已完成任务 → Token 消耗显示非 0 |
| UI-234 | Dashboard — ROI 随任务数变化 | P1 | 创建 N 个任务 → ROI 数值按公式计算正确 |
| UI-235 | Dashboard — 多用户隔离 | P0 | 用户 A 创建的数据不出现在用户 B 的 Dashboard |
| UI-236 | Dashboard — 时间筛选有效 | P1 | 本周筛选时，本周任务显示而上周任务不显示 |

#### 5.3.2 核心测试代码模式

```typescript
test('[UI-229] Dashboard — KPI 显示真实任务数', async ({ page, request }) => {
  const token = await loginAsAdmin(request);
  
  // 1. 创建 5 个真实任务
  const createdIds: string[] = [];
  for (let i = 0; i < 5; i++) {
    const res = await request.post(`${API_BASE}/tasks`, {
      headers: { Authorization: `Bearer ${token}` },
      data: { title: `Task ${i}`, skills: ['sql_executor'] },
    });
    const task = await res.json();
    createdIds.push(task.task_id);
  }
  
  // 2. 后端 API 验证 KPI
  const kpis = await getDashboardKPIs(token);
  expect(kpis.task_stats.total).toBeGreaterThanOrEqual(5);
  
  // 3. UI 验证
  await page.goto('/');
  await expect(page.locator('[data-testid="dashboard-stat-1"]')).toBeVisible();
  const kpiValue = await page.locator('[data-testid="dashboard-stat-1"]').textContent();
  // 解析出数字（KPI 卡显示 "5" 或 "↑8" 等）
  expect(kpiValue).toMatch(/[1-9]/);  // 至少 1 个非 0 数字
  
  // 4. 清理
  for (const id of createdIds) {
    await request.delete(`${API_BASE}/tasks/${id}`, {
      headers: { Authorization: `Bearer ${token}` },
    }).catch(() => {});
  }
});
```

#### 5.3.3 时间筛选真实数据验证

```typescript
test('[UI-236] Dashboard — 时间筛选有效', async ({ page, request }) => {
  const token = await loginAsAdmin(request);
  
  // 创建本周任务和上周任务（通过修改 created_at 字段）
  // 注: 后端需支持 created_at 自定义，或通过 API mock 时间
  
  // 切换筛选
  await page.locator('[data-testid="filter-today"]').click();
  const todayKpi = await getKPIValue(page, 1);
  
  await page.locator('[data-testid="filter-week"]').click();
  await page.waitForTimeout(500);
  const weekKpi = await getKPIValue(page, 1);
  
  expect(weekKpi).toBeGreaterThanOrEqual(todayKpi);  // 本周≥今日
});
```

### 5.4 现有 spec 用例强化（不新增文件，修改现有）

| 文件 | 强化点 | 当前 | 目标 |
|------|--------|------|------|
| `chat.spec.ts` UI-024/025 | AI 消息内容检查 | 仅检查 `<chat-msg-ai-X>` 出现 | 验证 mockllm 响应内容完全匹配 seed 的 response |
| `chat.spec.ts` UI-029 | 消息流复制 | 仅按钮可见 | 点击后剪贴板内容 = 消息原文 |
| `chat.spec.ts` UI-030 | 上下文保留 | 仅检查新消息 | 创建 Session → 发 3 条 → 检查 session_id 持久 |
| `agent.spec.ts` UI-047~056 | 任务执行验证 | 仅 page header 可见 | 完整跑通：建任务→查详情→状态变化→清理 |
| `kb.spec.ts` UI-115~124 | KB 真实使用 | 仅 UI 可见 | 强化为真实索引+检索（合并到 kb-integration.spec.ts） |
| `dashboard.spec.ts` UI-062~074 | 真实数据 | 仅元素可见 | 强化为真实数据验证（合并到 dashboard-integration.spec.ts） |

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No — 复用现有 kb_chunks / tasks / sessions |
| 是否影响现有 API | No — 工具调用 E2E 在 SPEC-048 完成后通过 chat 端点验证；SPEC-048 完成前通过 API 直调验证 |
| 是否需要新增 Skill | No |
| 是否需要新增测试 fixture | Yes — 3 个 fixture 模块、3 个 spec 文件 |
| 是否需要修改后端 | No（SPEC-048 单独处理后端变更） |
| 是否需要扩展 mockllm | No — LPUSH/LPOP 队列已支持同 key 多次注入，mockllm 无变更 |
| 是否影响 CI 时间 | 增加约 2 分钟（30 个新用例 × 平均 4s/用例） |
| 测试通过率影响 | 可能下降（暴露真实问题）— 这是预期目标 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `tests/ui/kb-integration.spec.ts` | New module | New（~300 行，6 用例） |
| `tests/ui/tool-call.spec.ts` | New module | New（~400 行，8 用例） |
| `tests/ui/dashboard-integration.spec.ts` | New module | New（~350 行，8 用例） |
| `tests/ui/mem0.spec.ts` | New module | New（~250 行，4 用例） |
| `tests/ui/utils/mongo-query.ts` | New helper | New（~100 行） |
| `tests/ui/fixtures/data/dashboard-fixture.ts` | New data helper | New（~120 行） |
| `tests/ui/fixtures/data/kb-index-fixture.ts` | New data helper | New（~100 行） |
| `tests/ui/fixtures/data/tool-call-fixture.ts` | New data helper | New（~150 行） |
| `tests/ui/fixtures/files/kb-finance.md` | New fixture | New |
| `tests/ui/fixtures/files/kb-sales.md` | New fixture | New |
| `tests/ui/fixtures/files/sales-data.csv` | New fixture | New |
| `tests/ui/coverage/cases.json` | Add new IDs | Edit（添加 UI-204~236） |
| `tests/ui/kb.spec.ts` | Strengthen | Edit（合并部分用例到 integration spec） |
| `tests/ui/dashboard.spec.ts` | Strengthen | Edit（合并部分用例到 integration spec） |
| `tests/ui/chat.spec.ts` | Strengthen | Edit（增加响应内容验证） |
| `tests/ui/agent.spec.ts` | Strengthen | Edit（增加完整任务生命周期） |
| `internal/api/handler/knowledge.go` | Add status endpoint | Edit（可选，便于测试断言） |
| `.github/workflows/ui-tests.yml` | Update coverage | Edit（增加新用例 ID 列表） |
| `docs/DataAgent-UI测试用例文档.md` | Sync docs | Edit（添加 UI-204~236 章节） |

## 8. 测试策略

### 8.1 单元测试

不影响本 spec（不涉及 Go 代码逻辑变更，ADK 迁移由 SPEC-048 负责）。

### 8.2 集成测试

- 工具调用链测试在 **SPEC-048 完成后** 通过 ADK Runner 的 ReAct loop 验证
- SPEC-048 完成前：通过 API 直调验证各 skill 行为

### 8.3 E2E 测试

| 阶段 | 用例 | 预期时间 |
|------|------|----------|
| 1. KB 索引全链路 | UI-204~209 | ~30s（等待索引） |
| 2. 工具调用链 | UI-211~218 | ~60s（多轮 SSE） |
| 3. Mem0 长期记忆 | UI-219~222 | ~60s（含 50 轮长对话） |
| 4. Dashboard 真实数据 | UI-229~236 | ~45s（数据生成） |
| 5. 现有用例强化 | UI-024/025/029/030 + Agent | +30s |

### 8.4 审计

- 使用 `.agent/skills/ui-test-audit` 审计新用例：
  - 禁止 `if (await x.isVisible())` 条件断言
  - 禁止 `page.route()` mock API
  - 禁止 `t.Skip()` 绕过
  - 必须每个 Success 测试 ≥ 2 个行为断言

## 9. UI Test / E2E 验收规则

- [x] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [x] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [x] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [x] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [x] **严禁** 以占位用例顶替真实功能测试

**本 spec 强制补充**:
- [x] **必须** 每个测试用例至少包含 **2 个行为验证断言**（除元素可见性外必须验证数据/状态/副作用）
- [x] **必须** 数据相关测试断言「写入数据 → 查询结果」一致性
- [x] **必须** 工具调用测试断言「mockllm 收到请求 → GO ADK ReAct loop → skill 执行 → 结果回传 → UI 展示」完整链路（依赖 SPEC-048）
- [x] **必须** 失败用例提供截图 + 后端日志链接（GitHub Actions artifact）

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

不适用（ADK 迁移由 SPEC-048 负责，沿用现有 100% 覆盖率标准）。

## 10. 验证标准

### 10.1 数值化指标

| 指标 | 当前 | 目标 |
|------|------|------|
| 假性用例数（仅验证可见性） | ~40 | 0 |
| 真实集成用例数 | 0 | 26 (UI-204~236) |
| 用例平均行为断言数 | 1.2 | ≥ 2.5 |
| 测试执行时间 | ~3 分钟 | ~5 分钟 |
| CI 失败定位准确率 | 70% | 95%（失败时附后端日志） |
| KB 索引端到端覆盖 | 0% | 100% |
| 工具调用链覆盖 | 0% | 100% |
| Dashboard 真实数据覆盖 | 0% | 100% |

### 10.2 通过/失败判据

- ✅ 所有 26 个新用例通过
- ✅ 现有 4 个 spec 文件强化后无回归
- ✅ CI sonar-check + ui-tests 同时通过
- ✅ 主分支 Dashboard 截图（手动验证）显示真实 KPI 数值（非 0）
- ✅ 主分支 KB 截图显示至少 1 个文档状态为「已索引」
- ✅ SPEC-048 完成后：工具调用截图显示 AdK ReAct loop 触发的工具结果卡片

### 10.3 回滚策略

如新测试频繁失败（非真实 bug 而是非确定性），优先级：
1. 增加 `await page.waitForTimeout()` 等待数据稳定
2. 增加重试逻辑（最多 3 次）
3. 标记为 `@flaky` 并 issue 跟进，**不降级**用例
4. 最后手段：在 spec 中显式记录问题，**不删除**用例

## 11. 实施步骤建议

> **前置条件**: SPEC-048（ADK 迁移）必须**至少完成步骤 1-4**（session 适配 + skill 改写 + chat_service 重写 + Runner 初始化），工具调用链 E2E 才能执行。

| 阶段 | 内容 | 依赖 |
|------|------|------|
| 1. SPEC-048 步骤 1-4 | ADK 迁移基础（Session 适配 + Skill 改写 + Runner） | 无 |
| 2. KB 索引端到端 | UI-204~209（6 用例） | SPEC-048 步骤 1-2（session.Service 就绪） |
| 3. 工具调用链端到端 | UI-211~218（8 用例） | SPEC-048 步骤 3-4（ReAct loop 就绪） |
| 4. Mem0 长期记忆 | UI-219~222（4 用例） | SPEC-048 步骤 5（Mem0 + Embedding 就绪） |
| 5. Dashboard 真实数据 | UI-229~236（8 用例）+ 现有用例强化 | SPEC-048 步骤 3-4 |
| 6. CI 联调 + 文档同步 + 截图验证 | — | 以上全部 |

每步均需通过 CI 才能进入下一步（沿用「禁止绕过 CI」红线规则）。
