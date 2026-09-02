# DataAgent - 工程决策日志

> 按日期追加的工程决策记录。新条目追加在顶部，最新在前。

## 2026-09-02: sysconfig description 走 DB + _id UUID + 模型下拉 use_case 过滤

- **上下文**: 晓军要求系统设置页描述列走 DB 而非前端兜底，并系统 review 修复是否严格符合 4 规则（字段走 DB / _id 用 uuid / 必要唯一索引 / 用 _id 查询更新）。
- **决策**:
  1. `SystemConfig.Description` 作为 DB 字段贯穿全链路，前端删硬编码 `BUILTIN_CONFIGS`，seed 幂等同步内置描述（已有 key 保留用户 value、仅同步 description）
  2. `_id` 用纯 `uuid.NewString()`（与 model_defaults 一致，无业务前缀），`key` 走独立唯一索引
  3. `system_configs.key` 唯一索引 + `model_defaults.use_case` 唯一索引，集中定义在 client.go `EnsureIndexes`
  4. `Upsert` 用 `_id` 作 filter（`resolveID` 复用已有 _id 或 mint UUID）
  5. 模型下拉 `attachDefaults`/`defaultIDs` 按 use_case 过滤，chat 下拉只标 chat 默认并排最上
- **理由**: 展示字段/描述是 DB 事实（SSOT），前端硬编码 = 双份真相；`_id` 承载业务语义违反主键铁律（见 CONVENTIONS #6/#34）。
- **踩坑**: MongoDB 真实库名是 `data_agent`（下划线）非 `data-agent`（连字符），误用连字符查到空库差点误判「默认配置被删」。
- **commit**: `4ea255b` + `bfdaecc`，已合并 main。

## 2026-07-17: SPEC-046 & SPEC-047 设计（UI 测试真实化 + 截图布局修复）

- **上下文**: 主分支 27 张 UI 截图（`docs/manual-screenshots/`）显示 9 个真实 bug — Dashboard 全 0 数据、KB 永远"已上传"、Chat Session 面板严重重叠等。现有 E2E 用例大量「仅验证可见性」未真正验证系统可用。
- **决策**:
  1. **SPEC-046**: 新增 22 个真实集成用例（UI-204~228），覆盖 KB 索引全链路、工具调用端到端、Dashboard 真实数据。要求每个 Success 用例 ≥ 2 个行为断言、必须验证「写入 → 查询」一致性
  2. **SPEC-047**: 修复 9 个截图 bug 严重度分级（3 P0 / 4 P1 / 2 P2），含 Chat Session 面板 `w-72 flex-shrink-0` 修复、fade 动画加 `forwards`、审计错误显示 status+endpoint
  3. mockllm 需扩展多轮工具调用协议（`scenario-id` + `steps`）以支持 UI-211~218
  4. 两个 spec 同步实施、互相依赖：046 完成后 Dashboard 才有真实数据，047 才能验证「非空数据图表」；047 完成后 046 的截图回归测试才不会被「布局错乱」假性失败
- **理由**:
  - 当前 81 个用例覆盖率 100% 但系统不可用 — 「覆盖率」≠「系统可用」
  - 截图是真相，E2E 截图中肉眼可见的 bug 必须修复

## 2026-08-12: external_api_* Tools 上线 + Web Search + API 集合 Bug 修复

### Skill/Seed 架构铁律
- ⛔ **新增 skill = 三步同步**: ① `predefinedSkills()` Seed 配置 ② `specs()` ADK Tool 注册 ③ Deps 字段初始化
- ⛔ **wire.go Deps 字段赋值必须在消费之前**: `deps.xxx = NewService()` 放 `toolDeps := &adktools.Deps{...}` 之前，否则 nil → tools 永远跳过
- DB Seed 与 ADK Tool 注册是两条独立代码路径，无编译期关联

### 联网搜索方案
- 中国使用: DuckDuckGo 不通、SearXNG 是 AGPL（禁止）
- 最终方案: 自实现 Bing + Baidu 双引擎，API key 配置化，降级返空

### MongoBSON 教训
- `interface{}` 字段 BSON 解码 → `primitive.D`（JSON 序列化为数组）
- 正确: 用 `json.RawMessage` 保持 JSON 结构端到端
  - mockllm 当前 SHA256 单 key 匹配不支持多轮工具调用链路
