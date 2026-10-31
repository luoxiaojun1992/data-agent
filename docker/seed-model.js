// 测试基建：预置 LLM + Embedding 模型配置到 MongoDB。
// 后端 modelcfg.Provider.models() 只从 model_configs 集合读取模型
// （modelsFromEnv 是死代码），CI 环境不预置则 chat/agent/kb 的 model
// 解析失败（DefaultModel → "no model for use case" → 500 → 前端
// "Chat request failed"）。
// 幂等：已存在同名模型则跳过。

const dbName = 'data_agent';

// 固定 UUID（ModelEntry.ID 为纯 UUID，无 "model_" 前缀）。
const LLM_ID = '11111111-1111-1111-1111-111111111111';
const EMB_ID = '22222222-2222-2222-2222-222222222222';

const llmModel = {
  _id: LLM_ID,
  name: 'mock-gpt-4o',
  base_url: 'http://mockllm:8082/v1',
  api_key: '',
  type: 'llm',
  instruction: '',
  capability: '',
  use_cases: ['chat', 'task', 'enhance', 'compaction', 'intent_check', 'relevance_check', 'kb_chunking', 'kb_image'],
  token_multiplier: 1.0,
  temperature: 0.7,
  max_tokens: 4096,
  context_len: 128000,
  fallback_order: 0,
  created_at: new Date(),
  updated_at: new Date(),
};

const embModel = {
  _id: EMB_ID,
  name: 'nomic-embed-text',
  base_url: 'http://ollama:11434/v1',
  api_key: '',
  type: 'embedding',
  instruction: '',
  capability: '',
  use_cases: ['embedding'],
  token_multiplier: 1.0,
  temperature: 0,
  max_tokens: 0,
  context_len: 8192,
  fallback_order: 0,
  embedding_dim: 768,
  created_at: new Date(),
  updated_at: new Date(),
};

// 注意：不能写 `const db = db.getSiblingDB(...)` —— 那会在初始化表达式里
// 触发 TDZ（ReferenceError: Cannot access 'db' before initialization），
// 导致 mongosh 退出非零、seed-admin 容器退出、compose 提前 abort。
const targetDb = db.getSiblingDB(dbName);
const cfgColl = targetDb.model_configs;
const defColl = targetDb.model_defaults;

// 模型（按 name 幂等）
if (cfgColl.findOne({ name: llmModel.name })) {
  print('model llm skip');
} else {
  cfgColl.insertOne(llmModel);
  print('model llm seeded');
}
if (cfgColl.findOne({ name: embModel.name })) {
  print('model embedding skip');
} else {
  cfgColl.insertOne(embModel);
  print('model embedding seeded');
}

// 默认模型映射（按 use_case 幂等）
function ensureDefault(useCase, modelId) {
  if (defColl.findOne({ use_case: useCase })) {
    print('default ' + useCase + ' skip');
  } else {
    defColl.insertOne({
      _id: 'seed_default_' + useCase,
      use_case: useCase,
      model_id: modelId,
    });
    print('default ' + useCase + ' seeded');
  }
}
ensureDefault('chat', LLM_ID);
ensureDefault('task', LLM_ID);
ensureDefault('enhance', LLM_ID);
ensureDefault('compaction', LLM_ID);
ensureDefault('intent_check', LLM_ID);
ensureDefault('relevance_check', LLM_ID);
ensureDefault('kb_chunking', LLM_ID);
ensureDefault('kb_image', LLM_ID);
ensureDefault('embedding', EMB_ID);
