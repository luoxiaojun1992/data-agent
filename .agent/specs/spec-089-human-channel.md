# LLM 人机交互独立信道（Human Channel）：工具授权 + 用户提问

> **SPEC-089** | Status: 设计中

## 1. 目标

在现有「用户 ↔ LLM 聊天信道（chat SSE）」之外，开辟一条**独立的、关联 chat session 的人机交互信道（Human Channel）**。该信道承载两类「LLM → 用户 → LLM」的交互：

1. **工具授权（confirm）**：LLM 执行敏感操作（文件/目录删除）前，通过信道询问用户是否执行，按授权结果决定是否真正执行。
2. **用户提问（ask）**：LLM 主动向用户提问（可带候选选项），获取用户的回复（选项或自由文本）后继续。

信道由前端发起（SSE），询问以弹窗形式展现（样式与现有弹窗统一），回复经独立 API 回传。信道生命周期与 chat session 绑定：chat SSE 关闭时信道释放，重启 session 后重建。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-048 ADK 引擎迁移 | ✅ | 依赖 ADK function tool 机制（`agent.ToolContext` 实现 `context.Context`，tool 内可阻塞） |
| SPEC-062 多模型与 Session 绑定 | ✅ | chat session 创建/复用/归属校验逻辑复用 |
| SPEC-064 RBAC 权限管理 | ✅ | 信道挂 `PermChatView`（同 chat），`role` 从 session state 注入 |
| SPEC-066 配置存储拆分 | ✅ | ask_user skill 走 predefinedSkills + SeedSkills |
| SPEC-078 前端 UI 规范 | ✅ | 弹窗复用玻璃样式（`.glass` + blur20） |

> 全部 ✅，无阻塞。可立即开始。

## 2. 背景

现有 chat 是一个**单向**的流：用户发消息 → LLM 流式回复（`POST /api/v1/chat` stream=true，SSE 只从后端流向前端）。LLM 在 ReAct 循环中调用工具（如 `file_delete` / `dir_delete`）时，是**静默同步执行**的，没有「停下来征求用户意见」的环节。

带来的问题：

1. **危险操作无确认**：`file_delete` / `dir_delete` 直接 `fsops.RemoveFile/RemoveDir`（`internal/adk/tools/tools.go:1020/1048`），LLM 一旦误判就会不可逆删除用户文件，无任何人工确认。
2. **LLM 无法提问**：当 LLM 需要用户澄清（"你要分析哪个维度？A/B/C？"）时，只能靠文本回复结束本轮，无法阻塞等待用户答案后再继续推理，多轮体验差。

ADK v1.5.0 虽内置了 `ToolConfirmation`（`adk_request_confirmation`）机制，但它**仅支持 boolean 确认、且经 chat 事件流传递（非独立信道）**，无法满足「问问题带选项 + 文本回复 + 独立信道 + 前端发起」的需求。故本 spec 自研一条通用的人机交互信道，作为 ADK 内置机制的超集。

## 3. 架构概述

```
┌────────────────────────────────────────────────────────────────────┐
│                              前端 (chat/page.tsx)                    │
│                                                                      │
│  ① chat SSE  (POST /chat, fetch+getReader)  —— LLM 文本/tool 事件     │
│  ② human-channel SSE (GET /chat/:sid/human-channel) —— 询问事件        │
│  ③ reply API (POST /chat/:sid/human-channel/:reqId/reply) —— 用户回复  │
│  ④ 弹窗组件 (HumanChannelDialog, 复用 .glass 玻璃样式)                 │
└────────────────────────────────────────────────────────────────────┘
                │ ①                     │ ② (SSE)         │ ③ (POST)
                ▼                       ▼                 │
┌──────────────────────────────────────────────────────────────┐    │
│                       后端 (Go)                               │    │
│  chat.Service.Stream ──► rt.RunContent ──► function tool       │    │
│                                    │                          │    │
│                                    ▼                          │    │
│                    file_delete / dir_delete / ask_user        │    │
│                                    │ 调 HumanGate              │    │
│                                    ▼                          │    │
│                       ┌───────────────────────┐              │    │
│                       │  HumanChannel Hub      │◄─────────────┘    │
│                       │  map[sessionID]Channel │                    │
│                       └───────────────────────┘                    │
│                                │ 推送询问事件(②)                     │
│                                │ 阻塞等回复(③ 注入)                   │
└────────────────────────────────────────────────────────────────┘
```

**核心机制**：function tool 在内部调用 `HumanGate.Confirm/Ask`，该调用通过 in-memory Hub 把「询问事件」推送给该 session 的信道 SSE 订阅者，然后**阻塞等待**（`select` + reply channel + `ctx.Done` + 超时），用户在前端弹窗回复后经 reply API 注入回复，tool 继续执行。