- **影响**:
  - 新增 3 个 spec 文件、3 个 fixture、1 个 mockllm 协议扩展
  - 测试时间从 ~3 分钟增至 ~5 分钟
  - 通过率可能下降（暴露真实问题）— 这是预期目标

## 2026-07-15: SPEC-038 安全层 E2E 测试

- **上下文**: 实现安全审计层完整 E2E 测试（输入拦截、输出脱敏、RBAC 越权）
- **决策**:
  1. 全部走真实后端链路 + mockllm，禁止 `page.route()` 截获
  2. `NewAuditor` 构造时调 `config.Compile()` 预编译所有 regex（不再依赖 lazy compile）
  3. OutputRules 按优先级排序：id_card (90) → phone (80) → api_key (90)
  4. mockllm 统一使用 SHA256 完整 hex 做 key 匹配，测试传原始消息不预 hash
  5. 前端 SSE parser 增加 `parsed.error` 处理
  6. task tests 删除 `test.skip()`，改为 API 预创建数据 + "全部" filter
- **理由**:
  - `Compile()` 缺失导致 `rule.compiled == nil`，在 alpine CI 环境下 regex 操作产生超过 10 秒的挂起
  - 手机号 regex 会误匹配身份证中连续 11 位数字（如 `199001011231`），需 id_card 先跑
  - `page.route()` 跨测试残留导致请求不到后端，mockllm 是唯一可靠的隔离方式
  - 预 hash key 被 mockllm 二次 hash 导致注入与查询 key 不一致
- **影响**:
  - 166 个 E2E 用例全部通过，覆盖率 100%
  - 7 个飞书客户端 + 拖拽上传标记为人工测试，其余全部自动化

## 2026-07-09: 移除 MinIO/etcd（SPEC-016）

- **上下文**: docker-compose 中残留 MinIO/etcd 服务，与架构设计不符（对象存储已统一为 SeaweedFS）；前后端应用服务未在 compose 中定义
- **决策**:
  1. 从 `docker-compose.yml` 和 `docker-compose.ui-test.yml` 移除 `minio` 和 `etcd` 服务块
  2. Qdrant v2.5.9 standalone 改用嵌入式存储（本地卷 `qdrant-data:/var/lib/qdrant`），不依赖外部 etcd
  3. 在两个 compose 中添加 `data-agent`（Go 后端，:8080）和 `frontend`（Next.js，:3000）服务
  4. 新建 `frontend/Dockerfile`（standalone 多阶段构建）
- **理由**: 
  - SeaweedFS 已统一对象存储（SPEC-003/005），MinIO 是历史残留
  - Qdrant standalone 自 v2.4+ 默认嵌入式 etcd，不再需要外部依赖
  - 开发者需要 `docker compose up` 一键启动完整技术栈
- **影响**: 
  - 启动时间减少 ~15s（无需 minio/etcd 健康检查）
  - 内存占用降低（移除两个 Java/Go 进程）

## 2026-07-05: 文档架构初始化

- **上下文**: 项目仓库创建，需要建立标准化文档架构
- **决策**: 采用 doc-architect 标准（Hub-and-Spoke 架构），以 `.agent/` 为 SSOT
- **理由**: 标准化文档架构确保 AI Agent 和人类开发者有一致的上下文来源，减少沟通成本
- **备选方案**: 无（绿地项目，没有历史文档需要迁移）

## 2026-07-01: 项目架构决策汇总

以下决策来自 PRD/RFC 设计评审阶段：

### 后端语言选型: Go
- **理由**: 高性能、并发原生、单二进制部署简单、ADK 框架 Go 生态成熟

### 部署形态: 单二进制
- **理由**: 简化部署运维，Worker/Scheduler 作为同进程 goroutine 运行
- **备选**: 微服务拆分各组件 → MVP 阶段运维成本太高，V2.0 再评估

### 消息队列: Redis Stream
- **理由**: 无额外中间件依赖，开发环境简单，吞吐量满足 MVP 需求
- **备选**: RabbitMQ/Kafka → MVP 阶段过度设计

### 业务数据库: MongoDB
- **理由**: 文档模型灵活，统一存储所有业务实体，Schema-less 适合快速迭代
- **备选**: PostgreSQL → PRD 中所有实体字段都在变化中，MongoDB 更灵活

### 向量分片: LLM 自行判断
- **理由**: 不引入额外的 embedding 模型，降低系统依赖和成本
- **备选**: text-embedding-3 → 额外 API 成本，且分片语义判断不如 LLM 灵活

