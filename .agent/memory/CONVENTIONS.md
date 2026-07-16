# DataAgent - 编码规范

## 编码风格

- **语言**: Go 1.22+
- **模块系统**: Go Modules
- **Linter**: golangci-lint
- **格式化**: gofmt / goimports
- **日志**: uber-go/zap 结构化日志
- **配置**: Viper (YAML + 环境变量覆盖)

### Go 特定规则

1. 使用 `internal/` 包组织私有代码，禁止外部包直接引用 `internal/`
2. Repository 模式封装所有数据访问，Service 层不直接操作 MongoDB/Redis
3. 错误通过 `fmt.Errorf("context: %w", err)` 包装传递，保留错误链
4. 所有公开函数必须有文档注释（`// FunctionName does X.`）
5. UUID v4 主键由应用层生成，格式: `{prefix}_{uuid}`（如 `task_3f7a2b1c-...`）
6. MongoDB 时间字段统一使用 `time.Time`，不存储 Unix 时间戳
7. 结构体 JSON tag 使用 snake_case，BSON tag 与 MongoDB 字段名一致

### 错误处理
- Handler 层校验失败返回 4xx，不进入 Service 层
- Service 层错误统一包装后返回，不做 panic
- Repository 层返回原始错误，由 Service 层包装上下文
- 使用 `internal/pkg/errcode/` 统一错误码

### 三层架构（Handler → Service → Repository）

```
Request → Handler (入口层) → Service (逻辑层) → Repository (存储层)
              │                    │                   │
     · 参数解析与校验         · 业务逻辑编排       · 数据持久化
     · 权限前置检查           · 跨模块协调         · 缓存操作
     · 响应格式化             · 事务管理           · 外部API调用
```

### Handler 层必须完成的校验

| 校验项 | 说明 | 示例 |
|--------|------|------|
| 参数存在性 | 必填字段非空检查 | `title` 不能为空 |
| 参数类型 | 字段类型匹配 | `retry_count` 必须为整数 |
| 参数范围 | 数值/枚举范围检查 | `report_type` 必须在预置枚举中 |
| 参数长度 | 字符串长度限制 | `content` 不超过 100KB |
| 参数格式 | 格式合规检查 | `email` 符合邮箱正则 |
| 权限校验 | 当前用户是否有操作权限 | RBAC `agent:create` 权限 |

### Logic 层设计原则

1. **无状态** — Logic 结构体不持有请求级状态，可安全并发调用
2. **接收 SkillContext 或 context.Context** — 不绑定 HTTP 或 Skill 特化上下文
3. **返回 Domain Model** — 不返回 HTTP 响应格式，由上层自行组装
4. **幂等性内置** — MongoDB upsert 在 Logic 层实施，上层无需关心重复调用

### 幂等性规范

**创建幂等**: 使用 MongoDB `$setOnInsert` upsert，相同参数多次调用不会产生多条记录。

**删除幂等**: 删除不存在或已删除的资源直接返回成功，**绝不返回 404**。所有 `DELETE` 端点遵循此规则。

**回滚模式**: 跨资源创建失败时，best-effort 清理已创建的子资源。

## 提交规范

```
type: description

Types: feat, fix, docs, test, refactor, chore, style
```

| 类型 | 用途 |
|------|------|
| `feat` | 新功能 |
| `fix` | 修复 Bug |
| `docs` | 文档更新 |
| `test` | 测试相关 |
| `refactor` | 重构（无功能变化） |
| `chore` | 依赖更新、构建配置等杂项 |

## 分支命名

| 分支类型 | 格式 | 示例 |
|-------------|--------|--------|
| 功能 | `feat/<spec>-<desc>` | `feat/spec-002-auth-middleware` |
| 修复 | `fix/<description>` | `fix/sql-ast-parse-error` |
| 文档 | `docs/<description>` | `docs/api-update` |
| 杂项 | `chore/<description>` | `chore/deps-upgrade` |

## 红线（禁止行为）

