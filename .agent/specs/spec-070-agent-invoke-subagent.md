# agent 调用子 agent（subagent invocation）

> **SPEC-070** | Status: 调研中（仅立项 + 可行性调研，暂不实现）

## 目标

让主 agent 能调用子 agent 完成子任务。核心约束（晓军 2026-08-24 提出）：

1. 子 agent **不是 tool**——避免「tool 内启动 agent session → agent session 又调 tool」的循环依赖；
2. 子 agent 的能力提示词**单独组装**进 LLM 系统提示词，与 tool 的声明完全无关；
3. LLM 对子 agent 的调用**单独路由**到 agent 调用，bypass 不走 tool 的调用；
4. 子 agent 的 session 在子 agent 返回后**销毁**（runtime 和 DB 都销毁）；
5. 只保留子 agent 的最终返回**写回主 agent 的 session**（同 tool response 的形态）；
6. 子 agent 使用的 model 与主 agent 一致。

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

### 并行性调研（2026-08-24 晓军追问）

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

**「LLM 动态 + 并行多个子 agent」（像并行 tool call 一样同时委派多个）在 ADK 原生里没有现成机制。**

**待晓军拍板的需求边界**：

- **串行委派**（一次一个子 agent，返回后再委派下一个）→ ADK transfer 模型可扩展，可行性高；
- **并行委派**（一次同时多个，像并行 tool call）→ 需自定义机制（子 agent 调用包装成异步任务并行执行、结果按 call ID 聚合），复杂度显著更高。

> 本 spec 其余设计先按「串行委派」展开；若拍板「并行委派」，需在详细设计补充并行编排章节。

## 详细设计（方向，待实现时展开）

### 1. 子 agent 注册（不是 tool）

- 子 agent 复用主 agent 的 model 与工具集（或按需裁剪），但**不注册进主 agent 的 tool 列表**。
- 子 agent 的能力描述单独维护，仅用于组装系统提示词（见下）。

### 2. 能力提示词组装

- 组装 LLM 系统提示词时，把「可用子 agent 及其能力描述」作为独立段落注入，与 tool 的 function declarations 完全分离。
- 参考 ADK `AgentTransferRequestProcessor` 的 `AppendInstructions` 机制（`agent_transfer.go:91`），但提示词内容为「子任务委派」语义，而非「控制权转移」。

### 3. 调用路由（bypass tool）

- LLM 发起子 agent 调用时，走**独立的 agent 调用路径**（非 tool 执行），语义为「子任务委派 + 等待返回」，而非 ADK 原生的「控制权转移」（transfer 后主 agent 不再接管）。
- 可参考 ADK `TransferToAgent` action 机制做扩展：新增「子 agent 调用」的 action/事件类型，runner 识别后启动子 agent 执行并回写结果。

### 4. session 生命周期（独立 + 销毁）

- 子 agent 用**独立 session**（独立 sessionID / runtime）。
- 执行结束（子 agent 产出最终回复）后，**销毁 runtime 与 DB session**。
- 最终返回写回主 session（同 tool response 的形态）。
- 待明确：子 agent session 的 compaction 是否启用（生命周期短，可能不需要）。

### 5. 返回写回

- 子 agent 的最终返回（最终 assistant 文本）作为 FunctionResponse 写回主 session，主 LLM 像看到普通 tool 结果一样继续。

### 6. model 复用

- 子 agent 的 model 与主 agent 一致（复用同一 model 配置，不额外选模）。

## 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | 待定（子 agent session 可能复用 `adk_sessions`，或独立集合） |
| 是否影响现有 API | 待定 |
| 性能影响 | 子 agent 独立 session 的创建/销毁开销；runtime 复用主 agent 的模型连接 |
| 是否需要新增 Skill | No（是 agent 运行时能力，非 tool skill） |
| 是否需要改 ADK vendor | 可能（需求 ④ 独立 session + 销毁需在自定义层或 vendor 补丁实现） |

## 相关文件（待实现时明确）

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/runtime/runtime.go` | Runtime 支持子 agent 执行 | 待定 |
| `internal/adk/runtime/registry.go` | 子 agent runtime 的创建/销毁 | 待定 |
| `internal/adk/session/mongo.go` | 子 agent session 独立 + 销毁 | 待定 |
| `internal/logic/agent/*` | 子 agent 调用编排（executor/orchestrator） | 待定 |
| `vendor_adk_v1.5.0/...` | 可能需补丁（独立 session + 销毁） | 待定 |

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

- [ ] 子 agent 不作为 tool 注册，无循环依赖
- [ ] 能力提示词单独组装，与 tool 声明分离
- [ ] 子 agent 调用 bypass tool，走独立路由
- [ ] 子 agent 返回后 runtime 与 DB session 均销毁（无残留）
- [ ] 最终返回写回主 session（同 tool response 形态）
- [ ] 子 agent model 与主 agent 一致

## 提交约定

```bash
git add .agent/specs/spec-070-agent-invoke-subagent.md .agent/specs/INDEX.md
git commit -m "docs: add SPEC-070 agent invoke subagent (调研)"
```