### 与现有模块对比

| 维度 | 现有 chat 信道 | 新 Human Channel |
|------|--------------|-----------------|
| 方向 | 后端 → 前端（单向） | 后端 → 前端（询问）+ 前端 → 后端（回复） |
| 承载内容 | LLM 文本 / tool_call / tool_result 事件 | confirm / ask 两类交互事件 |
| 触发 | 每次用户发消息 | 仅 tool 内主动请求（授权/提问） |
| 生命周期 | 每次请求 | 关联 session，chat SSE 关闭即释放 |
| 阻塞语义 | 无（流式） | 有（tool 内阻塞等回复） |

## 4. API 设计

| Method | Path | 鉴权 | Description |
|--------|------|------|-------------|
| GET | `/api/v1/chat/:session_id/human-channel` | `PermChatView` | 建立信道 SSE，接收该 session 的询问事件 |
| POST | `/api/v1/chat/:session_id/human-channel/:request_id/reply` | `PermChatView` | 用户回复（confirm 传 `confirmed`，ask 传 `answer`） |

> 两个端点均挂在 `/api/v1/chat` 路由组下（复用 `routes.go:158-165` 的 `PermChatView` 中间件），与 chat 权限一致：**普通用户即可用**。

## 5. 详细设计

### 5.1 数据流

**授权（file_delete / dir_delete）**：

```
LLM 决定调 file_delete
  → function tool 执行
  → sessionWorkspace(tc) 解析 workspace
  → HumanGate.Confirm(ctx, sessionID, hint="删除文件 src/a.txt？")
      → Hub 找到 sessionID 的 Channel，生成 request_id，注册 pending[reqId]=replyCh
      → 推送 SSE 事件 {type:confirm, request_id, hint} 到信道订阅者
      → select 阻塞：
          case reply := <-replyCh:  返回 reply.confirmed
          case <-ctx.Done():         返回 false（chat SSE 断开，信道释放）
          case <-time.After(5min):   返回 false（超时默认拒绝）
  → confirmed == true ? fsops.RemoveFile(ws, path) : 返回"用户拒绝"错误
  → tool 结果回 LLM → LLM 继续/结束 → chat SSE [DONE]
```

**提问（ask_user）**：

```
LLM 决定调 ask_user(question="分析维度？", options=["按地区","按产品","按时间"])
  → ask_user tool 执行
  → HumanGate.Ask(ctx, sessionID, question, options)
      → 推送 SSE {type:ask, request_id, question, options}
      → 阻塞等 reply.answer（选项或自由文本）
  → 返回 {answer} 给 LLM
```

### 5.2 模块设计

#### 5.2.1 `internal/service/humanchannel`（新增）

```go
// Channel 表示单个 session 的人机交互信道实例。
type Channel struct {
    sessionID string
    mu        sync.Mutex
    subs      map[string]chan Event   // 订阅者 ID → 事件发送 chan
    pending   map[string]chan Reply   // request_id → 回复 chan
    nextReq   int64                    // 自增 request_id
}

// Hub 维护 sessionID → Channel 的映射。
type Hub struct {
    mu       sync.RWMutex
    channels map[string]*Channel
}

// Gate 是 function tool 与 handler 共用的阻塞式人机交互接口。
type Gate interface {
    // Confirm 阻塞请求用户确认，返回授权结果。
    Confirm(ctx context.Context, sessionID, hint string) (bool, error)
    // Ask 阻塞向用户提问（可带选项），返回用户回复（选项或自由文本）。
    Ask(ctx context.Context, sessionID, question string, options []string) (string, error)
}
```

**关键语义（红线）**：

- **阻塞 + 取消**：`Confirm/Ask` 用 `select` 阻塞，`ctx` 是 tool 的 `agent.ToolContext`（实现 `context.Context`，随 chat SSE 断开而 cancel）。chat SSE 关闭 → `ctx.Done()` → 立即返回「已取消/拒绝」，**不残留泄漏 goroutine**。
- **超时**：内置 `time.After(HumanChannelTimeout)`（默认 5 分钟，见决策点 D1），超时默认**拒绝**（confirm=false / ask 返回错误）。
- **信道释放**：Hub 在「无订阅者且无 pending 请求」时从 map 删除该 Channel；信道 SSE handler 退出（`ctx.Done`）时移除订阅者。
- **request_id 归属**：reply 必须校验 `request_id` 属于该 session 的 pending 集，防跨 session 注入。

#### 5.2.2 事件与回复协议（自定义，2 类）

**后端 → 前端（信道 SSE）**：

