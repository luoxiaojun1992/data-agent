/**
 * SPEC-046: Tool call fixture — seeds mockllm with multi-step tool call scenarios.
 *
 * mockllm uses LPUSH/LPOP queue: same key injected multiple times = FIFO queue.
 * The ADK ReAct loop reads responses in order, enabling chain-of-tool-call simulation.
 */
const MOCKLLM = 'http://mockllm:8082';
const MOCK_ADMIN_TOKEN = 'test-admin-token';

export type ToolCallScenario =
  | 'kb-search-chain'
  | 'sql-executor'
  | 'stats-engine'
  | 'multi-tool-chain'
  | 'save-report';

/**
 * Seed a tool call scenario into mockllm.
 * Each scenario defines a sequence of LLM responses simulating ReAct tool calls.
 */
export async function seedToolCallScenario(
  request: any,
  scenario: ToolCallScenario,
  userMessage: string
): Promise<void> {
  // Clear any existing mocks for clean state
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});

  switch (scenario) {
    case 'kb-search-chain':
      // Round 1: LLM calls knowledge_search
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: JSON.stringify({
            type: 'tool_call',
            name: 'knowledge_search',
            input: { query: userMessage, top_k: 3 },
          }),
        },
      });
      // Round 2: After tool result, LLM generates final answer
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: '根据知识库检索结果，营收达到 1.2 亿元，同比增长 15%。',
        },
      });
      break;

    case 'sql-executor':
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: JSON.stringify({
            type: 'tool_call',
            name: 'sql_executor',
            input: { sql: 'SELECT product, SUM(revenue) AS total FROM sales GROUP BY product ORDER BY total DESC LIMIT 5' },
          }),
        },
      });
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: '查询结果：DataInsight Pro 销售额 2480 万元排名第一，CloudQuery 1860 万元排名第二。',
        },
      });
      break;

    case 'stats-engine':
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: JSON.stringify({
            type: 'tool_call',
            name: 'stats_engine',
            input: { calculation: 'monthly_trend', source: 'sales_data' },
          }),
        },
      });
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: '统计数据：6月销售额1720万为峰值，全线上涨趋势。客户留存率92.4%。',
        },
      });
      break;

    case 'multi-tool-chain':
      // Tool 1: knowledge_search
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: JSON.stringify({
            type: 'tool_call',
            name: 'knowledge_search',
            input: { query: '销售数据', top_k: 3 },
          }),
        },
      });
      // Tool 2: sql_executor after first result
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: JSON.stringify({
            type: 'tool_call',
            name: 'sql_executor',
            input: { sql: 'SELECT region, SUM(revenue) FROM sales GROUP BY region' },
          }),
        },
      });
      // Tool 3: stats_engine after second result
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: JSON.stringify({
            type: 'tool_call',
            name: 'stats_engine',
            input: { calculation: 'growth_rate', source: 'sales_data' },
          }),
        },
      });
      // Final answer
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: '综合分析完成：华北区域贡献最高，全线上涨趋势明显。',
        },
      });
      break;

    case 'save-report':
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: JSON.stringify({
            type: 'tool_call',
            name: 'save_report',
            input: { title: '销售分析报告', format: 'pdf', content: '2026年上半年销售数据分析...' },
          }),
        },
      });
      await request.post(`${MOCKLLM}/responses`, {
        headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
        data: {
          key: userMessage,
          response: '报告已保存，您可以在制品管理中查看。',
        },
      });
      break;
  }
}

/**
 * Clear all mockllm responses.
 */
export async function clearToolCallMocks(request: any): Promise<void> {
  await request.delete(`${MOCKLLM}/responses`, {
    headers: { Authorization: `Bearer ${MOCK_ADMIN_TOKEN}` },
  }).catch(() => {});
}
