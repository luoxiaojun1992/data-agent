# 时间 Skill + 规划 Skill + Plan 意图隐藏引导

> **SPEC-080** | Status: 设计中

## 1. 目标

1. 新增 **`get_current_time`** skill：返回服务器真实当前时间，供 LLM 在需要时间上下文时调用（而非依赖训练数据或猜测）。
2. 新增 **`get_plan_method`** skill：返回通用的任务规划步骤指南，供 LLM 在被要求「制定计划/方案」时调用并按指南规划。
3. **意图识别扩展**：chat/feishu 意图分类从 `task/chat` 二分类扩展为可识别「需要规划的任务」（plan 意图）。识别出 plan 意图时，注入一条润色后的建议提示（引导 LLM 依照 plan skill 的指导进行规划）。
4. **该建议提示不出现在前端 chat history**：以隐藏事件（hidden）落库，LLM 上下文可见、前端渲染过滤。

## 1.5. 前置依赖检查

| 前置 Spec | 状态 | 备注 |
|-----------|:---:|------|
| SPEC-048 / SPEC-062 | ✅ | ADK 引擎与 Runtime 已就绪；skill 均为 ADK function tool 机制 |
| SPEC-066 / SPEC-068 | ✅ | skill 三步注册机制（tools.go + predefinedSkills + seed）已稳定 |
| SPEC-069 | ✅ | session_events 独立 collection，事件落库路径清晰，hidden 标记加在该文档 |
| SPEC-067 | ✅ | guard 服务（意图分类 + 相关性检查）已存在，本次扩展 CheckIntent |
| SPEC-079 | ✅ | 健康检查/指示灯已上线；无冲突 |
| — | — | 无硬阻塞依赖，可独立开发 |

## 2. 背景

1. **时间盲区**：LLM 不知道「现在」的真实时间。用户问「今天几号」「现在几点」「本周三是什么日期」时，模型只能靠训练截止时间猜，经常答错。需要一个 `get_current_time` 工具返回服务器真实时间。
2. **规划质量不稳定**：用户要求「帮我制定 XX 计划/方案」时，模型有时直接开始执行或只给零散要点，缺少结构化规划。需要一个 `get_plan_method` 工具提供通用规划步骤指南，并让意图识别在识别出 plan 需求时主动引导模型调用它。
3. **内部提示污染聊天记录**：现有 `[intent] is_task=true` 以 system 事件落库，前端会渲染为居中胶囊（`chat-msg-system-*`），把内部提示暴露给用户。新的 plan 建议提示词明确要求**不显示**，需要引入 hidden 事件机制（详见 §5.2）。

## 3. 架构概述

```
用户消息
   │
   ▼
guard.CheckIntent（扩展三分类）── is_plan=true ──► 注入 hidden 建议事件（LLM 可见 / 前端隐藏）
   │  is_task=true（无 plan）
   ▼
ADK Runtime（chat/feishu）
   │  LLM 按需调用工具
   ├── get_current_time  ──► 服务器真实时间（Asia/Shanghai）
   └── get_plan_method   ──► 通用任务规划步骤指南
```

与现有模块的关系：只扩展 `guard` 的意图分类与事件注入，新增两个 ADK function tool；不改 Runtime 调度、不改模型配置结构、不新增 use case。

## 4. Skill 设计

### 4.1 `get_current_time`（获取当前时间）

| 项 | 定义 |
|----|------|
| Name | `get_current_time` |
| DisplayName | 获取当前时间 |
| Description | 获取服务器当前的真实日期和时间（Asia/Shanghai 时区），用于回答与「现在」相关的问题 |
| Args | 无 |
| Result | `{"time": "2026-09-03T20:49:13+08:00", "date": "2026-09-03", "weekday": "星期四", "timezone": "Asia/Shanghai", "unix": 1756903753}` |

实现要点：
- 时间源 `time.Now()` + 显式 `time.LoadLocation("Asia/Shanghai")`（服务器 TZ 可能是 UTC，不得依赖默认时区）。
- 纯函数、无外部依赖、无副作用 → L1 包 100% 覆盖可测。

### 4.2 `get_plan_method`（获取规划方法）

| 项 | 定义 |
|----|------|
| Name | `get_plan_method` |
| DisplayName | 获取规划方法 |
| Description | 获取通用的任务规划步骤指南。当用户要求制定计划/方案/规划时，先调用本工具获取规划方法论，再按指南拆解并输出结构化计划 |
| Args | 无 |
| Result | 通用规划步骤指南（固定文案，见 §4.3） |

### 4.3 通用任务规划步骤指南（get_plan_method 返回文案）

```
通用任务规划步骤：
1. 明确目标 —— 澄清任务的最终交付物与验收标准；
2. 任务拆解 —— 将目标拆分为 3~7 个可独立执行的子任务；
3. 排定顺序 —— 识别子任务间的依赖关系，安排执行先后；
4. 设定检查点 —— 为关键节点定义验证方式与完成判据；
5. 逐步执行 —— 按顺序执行，每步完成后核对结果再进入下一步；
6. 汇总交付 —— 整合各步结果，对照验收标准检查完整性后交付。
```