```json
// 授权
{"type":"confirm","request_id":"req_1","hint":"即将删除文件 src/report.docx，是否确认？"}
// 提问
{"type":"ask","request_id":"req_2","question":"请选择分析维度","options":["按地区","按产品","按时间"]}
```

**前端 → 后端（reply API）**：

```json
// confirm 回复
{"confirmed": true}
// ask 回复（选项或自由文本，二选一）
{"answer": "按地区"}
```

#### 5.2.3 `file_delete` / `dir_delete` 改造（`internal/adk/tools/tools.go`）

在现有 `sessionWorkspace(tc)` 解析之后、`fsops.RemoveFile/RemoveDir` 之前，插入授权：

```go
if deps.HumanGate != nil {
    ok, err := deps.HumanGate.Confirm(tc, sessionID, fmt.Sprintf("删除文件 %q？", args.Path))
    if err != nil { return FileDeleteResult{}, fmt.Errorf("file_delete: 授权失败: %w", err) }
    if !ok { return FileDeleteResult{}, fmt.Errorf("file_delete: 用户拒绝删除 %q", args.Path) }
}
```

> `dir_delete` 的 hint 明确「递归删除目录及其全部子项」。**仅 file_delete/dir_delete 挂授权**，file_write/file_read/dir_create/dir_list/save_artifact 不挂（需求第 3 点只指定删除类）。

#### 5.2.4 `ask_user` skill（新增）

- 输入：`question`（必填）、`options`（可选 `[]string`，最多 10 个）。
- 输出：`{answer string}`。
- 实现：`HumanGate.Ask(tc, sessionID, question, options)`，返回用户回复。
- 若 `options` 非空，弹窗渲染为单选选项 + 自由文本兜底；为空则纯文本输入框。

#### 5.2.5 Deps 扩展（`internal/adk/tools/tools.go`）

```go
type Deps struct {
    // ...existing...
    // HumanGate backs the confirm/ask human-in-the-loop interactions.
    // Nil → file_delete/dir_delete 直接执行（兼容），ask_user 返回错误。
    HumanGate HumanGate
}
```

> `HumanGate` 为 tools 包内定义的最小接口（`Confirm` + `Ask`），实现由 `service/humanchannel` 提供，wire 注入。

### 5.3 信道生命周期（需求第 5 点）

| 事件 | 后端动作 |
|------|---------|
| 前端 `GET /chat/:sid/human-channel` | Hub 为 session 注册 Channel + 新增订阅者 |
| 前端断开信道 SSE（chat SSE 关闭 / 页面卸载 / 网络断开） | handler `ctx.Done` → 移除订阅者；无订阅者且无 pending → Hub 删除 Channel |
| tool 阻塞中 chat SSE 关闭 | tool 的 `ctx.Done()` → `Confirm/Ask` 返回「已取消」→ tool 返回错误 → chat 结束 |
| 重启 session 再次发起 | 重新 `GET` 建立信道，重新注册 |

> **信道是进程内存态**，不落 DB。session 持久化仍由 chat session（Mongo）承担，信道只做「当前活跃对话期间」的实时交互，天然符合「chat sse closed 释放、重启重建」。

### 5.4 RBAC 与数据隔离（需求第 9 点）

- **权限**：两个端点挂 `PermChatView`，与 chat 完全一致，**普通 user 即可用**。
- **session 归属校验（防 IDOR）**：handler 加载 `session := sessions.Get(sessionID)`，校验 `session.UserID == userID || role == "system_admin"`（复用 chat `useExistingSession` 的归属语义 + 保留 system_admin 豁免），不匹配 → 403。
- **request_id 归属**：reply 校验 `request_id` 存在于该 session Channel 的 pending 集，防跨 session 注入回复。
- **system_admin 豁免**：`role` 从 JWT claims / session state 注入，`role == "system_admin"` 可操作任意 session 的信道（与 memory/task/kb skill 的豁免原则一致）。

### 5.5 seed 同步（需求第 8 点）

