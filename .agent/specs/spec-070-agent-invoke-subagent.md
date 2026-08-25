# agent 调用子 agent（subagent invocation）

> **SPEC-070** | Status: 调研完成（方案定为 sub agent tool + interface 解耦，暂不实现）

## 目标

让主 agent 能调用子 agent 完成子任务。核心约束（晓军 2026-08-24 提出，含并行拍板）：

1. 子 agent 实现为一个 **sub agent tool**（实现 ADK `tool.Tool` 接口），其 `Run` 里启动子 agent 的完整独立流程；
2. 子 agent 的能力提示词**单独组装**进 LLM 系统提示词，与普通 tool 的声明完全无关；
3. LLM 对子 agent 的调用**单独路由**（sub agent tool 触发 + 独立 agent 流程），不走普通 tool 执行；
4. 子 agent 的 session 在子 agent 返回后**销毁**（runtime 和 DB 都销毁）；
5. 只保留子 agent 的最终返回**写回主 agent 的 session**（同 tool response 的形态）；
6. 子 agent 使用的 model 与主 agent 一致；
7. **并行委派**（拍板）：子 agent 流程与主 agent 完全一致，唯一区别是被主 agent 触发。

## 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-066 配置存储拆分 | 📐 设计中 | 子 agent 的 model 复用主 agent 的 model 配置 |
| SPEC-067 意图识别 + 相关性检查 | 📐 设计中 | 子 agent 是否走 guard（intent/relevance）需明确 |
| SPEC-068 compaction 机制重构 | 📐 设计中 | 子 agent session 的 compaction 与「返回即销毁」的交互 |

## 背景 / 动机

### 现状：单 agent + 多 tool，无法表达多轮子任务

当前 data-agent 是「单 agent + 多个 ADK function tool」架构（`internal/adk/runtime/runtime.go` 一个 Runtime 绑一个 agent；`internal/adk/tools/tools.go` 注册 sql_executor / knowledge_search / stats_compute / pptx_generator / save_artifact 等）。所有能力都是单次调用、无状态的 tool。

但某些子任务需要**多轮 LLM + tool 编排**（如「生成一份完整分析报告」= 查数据 → 统计 → 生成 PPT → 保存产物），单个 tool 无法表达，需要能自主规划多轮的「子 agent」。

### 为什么 sub agent tool 依赖 agent 不会循环依赖（interface 解耦）

> 2026-08-25 晓军纠正：`runtime` 的 tool 注册用的是 **interface（`[]tool.Tool`）**，所以 sub agent tool 依赖 agent 包**不存在循环依赖**——此前「伪 tool 放上层包」是过度设计。

关键事实（代码实据）：

- `runtime.Config.Tools` 类型是 `[]tool.Tool`，其中 `tool` 是 **ADK vendor 的接口**（`google.golang.org/adk/tool`，见 `runtime.go:42`），**不是**我们自己的 `internal/adk/tools` 具体类型。
- `runtime` 包的 import 闭包只有 `internal/domain/security` 一个自家包（其余全是 ADK vendor），**不 import `internal/adk/tools`，也不 import `internal/logic/agent`**。
- 因此 sub agent tool 依赖 agent/runtime 是**单向**的：`sub agent tool → agent/runtime`，而 `runtime` 通过 `tool.Tool` 接口反向**不 import**任何 tool 实现包。

依赖方向图（无环）：

```
wire.go (cmd/server, 组装层)
   ├─ import runtime / tools / subagent
   └─ 组装: runtime.Config.Tools = [基础 tools…, sub agent tool]

sub agent tool
   ├─ import runtime (构建子 runtime)
   └─ import logic/agent (启动子 agent 完整流程)

logic/agent (executor/orchestrator)
   └─ import runtime

runtime
   ├─ Config.Tools []tool.Tool  ← ADK interface，不 import 任何 tool 实现包
   └─ import domain/security
```

→ 全链单向，无 `runtime → tools` / `runtime → subagent` 反向 import，**不存在 Go 包 import 循环**。sub agent tool 实现依赖 agent 没有问题。