## 5. 详细设计

### 5.1 意图识别扩展（guard.CheckIntent 三分类）

现有 prompt（`internal/service/guard/service.go:74`）：

> 你是用户意图分类器。判断用户的输入是「任务」还是「聊天」。只输出 JSON：`{"is_task": true}` 或 `{"is_task": false}`。

扩展为：

> 你是用户意图分类器。判断用户的输入是「任务」（需要数据分析、计算、查询、生成报告/文件、制定计划等）还是「聊天」（闲聊、问候、咨询）。若属于任务，进一步判断是否「需要规划」（用户要求制定计划/方案/路线图，或任务本身需要多步拆解后才能完成）。只输出 JSON：`{"is_task": true, "is_plan": false}` 形式（is_plan 仅在 is_task 为 true 时可为 true），不要输出其他内容。

- 返回解析：`parseIsTask` → 新的 `parseIntent` 返回 `(isTask bool, isPlan bool)`，解析失败按 is_task=false 处理（与现状一致，非致命）。
- 仍使用 `modelcfg.UseCaseIntentCheck`，**不新增 use case、不改模型配置结构**。

### 5.2 隐藏事件机制（plan 建议提示不显示在前端）

**方案**：session_events 文档增加 `hidden bool` 字段（bson tag，默认 false），新增 `appendHiddenSystemEvent`（写 events + session_events 带 hidden:true，不触发 compaction，与 appendSystemEvent 同路径仅多一个标记）；前端 history 接口透传 hidden 字段，渲染时过滤。

| 数据面 | 行为 |
|--------|------|
| events（LLM 上下文） | ✅ 保留（模型能看到提示并依此规划） |
| session_events / raw_events（前端展示源） | 写入但带 `hidden: true` |
| 前端 chat 渲染 | `hidden === true` 的 system 消息**不渲染**（过滤） |

**建议提示词（润色版，plan 意图时注入，hidden 事件文本）**：

> [plan_hint] 检测到本次任务需要制定执行计划。请先调用 get_plan_method 获取通用规划步骤，再依据该步骤拆解目标、安排执行顺序并逐步推进，确保交付完整、有条理。

润色说明：原需求为「建议依照 plan skill 的指导进行 plan」——修正为可执行指令（明确「先调用 get_plan_method」）、补充行动闭环（拆解 → 排序 → 推进）、加收尾质量要求（完整、有条理），并以 `[plan_hint]` 前缀标记便于排查与断言。

**顺带处理（推荐）**：现有 `[intent] is_task=%t` 事件同样加 hidden——它同样是内部提示，不应展示给用户。若保留现状，用户将在历史中同时看到 `[intent]` 与（隐藏的）plan 提示，体验不一致。此改动需同步检查现有 E2E 是否有断言 `[intent]` 文本的用例。

### 5.3 Skill 三步注册（铁律：三处同步）

| 步骤 | 落点 | 变更 |
|------|------|------|
| ① 工具注册 | `internal/adk/tools/tools.go` | `specs()` 新增 `get_current_time` / `get_plan_method` 两个 spec（`functiontool.Func[Args, Result]`，无外部依赖，无需 Deps 新字段） |
| ② skill 清单 | `internal/service/skill/config.go` | `predefinedSkills()` 新增两条（Enabled: true，ConfigJSON `{}`） |
| ③ seed | seed 幂等自动 | 无需手动（SeedSkills 幂等） |

### 5.4 数据模型

```go
// session_events 文档新增字段
type sessionEventDoc struct {
    // ...existing fields...
    Hidden bool `bson:"hidden"` // true = 内部提示事件，前端不渲染
}
```

```go
// guard 意图结果
type IntentResult struct {
    IsTask bool
    IsPlan bool
}
```

## 6. 可行性分析

| 检查项 | 结论 |
|--------|------|
| 是否需要新 DB 集合 | No（session_events 加字段，向后兼容：旧文档缺省 false） |
| 是否影响现有 API | Yes（history 接口透传 hidden；意图事件语义扩展） |
| 性能影响 | 极低（两个无参工具零外部调用；意图 prompt 略长） |
| 是否需要新增 Skill | Yes（2 个，三步注册） |
| 是否需要新 use case | No |
| 新增依赖 | 无（标准库 time） |
| 风险 | ① 三分类意图误判 plan 导致多余引导——可接受（引导仅是建议，模型可忽略）；② history 接口隐藏字段需前端同步上线，存在短暂版本不一致——hidden 默认 false，旧前端不识别时仅多显示一条系统胶囊，无功能破坏 |

## 7. 相关文件