- 新增 `ask_user` 同步进 `internal/service/skill/config.go` 的 `predefinedSkills()`，`SeedSkills` 启动幂等自动补齐（全新空 DB 自动插入、存量 DB 增量补齐）。
- `file_delete`/`dir_delete` 是**已存在**的 skill（修改其行为，不新增 skill 条目），无需 seed 变更。

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（信道为进程内存态；session 复用 Mongo） |
| 是否影响现有 API | No（新增 2 端点；chat/agent/task 路由不变） |
| 性能影响 | 低（每 session 一次 SSE 长连接 + 内存 map；tool 阻塞仅影响当前 session 的 run goroutine） |
| 是否需要新增 Skill | Yes（`ask_user`；`file_delete`/`dir_delete` 为行为改造） |
| ADK 侵入 | 无（tool 内用 `agent.ToolContext` 阻塞，不碰 ADK runner/llmagent） |
| 并发/泄漏 | 可控（`select` + ctx 取消 + 超时 + pending 清理，无泄漏 goroutine） |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/service/humanchannel/hub.go` | New：Hub/Channel/Gate 实现 | New |
| `internal/service/humanchannel/gate.go` | New：Confirm/Ask 阻塞逻辑 | New |
| `internal/api/handler/humanchannel.go` | New：信道 SSE + reply handler | New |
| `internal/api/handler/routes.go` | 注册 2 端点（挂 `PermChatView`） | Small |
| `internal/adk/tools/tools.go` | Deps 加 `HumanGate`；file_delete/dir_delete 授权；新增 ask_user | Medium |
| `internal/service/skill/config.go` | predefinedSkills 加 ask_user | Small |
| `cmd/server/wire.go` | 构建 Hub/Gate，注入 Deps + handler | Medium |
| `frontend/app/chat/page.tsx` | 建立信道 SSE + 弹窗组件接入 | Medium |
| `frontend/app/components/HumanChannelDialog.tsx` | New：弹窗（confirm/ask 两态，复用 `.glass`） | New |
| `frontend/lib/api.ts` | 新增 human-channel 相关请求封装 | Small |

## 8. 测试策略

1. **Unit tests**（Go）：`service/humanchannel`（Hub 注册/释放、Confirm/Ask 阻塞/超时/取消、request_id 归属）L1 纯逻辑 100%；`handler/humanchannel`（session 归属校验、PermChatView、403/404/200）L3 98%；`tools.go`（file_delete 授权通过/拒绝、ask_user 返回、HumanGate nil 兼容）L3 98%。
2. **Integration tests**：条件使用 Docker Compose 环境验证 SSE 长连接 + reply 注入全链路。
3. **E2E tests**：用例编号 `UI-XXX`（见 §9）。
4. **审计**：`.agent/skills/go-ut-audit` 审查 UT 质量。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性（弹窗、确认/拒绝按钮、选项、文本输入）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `service/humanchannel`（Hub/Gate 纯逻辑） |
| L2 | 依赖接口，可 mock | **100%** | Gate 接口 mock |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `handler/humanchannel`, `tools.go` |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc` 验证 handler→service 参数传递
- [ ] **必须** Service 测试的写操作验证写入内容的字段和值
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景
- [ ] **严禁** Success 测试只验证 `err == nil`

### CI 门禁

- [ ] `go test -race -gcflags=all=-l -coverprofile=coverage.out ./internal/... ./skills/...` 全部通过
- [ ] 覆盖率 ≥ 98%
- [ ] `go vet` 无警告

## 10. 验证标准

1. **信道建立**：`GET /chat/:sid/human-channel` 返回 SSE 流（`text/event-stream`），`X-Accel-Buffering: no`。
2. **授权通过**：chat 中 LLM 调 file_delete → 前端弹窗出现（含 hint）→ 用户点「确认」→ 文件被删 → LLM 收到 tool 结果继续。
3. **授权拒绝**：用户点「拒绝」→ 文件**未删** → LLM 收到「用户拒绝」错误。
4. **提问**：LLM 调 ask_user(question+options) → 弹窗渲染选项 → 用户选「按地区」/ 输自由文本 → ask_user 返回对应 answer。
5. **信道释放**：chat SSE 断开（如前端停止）→ 阻塞中的 tool 立即返回「已取消」，Hub 中该 session Channel 被清理，无残留。
6. **seed 同步**：全新空 DB 启动后 `SeedSkills` 自动补齐 `ask_user`（`/skills` 可见、`skill_search` 可搜到）；存量 DB 增量部署后同样补齐。
7. **RBAC 隔离**：普通 user 可建立信道 + 回复；跨用户访问他人 session 信道 → 403；system_admin 豁免可访问；request_id 不属于该 session → 404/403。
8. **回归**：file_write/file_read/dir_create/dir_list/save_artifact 行为不变（不挂授权）；chat/agent/task 现有路由与流程不变。

## 11. 待拍板决策点

| # | 决策点 | 推荐 | 备选 |
|---|--------|------|------|
| D1 | 授权/提问超时默认值 | **5 分钟**（超时默认拒绝） | 30s / 10 分钟 |
| D2 | task/异步模式（无前端信道）下 file_delete 行为 | **拒绝（deny）**，避免无人确认的破坏性删除 | 跳过授权（trust） |
| D3 | ask_user 在 task/异步模式 | **返回错误**（无前端可交互） | 跳过并返回空 |
| D4 | 授权粒度 | **file_delete/dir_delete 均挂**（每次删除都确认） | 仅 dir_delete 递归删挂 |
