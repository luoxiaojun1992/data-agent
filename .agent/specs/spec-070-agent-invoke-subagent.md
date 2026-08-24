# agent 调用子 agent（subagent invocation）

> **SPEC-070** | Status: 调研完成（可行性确认 + 并行委派拍板，暂不实现）

## 目标

让主 agent 能调用子 agent 完成子任务。核心约束（晓军 2026-08-24 提出，含并行拍板）：

1. 子 agent **不是 tool**（语义上）——避免「tool 内启动 agent session → agent session 又调 tool」的循环依赖；
2. 子 agent 的能力提示词**单独组装**进 LLM 系统提示词，与普通 tool 的声明完全无关；
3. LLM 对子 agent 的调用**单独路由**（伪 tool 触发 + 独立 agent 流程），不走普通 tool 执行；
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

### 为什么子 agent 不是 tool（循环依赖）

若把子 agent 实现为一个普通 tool：`tool.Run` 里需要启动子 agent 的 session（子 agent 跑多轮 LLM + tool），而子 agent 又能调用 tool（包括这个子 agent tool 自己）→ 形成 `tool → agent session → tool` 的循环依赖。因此子 agent 必须是**独立的调用路径**，不是 tool。

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

| 需求 | 可行性 | 结论 |
|------|:---:|------|
| ① 子 agent 不是 tool | ✅ | ADK 的 transfer 是「伪 tool」：`Run` 不启动 agent session，只设置 action，天然避免循环依赖 |
| ② 能力提示词单独组装 | ✅ | `AppendInstructions` 注入，与 tool 的 function declarations 分离 |
| ③ bypass tool 调用，单独路由 | ✅ | transfer 走 runner 的 agent 切换流程，不走普通 tool 执行 |
| ④ 独立 session + 返回后销毁 | ⚠️ | ADK 原生**共享 session、不销毁**，需自定义实现 |
| ⑤ 最终返回写回主 session | ✅ | 自定义：子 agent 结束把最终返回写回（同 tool response 形态） |
| ⑥ model 与主 agent 一致 | ✅ | agent 定义时复用主 agent 的 model 配置 |

### 核心挑战

**「子 agent 独立 session + 返回后销毁」（需求 ④）是 ADK 原生不支持、需自定义实现的关键点。** 需要：

1. 为子 agent 创建**独立 session**（独立 sessionID，不复用主 session）；
2. 子 agent 执行完成（产出最终回复）后，**销毁 runtime 与 DB session**；
3. 把子 agent 的最终返回（最终 assistant 文本）作为 FunctionResponse **写回主 session**。

其余需求（①②③⑤⑥）均可基于 ADK 原生机制或其扩展实现。

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

因此「子 agent 调用」实现为**伪 tool（形式上 function call 触发，Run 启动完整独立 agent 流程）**：

| 需求 | 实现方式 | 可行性 |
|------|---------|:---:|
| 并行委派 | 多个子 agent 调用走 WaitGroup 天然并行 | ✅ |
| 子 agent 完整流程（与主 agent 一致） | tool.Run 启动**独立 session + 独立 runner + 完整 LLM 循环**，阻塞等最终结果 | ✅ |
| 循环依赖 | **子 agent 工具集裁剪**：不含「子 agent 调用」本身 → 子 agent 不能委派，递归斩断 | ✅ 可控 |
| 独立 session + 返回销毁 | Run 里创建独立 session，完成后 `Delete`（runtime + DB） | ✅ |
| 结果写回主 session | Run 返回 map → ADK 自动包装 FunctionResponse 写回（同 tool response） | ✅ |
| model 与主 agent 一致 | Deps 注入 `modelcfg.Provider`，子 agent 复用主 model | ✅ |
| **是否需要改 ADK vendor** | **不需要**，全部在自定义 tool + DI 层实现 | ✅ |

**两个必须处理的工程点**：

1. **ctx 生命周期**：子 agent 跑多轮 LLM，主请求 ctx 可能超时——tool.Run 内需用 detached ctx（`context.WithTimeout(context.Background(), …)`，同 runner.go AppendEvent 补丁做法）；
2. **DI 扩展**：当前 `tools.Deps`（`internal/adk/tools/tools.go`）只有 KB/Skill/Memory/Tasks/SessionSvc/Artifacts/APICollections，需新增 `modelcfg.Provider`（子 agent 选模）+ adk session service（创建/销毁独立 session）+ runtime 构建能力。