| File | Role | Change Magnitude |
|------|------|-----------------|
| `internal/adk/tools/tools.go` | 新增两个 function tool + specs 注册 | Medium |
| `internal/service/skill/config.go` | predefinedSkills 加两条 | Small |
| `internal/service/guard/service.go` | CheckIntent 三分类 + parseIntent | Medium |
| `internal/service/chat/chat_service.go` | recordIntent 扩展 + appendHiddenSystemEvent | Medium |
| `internal/adk/session/mongo.go` | sessionEventDoc 加 Hidden 字段 + 落库路径 | Small |
| `internal/api/handler/`（history 端点） | 透传 hidden 字段 | Small |
| `frontend/app/chat/page.tsx` | 渲染过滤 hidden system 消息 | Small |
| `tests/ui/online-*.spec.ts`（或新建 spec） | E2E：时间/规划工具调用 + hidden 断言 | New |
| `internal/service/guard/service_test.go` 等 | 三分类解析单测 | Small |

## 8. 测试策略

1. **Unit tests**（Go）：
   - `get_current_time` / `get_plan_method` 工具函数：结果字段、时区正确性（Asia/Shanghai）、plan 指南文案完整；
   - `guard.parseIntent`：三分类 JSON 解析（含 is_plan=false、格式异常降级 is_task=false）；
   - hidden 事件：sessionEventDoc 序列化含 hidden；appendHiddenSystemEvent 写入 hidden:true 而 appendSystemEvent 写入 hidden:false；
   - history 端点：hidden 字段透传。
2. **Integration tests**：不新增（无新外部依赖）。
3. **E2E tests**（`tests/ui/`，编号 UI-XXX，避开 UI-242~UI-249）：
   - chat 提问「今天几号」→ 断言工具调用 `get_current_time` 且回答日期与服务器日期一致；
   - chat 提问「帮我制定一个 X 计划」→ 断言调用 `get_plan_method` 且回答含步骤结构；
   - 断言 `[plan_hint]` 文本不出现在 chat history（`chat-msg-system-*` 中无该文本）。
4. **审计**：`.agent/skills/go-ut-audit` 审查新增 UT 质量。

## 9. UI Test / E2E 验收规则

> 开发任务完成后必须编写真实 E2E 用例并通过 CI（sonar-check + ui-tests）。

- [ ] **必须** 新增前端交互功能时同步编写对应 E2E 用例（`tests/ui/`，编号 `UI-XXX`）
- [ ] **必须** 修改 UI 组件时更新 `data-testid` 属性
- [ ] **必须** CI Pipeline 中 sonar-check 和 ui-tests 均通过才可合并
- [ ] **严禁** 删除/降级测试用例、修改业务逻辑绕过测试
- [ ] **严禁** 以占位用例顶替真实功能测试

参考: `.agent/memory/E2E_TESTING.md`

## 9.5. Go Unit Test 验收规则

> 开发任务完成后必须编写 Go 单元测试并通过 CI（ut-workflow）。

### 覆盖率底线

| Tier | 特征 | 目标 | 示例 |
|:---:|------|:---:|------|
| L1 | 纯函数/纯结构体，无外部依赖 | **100%** | `get_current_time` / `get_plan_method` / `parseIntent` |
| L2 | 依赖接口，可 mock | **100%** | guard 意图解析 |
| L3 | 依赖 MongoDB/Redis/HTTP | **98%** | `session_events` 落库路径、history handler |
| Overall | 全量 | ≥98% | CI `ut-workflow.yml` gate |

### 断言质量要求

- [ ] **必须** 每个 Success 测试至少包含 **2 个行为验证断言**（除 `err == nil` 外必须验证实际值/状态/副作用）
- [ ] **必须** Handler 测试使用 `gomonkey.ApplyMethodFunc`（非 `ApplyMethodReturn`）验证 handler→service 参数传递正确性
- [ ] **严禁** `t.Skip()` 绕过无法测试的场景（如确实不可行，需文档注释说明原因并记录到 spec 中）
- [ ] **严禁** Success 测试只验证 `err == nil` 而不验证操作的实际结果

参考:
- `.agent/specs/spec-045-go-service-ut.md` — Go UT 全覆盖 spec
- `.agent/skills/go-ut-audit/SKILL.md` — UT 审计 skill

## 10. 验证标准

1. chat 提问「今天是几月几号星期几」→ 回答日期/星期与服务器真实时间一致（`get_current_time` 被调用）。
2. chat 提问「帮我制定一个学习计划」→ `get_plan_method` 被调用，回答包含目标/拆解/顺序等结构化步骤。
3. plan 意图触发后，`[plan_hint]` 事件存在于 LLM 上下文（events）但**不在**前端 chat history 渲染（0 处）。
4. 现有 `[intent]` 事件（若按推荐同步 hidden）同样不在前端渲染；相关旧 E2E 断言已同步。
5. `go test ./internal/...` 全绿；覆盖率 ≥98%；E2E 通过。
6. 时间工具返回时区恒为 Asia/Shanghai（不随服务器 TZ 漂移）。