| # | 行为 | 原因 |
|---|------|------|
| 1 | 不做根因分析的 workaround | 掩盖真实 Bug |
| 2 | 放宽测试断言让 CI 通过 | 测试失去意义 |
| 3 | 不经审批修改受保护代码 | 文档声明在 AI_AGENT_COMMON_INSTRUCTIONS.md |
| 4 | 直连数据库绕过 Repository 层 | 违反三层架构 |
| 5 | Skill 接受外部 session_id 参数 | Session 归属必须由 SkillContext 注入 |
| 6 | 返回 404 给资源不存在的 DELETE | 违反删除幂等性 |
| 7 | 在 Handler 层写业务逻辑 | Handler 仅做参数校验和响应格式化 |
| 8 | 前端 `catch { /* ignore */ }` 静默吞异常 | 必须 `console.error` 记录错误，否则 API 失败无法排查 |
| 9 | E2E 测试用条件断言（`if (visible) then test else skip`） | 条件断言 = 假通过，比不测更危险。不能确定性断言就删除测试 |
| 10 | 测试非确定性 UI 状态（后端依赖的按钮、瞬时 toast、异步任务进度） | 只测试页面渲染、导航、表单、模态框等确定性 UI |
| 11 | 有效测试只验证 "page header 存在" 而不验证实际行为 | 每个测试必须验证完整状态变更链（如创建→出现→取消→消失） |
| 12 | 测试失败后降低断言强度迁就 Bug | 修复根因（bug/race condition/缺失 error log），不准削弱测试 |

## 开发工作流约定

### 标准开发流程
1. 开发功能 → 代码 push 到分支
2. 晓军手动创建 PR
3. 通过 PAT 轮询等待 GitHub Actions CI（sonar-check → ui-tests）
4. UI test 失败则分析日志 → 修复代码 → push → 回到步骤 3
5. UI test 全部通过 → 完成

### 轮询参数
- 轮询间隔：120s
- 使用 `gh api` 或 `curl` + PAT 调用 GitHub REST API
- PAT 从 `.github-pat` 文件读取（已排除 git 版本控制）

## 已纠正的错误

> 记录过去的错误及其修复方案，用于团队学习。

| # | 日期 | 错误做法 | 正确做法 | 影响 |
|---|------|---------------|-----------------|--------|
| 1 | 2026-07-16 | 前端 `catch { /* ignore */ }` 静默吞异常 | `catch` 块至少 `console.error` 记录错误信息 | UI 测试超时 30s 无法定位根因 |
| 2 | 2026-07-16 | 测试中用 `page.goto` + `page.reload` 连环重载等 task row | 利用组件自带的 `loadTasks()` 自刷新，modal 关闭即断言 | 测试不稳定，频繁 timeout |
| 3 | 2026-07-16 | 条件断言 `if (await btn.isVisible().catch(() => false))` 静默跳过后端 API 故障 | 刚性 `expect().toBeVisible({ timeout })`，超时即 FAIL | 大量测试假通过，后端 bug 被掩盖 |
| 4 | 2026-07-16 | 用 `.catch(() => {})` 吞掉 `not.toBeVisible()` 失败 | 移除 `.catch()`，改用刚性断言 | 权限测试可能假绿色 |
| 5 | 2026-07-16 | 测试用 `page.route()` mock API 响应来测试 Chat 功能 | 使用 mockllm seed + 真实 SSE 流，走完整 Handler→Service→Repository 栈 | Chat 测试等于没测后端 |
| 6 | 2026-07-16 | 测试只验证 `agent-page-header` 可见就当"测试了取消行为" | 必须验证完整链：创建→row 出现→展开→点击 cancel→row 消失 | 假性测试，取消行为从未被验证 |
| 7 | 2026-07-16 | Agent UI-052 测试超时 → 移除 task row 断言迁就 Bug，而不是追查 `loadTasks()` 为什么没渲染 | 用后端日志定位根因，修复前端 `createTask` → `loadTasks` 链路的 bug，保留刚性断言 | 真 bug 被掩盖，后续修改可能再次触发 |