**「子 agent 不是 tool」的精确表述**：ADK 的 LLM 调用只有 function call 一条路，不存在「完全非 tool 的调用」。最终形态——**形式上是个伪 tool（LLM 触发用），但 Run 不执行函数，而是启动与主 agent 完全一致的独立 agent 流程**；循环依赖靠子 agent 工具集裁剪斩断。与「子 agent 不是 tool」的动机一致，实现上挂在 tool 触发形式上。

## 详细设计（方向，待实现时展开）

### 1. 子 agent 注册（伪 tool 触发 + 独立流程）

- 子 agent 复用主 agent 的 model 与工具集（**工具集裁剪：不含「子 agent 调用」伪 tool**，斩断递归委派）。
- 主 agent 侧注册「子 agent 调用」伪 tool（每个子 agent 一个，或统一的 `invoke_subagent` 带 `agent_name` 参数）。
- 子 agent 的能力描述单独维护，用于组装系统提示词（见下）。

### 2. 能力提示词组装

- 组装 LLM 系统提示词时，把「可用子 agent 及其能力描述」作为独立段落注入，与普通 tool 的 function declarations 分开（子 agent 是完整 agent 流程，不是函数）。
- 参考 ADK `AgentTransferRequestProcessor` 的 `AppendInstructions` 机制（`agent_transfer.go:91`），但提示词内容为「子任务委派」语义（委派后等待返回），而非「控制权转移」。

### 3. 调用路由

- 形式上：子 agent 调用作为**伪 tool** 挂进主 agent 的 function declarations（LLM 通过 function call 触发），能力描述单独注入系统提示词（与普通 tool 无关）。
- 执行上：伪 tool 的 `Run` **不执行普通函数**，而是启动子 agent 的独立完整流程（独立 session + runner + LLM 循环），阻塞等待最终返回——语义为「子任务委派 + 等待返回」。
- 并行：多个子 agent 调用由 `handleFunctionCalls` 的 WaitGroup 天然并行执行。
- 循环依赖防护：**子 agent 的工具集裁剪**，不含「子 agent 调用」伪 tool（子 agent 不能再委派）。

### 4. session 生命周期（独立 + 销毁）

- 子 agent 用**独立 session**（独立 sessionID / runtime）。
- 执行结束（子 agent 产出最终回复）后，**销毁 runtime 与 DB session**。
- 最终返回写回主 session（同 tool response 的形态）。
- 待明确：子 agent session 的 compaction 是否启用（生命周期短，可能不需要）。
- 工程点：tool.Run 内用 **detached ctx**（`context.WithTimeout(context.Background(), …)`），避免主请求 ctx 超时中断子 agent。

### 5. 返回写回

- 子 agent 的最终返回（最终 assistant 文本）作为 FunctionResponse 写回主 session，主 LLM 像看到普通 tool 结果一样继续（ADK 自动包装）。

### 6. model 复用

- 子 agent 的 model 与主 agent 一致（复用同一 model 配置，不额外选模）；Deps 注入 `modelcfg.Provider`。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | 待定（子 agent session 复用 `adk_sessions` 独立 sessionID，完成即 Delete） |
| 是否影响现有 API | 待定 |
| 性能影响 | 子 agent 独立 session 的创建/销毁开销；runtime 复用主 agent 的模型连接 |
| 是否需要新增 Skill | No（是 agent 运行时能力，非 tool skill） |
| 是否需要改 ADK vendor | **No**（伪 tool + DI 层实现，无需 vendor 补丁） |

## 相关文件（待实现时明确）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/tools/tools.go` | 新增「子 agent 调用」伪 tool + Deps 注入 modelcfg.Provider/session service | 待定 |
| `internal/adk/runtime/*` | 子 agent 独立 runtime 构建（复用主 model + 裁剪工具集） | 待定 |
| `internal/adk/session/mongo.go` | 子 agent 独立 session 创建/销毁 | 待定 |
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

- [ ] 子 agent 调用以伪 tool 触发，Run 启动独立 agent 流程（非普通函数执行）
- [ ] 子 agent 工具集裁剪（不含子 agent 调用伪 tool），无循环依赖
- [ ] 能力提示词单独组装，与普通 tool 声明分离
- [ ] **并行委派**：多个子 agent 调用并行执行（WaitGroup），子 agent 流程与主 agent 一致
- [ ] 子 agent 返回后 runtime 与 DB session 均销毁（无残留）
- [ ] 最终返回写回主 session（同 tool response 形态）
- [ ] 子 agent model 与主 agent 一致
- [ ] detached ctx：主请求超时不中断子 agent 执行

## 提交约定

```bash
git add .agent/specs/spec-070-agent-invoke-subagent.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-070 agent invoke subagent (调研)"
```