> 仍需防护的是**运行时递归**（非 import 循环）：子 agent 的 tool 集若含 sub agent tool 本身，会无限委派。靠「子 agent 工具集裁剪（不含 sub agent tool）」斩断（见详细设计）。

### ctx 生命周期约束（2026-08-24 晓军补充）

1. **父子 agent ctx 必须是继承关系**：子 agent 的 ctx 是父 agent ctx 派生的子 ctx（`context.WithCancel(parentCtx)`），父 agent ctx 取消或超时 → 子 agent ctx 一并取消。
2. **子 agent ctx 取消必须同时销毁**：session（DB 记录）、runtime（内存），做到无残留。

## 调研结论（可行性）

### ADK 原生机制（vendor v1.5.0）

| 能力 | 位置 | 说明 |
|------|------|------|
| 子 agent 声明 | `agent/llmagent/llmagent.go:142` `SubAgents []agent.Agent` | 父 agent 声明可委派的子 agent |
| agent transfer 工具 | `internal/llminternal/agent_transfer.go:101` `TransferToAgentTool` | **伪 tool**：LLM 看到 `transfer_to_agent` function call，但 `Run` 只设置 `ctx.Actions().TransferToAgent`（:179），**不执行普通函数** |
| transfer 指令注入 | `agent_transfer.go:91` `AppendInstructions` | transfer 指令单独加进 prompt，与 tool 声明分开 |
| agent 切换 | `runner/runner.go:598` `findAgentToRun` | 根据 TransferToAgent action / 事件 author 切换到子 agent |
| session 管理 | `runner` | **共享 session**（子 agent 事件写入同一 session，author=子 agent 名） |

### 需求可行性评估

> 方案已定（2026-08-25）：**自定义 sub agent tool**（实现 `tool.Tool`，`Run` 启动独立 agent 流程），不用 ADK 原生 transfer/SubAgents（其共享 session、串行切换不满足需求 ④⑦）。

| 需求 | 可行性 | 结论 |
|------|:---:|------|
| ① sub agent tool（实现 tool.Tool） | ✅ | 自定义 tool，`Run` 启动子 agent 完整流程；无 import 循环（runtime 用 interface 注册） |
| ② 能力提示词单独组装 | ✅ | 子 agent 能力描述独立注入系统提示词，与 tool function declarations 分离 |
| ③ bypass 普通 tool，单独路由 | ✅ | sub agent tool 的 `Run` 走独立 agent 流程，非普通函数执行 |
| ④ 独立 session + 返回后销毁 | ✅ | 自定义：创建独立 session，完成/取消即 `Delete`（runtime + DB） |
| ⑤ 最终返回写回主 session | ✅ | `Run` 返回 map → ADK 自动包装 FunctionResponse 写回（同 tool response 形态） |
| ⑥ model 与主 agent 一致 | ✅ | Deps 注入 `modelcfg.Provider`，复用主 model 配置 |
| ⑦ 并行委派 | ✅ | `handleFunctionCalls` WaitGroup 天然并行执行多个 sub agent 调用 |

### 核心挑战

**「子 agent 独立 session + 返回后销毁」（需求 ④）+ ctx 继承取消清理，是需自定义实现的关键点。** 需要：

1. 为子 agent 创建**独立 session**（独立 sessionID，不复用主 session）；
2. 子 agent 执行完成（产出最终回复）后，**销毁 runtime 与 DB session**；
3. 把子 agent 的最终返回（最终 assistant 文本）作为 FunctionResponse **写回主 session**；
4. **ctx 继承 + 取消清理**：子 ctx 由父 ctx 派生，取消/超时同步销毁 session（DB）+ runtime（内存）。

其余需求（①②③⑤⑥⑦）均可基于 ADK 原生机制（`handleFunctionCalls` 并行、FunctionResponse 自动写回）或其扩展实现。

### 并行性调研（2026-08-24 晓军追问 + 拍板）