### 前端框架: React/Next.js
- **理由**: 生态丰富、SSR 支持好、社区活跃
- **备选**: Vue → 团队偏好 React

### 飞书优先 IM 集成
- **理由**: Go SDK (go-lark) 成熟，接入步骤少，内部应用无需复杂审批
- **后续**: V1.1 扩展钉钉和企业微信

### 安全: SQL AST 白名单
- **理由**: 通过 pingcap/tidb/parser 在 SQL 执行前进行 AST 解析，从语法层面拦截写入操作，而非依赖 LLM 自觉
- **备选**: 纯 Prompt 约束 LLM → 不可靠，LLM 可能生成恶意 SQL

## RBAC 权限系统关键约定（2026-08-06）

- ⛔ Seed 只做首次幂等插入，禁止写补偿修复函数（线上数据修正用 mongosh 一次性执行）
- ⛔ 新增权限必须覆盖三层：rbac.go 常量 → routes.go RequirePermission → 前端 canAccess
- ⛔ 改路由必须三步验证：wire.go DI → routes.go 注册 → 前端 API 路径
- ⛔ Docker 部署必须 `--no-cache`
- 权限继承：user→admin→sysAdmin 通过 parent_id 链，GetAllRoleIDsWithAncestors 查询
- Admin 限制：5 层防守（CRUD/角色升级/RBAC分配/邀请/前端）
- 敏感接口权限不可共用（如 model:list ≠ model:config:view）

## 2026-08-19: KB 图片解析接入多模态模型

- **决策**: 新增 `kb_image` use case，图片解析（多模态）与文本分片（kb_chunking）解耦，每 use case 恰好一个默认模型
- **模型**: `qwen-vl-max`（DashScope 兼容端点 `https://dashscope.aliyuncs.com/compatible-mode/v1`），API key 存 Vault，`max_tokens=8192`，设为 kb_image 默认
- **理由**: deepseek-v4-pro 是纯文本模型（不支持 image_url）；文本 chunking 继续用 deepseek，图片描述用 qwen-vl-max
- **链路**: 图片 base64 上传 → `file_type=image` → kb_index 走 `kb_image` → qwen-vl-max 多模态解析 → chunk → embed → Qdrant + MongoDB
- **备注**: 多模态模型名在第三方平台常有别名（DashScope 叫 `qwen-vl-max`，开源叫 `Qwen2.5-VL-72B`），配模型前先查平台模型列表，不要照抄现有配置的 name/max_tokens

## 2026-08-28: 模型配置页 UI 优化 + 运维 502 排查确认

### 三个设计确认（晓军：实现符合预期，不改）
1. **nginx 502 不需要改**：单容器重启换 IP 后 nginx 缓存旧 IP 导致 502，`nginx -s reload` 即可修复。晓军明确不加固定 IP / resolver——正常部署整集群重建，生产可能用 k8s 而非 docker compose，此为测试环境单容器重启的临时现象。
2. **API key 明文返回符合预期（admin UI trusted）**：`/models/list` + `/models/embedding` 后端返回明文 key，mask 由前端视觉层做（`keyDisplay = m.api_key ? (isRevealed ? m.api_key : MASK) : '未设置'`），**无**单独 decrypt 接口、**无** `api_key_exists` 字段。`modelconfig.go` 注释里「masked/decrypt endpoint」是历史遗留未更新，勿据此误判。
3. **默认模型 fallback 非随机**：`GetModelByUseCase` / `GetDefaultEmbeddingModel` 无 `model_defaults` 记录时 fallback 到「第一个」该类型模型（Type==llm / Type==embedding）；完全没有该类型才报错。compaction 另有 `defaultCompactionMaxTokens=4000` 兜底。

### 前端两个修复
- **Use Case 下拉框 Portal**：`.glass` 的 `backdrop-filter: blur(20px)` 会创建 containing block，`position:fixed` 实际相对 glass 容器定位导致错位。改用 React Portal 渲染到 `document.body`（见 REUSABLE_PATTERNS.md）。
- **Embedding 默认状态判断**：后端 `ModelEntry` 只有 `IsDefaultFor []string` 无 `IsDefault bool`，前端用 `m.is_default` 判断导致永远显示「设为默认」。改 `!!m.is_default || (m.is_default_for || []).includes('embedding')`。

### commit 时间线
538661e（UI 首次优化）→ 9ba0f85（Portal + 列宽压缩）→ 080b168（列宽再压缩 + nowrap）→ a51b1c0（embedding 默认判断修复）。
