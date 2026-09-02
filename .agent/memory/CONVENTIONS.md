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
6. **主键约定（铁律）**：`_id` 一律用 uuid（或 MongoDB ObjectId），**不得**承载业务语义（key/use_case/name 等）。业务标识一律单独设字段 + 唯一索引。唯一例外：必须存在 seed 数据且存在关联数据（如 rbac 角色 id）时，业务标识可用语义化字段值，但**仍不作为 `_id`**。
7. MongoDB 时间字段统一使用 `time.Time`，不存储 Unix 时间戳
8. 结构体 JSON tag 使用 snake_case，BSON tag 与 MongoDB 字段名一致

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
| 13 | Go UT Success 测试只验证 `err == nil` 不验证实际操作结果 | 每个 Success 测试必须包含 ≥2 个行为验证断言（验证写入的字段值、状态变更、副作用等） |
| 14 | Handler 测试用 `gomonkey.ApplyMethodReturn` mock service 方法 | 使用 `ApplyMethodFunc` 验证 handler→service 的参数传递正确性（`req.Username`、`req.Password` 等字段） |
| 15 | Go UT 使用 `t.Skip()` 绕过不可测场景 | 如确实不可测（如 WebSocket Hijacker），必须文档注释说明原因并记录到 spec |
| 16 | L1 纯逻辑包（`logic/sql`, `logic/openapi`, `logic/report`, `config` 等）无测试或覆盖率不足 | L1 包必须 **100%** 覆盖率，CI `ut-workflow.yml` 98% gate 强制执行 |
| 17 | **降级/删除功能以通过测试** | 测试挂 → 修实现 bug 或修测试基础设施，**严禁删除功能**。功能设计是深思熟虑的，测试是保护功能的。 |
| 18 | **绕过用户的技术方案自搞一套** | 用户给的技术方案必须先严格执行。被技术限制卡住 → 解释限制并询问，**不要静默替换成简单/hack 方案** |
| 19 | **测试里凭想象写 data-testid** | 写 `data-testid="xxx"` 前必须 `grep -r xxx frontend/` 确认元素存在。避免一轮 CI 白跑 |
| 20 | **MongoDB `$push` 字段名与 Go struct bson tag 不一致** | `$push` 的目标 MongoDB 字段名必须与对应 Go struct 的 `bson:"field_name"` tag 完全一致 |
| 21 | **MongoDB 文档初始化 array 字段用 nil** | 所有需要 `$push` 的 array 字段必须在 Create 时初始化为空数组 `[]*T{}`，不能依赖 nil → `$push` 报错 |
| 22 | **前端 `NEXT_PUBLIC_API_URL` 用 Docker 内部 hostname** | 前端 client-side API 调用必须用相对路径（走 nginx 代理）或公网地址，浏览器无法解析 Docker 内部名称 |
| 23 | **Domain struct 新增字段不同步更新 converter 序列化/反序列化** | 新增 domain 字段时必须同时更新 `taskDefToDoc`/`docToTaskDef` 等所有序列化路径。Go zero value 不会 panic/报错 → 字段默默为空值。 |
| 24 | **Docker 部署不先删除旧容器和镜像** | `--no-cache` 只控制构建层缓存。部署流程必须：`docker rm -f <container>` + `docker rmi -f <image>` + `rm -rf .next` → `build --no-cache` → `up -d` |
| 25 | **部署后不检查容器内 binary 时间戳** | 部署验证最后一步必须是 `docker exec <container> ls -la /path/to/binary`，不只是 `curl HTTP` 状态码。旧容器可能 start 旧的 binary。 |
| 26 | **DB 手动改数据不按 Go model struct 全字段对齐** | mongosh 手动插入/更新记录时必须与对应 Go struct 的 bson tag 全字段对齐。漏字段 = Go 读为零值 = 功能静默失效且无报错。新增数据优先走 seed 幂等插入。 |
| 27 | **Scheduler/轮询器只启动加载，不运行时 reload** | 所有定时器/轮询器必须有从数据源动态刷新的机制。运行时可能创建新任务/修改参数。 |
| 28 | **新增分支逻辑后只更新部分 if 条件** | 新增 `one_time` 模式后校验只检查 `!newTask.cron` → 一次性定时被错误拦截。所有涉及相同分支的条件必须全部覆盖。 |
| 29 | **消息队列用平铺 struct 或 map[string]interface{}** | 使用 `{type, payload: json.RawMessage}` envelope。worker 用 switch type + json.Unmarshal per-type，清晰可扩展。 |
| 30 | **Git checkout 回退时不检查是否丢失其他变更** | checkout 前必须 `git diff` 确认。Python heredoc 写好的修复可能被无差别回退。 |
| 31 | **用户给的具体条件（含比较运算符）只做一半** | 用户说 `<= 当前时间` 就必须有 `$lte: now`，不能降级为 `$exists: true` 或 `$ne: nil`。逐字翻译，不打折扣。 |
| 32 | **新增过滤字段只在 write 端加、漏掉 read 端的 query filter** | toggle 写入 `scheduled_enabled=false` 后，ListScheduled 也必须加 `scheduled_enabled: {$ne: false}`。write 和 read 是两次独立操作，必须同步。 |
| 33 | **Go interface 签名变更不问全链路就声称完成** | 改 repo 接口签名 → grep 所有 implement + 所有 caller。adapter、provider interface、两个调用处一个都不能漏。 |
| 34 | **`_id` 用业务语义字段（key/use_case/name 等）当主键** | `_id` 一律 uuid，业务字段单独设字段 + 唯一索引。唯一例外：seed 数据 + 有关联数据可用语义化字段值，但仍不作 `_id`。 |
| 35 | **前端判断状态不先确认后端 DTO 实际字段** | 写前端条件判断前，先看后端 struct 的 json tag + `curl` 实测，确认字段名/语义（如 `is_default_for []string` 而非 `is_default bool`）。字段契约不一致时 JSON 缺字段不报错、静默失效，状态永不刷新。 |
| 36 | **改实现不同步更新代码注释** | 注释里声明的行为（如 "masked / decrypt endpoint"）必须与代码一致。改实现必须同步注释，否则过时注释成为「假约束」，误导后续排查（把符合预期的行为误判成 bug）。 |
| 37 | **展示字段（description 等）前端硬编码兜底** | 描述/标签/枚举说明等展示字段必须作为 DB 字段 + seed 同步，前端直接消费后端返回值。前端硬编码兜底 = 双份真相，后端改了前端静默失效。 |
| 38 | **查 MongoDB 前不确认真实 db name** | 本项目数据库名是 `data_agent`（下划线），不是仓库名 `data-agent`（连字符）。查库前先 `docker exec <mongo> env | grep MONGO` 确认连接串，否则查到空库误判「数据被删」。 |
| 39 | **加唯一索引前不查重复数据** | 对已有数据集合加唯一索引前，必须先 `aggregate $group` 查重复 key，否则 `ensureIndex({unique:true})` 因 E11000 直接失败。 |
| 40 | **前端判断「默认」不用 use_case scope** | `is_default_for` 是 per-use-case 的。chat 下拉只标 chat 默认，否则多个 use_case 默认全标上 → 下拉出现「多个默认」。 |
| 41 | **同类弹窗/面板视觉混用不同 class（`.glass` 半透明 vs 实色 `var(--bg-secondary)`）** | 弹窗面板必须统一视觉：遮罩 `rgba(0,0,0,0.6)`+blur(4px)、面板 `var(--bg-secondary)` 实色 + `var(--border-glass)` 边框 + 16px 圆角 + `0 8px 32px rgba(0,0,0,0.5)` 阴影。集中用 `components/ui.ts` 的 `modalOverlayStyle`/`modalPanelStyle` 常量，禁用 `.glass` 用于弹窗面板（半透明+blur(20px) 与实色面板观感不一致）。 |
| 42 | **视觉/样式回归只凭源码 class 名判断** | 样式一致性回归必须读 `getComputedStyle` 运行时计算值（定位类型+颜色值+尺寸阈值组合定位元素），Tailwind `bg-black/60` 等类名的最终值、`position:fixed/absolute` 差异只能在运行时确定。 |
| 43 | **验证 API 前不确认后端 DTO 字段名（json tag）** | 凭前端 input testid/placeholder 猜字段名 curl 验证 → 400 误判成 bug。先看后端 struct json tag + curl 实测。400 带字段级错误是正常参数校验，读错误里的字段名。 |
| 44 | **判断镜像是否含新代码只信 CACHED 状态** | BuildKit `COPY . .` 会偶发误判 CACHED。判断镜像新旧必须查镜像内编译产物（grep `.next` / stat mtime），不凭构建日志 CACHED 标记。cache 误判用 `--no-cache` 绕过。 |
| 45 | **构建机连不上 npm 官方源时不配镜像** | 服务器连不上 `registry.npmjs.org`（curl 000）时，Dockerfile 必须显式 `npm ci --registry=https://registry.npmmirror.com`，否则 no-cache 构建卡死在依赖下载。 |

## 开发工作流约定

### 标准开发流程
1. 开发功能 → 代码 push 到分支
2. 晓军手动创建 PR
3. 通过 PAT 轮询等待 GitHub Actions CI（ut-workflow → sonar-check → ui-tests）
4. CI 失败则分析日志 → 修复代码 → push → 回到步骤 3
5. CI 全部通过 → 合并

### 轮询参数
- 轮询间隔：120s
- 使用 `gh api` 或 `curl` + PAT 调用 GitHub REST API
- PAT 从 `.github-pat` 文件读取（已排除 git 版本控制）