**runner 切换（transfer）是串行的**，不支持同时多个子 agent：

- `runner/runner.go:598` `findAgentToRun` 从 session events **从后往前**找「最后一个可识别 agent 的事件 author」，**一次只返回一个 agent**（控制权转移模型，返回类型是单个 `agent.Agent` 而非数组）。
- 若两个子 agent 事件交错写入 session，「最后写入者」随机决定下一个接管 agent，另一子 agent 的任务被忽略。

ADK 的并行路径是另一套：`agent/workflowagents/parallelagent`（静态工作流）：

| 维度 | runner transfer | parallelagent |
|------|----------------|---------------|
| 触发方式 | LLM 动态（transfer_to_agent 伪 tool） | **静态预定义**（预先配置跑哪些 SubAgents） |
| 并行性 | 串行，一次一个 | 并行（errgroup） |
| session | 共享 | 共享 + branch 隔离（`parent.child` 前缀） |
| 适用场景 | 委派一个子任务 | 多视角/多候选（多回复再评估） |

**晓军拍板（2026-08-24）：并行委派。子 agent 的流程完全和主 agent 一致，唯一区别是被主 agent 触发。**

### 可行性结论（2026-08-24 深入调研，确定可行）

关键机制：ADK `handleFunctionCalls`（`base_flow.go:1035`）用 `sync.WaitGroup` + goroutine（`:1052-1055`）**并行执行一个 LLM 响应里的多个 FunctionCall**。

因此「子 agent 调用」实现为**一个 sub agent tool**（实现 `tool.Tool`，`Run` 启动完整独立 agent 流程）：

| 需求 | 实现方式 | 可行性 |
|------|---------|:---:|
| 并行委派 | 多个子 agent 调用走 WaitGroup 天然并行 | ✅ |
| 子 agent 完整流程（与主 agent 一致） | sub agent tool 的 `Run` 启动**独立 session + 独立 runner + 完整 LLM 循环**，阻塞等最终结果 | ✅ |
| Go import 循环 | **不存在**：`runtime.Config.Tools` 是 `tool.Tool` interface，runtime 不反向 import tool 实现包 | ✅ |
| 运行时递归 | **子 agent 工具集裁剪**：不含 sub agent tool 本身 → 子 agent 不能委派，递归斩断 | ✅ 可控 |
| 独立 session + 返回销毁 | Run 里创建独立 session，完成后 `Delete`（runtime + DB） | ✅ |
| 结果写回主 session | Run 返回 map → ADK 自动包装 FunctionResponse 写回（同 tool response） | ✅ |
| model 与主 agent 一致 | Deps 注入 `modelcfg.Provider`，子 agent 复用主 model | ✅ |
| **是否需要改 ADK vendor** | **不需要**，全部在自定义 tool + DI 层实现 | ✅ |

**两个必须处理的工程点**：

1. **ctx 继承（非 detached）**：子 agent 的 ctx 是父 agent ctx 的**子 ctx**（`context.WithCancel(parentCtx)`）——父 agent ctx 取消/超时，子 agent ctx 一并取消。子 agent 执行循环监听 `ctx.Done()`，取消即停止；**取消时销毁 session（DB）+ runtime（内存）**。
2. **DI 扩展**：当前 `tools.Deps`（`internal/adk/tools/tools.go`）只有 KB/Skill/Memory/Tasks/SessionSvc/Artifacts/APICollections，需新增 `modelcfg.Provider`（子 agent 选模）+ adk session service（创建/销毁独立 session）+ runtime 构建能力。

**「子 agent 是 tool」的精确表述**：ADK 的 LLM 调用只有 function call 一条路，不存在「完全非 tool 的调用」。最终形态——**sub agent tool 实现 `tool.Tool` 接口，其 `Run` 不执行普通函数，而是启动与主 agent 完全一致的独立 agent 流程**。Go import 循环因 runtime 用 interface 注册而天然不存在；运行时递归靠子 agent 工具集裁剪斩断。

## 详细设计（方向，待实现时展开）

