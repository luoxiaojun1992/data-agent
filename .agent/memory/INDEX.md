# DataAgent - 工作记忆索引

> 所有记忆文档的主索引。每个维度拆分为独立文件。

## 文档索引

| 文档 | 内容 |
|----------|---------|
| [ARCHITECTURE.md](./ARCHITECTURE.md) | 系统架构、关键文件、模块交互 |
| [CONVENTIONS.md](./CONVENTIONS.md) | 编码规范、分支命名、提交规范、开发工作流、红线 |
| [LESSONS_LEARNED.md](./LESSONS_LEARNED.md) | 工程教训——所有已纠正的错误和方法论（测试/覆盖率/工具/CI） |
| [MEMORY.md](./MEMORY.md) | 工程决策日志（按日期追加） |
| [E2E_TESTING.md](./E2E_TESTING.md) | E2E 测试模式、data-testid 规范、用例矩阵（MVP 占位） |
| [REUSABLE_PATTERNS.md](./REUSABLE_PATTERNS.md) | 代码片段与设计模式复用 |

### 测试规范

| 资源 | 用途 |
|------|------|
| `.github/workflows/ut-workflow.yml` | Go UT CI 门禁（98% 覆盖率 gate, `-race` flag） |
| `.agent/skills/go-ut-audit/SKILL.md` | Go UT 审计 skill（四维度检查） |
| `.agent/specs/spec-045-go-service-ut.md` | Go UT 全覆盖 spec（L1/L2/L3 tier 要求） |

## 快速参考

### 核心架构
- **单二进制部署**: Go 服务编译为单一二进制，Worker/Scheduler 为同进程 goroutine
- **Agent Service 唯一入口**: Chat 和 Agent 模式共享同一 Agent Engine
- **三层架构**: Handler（参数校验）→ Service（业务逻辑）→ Repository（数据持久化）
- **Logic 层**: `internal/logic/` 抽取 Skill/Handler/Service 共用逻辑
- **SkillContext 注入**: Skill 通过 SkillContext 自动获取 Session/User/Task 绑定
- **幂等性**: 创建用 upsert，删除永远返回成功

### 技术栈要点
- **Go 1.22+** 后端，**React/Next.js** 前端
- **MongoDB** 业务主存储，**Qdrant** 向量搜索，**SeaweedFS** 对象存储
- **Redis** 缓存 + Stream 消息队列
- **HashiCorp Vault** 密钥管理
- **go-lark/lark** 飞书 SDK

### 关键约束
- **SQL 仅允许只读**: SQL Executor 通过 AST 解析拦截所有写入操作
- **Session 归属不可篡改**: SkillContext 自动注入，Skill 不接受外部 session_id
- **幂等删除**: 删除不存在的资源必须返回成功

### 红线
- **禁止 workaround** — 所有问题必须做根因分析
- **禁止绕过测试** — 不允许放宽断言让测试通过
- **禁止直连数据库** — 必须通过 Repository 层
- **保留孤儿代码清理** — 未使用的 import/变量/函数必须删除
