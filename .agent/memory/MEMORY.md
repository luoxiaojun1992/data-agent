# DataAgent - 工程决策日志

> 按日期追加的工程决策记录。新条目追加在顶部，最新在前。

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