### 1. 子 agent 注册（sub agent tool + 独立流程）

- 子 agent 复用主 agent 的 model 与工具集（**工具集裁剪：不含 sub agent tool 本身**，斩断递归委派）。
- 主 agent 侧注册 sub agent tool（每个子 agent 一个，或统一的 `invoke_subagent` 带 `agent_name` 参数），实现 ADK `tool.Tool` 接口。
- sub agent tool 的 `Run` 启动子 agent 完整独立流程（独立 session + runner + LLM 循环），阻塞等最终返回。
- 子 agent 的能力描述单独维护，用于组装系统提示词（见下）。

### 2. 能力提示词组装

- 组装 LLM 系统提示词时，把「可用子 agent 及其能力描述」作为独立段落注入，与普通 tool 的 function declarations 分开（子 agent 是完整 agent 流程，不是函数）。
- 参考 ADK `AgentTransferRequestProcessor` 的 `AppendInstructions` 机制（`agent_transfer.go:91`），但提示词内容为「子任务委派」语义（委派后等待返回），而非「控制权转移」。

### 3. 调用路由

- 形式上：sub agent tool 挂进主 agent 的 function declarations（LLM 通过 function call 触发），能力描述单独注入系统提示词（与普通 tool 无关）。
- 执行上：sub agent tool 的 `Run` **不执行普通函数**，而是启动子 agent 的独立完整流程（独立 session + runner + LLM 循环），阻塞等待最终返回——语义为「子任务委派 + 等待返回」。
- 并行：多个子 agent 调用由 `handleFunctionCalls` 的 WaitGroup 天然并行执行。
- **Go import 循环防护**：不存在循环——`runtime.Config.Tools` 是 `tool.Tool` interface（ADK vendor），runtime 不 import 任何 tool 实现包，sub agent tool 依赖 agent/runtime 是单向的。
- 运行时递归防护：子 agent 工具集裁剪，不含 sub agent tool（子 agent 不能再委派，限制委派深度）。

### 4. session 生命周期（独立 + 父绑定 + 销毁 + ctx 继承）

#### 4.1 session 存储结构（加父 sessionID）

- `sessionDoc`（`internal/adk/session/mongo.go`，ADK 底层 session 存储）新增字段 **`parent_session_id`**（`bson:"parent_session_id"`，可空）：
  - 主 agent session：`parent_session_id` 为空；
  - 子 agent session：`parent_session_id` = 主 agent sessionID，绑定父子关系。
- 加索引 `{parent_session_id: 1}` 支撑级联删除查询。

#### 4.2 sub agent 调用 session 方式（同 chat session）

- sub agent tool 的 `Run` 里**复用与 chat session 相同的 ADK session service 流程**创建独立 session（独立 sessionID / runtime），`parent_session_id` 设为主 agent sessionID。
- **接收主 agent 的 ctx**：子 agent 执行循环直接用父 agent 传入的 ctx（或其派生子 ctx `context.WithCancel`），监听 `ctx.Done()` 处理 cancel。

#### 4.3 删除策略（硬删除，无软删除/恢复）

- **子 session 实时删**：子 agent 返回（产出最终回复）即 `Delete` 独立 session，硬删（`DeleteOne`），不留软删除标记。
- **主 session 级联删**：删除主 session DB 记录时，先查 `parent_session_id == 主sessionID` 的关联子 session，若有则一并**硬删除**（`DeleteMany`）。
- **取消清理**：子 agent ctx 取消/超时时，同步硬删子 session（DB）+ runtime（内存），无残留。
- ⛔ **全程硬删除，不用软删除、不用恢复**：子 session 与主 session 的删除都不写 `deleted_at` 标记、不进恢复列表，`DeleteOne`/`DeleteMany` 直接物理删除。
  - 注意区分：chat 业务层 session（`internal/service/chat/session.go` 的 `Manager`，`repository.SessionRepository`）有软删除（`Restore`/`ListDeleted`）；但**子 agent session 只落在 ADK session 层（`adk_sessions`），不落 chat 业务层 session**，故天然无软删除。子 agent session 走 ADK session service 的 `Delete`（本就是 `DeleteOne` 硬删）。

#### 4.4 其余

- 最终返回写回主 session（同 tool response 的形态）。
- 待明确：子 agent session 的 compaction 是否启用（生命周期短，可能不需要）。

### 5. 返回写回

- 子 agent 的最终返回（最终 assistant 文本）作为 FunctionResponse 写回主 session，主 LLM 像看到普通 tool 结果一样继续（ADK 自动包装）。

### 6. model 复用

- 子 agent 的 model 与主 agent 一致（复用同一 model 配置，不额外选模）；Deps 注入 `modelcfg.Provider`。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（子 agent session 复用 `adk_sessions`，加 `parent_session_id` 字段 + 索引，完成即硬删） |
| 是否影响现有 API | 待定 |
| 性能影响 | 子 agent 独立 session 的创建/销毁开销；runtime 复用主 agent 的模型连接 |
| 是否需要新增 Skill | No（是 agent 运行时能力，非 tool skill） |
| 是否需要改 ADK vendor | **No**（sub agent tool + DI 层实现，无需 vendor 补丁） |

## 相关文件（待实现时明确）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/tools/` 或新建 `internal/adk/subagent/` | **新增 sub agent tool**（实现 `tool.Tool`，依赖 agent/runtime，无 import 循环） | 待定 |
| `internal/adk/runtime/*` | 子 agent 独立 runtime 构建（复用主 model + 裁剪工具集） | 待定 |
| `internal/adk/session/mongo.go` | `sessionDoc` 加 `parent_session_id` + 索引；`Delete` 改为级联硬删子 session；子 agent session 创建/销毁 | 待定 |
| `internal/logic/agent/*` | 子 agent 调用编排（executor/orchestrator） | 待定 |

## 测试策略（待展开）

1. **Unit tests**（Go）: 子 agent 路由、session 创建/销毁、返回写回的单测。
2. **Integration / E2E**：主 agent 委派子 agent 完成多轮子任务的端到端验证；子 agent 返回后 session 销毁验证（DB 无残留）。

## UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 若影响前端展示（子 agent 调用链/消息），同步更新对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试

## Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

- [ ] 子 agent 相关新增/修改逻辑的 UT 覆盖率达标
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因）

## 验证标准（待展开）

- [ ] 子 agent 实现为 sub agent tool（实现 `tool.Tool` 接口），`Run` 启动独立 agent 流程（非普通函数执行）
- [ ] sub agent tool 依赖 agent/runtime 无 Go import 循环（runtime 经 `tool.Tool` interface 注册，不反向 import）
- [ ] 能力提示词单独组装，与普通 tool 声明分离
- [ ] **并行委派**：多个子 agent 调用并行执行（WaitGroup），子 agent 流程与主 agent 一致
- [ ] 子 agent 返回后 runtime 与 DB session 均销毁（无残留）
- [ ] **父绑定**：`sessionDoc` 加 `parent_session_id`，子 session 绑定主 sessionID
- [ ] **级联硬删**：删除主 session 时查 `parent_session_id == 主sessionID` 并硬删全部子 session
- [ ] **硬删除（无软删除/恢复）**：子 session 实时删或随主删，均为 `DeleteOne`/`DeleteMany` 物理删除，不写 `deleted_at`、不进恢复列表
- [ ] 最终返回写回主 session（同 tool response 形态）
- [ ] 子 agent model 与主 agent 一致
- [ ] **ctx 继承**：父 agent ctx 取消/超时，子 agent ctx 一并取消
- [ ] **取消清理**：子 agent ctx 取消时同步销毁 session + runtime + DB 记录（无残留）
- [ ] 子 agent 工具集裁剪（不含 sub agent tool），无运行时递归委派

## 提交约定

```bash
git add .agent/specs/spec-070-agent-invoke-subagent.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-070 agent invoke subagent (调研)"
```
